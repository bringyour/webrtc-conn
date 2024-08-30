# WebRTC Conn

`WebRTC Conn` is a Go library that seamlessly integrates the [Pion WebRTC](https://github.com/pion/webrtc) library with the [net.Conn](https://pkg.go.dev/net#Conn) interface. This allows developers to easily incorporate WebRTC data channels into applications that rely on traditional network connections.

WebRTC is a powerful yet complex protocol, involving intricate message flows to establish peer-to-peer connections. `WebRTC Conn` abstracts away these complexities, providing a straightforward adapter that presents WebRTC data channels as standard `net.Conn` interfaces.

## Features

- **Seamless WebRTC Integration**: Leverage the power of WebRTC for reliable peer-to-peer communication.
- **net.Conn Compatibility**: Treat WebRTC data channels as standard `net.Conn` connections, simplifying integration with existing codebases.
- **Flexible Channel Configuration**: Customize data channel properties such as message ordering and retransmission limits to meet your application's requirements.

## Usage

### Roles in WebRTC Connection

A WebRTC connection involves two primary roles:

1. **Offer**: The initiator of the connection, responsible for opening the data channel and configuring its parameters.
2. **Answer**: The responder, which joins the data channel established by the Offer.

### Exchanging SDP and ICE Candidates

To establish a WebRTC connection, Offer/Answer Session Description Protocol (SDP) messages must be exchanged between peers, along with ICE (Interactive Connectivity Establishment) candidates. WebRTC does not define a specific transport mechanism for this exchange, so you can implement it using protocols like WebSockets.

`WebRTC Conn` provides a transport-agnostic approach through the following interfaces, allowing you to define your own transport mechanism:

```go
type OfferSync interface {
    // OfferSDP sends the offer SDP and receives the answer SDP.
    OfferSDP(ctx context.Context, offer webrtc.SessionDescription) (webrtc.SessionDescription, error)

    // GetAnswerPeerCandidates retrieves ICE candidates from the answering peer.
    GetAnswerPeerCandidates(ctx context.Context) ([]webrtc.ICECandidate, error)

    // AddOfferPeerCandidate adds an ICE candidate for the answering peer.
    AddOfferPeerCandidate(ctx context.Context, candidate webrtc.ICECandidate) error
}

type AnswerSync interface {
    // AnswerSDP handles the offer SDP and generates the answer SDP.
    AnswerSDP(ctx context.Context, answer func(ctx context.Context, offer webrtc.SessionDescription) (webrtc.SessionDescription, error)) error

    // GetOfferPeerCandidates retrieves ICE candidates from the offering peer.
    GetOfferPeerCandidates(ctx context.Context) ([]webrtc.ICECandidate, error)

    // AddAnswerPeerCandidate adds an ICE candidate for the offering peer.
    AddAnswerPeerCandidate(ctx context.Context, candidate webrtc.ICECandidate) error
}
```

### Creating an Offer

To create an offer, use the `webrtcconn.Offer` function. It requires a context, a `webrtc.Configuration` object for the peer connection, an implementation of the `OfferSync` interface, a flag to indicate whether RTCP (Real-time Transport Control Protocol) should be used to detect and retransmit lost packets, the maximum number of retransmissions, and a connection timeout.

```go
oc, err := webrtcconn.Offer(
    ctx, 
    webrtc.Configuration{}, 
    offerSync,
    true,
    2,
    15*time.Second,
)
if err != nil {
    return err
}
```

### Responding to an Offer

To respond to an offer, use the `webrtcconn.Answer` function. Similar to the offer process, it requires a context, a `webrtc.Configuration` object, an implementation of the `AnswerSync` interface, and a connection timeout.

```go
ac, err := webrtcconn.Answer(
    ctx, 
    webrtc.Configuration{}, 
    answerSync,
    15*time.Second,
)
if err != nil {
    return err
}
```

### Data Channel Configuration

When establishing a connection, the Offer role defines the data channel's configuration, including:

- **Ordered**: Specifies whether messages should be delivered in the order they were sent.
- **Max Retransmits**: Sets the maximum number of retransmission attempts for unreliable data channels.
