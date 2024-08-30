package webrtcconn

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/pion/webrtc/v3"
)

type AnswerSync interface {
	// AnswerSDP calls the answer function with the offer SDP and returns the answer SDP.
	// The call must block until the offer is available or the context has been cancelled.
	AnswerSDP(ctx context.Context, answer func(ctx context.Context, offer webrtc.SessionDescription) (webrtc.SessionDescription, error)) error

	// GetOfferPeerCandidates returns the ICE candidates of the other client.
	// Every call should block until either at least one candidate is available or the context has been cancelled.
	GetOfferPeerCandidates(ctx context.Context) ([]webrtc.ICECandidate, error)

	// AddAnswerPeerCandidate adds an ICE candidate for the other client.
	AddAnswerPeerCandidate(ctx context.Context, candidate webrtc.ICECandidate) error
}

func Answer(
	ctx context.Context,
	config webrtc.Configuration,
	as AnswerSync,
	timeout time.Duration,
) (c net.Conn, err error) {

	defer func() {
		if err != nil {
			err = fmt.Errorf("cannot offer: %w", err)
		}
	}()

	ctx, cancel := context.WithCancel(ctx)

	defer func() {
		// If there is an error, cancel the context
		if err != nil {
			cancel()
		}
	}()

	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("cannot create peer connection: %w", err)
	}

	go func() {
		<-ctx.Done()
		_ = peerConnection.Close()
	}()

	// Set the handler for Peer connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {

		if s == webrtc.PeerConnectionStateFailed {
			// cancel the context
			cancel()
		}

		if s == webrtc.PeerConnectionStateClosed {
			// cancel the context
			cancel()
		}
	})

	// Send the local candidates.
	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}

		as.AddAnswerPeerCandidate(ctx, *candidate)
	})

	opened := make(chan *webrtc.DataChannel, 1)

	peerConnection.OnDataChannel(func(dataChannel *webrtc.DataChannel) {
		if dataChannel.Label() == "data" {
			opened <- dataChannel
		}
	})

	err = as.AnswerSDP(ctx, func(ctx context.Context, offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {

		err = peerConnection.SetRemoteDescription(offer)
		if err != nil {
			return webrtc.SessionDescription{}, fmt.Errorf("cannot set remote description: %w", err)
		}

		answer, err := peerConnection.CreateAnswer(&webrtc.AnswerOptions{})
		if err != nil {
			return webrtc.SessionDescription{}, fmt.Errorf("cannot create answer: %w", err)
		}

		err = peerConnection.SetLocalDescription(answer)
		if err != nil {
			return webrtc.SessionDescription{}, fmt.Errorf("cannot set local description: %w", err)
		}

		go func() {
			for {
				candidates, err := as.GetOfferPeerCandidates(ctx)
				if err != nil {
					return
				}

				for _, c := range candidates {
					err = peerConnection.AddICECandidate(c.ToJSON())
					if err != nil {
						return
					}
				}

			}

		}()

		return answer, nil

	})

	if err != nil {
		return nil, fmt.Errorf("cannot answer: %w", err)
	}

	openContext, openCancel := context.WithTimeout(ctx, timeout)
	defer openCancel()

	var dataChannel *webrtc.DataChannel

	select {
	case <-openContext.Done():
		return nil, fmt.Errorf("data channel did not open within %v: %w", timeout, ctx.Err())
	case dataChannel = <-opened:
	}

	dataChannel.OnClose(func() {
		// cancel the context
		cancel()
	})

	messageChan := make(chan webrtc.DataChannelMessage, 1)

	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		// Send the message to the message channel
		select {
		case <-ctx.Done():
			return

		case messageChan <- msg:
			//all good
		}
	})

	co := &conn{
		dataChannel:  dataChannel,
		messageChan:  messageChan,
		maxBuffered:  defaultMaxBuffered,
		writeChannel: make(chan []byte, 16),
		ctx:          ctx,
	}

	go co.writingProcess(ctx)

	return co, nil

}
