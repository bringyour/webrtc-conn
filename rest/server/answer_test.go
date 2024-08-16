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

func TestSendAnswer(t *testing.T) {
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
		u.JoinPath("abc/answer/").String(),
		strings.NewReader(`{"type":"answer","sdp":"v=0"}`),
	)
	require.NoError(err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(err)

	defer resp.Body.Close()

	require.Equal(http.StatusOK, resp.StatusCode)

	offer, err := exchangeStore.GetAnswer(testCtx, "abc")
	require.NoError(err)
	require.Equal(`{"type":"answer","sdp":"v=0"}`, offer)

}

func TestReceiveAnswer(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	exchangeStore := newMockExchangeStore()

	s := httptest.NewServer(server.NewHandler(exchangeStore))
	defer s.Close()

	u, err := url.Parse(s.URL)
	require.NoError(err)

	testCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err = exchangeStore.SetAnswer(testCtx, "abc", `{"type":"answer","sdp":"v=0"}`)
	require.NoError(err)

	req, err := http.NewRequestWithContext(
		testCtx,
		"GET",
		u.JoinPath("abc/answer/").String(),
		nil,
	)
	require.NoError(err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(err)

	defer resp.Body.Close()

	require.Equal(http.StatusOK, resp.StatusCode)

	offer, err := io.ReadAll(resp.Body)
	require.NoError(err)

	require.Equal(`{"type":"answer","sdp":"v=0"}`, string(offer))
}

func TestSendAnswerPeerCandidate(t *testing.T) {
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
		u.JoinPath("abc/answer/peer_candidate/").String(),
		strings.NewReader(peerCandidateJSON),
	)
	require.NoError(err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(err)

	defer resp.Body.Close()

	require.Equal(http.StatusOK, resp.StatusCode)

	offer, err := exchangeStore.GetAnswerICEPeerCandidates(testCtx, "abc", 0)
	require.NoError(err)
	require.Equal([]string{peerCandidateJSON}, offer)

}

func TestReceiveAnswerPeerCandidate(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	exchangeStore := newMockExchangeStore()

	exchangeStore.AddAnswerICEPeerCandidate(context.Background(), "abc", peerCandidateJSON)

	s := httptest.NewServer(server.NewHandler(exchangeStore))
	defer s.Close()

	u, err := url.Parse(s.URL)
	require.NoError(err)

	testCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		testCtx,
		"GET",
		u.JoinPath("abc/answer/peer_candidates/").String(),
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
