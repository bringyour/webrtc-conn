package webrtcconn_test

import (
	"context"
	"net"
	"testing"
	"time"

	webrtcconn "github.com/bringyour/webrtc-conn"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

func TestConn(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	type ConnOrError struct {
		Conn net.Conn
		Err  error
	}

	answerConn := make(chan ConnOrError)
	offerConn := make(chan ConnOrError)

	answerOfferChan := make(chan webrtc.SessionDescription)
	offerAnswerChan := make(chan webrtc.SessionDescription)

	answerCandidates := make(chan webrtc.ICECandidateInit)
	offerCandidates := make(chan webrtc.ICECandidateInit)

	go func() {
		ac, err := webrtcconn.Answer(ctx, webrtc.Configuration{}, webrtcconn.AnswerOptions{
			OfferChan:        answerOfferChan,
			AnswerChan:       offerAnswerChan,
			RemoteCandidates: offerCandidates,
			LocalCandidates:  answerCandidates,
		})
		answerConn <- ConnOrError{Conn: ac, Err: err}
	}()

	go func() {
		oc, err := webrtcconn.Offer(ctx, webrtc.Configuration{}, webrtcconn.OfferOptions{
			AnswerChan:       offerAnswerChan,
			OfferChan:        answerOfferChan,
			RemoteCandidates: answerCandidates,
			LocalCandidates:  offerCandidates,
		})
		offerConn <- ConnOrError{Conn: oc, Err: err}
	}()

	ans := <-answerConn
	require.NoError(t, ans.Err)

	off := <-offerConn
	require.NoError(t, off.Err)

	_, err := ans.Conn.Write([]byte("hello1"))
	require.NoError(t, err)

	buf := make([]byte, 1024)
	n, err := off.Conn.Read(buf)
	require.NoError(t, err)

	require.Equal(t, "hello1", string(buf[:n]))

	_, err = off.Conn.Write([]byte("hello2"))
	require.NoError(t, err)

	err = ans.Conn.Close()
	require.NoError(t, err)

	err = off.Conn.Close()
	require.NoError(t, err)

}
