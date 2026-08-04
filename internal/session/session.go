package session

import (
	"context"
	"log"
	"sync"
	"time"
)

type Manager struct {
	mu          sync.Mutex
	sessions    map[string]bool
	cancelTimer context.CancelFunc
	onEmpty     func()
}

func NewManager(onEmpty func()) *Manager {
	return &Manager{
		sessions: make(map[string]bool),
		onEmpty:  onEmpty,
	}
}

func (m *Manager) Join(user string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[user] = true
	log.Printf("[Session] User %s joined. Total: %d", user, len(m.sessions))

	if m.cancelTimer != nil {
		m.cancelTimer()
		m.cancelTimer = nil
		log.Println("[Session] Shutdown timer cancelled.")
	}
}

func (m *Manager) Leave(user string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, user)
	log.Printf("[Session] User %s left. Total: %d", user, len(m.sessions))

	if len(m.sessions) == 0 {
		log.Println("[Session] No sessions left. Starting 15s grace period...")
		ctx, cancel := context.WithCancel(context.Background())
		m.cancelTimer = cancel

		go func() {
			select {
			case <-time.After(15 * time.Second):
				m.mu.Lock()
				if len(m.sessions) == 0 {
					m.onEmpty()
				}
				m.mu.Unlock()
			case <-ctx.Done():
				// Таймер отменен (кто-то зашел)
			}
		}()
	}
}

func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}