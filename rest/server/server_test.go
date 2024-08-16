package server_test

import (
	"context"
	"sync"
)

// mockExchangeStore is a mock implementation of the ExchangeStore interface.
type mockExchangeStore struct {
	offer            map[string]string
	answer           map[string]string
	offerCandidates  map[string][]string
	answerCandidates map[string][]string
	mu               *sync.Mutex
	cond             *sync.Cond
}

func newMockExchangeStore() *mockExchangeStore {
	mu := &sync.Mutex{}
	return &mockExchangeStore{
		offer:            make(map[string]string),
		answer:           make(map[string]string),
		offerCandidates:  make(map[string][]string),
		answerCandidates: make(map[string][]string),
		mu:               mu,
		cond:             sync.NewCond(mu),
	}
}

func (m *mockExchangeStore) GetOffer(ctx context.Context, id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	context.AfterFunc(ctx, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.cond.Broadcast()
	})

	for {
		offer, ok := m.offer[id]
		if ok {
			return offer, nil
		}

		m.cond.Wait()
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}

}

func (m *mockExchangeStore) SetOffer(ctx context.Context, id, sdp string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.offer[id] = sdp
	m.cond.Broadcast()
	return nil
}

func (m *mockExchangeStore) GetAnswer(ctx context.Context, id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	context.AfterFunc(ctx, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.cond.Broadcast()
	})

	for {
		answer, ok := m.answer[id]
		if ok {
			return answer, nil
		}

		m.cond.Wait()
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}

}

func (m *mockExchangeStore) SetAnswer(ctx context.Context, id, sdp string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answer[id] = sdp
	m.cond.Broadcast()
	return nil
}

func (m *mockExchangeStore) AddOfferICEPeerCandidate(ctx context.Context, id, candidate string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.offerCandidates[id] = append(m.offerCandidates[id], candidate)
	m.cond.Broadcast()
	return nil
}

func (m *mockExchangeStore) AddAnswerICEPeerCandidate(ctx context.Context, id, candidate string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answerCandidates[id] = append(m.answerCandidates[id], candidate)
	m.cond.Broadcast()
	return nil
}

func (m *mockExchangeStore) GetOfferICEPeerCandidates(ctx context.Context, id string, seenSoFar int) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	context.AfterFunc(ctx, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.cond.Broadcast()
	})

	for {
		candidates, ok := m.offerCandidates[id]
		if ok && len(candidates) > seenSoFar {
			return candidates[seenSoFar:], nil
		}

		m.cond.Wait()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
}

func (m *mockExchangeStore) GetAnswerICEPeerCandidates(ctx context.Context, id string, seenSoFar int) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	context.AfterFunc(ctx, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.cond.Broadcast()
	})

	for {
		candidates, ok := m.answerCandidates[id]
		if ok && len(candidates) > seenSoFar {
			return candidates[seenSoFar:], nil
		}

		m.cond.Wait()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
}
