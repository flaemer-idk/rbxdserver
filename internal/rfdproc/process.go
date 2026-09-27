// Package rfdproc — обёртка над дочерним процессом rbxd (python3 Source/_main.py …).
// Используется и супервизором сессий, и постоянным CDN-вебом.
package rfdproc

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"rbxdserver/internal/logtags"
)

// LogBuffer — кольцевой буфер последних строк stdout/stderr ребёнка.
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
}

func (b *LogBuffer) Get() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	res := make([]string, len(b.lines))
	copy(res, b.lines)
	return res
}

// Process — дочерний процесс rbxd в собственной process group.
//
// stdout и stderr сливаются в одну трубу (каждая строка идёт и в LogBuffer,
// и в опциональный LineHook — так супервизор ловит `RFD_RCC_READY`).
// Exited() закрывается по выходу процесса; повторные Wait не нужны.
type Process struct {
	cmd       *exec.Cmd
	logBuf    *LogBuffer
	logPrefix string
	exited    chan struct{} // закрывается по выходе процесса
	mu        sync.Mutex
	lineHook  func(line string)
}

func NewProcess(name string, args []string, dir string, env []string, logPrefix string) *Process {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	return &Process{
		cmd:       cmd,
		logBuf:    NewLogBuffer(1000),
		logPrefix: logPrefix,
		exited:    make(chan struct{}),
	}
}

func (p *Process) SetLineHook(hook func(line string)) {
	p.mu.Lock()
	p.lineHook = hook
	p.mu.Unlock()
}

func (p *Process) Start() error {
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	// stdout и stderr ребёнка — в одну и ту же трубу: иначе Stderr = Stdout
	// не работает с StdoutPipe (писатель недоступен снаружи).
	p.cmd.Stdout = pw
	p.cmd.Stderr = pw

	if err := p.cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return err
	}
	// Родительская копия больше не нужна — трубу держит ребёнок и ридер.
	pw.Close()

	go func() {
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			p.logBuf.WriteLine(line)
			log.Printf("%s %s", logtags.ByPrefix(p.logPrefix), line)
			p.mu.Lock()
			hook := p.lineHook
			p.mu.Unlock()
			if hook != nil {
				hook(line)
			}
		}
		io.Copy(io.Discard, pr) // хвост после последнего \n
		pr.Close()
	}()

	go func() {
		err := p.cmd.Wait()
		if err != nil {
			log.Printf("%s %s: exited: %v", logtags.Proc, p.logPrefix, err)
		} else {
			log.Printf("%s %s: exited cleanly", logtags.Proc, p.logPrefix)
		}
		close(p.exited)
	}()

	return nil
}

// Exited — канал, закрывающийся по завершении процесса.
func (p *Process) Exited() <-chan struct{} { return p.exited }

// Alive — жив ли процесс на момент вызова.
func (p *Process) Alive() bool {
	select {
	case <-p.exited:
		return false
	default:
		return true
	}
}

func (p *Process) Pid() int { return p.cmd.Process.Pid }

// Stop — SIGTERM всей process group; если через 5 секунд процесс ещё жив,
// SIGKILL. Килы прекращаются сразу после выхода (Exited), поэтому пид не
// успевает переиспользоваться: сначала ждём фактического завершения.
func (p *Process) Stop() {
	if p.cmd.Process == nil {
		return
	}
	pid := p.cmd.Process.Pid
	_ = syscall.Kill(-pid, syscall.SIGTERM)

	go func() {
		select {
		case <-p.exited:
			return
		case <-time.After(5 * time.Second):
			if !p.Alive() {
				return
			}
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	}()
}

func (p *Process) GetLogs() []string { return p.logBuf.Get() }

// WaitForHTTP — готовность = осмысленный HTTP-ответ, а не открытый TCP-порт
// (порт открыт ≠ сервер готов; к тому же TCP-поллинг может «прокликать» чужой
// сервис, случайно занявший порт).
func WaitForHTTP(port int, path string, timeout time.Duration) error {
	client := &http.Client{
		Timeout: 2 * time.Second,
		// self-signed trustme-сертификат rbxd; проверка везде отключена
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	url := fmt.Sprintf("https://127.0.0.1:%d%s", port, path)
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for HTTP %s", url)
		}
		<-ticker.C
	}
}
