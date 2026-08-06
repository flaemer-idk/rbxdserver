// internal/supervisor/process.go
package supervisor

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type LogBuffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func NewLogBuffer(max int) *LogBuffer {
	return &LogBuffer{max: max, lines: make([]string, 0, max)}
}

func (b *LogBuffer) WriteLine(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.lines) >= b.max {
		b.lines = b.lines[1:]
	}
	b.lines = append(b.lines, line)
	log.Println("[RFD]", line)
}

func (b *LogBuffer) Get() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	res := make([]string, len(b.lines))
	copy(res, b.lines)
	return res
}

type Process struct {
	cmd             *exec.Cmd
	logBuf          *LogBuffer
	intentionalStop bool
	mu              sync.Mutex
}

func NewProcess(rfdDir, configPath string, rccPort, webPort int, stateDir string, isTest bool) *Process {
	mainPy := filepath.Join(rfdDir, "Source", "_main.py")
	
	cmd := exec.Command("python3", mainPy, "server", 
		"--config", configPath, 
		"--port", fmt.Sprintf("%d", rccPort), 
		"--web_port", fmt.Sprintf("%d", webPort), 
		"--ipv4-only", 
		"--backend", "wine", // Сервер всегда использует бэкенд wine
	)
	cmd.Dir = rfdDir
	
	winePrefix := os.Getenv("WINEPREFIX")
	if winePrefix == "" {
		winePrefix = filepath.Join(stateDir, "wine", ".wine-rfd")
	}

	// ИСПРАВЛЕНИЕ: Гарантируем физическое существование директории WINEPREFIX до запуска Wine
	if err := os.MkdirAll(winePrefix, 0755); err != nil {
		log.Printf("[Supervisor] Warning: failed to create WINEPREFIX directory %s: %v", winePrefix, err)
	}

	cmd.Env = append(os.Environ(), 
		fmt.Sprintf("RFD_DATA_DIR=%s", stateDir),
		fmt.Sprintf("WINEPREFIX=%s", winePrefix),
		"WINEDEBUG=-all",
	)

	if isTest {
		cmd.Env = append(cmd.Env, "RFD_NO_CAGE=1")
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	return &Process{
		cmd:    cmd,
		logBuf: NewLogBuffer(1000),
	}
}

func (p *Process) Start() error {
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	p.cmd.Stderr = p.cmd.Stdout

	if err := p.cmd.Start(); err != nil {
		return err
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			p.logBuf.WriteLine(scanner.Text())
		}
	}()

	return nil
}

func (p *Process) Wait() {
	err := p.cmd.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.intentionalStop {
		log.Printf("Process exited unexpectedly: %v", err)
	}
}

func (p *Process) Stop() {
	p.mu.Lock()
	p.intentionalStop = true
	p.mu.Unlock()

	if p.cmd.Process != nil {
		syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
		go func(pid int) {
			time.Sleep(5 * time.Second)
			syscall.Kill(-pid, syscall.SIGKILL)
		}(p.cmd.Process.Pid)
	}
}

func (p *Process) GetLogs() []string {
	return p.logBuf.Get()
}