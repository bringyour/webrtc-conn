package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	webrtcconn "github.com/bringyour/webrtc-conn"
	"github.com/pion/webrtc/v3"
	"golang.org/x/sync/errgroup"
)

func contextSend[T any](ctx context.Context, ch chan T, value T) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- value:
		return nil
	}
}

var errChanClosed = fmt.Errorf("channel closed")

func contextReceive[T any](ctx context.Context, ch chan T) (T, error) {
	select {
	case <-ctx.Done():
		var empty T
		return empty, ctx.Err()
	case value, ok := <-ch:
		if !ok {
			return value, errChanClosed
		}
		return value, nil
	}
}

func Offer(
	ctx context.Context,
	baseURL string,
	requestHeader http.Header,
	config webrtc.Configuration,
	ordered bool,
	maxRetransmits uint16,
) (c net.Conn, err error) {

	offerContext, cancel := context.WithCancel(ctx)
	defer cancel()

	eg, egCtx := errgroup.WithContext(offerContext)

	offerChan := make(chan webrtc.SessionDescription)
	answerChan := make(chan webrtc.SessionDescription)

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

		var answer webrtc.SessionDescription
		err = co.longPoll(egCtx, "/answer/sdp", requestHeader, nil, &answer)
		if err != nil {
			return fmt.Errorf("cannot get answer: %w", err)
		}
		return contextSend(egCtx, answerChan, answer)
	})

	eg.Go(func() (err error) {

		offer, err := contextReceive(egCtx, offerChan)
		if err != nil {
			return err
		}

		d, err := json.Marshal(offer)
		if err != nil {
			return fmt.Errorf("cannot marshal offer: %w", err)
		}

		err = co.put(offerContext, "/offer/sdp", requestHeader, string(d))
		if err != nil {
			return fmt.Errorf("cannot send offer: %w", err)
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

			err = co.post(offerContext, "/offer/peer_candidates", requestHeader, string(d))
			if err != nil {
				return fmt.Errorf("cannot send candidate: %w", err)
			}
		}
	})

	eg.Go(func() (err error) {

		receivedCandidates := 0
		for {
			candidates := []string{}
			err = co.longPoll(
				egCtx,
				"/answer/peer_candidates",
				requestHeader,
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
		conn, err = webrtcconn.Offer(ctx, config, webrtcconn.OfferOptions{
			OfferChan:       offerChan,
			AnswerChan:      answerChan,
			LocalCandidates: localCandidates,

			RemoteCandidates: remoteCandidates,
			Ordered:          ordered,
			MaxRetransmits:   maxRetransmits,
		})
		if err != nil {
			return fmt.Errorf("cannot create offer: %w", err)
		}
		return nil
	})

	err = eg.Wait()

	if conn == nil {
		return nil, err
	}

	return conn, nil

}

type comm struct {
	u  *url.URL
	cl *http.Client
}

func (c *comm) post(ctx context.Context, path string, requestHeader http.Header, body string) error {

	req, err := http.NewRequestWithContext(ctx, "POST", c.u.JoinPath(path).String(), strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("cannot create request: %w", err)
	}

	mergeIntoHeaders(req.Header, requestHeader)

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.cl.Do(req)
	if err != nil {
		return fmt.Errorf("cannot send request: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil

}

func (c *comm) put(ctx context.Context, path string, requestHeader http.Header, body string) error {

	req, err := http.NewRequestWithContext(ctx, "PUT", c.u.JoinPath(path).String(), strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("cannot create request: %w", err)
	}

	mergeIntoHeaders(req.Header, requestHeader)

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.cl.Do(req)
	if err != nil {
		return fmt.Errorf("cannot send request: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil

}

var errPollTimeout = fmt.Errorf("poll timeout")

func (c *comm) longPoll(ctx context.Context, path string, requestHeader http.Header, q map[string]string, res any) error {

	u := c.u.JoinPath(path)
	if q != nil {
		qs := u.Query()
		for k, v := range q {
			qs.Set(k, v)
		}
		u.RawQuery = qs.Encode()
	}

	for {
		err := c.poll(ctx, u.String(), requestHeader, res)
		if err == errPollTimeout {
			continue
		}
		if err != nil {
			return err
		}
		break
	}

	return nil
}

func mergeIntoHeaders(a, b http.Header) {
	if b == nil {
		return
	}

	for k, v := range b {
		a[k] = append(a[k], v...)
	}
}

func (c *comm) poll(ctx context.Context, u string, requestHeader http.Header, res any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return fmt.Errorf("cannot create request: %w", err)
	}

	mergeIntoHeaders(req.Header, requestHeader)

	resp, err := c.cl.Do(req)
	if err != nil {
		return fmt.Errorf("cannot send request: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return errPollTimeout
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	d, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("cannot read response: %w", err)
	}

	err = json.Unmarshal(d, res)
	if err != nil {
		return fmt.Errorf("cannot decode response: %w", err)
	}

	return nil
}
