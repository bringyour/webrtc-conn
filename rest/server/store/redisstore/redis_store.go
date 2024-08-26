package redisstore

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const objectExpire = 60 * time.Second

type ExchangeStore struct {
	keyPrefix string
	cl        *redis.Client
}

func NewExchangeStore(
	keyPrefix string,
	redisOpts *redis.Options,
) *ExchangeStore {

	cl := redis.NewClient(redisOpts)

	return &ExchangeStore{
		cl: cl,
	}
}

func (e *ExchangeStore) offerKey(id string) string {
	return fmt.Sprintf("%s:%s:offer", e.keyPrefix, id)
}

func (e *ExchangeStore) answerKey(id string) string {
	return fmt.Sprintf("%s:%s:answer", e.keyPrefix, id)
}

func (e *ExchangeStore) GetOffer(ctx context.Context, id string) (string, error) {
	offerKey := e.offerKey(id)
	sub := e.cl.Subscribe(ctx, offerKey)
	defer sub.Close()

	res := e.cl.Get(ctx, offerKey)

	if res.Err() == redis.Nil {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-sub.Channel():
			res = e.cl.Get(ctx, offerKey)
		}
	}

	if res.Err() != nil {
		return "", res.Err()
	}

	return res.Val(), nil

}

func (e *ExchangeStore) SetOffer(ctx context.Context, id, sdp string) error {

	offerKey := e.offerKey(id)

	pipeline := e.cl.Pipeline()
	pipeline.Set(ctx, offerKey, sdp, 0)
	pipeline.Expire(ctx, offerKey, objectExpire)
	pipeline.Publish(ctx, offerKey, "sent")
	_, err := pipeline.Exec(ctx)

	return err
}

func (e *ExchangeStore) GetAnswer(ctx context.Context, id string) (string, error) {
	answerKey := e.answerKey(id)
	sub := e.cl.Subscribe(ctx, answerKey)
	defer sub.Close()

	res := e.cl.Get(ctx, answerKey)

	if res.Err() == redis.Nil {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-sub.Channel():
			res = e.cl.Get(ctx, answerKey)
		}
	}

	if res.Err() != nil {
		return "", res.Err()
	}

	return res.Val(), nil
}

func (e *ExchangeStore) SetAnswer(ctx context.Context, id, sdp string) error {
	answerKey := e.answerKey(id)

	pipeline := e.cl.Pipeline()
	pipeline.Set(ctx, answerKey, sdp, 0)
	pipeline.Publish(ctx, answerKey, "sent")
	pipeline.Expire(ctx, answerKey, objectExpire)

	_, err := pipeline.Exec(ctx)

	return err
}

func (e *ExchangeStore) offerPeerCandidatesKey(id string) string {
	return fmt.Sprintf("%s:%s:offer:candidates", e.keyPrefix, id)
}

func (e *ExchangeStore) AddOfferICEPeerCandidate(ctx context.Context, id, candidate string) error {

	candidatesKey := e.offerPeerCandidatesKey(id)

	pl := e.cl.Pipeline()

	pl.RPush(ctx, candidatesKey, candidate)
	pl.ExpireNX(ctx, candidatesKey, objectExpire)
	pl.Publish(ctx, candidatesKey, "added")

	_, err := pl.Exec(ctx)

	return err
}

func (e *ExchangeStore) GetOfferICEPeerCandidates(ctx context.Context, id string, seenSoFar int) ([]string, error) {
	candidatesKey := e.offerPeerCandidatesKey(id)

	sub := e.cl.Subscribe(ctx, candidatesKey)
	defer sub.Close()

	slCmd := e.cl.LRange(ctx, candidatesKey, int64(seenSoFar), -1)
	err := slCmd.Err()
	if err != nil {
		return nil, err
	}

	vals := slCmd.Val()
	if len(vals) == 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-sub.Channel():
			slCmd = e.cl.LRange(ctx, candidatesKey, int64(seenSoFar), -1)
		}
	}

	vals = slCmd.Val()

	return vals, nil
}

func (e *ExchangeStore) answerPeerCandidatesKey(id string) string {
	return fmt.Sprintf("%s:%s:answer:candidates", e.keyPrefix, id)
}

func (e *ExchangeStore) AddAnswerICEPeerCandidate(ctx context.Context, id, candidate string) error {

	candidatesKey := e.answerPeerCandidatesKey(id)

	pl := e.cl.Pipeline()

	pl.RPush(ctx, candidatesKey, candidate)
	pl.ExpireNX(ctx, candidatesKey, objectExpire)
	pl.Publish(ctx, candidatesKey, "added")

	_, err := pl.Exec(ctx)

	return err
}

func (e *ExchangeStore) GetAnswerICEPeerCandidates(ctx context.Context, id string, seenSoFar int) ([]string, error) {
	candidatesKey := e.answerPeerCandidatesKey(id)

	sub := e.cl.Subscribe(ctx, candidatesKey)
	defer sub.Close()

	slCmd := e.cl.LRange(ctx, candidatesKey, int64(seenSoFar), -1)
	err := slCmd.Err()
	if err != nil {
		return nil, err
	}

	vals := slCmd.Val()

	fmt.Println("GetAnswerICEPeerCandidates t1", id, vals)

	if len(vals) == 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-sub.Channel():
			slCmd = e.cl.LRange(ctx, candidatesKey, int64(seenSoFar), -1)
		}
	}

	vals = slCmd.Val()

	fmt.Println("GetAnswerICEPeerCandidates t2", id, vals)

	return vals, nil
}
