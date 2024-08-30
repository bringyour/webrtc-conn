package webrtcconn_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	webrtcconn "github.com/bringyour/webrtc-conn"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

type inMemorySync struct {
	offer            *webrtc.SessionDescription
	answer           *webrtc.SessionDescription
	offerCandidates  []webrtc.ICECandidate
	answerCandidates []webrtc.ICECandidate
	mu               *sync.Mutex
	cond             *sync.Cond
}

func (s *inMemorySync) OfferSDP(ctx context.Context, offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {

	s.mu.Lock()
	defer s.mu.Unlock()

	s.offer = &offer
	s.cond.Broadcast()

	for {

		if ctx.Err() != nil {
			return webrtc.SessionDescription{}, ctx.Err()
		}

		if s.answer != nil {
			return *s.answer, nil
		}

		s.cond.Wait()
	}

}

func (s *inMemorySync) AnswerSDP(ctx context.Context, answer func(ctx context.Context, offer webrtc.SessionDescription) (webrtc.SessionDescription, error)) error {
	s.mu.Lock()

	for {
		if ctx.Err() != nil {
			s.mu.Unlock()
			return ctx.Err()
		}

		if s.offer != nil {
			break
		}
		s.cond.Wait()
	}

	s.mu.Unlock()

	ans, err := answer(ctx, *s.offer)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.answer = &ans
	s.cond.Broadcast()
	s.mu.Unlock()

	return nil

}

// GetPeerCandidates returns the ICE candidates of the other client.
func (s *inMemorySync) GetOfferPeerCandidates(ctx context.Context) ([]webrtc.ICECandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if len(s.offerCandidates) > 0 {
			candidates := s.offerCandidates
			s.offerCandidates = nil
			return candidates, nil
		}
		s.cond.Wait()
	}

}

func (s *inMemorySync) GetAnswerPeerCandidates(ctx context.Context) ([]webrtc.ICECandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if len(s.answerCandidates) > 0 {
			candidates := s.answerCandidates
			s.answerCandidates = nil
			return candidates, nil
		}
		s.cond.Wait()
	}

}

// AddPeerCandidate adds an ICE candidate for the other client.
func (s *inMemorySync) AddAnswerPeerCandidate(ctx context.Context, candidate webrtc.ICECandidate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.answerCandidates = append(s.answerCandidates, candidate)
	s.cond.Broadcast()

	return nil
}

// AddPeerCandidate adds an ICE candidate for the other client.
func (s *inMemorySync) AddOfferPeerCandidate(ctx context.Context, candidate webrtc.ICECandidate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.offerCandidates = append(s.offerCandidates, candidate)
	s.cond.Broadcast()

	return nil
}

func newInMemorySync() *inMemorySync {
	mu := &sync.Mutex{}
	return &inMemorySync{
		mu:   mu,
		cond: sync.NewCond(mu),
	}
}

func TestConn(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type ConnOrError struct {
		Conn net.Conn
		Err  error
	}

	answerConn := make(chan ConnOrError)
	offerConn := make(chan ConnOrError)

	s := newInMemorySync()

	go func() {
		ac, err := webrtcconn.Answer(ctx, webrtc.Configuration{}, s, 3*time.Second)
		answerConn <- ConnOrError{Conn: ac, Err: err}
	}()

	go func() {
		oc, err := webrtcconn.Offer(ctx, webrtc.Configuration{}, s, true, 1, 3*time.Second)
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
