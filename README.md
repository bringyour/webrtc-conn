# WebRTC Conn

`WebRTC Conn` is a Go library that adapts the [Pion WebRTC](https://github.com/pion/webrtc) library to the [net.Conn](https://pkg.go.dev/net#Conn) interface. This enables developers to seamlessly integrate WebRTC data channels into applications that rely on traditional network connections.

WebRTC involves complex protocols and message flows for establishing peer-to-peer connections. This library abstracts away those complexities, offering a simplified adapter that presents WebRTC data channels as standard `net.Conn` interfaces.

## Features

- **Seamless WebRTC Integration**: Leverage WebRTC for robust peer-to-peer communication.
- **net.Conn Compatibility**: Treat WebRTC data channels as standard `net.Conn` connections, facilitating easier integration with existing codebases.
- **Flexible Channel Configuration**: Customize data channel properties, such as message ordering and retransmission limits, according to your application's needs.

## Usage

### Roles

In a WebRTC connection, two primary roles exist:

1. **Offer**: The initiator of the connection, responsible for opening the data channel and defining its configuration.
2. **Answer**: The responder, which joins the data channel established by the Offer.

### Exchanging SDP and ICE Candidates

To establish a WebRTC connection, Offer/Answer Session Description Protocol (SDP) messages must be exchanged between peers, along with ICE (Interactive Connectivity Establishment) candidates. Since WebRTC does not specify the mechanism for this message exchange, it must be facilitated through another protocol, such as WebSockets.

`WebRTC Conn` is transport-agnostic and uses Go channels to handle this exchange, allowing you to define your own transport mechanism.

Both `webrtcconn.OfferOptions` and `webrtcconn.AnswerOptions`  defines the following channels:

- **OfferChan**: Channel for sending the SDP offer.
- **AnswerChan**: Channel for receiving the SDP answer.
- **RemoteCandidates**: Channel for receiving remote ICE candidates from the other peer.
- **LocalCandidates**: Channel for sending local ICE candidates to the other peer.

### Creating an Offer

To create an offer, use the `webrtcconn.Offer` function. It requires a context, a `webrtc.Configuration` object for the peer connection, and `webrtcconn.OfferOptions` to specify the channels for exchanging SDP messages and ICE candidates with the answering peer.

```go
answerOfferChan := make(chan webrtc.SessionDescription)
offerAnswerChan := make(chan webrtc.SessionDescription)

answerCandidates := make(chan webrtc.ICECandidateInit)
offerCandidates := make(chan webrtc.ICECandidateInit)

oc, err := webrtcconn.Offer(ctx, webrtc.Configuration{}, webrtcconn.OfferOptions{
    AnswerChan:       offerAnswerChan,
    OfferChan:        answerOfferChan,
    RemoteCandidates: answerCandidates,
    LocalCandidates:  offerCandidates,
})
if err != nil {
    return err
}
```

### Responding to an Offer

To respond to an offer, use the `webrtcconn.Answer` function. Like the offer process, it requires a context, a `webrtc.Configuration`, and `webrtcconn.AnswerOptions` to manage the SDP and ICE candidate exchange.

```go
answerOfferChan := make(chan webrtc.SessionDescription)
offerAnswerChan := make(chan webrtc.SessionDescription)

answerCandidates := make(chan webrtc.ICECandidateInit)
offerCandidates := make(chan webrtc.ICECandidateInit)

ac, err := webrtcconn.Answer(ctx, webrtc.Configuration{}, webrtcconn.AnswerOptions{
    OfferChan:        answerOfferChan,
    AnswerChan:       offerAnswerChan,
    RemoteCandidates: offerCandidates,
    LocalCandidates:  answerCandidates,
})
if err != nil {
    return err
}
```

### Data Channel Configuration

When establishing a connection, the Offer role defines the data channel's configuration, which includes:

- **Ordered**: Specifies whether messages should be delivered in the order they were sent.
- **Max Retransmits**: Determines the maximum number of retransmission attempts for unreliable data channels.

