package webrtcconn

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/pion/webrtc/v3"
)

type AnswerOptions struct {

	// OfferChan is a channel that we'll receive the offer from.
	OfferChan <-chan webrtc.SessionDescription

	// AnswerChan is a channel that we'll send the answer to.
	AnswerChan chan<- webrtc.SessionDescription

	// RemoteCandidates is a channel on which we will receive the remote candidates.
	RemoteCandidates <-chan webrtc.ICECandidateInit

	// LocalCandidates is a channel to send the local candidates.
	LocalCandidates chan<- webrtc.ICECandidateInit

	// OpenTimeout is the timeout for the data channel to open.
	// If the data channel does not open within this time, the offer will fail.
	// Default is 15 seconds.
	OpenTimeout time.Duration
}

func (o AnswerOptions) openTimeout() time.Duration {
	if o.OpenTimeout == 0 {
		return 15 * time.Second
	}

	return o.OpenTimeout
}

func (o AnswerOptions) validate() error {
	if o.OfferChan == nil {
		return fmt.Errorf("offer channel is required")
	}

	if o.AnswerChan == nil {
		return fmt.Errorf("answer channel is required")
	}

	if o.RemoteCandidates == nil {
		return fmt.Errorf("remote candidates channel is required")
	}

	if o.LocalCandidates == nil {
		return fmt.Errorf("local candidates channel is required")
	}

	return nil
}

func Answer(
	ctx context.Context,
	config webrtc.Configuration,
	opts AnswerOptions,
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

	err = opts.validate()
	if err != nil {
		return nil, fmt.Errorf("invalid offer options: %w", err)
	}

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

		select {
		case <-ctx.Done():
			return
		case opts.LocalCandidates <- candidate.ToJSON():
		}
	})

	opened := make(chan *webrtc.DataChannel, 1)

	peerConnection.OnDataChannel(func(dataChannel *webrtc.DataChannel) {
		if dataChannel.Label() == "data" {
			opened <- dataChannel
		}
	})

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
	case offer, ok := <-opts.OfferChan:
		if !ok {
			return nil, fmt.Errorf("offer channel closed")
		}

		err = peerConnection.SetRemoteDescription(offer)
		if err != nil {
			return nil, fmt.Errorf("cannot set remote description: %w", err)
		}
	}

	answer, err := peerConnection.CreateAnswer(&webrtc.AnswerOptions{})
	if err != nil {
		return nil, fmt.Errorf("cannot create answer: %w", err)
	}

	// Send the answer
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
	case opts.AnswerChan <- answer:

	}

	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		return nil, fmt.Errorf("cannot set local description: %w", err)
	}

	// Receive the remote candidates
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case candidate, ok := <-opts.RemoteCandidates:
				if !ok {
					return
				}
				err = peerConnection.AddICECandidate(
					candidate,
				)
				if err != nil {
					return
				}
			}
		}
	}()

	openContext, openCancel := context.WithTimeout(ctx, opts.openTimeout())
	defer openCancel()

	var dataChannel *webrtc.DataChannel

	select {
	case <-openContext.Done():
		return nil, fmt.Errorf("data channel did not open within %s: %w", opts.openTimeout(), ctx.Err())
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

	return &conn{
		dataChannel: dataChannel,
		messageChan: messageChan,
	}, nil

}
