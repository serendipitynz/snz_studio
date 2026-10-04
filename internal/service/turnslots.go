package service

import "sync"

// turnSlots holds the chats with a generation in flight, one slot per chat. A
// second request for a held chat is refused (ErrTurnInProgress) rather than
// queued: the caller is a person who can see the first one still running, and a
// queued request would generate against history the first has not finished
// writing yet.
type turnSlots struct {
	mu   sync.Mutex
	held map[string]bool
}

func newTurnSlots() *turnSlots {
	return &turnSlots{held: map[string]bool{}}
}

func (s *turnSlots) acquire(chatID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held[chatID] {
		return false
	}
	s.held[chatID] = true
	return true
}

func (s *turnSlots) release(chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.held, chatID)
}
