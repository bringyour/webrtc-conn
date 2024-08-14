package webrtcconn

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/pion/webrtc/v3"
)

type conn struct {
	dataChannel *webrtc.DataChannel
	messageChan chan webrtc.DataChannelMessage
}

type OfferOptions struct {

	// OfferChan is a channel that we'll send the offer to.
	OfferChan chan<- webrtc.SessionDescription

	// AnswerChan is a channel that we'll receive the answer from.
	AnswerChan <-chan webrtc.SessionDescription

	// RemoteCandidates is a channel on which we will receive the remote candidates.
	RemoteCandidates <-chan webrtc.ICECandidateInit

	// LocalCandidates is a channel to send the local candidates.
	LocalCandidates chan<- webrtc.ICECandidateInit

	// Ordered is a boolean that indicates if the data channel is ordered
	Ordered bool

	// MaxRetransmits is the maximum number of retransmits
	MaxRetransmits uint16

	// OpenTimeout is the timeout for the data channel to open.
	// If the data channel does not open within this time, the offer will fail.
	// Default is 15 seconds.
	OpenTimeout time.Duration
}

func (o OfferOptions) openTimeout() time.Duration {
	if o.OpenTimeout == 0 {
		return 15 * time.Second
	}

	return o.OpenTimeout
}

func (o OfferOptions) validate() error {
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

func Offer(
	ctx context.Context,
	config webrtc.Configuration,
	opts OfferOptions,
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

	go func() {
		<-ctx.Done()
		_ = peerConnection.Close()
	}()

	// Create a datachannel with label 'data'
	dataChannel, err := peerConnection.CreateDataChannel("data", &webrtc.DataChannelInit{
		Ordered:        &opts.Ordered,
		MaxRetransmits: &opts.MaxRetransmits,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot create data channel: %w", err)
	}

	opened := make(chan struct{})

	dataChannel.OnOpen(func() {
		close(opened)
	})

	offer, err := peerConnection.CreateOffer(&webrtc.OfferOptions{})
	if err != nil {
		return nil, fmt.Errorf("cannot create offer: %w", err)
	}

	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		return nil, fmt.Errorf("cannot set local description: %w", err)
	}

	// Send the offer
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
	case opts.OfferChan <- offer:
	}

	// Receive the answer
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
	case answer, ok := <-opts.AnswerChan:

		if !ok {
			return nil, fmt.Errorf("answer channel closed")
		}

		err = peerConnection.SetRemoteDescription(answer)

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
		return nil, fmt.Errorf("data channel did not open within %s: %w", opts.openTimeout(), ctx.Err())
	case _, _ = <-opened:
		// Data channel is open
	}

	return &conn{
		dataChannel: dataChannel,
		messageChan: messageChan,
	}, nil

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
	err = c.dataChannel.Send(b)
	if err != nil {
		return 0, fmt.Errorf("cannot send message: %w", err)
	}
	return len(b), nil
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
