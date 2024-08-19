package redisstore_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bringyour/webrtc-conn/rest/server/store/redisstore"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/sync/errgroup"
)

func TestExchangeOffer(t *testing.T) {

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "redis:latest",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections"),
	}
	redisC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Could not start redis: %s", err)
	}
	defer func() {
		if err := redisC.Terminate(ctx); err != nil {
			t.Fatalf("Could not stop redis: %s", err)
		}
	}()

	ep, err := redisC.Endpoint(ctx, "")
	require.NoError(t, err)

	st := redisstore.NewExchangeStore(
		"test:",
		&redis.Options{
			Addr: ep,
		},
	)

	t.Run("ExchangeOffer", func(t *testing.T) {

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		eg, ctx := errgroup.WithContext(ctx)

		eg.Go(func() error {
			time.Sleep(300 * time.Millisecond)
			err := st.SetOffer(ctx, "offerTest", "offer body")
			if err != nil {
				return fmt.Errorf("SetOffer failed: %w", err)
			}
			return nil
		})

		var receivedOffer string

		eg.Go(func() error {
			offer, err := st.GetOffer(ctx, "offerTest")
			if err != nil {
				return fmt.Errorf("GetOffer failed: %w", err)
			}
			receivedOffer = offer
			return nil
		})

		err := eg.Wait()
		require.NoError(t, err)

		require.Equal(t, "offer body", receivedOffer)
	})

	t.Run("ExchangeAnswer", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		eg, ctx := errgroup.WithContext(ctx)

		eg.Go(func() error {
			time.Sleep(300 * time.Millisecond)
			err := st.SetAnswer(ctx, "answerTest", "answer body")
			if err != nil {
				return fmt.Errorf("SetAnswer failed: %w", err)
			}
			return nil
		})

		var receivedAnswer string

		eg.Go(func() error {
			answer, err := st.GetAnswer(ctx, "answerTest")
			if err != nil {
				return fmt.Errorf("GetAnswer failed: %w", err)
			}
			receivedAnswer = answer
			return nil
		})

		err := eg.Wait()
		require.NoError(t, err)

		require.Equal(t, "answer body", receivedAnswer)
	})

	t.Run("ExchangeOfferICEPeerCandidates", func(t *testing.T) {

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		eg, ctx := errgroup.WithContext(ctx)

		eg.Go(func() error {

			for i := 0; i < 5; i++ {
				err = st.AddOfferICEPeerCandidate(ctx, "offerTest", fmt.Sprintf("candidate_%d", i))
				if err != nil {
					return fmt.Errorf("AddOfferICEPeerCandidate failed: %w", err)
				}
				time.Sleep(100 * time.Millisecond)
			}
			return nil
		})

		var receivedCandidates []string

		eg.Go(func() error {
			for {
				candidates, err := st.GetOfferICEPeerCandidates(ctx, "offerTest", len(receivedCandidates))
				if err != nil {
					return fmt.Errorf("GetOfferICEPeerCandidates failed: %w", err)
				}

				receivedCandidates = append(receivedCandidates, candidates...)

				if len(receivedCandidates) == 5 {
					break
				}
			}
			return nil
		})

		err := eg.Wait()
		require.NoError(t, err)

		require.Equal(
			t,
			[]string{
				"candidate_0",
				"candidate_1",
				"candidate_2",
				"candidate_3",
				"candidate_4",
			},
			receivedCandidates,
		)

	})

	t.Run("ExchangeAnswerICEPeerCandidates", func(t *testing.T) {

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		eg, ctx := errgroup.WithContext(ctx)

		eg.Go(func() error {

			for i := 0; i < 5; i++ {
				err = st.AddAnswerICEPeerCandidate(ctx, "answerTest", fmt.Sprintf("candidate_%d", i))
				if err != nil {
					return fmt.Errorf("AddAnswerICEPeerCandidate failed: %w", err)
				}
				time.Sleep(100 * time.Millisecond)
			}
			return nil
		})

		var receivedCandidates []string

		eg.Go(func() error {
			for {
				candidates, err := st.GetAnswerICEPeerCandidates(ctx, "answerTest", len(receivedCandidates))
				if err != nil {
					return fmt.Errorf("GetOfferICEPeerCandidates failed: %w", err)
				}

				receivedCandidates = append(receivedCandidates, candidates...)

				if len(receivedCandidates) == 5 {
					break
				}
			}
			return nil
		})

		err := eg.Wait()
		require.NoError(t, err)

		require.Equal(
			t,
			[]string{
				"candidate_0",
				"candidate_1",
				"candidate_2",
				"candidate_3",
				"candidate_4",
			},
			receivedCandidates,
		)

	})

}
