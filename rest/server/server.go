package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"
)

const pollTimeout = time.Second * 15

type ExchangeStore interface {
	GetOffer(ctx context.Context, id string) (string, error)
	SetOffer(ctx context.Context, id, sdp string) error

	GetAnswer(ctx context.Context, id string) (string, error)
	SetAnswer(ctx context.Context, id, sdp string) error

	AddOfferICEPeerCandidate(ctx context.Context, id, candidate string) error
	GetOfferICEPeerCandidates(ctx context.Context, id string, seenSoFar int) ([]string, error)

	AddAnswerICEPeerCandidate(ctx context.Context, id, candidate string) error
	GetAnswerICEPeerCandidates(ctx context.Context, id string, seenSoFar int) ([]string, error)
}

func Run(ctx context.Context, addr string, store ExchangeStore) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot listen: %w", err)
	}

	defer l.Close()

	eg, ctx := errgroup.WithContext(ctx)

	mux := NewHandler(store)

	s := &http.Server{
		Handler: mux,
	}

	eg.Go(func() error {
		<-ctx.Done()
		// Shutdown the server when the context is canceled
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		err := s.Shutdown(shutdownCtx)
		if err != nil {
			return s.Close()
		}
		return nil
	})

	eg.Go(func() error {
		return s.Serve(l)
	})

	return eg.Wait()

}

func NewHandler(store ExchangeStore) http.Handler {

	mux := http.NewServeMux()

	mux.HandleFunc("POST /{id}/offer/sdp/{$}", func(w http.ResponseWriter, r *http.Request) {
		d, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		id := r.PathValue("id")

		err = store.SetOffer(r.Context(), id, string(d))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	mux.HandleFunc("POST /{id}/offer/peer_candidate/{$}", func(w http.ResponseWriter, r *http.Request) {
		d, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		id := r.PathValue("id")

		err = store.AddOfferICEPeerCandidate(r.Context(), id, string(d))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	mux.HandleFunc("GET /{id}/offer/sdp/{$}", func(w http.ResponseWriter, r *http.Request) {

		id := r.PathValue("id")

		ctx, cancel := context.WithTimeout(r.Context(), pollTimeout)
		defer cancel()

		sdp, err := store.GetOffer(ctx, id)
		if errors.Is(err, context.DeadlineExceeded) {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.Write([]byte(sdp))
	})

	mux.HandleFunc("GET /{id}/offer/peer_candidates/{$}", func(w http.ResponseWriter, r *http.Request) {

		fromString := r.URL.Query().Get("from")

		from := uint64(0)
		if fromString != "" {
			var err error
			from, err = strconv.ParseUint(fromString, 10, 64)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		id := r.PathValue("id")

		ctx, cancel := context.WithTimeout(r.Context(), pollTimeout)
		defer cancel()

		candidates, err := store.GetOfferICEPeerCandidates(ctx, id, int(from))
		if errors.Is(err, context.DeadlineExceeded) {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		candidatesArray := []json.RawMessage{}
		for _, c := range candidates {
			candidatesArray = append(candidatesArray, json.RawMessage(c))
		}

		json.NewEncoder(w).Encode(candidatesArray)

	})

	mux.HandleFunc("POST /{id}/answer/sdp/{$}", func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()

		d, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		id := r.PathValue("id")

		err = store.SetAnswer(ctx, id, string(d))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	mux.HandleFunc("POST /{id}/answer/peer_candidate/{$}", func(w http.ResponseWriter, r *http.Request) {
		d, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		id := r.PathValue("id")

		err = store.AddAnswerICEPeerCandidate(r.Context(), id, string(d))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	mux.HandleFunc("GET /{id}/answer/sdp/{$}", func(w http.ResponseWriter, r *http.Request) {

		id := r.PathValue("id")

		ctx, cancel := context.WithTimeout(r.Context(), pollTimeout)
		defer cancel()

		sdp, err := store.GetAnswer(ctx, id)
		if errors.Is(err, context.DeadlineExceeded) {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.Write([]byte(sdp))
	})

	mux.HandleFunc("GET /{id}/answer/peer_candidates/{$}", func(w http.ResponseWriter, r *http.Request) {

		fromString := r.URL.Query().Get("from")

		from := uint64(0)
		if fromString != "" {
			var err error
			from, err = strconv.ParseUint(fromString, 10, 64)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		id := r.PathValue("id")

		ctx, cancel := context.WithTimeout(r.Context(), pollTimeout)
		defer cancel()

		candidates, err := store.GetAnswerICEPeerCandidates(ctx, id, int(from))
		if errors.Is(err, context.DeadlineExceeded) {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		candidatesArray := []json.RawMessage{}
		for _, c := range candidates {
			candidatesArray = append(candidatesArray, json.RawMessage(c))
		}

		json.NewEncoder(w).Encode(candidatesArray)

	})

	return mux

}
