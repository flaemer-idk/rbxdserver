package supervisor

import (
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"rbxdserver/internal/config"
)

type cmdReq struct {
	action string
	slug   string
	reply  chan error
}

type Supervisor struct {
	cfg          *config.Config
	state        string
	currentPlace string
	rccPort      int // ПАТЧ: Динамические порты
	webPort      int // ПАТЧ: Динамические порты
	proc         *Process
	cmds         chan cmdReq
	mu           sync.RWMutex
}

func New(cfg *config.Config) *Supervisor {
	s := &Supervisor{
		cfg:   cfg,
		state: "Idle",
		cmds:  make(chan cmdReq),
	}
	go s.loop()
	return s
}

func (s *Supervisor) loop() {
	for req := range s.cmds {
		switch req.action {
		case "start":
			req.reply <- s.handleStart(req.slug)
		case "stop":
			req.reply <- s.handleStop()
		}
	}
}

func (s *Supervisor) handleStart(slug string) error {
	if s.state == "Running" || s.state == "Starting" {
		if s.currentPlace == slug {
			return nil
		}
		s.handleStop()
	}

	s.setState("Starting", slug)
	
	placeConf := filepath.Join(s.cfg.PlacesDir, slug, "GameConfig.toml")
	
	// Находим случайные свободные порты
	rccPort, err := GetFreePort()
	if err != nil {
		s.setState("Idle", "")
		return fmt.Errorf("failed to allocate RCC port: %w", err)
	}
	webPort, err := GetFreePort()
	if err != nil {
		s.setState("Idle", "")
		return fmt.Errorf("failed to allocate Web port: %w", err)
	}

	s.mu.Lock()
	s.rccPort = rccPort
	s.webPort = webPort
	s.mu.Unlock()

	s.proc = NewProcess(s.cfg.RfdDir, placeConf, rccPort, webPort, s.cfg.StateDir, s.cfg.Test)
	
	if err := s.proc.Start(); err != nil {
		s.setState("Idle", "")
		return fmt.Errorf("failed to start process: %w", err)
	}

	// Опрашиваем Web-порт (TCP) вместо RCC-порта (UDP)
	if err := WaitForPort(webPort, 150*time.Second); err != nil {
		s.proc.Stop()
		s.setState("Idle", "")
		return fmt.Errorf("Web port check failed: %w", err)
	}

	s.setState("Running", slug)

	go func(p *Process, expectedSlug string) {
		p.Wait()
		s.cmds <- cmdReq{action: "crash_check", slug: expectedSlug, reply: make(chan error, 1)}
	}(s.proc, slug)

	return nil
}

func (s *Supervisor) handleStop() error {
	if s.state == "Idle" || s.proc == nil {
		return nil
	}
	s.setState("Stopping", s.currentPlace)
	s.proc.Stop()
	s.proc = nil
	s.setState("Idle", "")
	return nil
}

func (s *Supervisor) setState(state, place string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	s.currentPlace = place
	if state == "Idle" {
		s.rccPort = 0
		s.webPort = 0
	}
	log.Printf("[Supervisor] State changed to: %s (Place: %s)", state, place)
}

func (s *Supervisor) StartPlace(slug string) error {
	reply := make(chan error, 1)
	s.cmds <- cmdReq{action: "start", slug: slug, reply: reply}
	return <-reply
}

func (s *Supervisor) StopCurrentPlace() error {
	reply := make(chan error, 1)
	s.cmds <- cmdReq{action: "stop", reply: reply}
	return <-reply
}

func (s *Supervisor) GetStatus() (string, string, int, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentPlace, s.state, s.rccPort, s.webPort
}

func (s *Supervisor) GetLogs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.proc != nil {
		return s.proc.GetLogs()
	}
	return []string{}
}