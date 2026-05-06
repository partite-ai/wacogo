package streams

import "sync"

type subscriptionSet struct {
	mu            sync.Mutex
	subscriptions []*subscription
}

type subscription struct {
	cond func() bool
	ch   chan struct{}
}

func (s *subscriptionSet) subscribe(cond func() bool) chan struct{} {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	if cond() {
		s.mu.Unlock()
		close(ch)
		return ch
	}
	s.subscriptions = append(s.subscriptions, &subscription{cond: cond, ch: ch})
	s.mu.Unlock()
	return ch
}

func (s *subscriptionSet) notify() {
	s.mu.Lock()
	remaining := s.subscriptions[:0]
	for _, sub := range s.subscriptions {
		if sub.cond() {
			close(sub.ch)
		} else {
			remaining = append(remaining, sub)
		}
	}
	s.subscriptions = remaining
	s.mu.Unlock()
}
