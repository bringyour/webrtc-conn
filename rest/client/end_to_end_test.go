package client_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bringyour/webrtc-conn/rest/client"
	"github.com/bringyour/webrtc-conn/rest/server"
	"github.com/bringyour/webrtc-conn/rest/server/store/redisstore"
	"github.com/pion/webrtc/v3"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/sync/errgroup"
)

func TestEndToEnd(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := testcontainers.ContainerRequest{
		Image:        "redis:latest",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections"),
	}
	redisC, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		},
	)
	if err != nil {
		t.Fatalf("Could not start redis: %s", err)
	}

	defer func() {
		err := redisC.Terminate(context.Background())
		if err != nil {
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

	s := httptest.NewServer(server.NewHandler(st))
	defer s.Close()

	wg, ctx := errgroup.WithContext(ctx)

	var answerReceived string

	wg.Go(func() error {
		c, err := client.Answer(ctx, s.URL+"/abc/", nil, webrtc.Configuration{})
		if err != nil {
			return fmt.Errorf("cannot answer: %w", err)
		}
		_, err = c.Write([]byte("hello from answer"))
		if err != nil {
			return fmt.Errorf("cannot write: %w", err)
		}

		b := make([]byte, 1024)
		n, err := c.Read(b)
		if err != nil {
			return fmt.Errorf("cannot read: %w", err)
		}

		answerReceived = string(b[:n])
		return c.Close()
	})

	var offerReceived string

	wg.Go(func() error {
		c, err := client.Offer(ctx, s.URL+"/abc/", nil, webrtc.Configuration{}, false, 0)
		if err != nil {
			return fmt.Errorf("cannot offer: %w", err)
		}

		_, err = c.Write([]byte("hello from offer"))
		if err != nil {
			return fmt.Errorf("cannot write: %w", err)
		}

		b := make([]byte, 1024)
		n, err := c.Read(b)
		if err != nil {
			return fmt.Errorf("cannot read: %w", err)
		}

		offerReceived = string(b[:n])

		return c.Close()
	})

	err = wg.Wait()
	require.NoError(t, err)

	require.Equal(t, "hello from offer", answerReceived)
	require.Equal(t, "hello from answer", offerReceived)

}
