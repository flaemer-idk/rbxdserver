// Package session — учёт игроков.
//
// Источник правды — presence из rbxd: вебсервер плейса сам знает, кто в игре
// (заполняется при проходе через join-турникет и чистится Lua-хуком
// PlayerRemoving внутри RCC через /rfd/player-left). rbxdserver опрашивает
// `/rfd/presence` и кормит сюда снапшоты (SetPresence).
//
// Правило автостопа: если в игре никого `EmptyTimeout` подряд — onEmpty()
// (остановка сессии). Зашёл хотя бы один — таймер снимается, в т.ч. в
// последние секунды: зашёл → таймер отменён, сессия живёт.
//
// WebSocket /session — легаси-канал от старого клиента: считается только как
// «соединение» (для статистики), на автостоп и на список больше не влияет.
package session

import (
	"context"
	"log"
	"sync"

	"rbxdserver/internal/logtags"
	"time"
)

type PresencePlayer struct {
	IDNum    int     `json:"id_num"`
	UserCode string  `json:"user_code"`
	Username string  `json:"username"`
	JoinedAt float64 `json:"joined_at"`
}

type Manager struct {
	mu           sync.Mutex
	connections  map[string]bool        // легаси: открытые WS-соединения
	players      map[int]PresencePlayer // кто реально в игре (из presence rbxd)
	stopTimer    context.CancelFunc
	timerSeq     int
	emptyTimeout time.Duration
	onEmpty      func()
}

func NewManager(onEmpty func(), emptyTimeout time.Duration) *Manager {
	return &Manager{
		connections:  make(map[string]bool),
		players:      make(map[int]PresencePlayer),
		emptyTimeout: emptyTimeout,
		onEmpty:      onEmpty,
	}
}

// SetPresence заменяет снапшот присутствия и управляет таймером пустоты.
func (m *Manager) SetPresence(list []PresencePlayer) {
	m.mu.Lock()

	prev := m.players
	next := make(map[int]PresencePlayer, len(list))
	for _, p := range list {
		next[p.IDNum] = p
	}
	m.players = next

	for id, p := range next {
		if _, ok := prev[id]; !ok {
			log.Printf(logtags.Session+" in game: %s (id %d, user_code %s)", p.Username, id, p.UserCode)
		}
	}
	for id, p := range prev {
		if _, ok := next[id]; !ok {
			log.Printf(logtags.Session+" left game: %s (id %d)", p.Username, id)
		}
	}

	if len(next) > 0 {
		// Кто-то в игре (в т.ч. зашёл в последние секунды таймера) — не умираем.
		if m.stopTimer != nil {
			m.stopTimer()
			m.stopTimer = nil
			log.Println(logtags.Session + " empty timer cancelled: someone is in game.")
		}
		m.mu.Unlock()
		return
	}

	// Пусто: заводим таймер (если ещё не заведён).
	if m.stopTimer != nil {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.stopTimer = cancel
	m.timerSeq++
	seq := m.timerSeq
	m.mu.Unlock()

	log.Printf(logtags.Session+" nobody in game. Empty timer: %s", m.emptyTimeout)
	go func() {
		select {
		case <-time.After(m.emptyTimeout):
			m.mu.Lock()
			fire := len(m.players) == 0 && m.timerSeq == seq
			if fire {
				m.stopTimer = nil
			}
			m.mu.Unlock()
			if fire {
				log.Println(logtags.Session + " empty timer expired, stopping session.")
				m.onEmpty()
			}
		case <-ctx.Done():
			// Кто-то зашёл — таймер отменён.
		}
	}()
}

// ---- легаси WS-соединения (старый клиент): только счётчик ----

func (m *Manager) Join(user string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connections[user] = true
	log.Printf(logtags.Session+" WS connection %s opened. Total: %d", user, len(m.connections))
}

func (m *Manager) Leave(user string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.connections, user)
	log.Printf(logtags.Session+" WS connection %s closed. Total: %d", user, len(m.connections))
}

func (m *Manager) Connections() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.connections)
}

// ---- снимки для /status ----

func (m *Manager) InGame() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.players)
}

func (m *Manager) Players() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.players))
	for _, p := range m.players {
		names = append(names, p.Username)
	}
	return names
}

func (m *Manager) PlayersDetail() []PresencePlayer {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]PresencePlayer, 0, len(m.players))
	for _, p := range m.players {
		out = append(out, p)
	}
	return out
}
