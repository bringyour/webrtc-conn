package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"

	webrtcconn "github.com/bringyour/webrtc-conn"
	"github.com/pion/webrtc/v3"
	"golang.org/x/sync/errgroup"
)

func Answer(
	ctx context.Context,
	baseURL string,
	config webrtc.Configuration,
) (c net.Conn, err error) {

	answerContext, cancel := context.WithCancel(ctx)
	defer cancel()

	eg, egCtx := errgroup.WithContext(answerContext)

	answerChan := make(chan webrtc.SessionDescription)
	offerChan := make(chan webrtc.SessionDescription)

	localCandidates := make(chan webrtc.ICECandidateInit)
	remoteCandidates := make(chan webrtc.ICECandidateInit)

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("cannot parse base url: %w", err)
	}

	co := &comm{
		u:  u,
		cl: http.DefaultClient,
	}

	eg.Go(func() (err error) {

		var offer webrtc.SessionDescription
		err = co.longPoll(egCtx, "/offer/sdp", nil, &offer)
		if err != nil {
			return fmt.Errorf("cannot get offer: %w", err)
		}
		return contextSend(egCtx, offerChan, offer)
	})

	eg.Go(func() (err error) {

		answer, err := contextReceive(egCtx, answerChan)
		if err != nil {
			return err
		}

		d, err := json.Marshal(answer)
		if err != nil {
			return fmt.Errorf("cannot marshal answer: %w", err)
		}

		err = co.put(answerContext, "/answer/sdp", string(d))
		if err != nil {
			return fmt.Errorf("cannot send answer: %w", err)
		}

		return nil

	})

	eg.Go(func() (err error) {

		for {
			c, err := contextReceive(egCtx, localCandidates)

			if err == errChanClosed {
				return nil
			}

			if err != nil {
				return err
			}

			d, err := json.Marshal(c.Candidate)
			if err != nil {
				return fmt.Errorf("cannot marshal candidate: %w", err)
			}

			err = co.post(answerContext, "/answer/peer_candidates", string(d))
			if err != nil {
				return fmt.Errorf("cannot send peer candidate: %w", err)
			}
		}
	})

	eg.Go(func() (err error) {

		receivedCandidates := 0
		for {
			candidates := []string{}
			err = co.longPoll(
				egCtx,
				"/offer/peer_candidates",
				map[string]string{
					"from": strconv.FormatInt(int64(receivedCandidates), 10),
				},
				&candidates,
			)
			if err != nil {
				return fmt.Errorf("cannot get offer peer candidates: %w", err)
			}

			for _, c := range candidates {
				err = contextSend(egCtx, remoteCandidates, webrtc.ICECandidateInit{Candidate: c})
				if err != nil {
					return err
				}
				receivedCandidates++
			}

		}
	})

	var conn net.Conn

	eg.Go(func() (err error) {

		defer cancel()

		conn, err = webrtcconn.Answer(ctx, config, webrtcconn.AnswerOptions{
			OfferChan:        offerChan,
			AnswerChan:       answerChan,
			LocalCandidates:  localCandidates,
			RemoteCandidates: remoteCandidates,
		})
		if err != nil {
			return fmt.Errorf("cannot answer: %w", err)
		}

		return nil
	})

	err = eg.Wait()
	if conn == nil {
		return nil, err
	}

	return conn, nil

}
