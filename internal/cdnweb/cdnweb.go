// Package cdnweb — постоянный CDN-веб (rbxd `webserver` без --config).
//
// Стартует вместе с демоном и живёт всё время: раздаёт общий пул ассетов
// (data/Assets), скины и thumbnails для клиентов (rbxdclient) независимо от
// того, идёт ли сейчас какая-то сессия плейса. Чистый Python, без Wine.
package cdnweb

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"rbxdserver/internal/config"

	"rbxdserver/internal/logtags"
	"rbxdserver/internal/rfdproc"
)

type Manager struct {
	cfg     *config.Config
	mu      sync.Mutex
	proc    *rfdproc.Process
	stopped bool
	stopCh  chan struct{}
}

func New(cfg *config.Config) *Manager {
	return &Manager{cfg: cfg, stopCh: make(chan struct{})}
}

func (m *Manager) stopChLocked() <-chan struct{} { return m.stopCh }

// Start запускает CDN-веб в фоне и поддерживает его авторестартом с backoff.
// Готовность проверяется HTTP-запросом (GET /rfd/status), не TCP-поллингом.
func (m *Manager) Start() {
	go m.loop()
}

func (m *Manager) loop() {
	backoff := 2 * time.Second
	for {
		m.mu.Lock()
		if m.stopped {
			m.mu.Unlock()
			return
		}
		m.mu.Unlock()

		port := m.cfg.CDNPort
		proc := m.spawn(port)
		if err := proc.Start(); err != nil {
			log.Printf(logtags.CDN+" failed to spawn webserver: %v, retrying in %s", err, backoff)
			if !m.sleep(backoff) {
				return
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}

		m.mu.Lock()
		m.proc = proc
		m.mu.Unlock()

		log.Printf(logtags.CDN+" webserver starting on port %d (rbxd 'webserver' mode, no wine)", port)
		err := rfdproc.WaitForHTTP(port, "/rfd/status", 90*time.Second)
		if err == nil {
			log.Printf(logtags.CDN+" ready on https://127.0.0.1:%d/rfd/status — serving assets/skins for clients", port)
			backoff = 2 * time.Second
		} else {
			log.Printf(logtags.CDN+" readiness check failed: %v", err)
			proc.Stop()
		}

		// Ждём либо падения процесса, либо остановки демона.
		select {
		case <-proc.Exited():
			log.Printf(logtags.CDN + " webserver died, restarting")
			if !m.sleep(backoff) {
				return
			}
		case <-m.stopChLocked():
			return
		}
	}
}

func (m *Manager) spawn(port int) *rfdproc.Process {
	mainPy := filepath.Join(m.cfg.RfdDir, "Source", "_main.py")
	args := []string{
		mainPy, "webserver",
		"--ipv4-only",
		"--web_port", fmt.Sprintf("%d", port),
	}
	return rfdproc.NewProcess("python3", args, m.cfg.RfdDir, os.Environ(), "CDN")
}

func (m *Manager) sleep(d time.Duration) bool {
	m.mu.Lock()
	stopped := m.stopped
	m.mu.Unlock()
	if stopped {
		return false
	}
	select {
	case <-time.After(d):
		return true
	case <-m.stopChLocked():
		return false
	}
}

// Stop завершает CDN-веб и цикл авторестарта.
func (m *Manager) Stop() {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.stopped = true
	proc := m.proc
	m.proc = nil
	close(m.stopCh)
	m.mu.Unlock()

	if proc != nil {
		proc.Stop()
	}
}
