package webrtcconn

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/pion/webrtc/v3"
)

const defaultMaxBuffered = 8192

type conn struct {
	dataChannel  *webrtc.DataChannel
	messageChan  chan webrtc.DataChannelMessage
	maxBuffered  uint64
	writeChannel chan []byte
	ctx          context.Context
}

type OfferSync interface {
	// OfferSDP calls the answer function with the offer SDP. The function returns the answer SDP.
	// The call must block until the answer is available or the context has been cancelled.
	OfferSDP(ctx context.Context, offer webrtc.SessionDescription) (webrtc.SessionDescription, error)

	// GetAnswerPeerCandidates returns the ICE candidates of the other client.
	// Every call should block until either at least one candidate is available or the context has been cancelled.
	GetAnswerPeerCandidates(ctx context.Context) ([]webrtc.ICECandidate, error)

	// AddOfferPeerCandidate adds an ICE candidate for the other client.
	AddOfferPeerCandidate(ctx context.Context, candidate webrtc.ICECandidate) error
}

func Offer(
	ctx context.Context,
	config webrtc.Configuration,
	os OfferSync,
	ordered bool,
	maxRetransmits uint16,
	openTimeout time.Duration,
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

		os.AddOfferPeerCandidate(ctx, *candidate)
	})

	go func() {
		<-ctx.Done()
		_ = peerConnection.Close()
	}()

	// Create a datachannel with label 'data'
	dataChannel, err := peerConnection.CreateDataChannel("data", &webrtc.DataChannelInit{
		Ordered:        &ordered,
		MaxRetransmits: &maxRetransmits,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot create data channel: %w", err)
	}

	opened := make(chan struct{})

	dataChannel.OnOpen(func() {
		close(opened)
	})

	// Receive the remote candidates
	go func() {
		for {
			candidates, err := os.GetAnswerPeerCandidates(ctx)
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

	offer, err := peerConnection.CreateOffer(&webrtc.OfferOptions{})
	if err != nil {
		return nil, fmt.Errorf("cannot create offer: %w", err)
	}

	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		return nil, fmt.Errorf("cannot set local description: %w", err)
	}

	answer, err := os.OfferSDP(ctx, offer)
	if err != nil {
		return nil, fmt.Errorf("cannot get answer: %w", err)
	}

	err = peerConnection.SetRemoteDescription(answer)
	if err != nil {
		return nil, fmt.Errorf("cannot set remote description: %w", err)
	}

	openContext, openCancel := context.WithTimeout(ctx, openTimeout)
	defer openCancel()

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

	select {
	case <-openContext.Done():
		return nil, fmt.Errorf("data channel did not open within %s: %w", openTimeout, ctx.Err())
	case <-opened:
		// Data channel is open
	}

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

func (c *conn) writingProcess(ctx context.Context) {

	sendMoreCh := make(chan struct{}, 1)

	c.dataChannel.SetBufferedAmountLowThreshold(c.maxBuffered)
	c.dataChannel.OnBufferedAmountLow(func() {
		select {
		case <-ctx.Done():
			return
		case sendMoreCh <- struct{}{}:
		default:
		}
	})

	for {

		select {
		case <-ctx.Done():
			return
		case d := <-c.writeChannel:
			err := c.dataChannel.Send(d)
			if err != nil {
				return
			}

			if c.dataChannel.BufferedAmount() > c.maxBuffered {
				select {
				case <-ctx.Done():
					return
				case <-sendMoreCh:
				}
			}
		}
	}
}

func (c *conn) Read(b []byte) (n int, err error) {
	msg := <-c.messageChan

	if len(msg.Data) > len(b) {
		return 0, fmt.Errorf("message too large")
	}

	copy(b, msg.Data)

	return len(msg.Data), nil

}

func (c *conn) Write(b []byte) (n int, err error) {
	select {
	case <-c.ctx.Done():
		return 0, fmt.Errorf("context cancelled")
	case c.writeChannel <- b:
		return len(b), nil
	}
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (c *conn) Close() error {
	return c.dataChannel.Close()
}

// LocalAddr returns the local network address, if known.
func (c *conn) LocalAddr() net.Addr {
	return rtcaddr("offer")
}

// RemoteAddr returns the remote network address, if known.
func (c *conn) RemoteAddr() net.Addr {
	return rtcaddr("answer")
}

func (c *conn) SetDeadline(t time.Time) error {
	return fmt.Errorf("not implemented")
}

func (c *conn) SetReadDeadline(t time.Time) error {
	return fmt.Errorf("not implemented")
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	return fmt.Errorf("not implemented")
}

type rtcaddr string

func (a rtcaddr) Network() string {
	return "rtc"
}

func (a rtcaddr) String() string {
	return string(a)
}
