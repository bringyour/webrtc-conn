package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bringyour/webrtc-conn/rest/server"
	"github.com/stretchr/testify/require"
)

func TestSendOffer(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	exchangeStore := newMockExchangeStore()

	s := httptest.NewServer(server.NewHandler(exchangeStore))
	defer s.Close()

	u, err := url.Parse(s.URL)
	require.NoError(err)

	testCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		testCtx,
		"POST",
		u.JoinPath("abc/offer/").String(),
		strings.NewReader(`{"type":"offer","sdp":"v=0"}`),
	)
	require.NoError(err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(err)

	defer resp.Body.Close()

	require.Equal(http.StatusOK, resp.StatusCode)

	offer, err := exchangeStore.GetOffer(testCtx, "abc")
	require.NoError(err)
	require.Equal(`{"type":"offer","sdp":"v=0"}`, offer)

}

func TestReceiveOffer(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	exchangeStore := newMockExchangeStore()

	s := httptest.NewServer(server.NewHandler(exchangeStore))
	defer s.Close()

	u, err := url.Parse(s.URL)
	require.NoError(err)

	testCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err = exchangeStore.SetOffer(testCtx, "abc", `{"type":"offer","sdp":"v=0"}`)
	require.NoError(err)

	req, err := http.NewRequestWithContext(
		testCtx,
		"GET",
		u.JoinPath("abc/offer/").String(),
		nil,
	)
	require.NoError(err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(err)

	defer resp.Body.Close()

	require.Equal(http.StatusOK, resp.StatusCode)

	offer, err := io.ReadAll(resp.Body)
	require.NoError(err)

	require.Equal(`{"type":"offer","sdp":"v=0"}`, string(offer))
}

const peerCandidateJSON = `
{
	"candidate": "candidate:842163049 1 udp 1677729535 192.168.1.1 3478 typ srflx raddr 10.0.0.1 rport 3478 generation 0 ufrag EEtu network-cost 50",
	"sdpMid": "0",
	"sdpMLineIndex": 0
}`

const secondPeerCandidateJSON = `
{
	"candidate": "candidate:142163049 1 udp 1677729535 192.168.1.1 3478 typ srflx raddr 10.0.0.1 rport 3478 generation 0 ufrag EEtu network-cost 50",
	"sdpMid": "0",
	"sdpMLineIndex": 0
}`

func TestSendOfferPeerCandidate(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	exchangeStore := newMockExchangeStore()

	s := httptest.NewServer(server.NewHandler(exchangeStore))
	defer s.Close()

	u, err := url.Parse(s.URL)
	require.NoError(err)

	testCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		testCtx,
		"POST",
		u.JoinPath("abc/offer/peer_candidate/").String(),
		strings.NewReader(peerCandidateJSON),
	)
	require.NoError(err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(err)

	defer resp.Body.Close()

	require.Equal(http.StatusOK, resp.StatusCode)

	offer, err := exchangeStore.GetOfferICEPeerCandidates(testCtx, "abc", 0)
	require.NoError(err)
	require.Equal([]string{peerCandidateJSON}, offer)

}

func TestReceiveOfferPeerCandidate(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	exchangeStore := newMockExchangeStore()

	exchangeStore.AddOfferICEPeerCandidate(context.Background(), "abc", peerCandidateJSON)

	s := httptest.NewServer(server.NewHandler(exchangeStore))
	defer s.Close()

	u, err := url.Parse(s.URL)
	require.NoError(err)

	testCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		testCtx,
		"GET",
		u.JoinPath("abc/offer/peer_candidates/").String(),
		nil,
	)
	require.NoError(err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(err)
	defer resp.Body.Close()

	require.Equal(http.StatusOK, resp.StatusCode)

	responseBytes, err := io.ReadAll(resp.Body)
	require.NoError(err)

	require.JSONEq("["+string(peerCandidateJSON)+"]", string(responseBytes))

}
