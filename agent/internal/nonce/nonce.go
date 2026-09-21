// Package nonce provides replay attack protection for the agent.
// It maintains an in-memory store of recently seen nonces with TTL expiry.
//
// Thread-safe. Designed for single-node use (one agent process per node).
//
// SECURITY:
//   - Nonces are stored for 2× the task max age
//   - A nonce that appears twice is ALWAYS rejected (even if valid signature)
//   - The store survives within the agent process lifetime
//   - On agent restart, nonce history is lost — but tasks issued before restart
//     will be expired by their expires_at timestamp, so replay is still prevented
package nonce

import (
	"sync"
	"time"
)

// Store is a thread-safe in-memory nonce store with TTL expiry.
// It implements protocol.NonceStore.
type Store struct {
	mu      sync.RWMutex
	entries map[string]time.Time // nonce → expiry time
}

// New creates a new nonce Store and starts the background cleanup goroutine.
func New() *Store {
	s := &Store{
		entries: make(map[string]time.Time),
	}
	go s.cleanup()
	return s
}

// HasSeen returns true if the nonce was already seen and has not expired.
// A nonce that expired from the store CAN be replayed — but this is acceptable
// because the task's own expires_at timestamp would also have passed.
func (s *Store) HasSeen(nonce string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	expiry, exists := s.entries[nonce]
	if !exists {
		return false
	}
	// If the nonce TTL expired, treat as not seen
	// (task's expires_at would also have passed, so this is safe)
	return time.Now().Before(expiry)
}

// MarkSeen records a nonce with the given TTL.
// After TTL expires, the nonce entry is eligible for cleanup.
func (s *Store) MarkSeen(nonce string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[nonce] = time.Now().Add(ttl)
}

// Len returns the current number of stored nonces.
// Useful for monitoring.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// cleanup removes expired nonce entries periodically.
// Runs as a background goroutine for the lifetime of the store.
func (s *Store) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for nonce, expiry := range s.entries {
			if now.After(expiry) {
				delete(s.entries, nonce)
			}
		}
		s.mu.Unlock()
	}
}
