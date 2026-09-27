// Package supervisor — оркестрация сессии плейса.
//
// Сессия = два независимых ребёнка rbxd:
//
//	веб сессии:  python3 _main.py webserver --config <плейс> --web_port W --ipv4-only
//	RCC:         python3 _main.py server  --config <плейс> --skip_web --port R
//	             --web_port W --ipv4-only --backend wine
//
// Жизненный цикл: RCC умирает в конце сессии сразу; веб сессии переживает её
// на --web-cooldown (по умолчанию 4 мин) и переиспользуется, если тот же
// плейс стартовали снова. Все переходы делает единственная goroutine-петля
// (cmds channel): тяжёлые ожидания живут в отдельных goroutine и рапортуют
// событиями, поэтому /stop не блокируется стартом.
//
// Каждый переход инкрементирует generation; watcher'ы рапортуют события со
// своим generation, и устаревшие события отбрасываются — это чинка ABA-гонки
// старого crash_check (сравнение по slug).
package supervisor

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"rbxdserver/internal/config"
	"rbxdserver/internal/logtags"
	"rbxdserver/internal/rfdproc"
	"rbxdserver/internal/session"
)

const (
	StateIdle     = "Idle"
	StateStarting = "Starting"
	StateRunning  = "Running"
)

const (
	webReadyTimeout = 90 * time.Second // веб — чистый Python, ему хватает
	// RCC: ждём строку RFD_RCC_READY. У v463 она есть, у v347 (2018) — нет,
	// поэтому по таймауту живой RCC принимается как готовый (fallback), а не
	// считается упавшим.
	rccReadyTimeout    = 60 * time.Second
	webStartRetries    = 3
	rccReadyLinePrefix = "RFD_RCC_READY" // печатает rbxd (routines/rcc/__init__.py)

	presencePollInterval = 2 * time.Second
)

// logSup — все строки супервизора идут с фиолетовым тегом (см. logtags).
func logSup(format string, args ...any) {
	log.Printf("%s "+format, append([]any{logtags.Supervisor}, args...)...)
}

type cmd struct {
	action  string // start | stop | kill | web_up | web_fail | rcc_spawned | rcc_ready | start_failed | rcc_exited | web_exited | web_respawned | cooldown_expired
	slug    string
	gen     uint64
	reply   chan error
	err     error
	proc    *rfdproc.Process
	webPort int
	rccPort int
}

// pendingStart — старт, который ещё не завершён (ожидает веб/RCC).
type pendingStart struct {
	slug     string
	gen      uint64
	conf     string
	webPort  int
	rccPort  int
	reply    chan error
	once     sync.Once
	cancelCh chan struct{}
	mu       sync.Mutex
	dropped  bool
}

func (p *pendingStart) cancel() {
	p.mu.Lock()
	if p.dropped {
		p.mu.Unlock()
		return
	}
	p.dropped = true
	close(p.cancelCh)
	p.mu.Unlock()
}

func (p *pendingStart) isDropped() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dropped
}

func (p *pendingStart) droppedChan() <-chan struct{} { return p.cancelCh }

func (p *pendingStart) deliver(err error) {
	p.once.Do(func() { p.reply <- err })
}

type Status struct {
	Place   string
	State   string
	RccPort int
	WebPort int
}

type Supervisor struct {
	cfg  *config.Config
	sess *session.Manager // presence-список игроков (rbxd — источник правды)
	cmds chan cmd

	mu       sync.Mutex
	state    string
	place    string // плейс текущей/последней сессии
	gen      uint64
	pending  *pendingStart
	rcc      *rfdproc.Process
	rccPort  int
	web      *rfdproc.Process // веб сессии; может переживать сессию (кулдаун)
	webPlace string
	webPort  int
	cooldown *time.Timer
}

func New(cfg *config.Config, sess *session.Manager) *Supervisor {
	s := &Supervisor{
		cfg:   cfg,
		sess:  sess,
		cmds:  make(chan cmd, 16),
		state: StateIdle,
	}
	go s.loop()
	return s
}

// ---- публичный API (потокобезопасно) ----

var reSlug = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$`)
var reDotDot = regexp.MustCompile(`\.\.`)

// validSlug отсекает path traversal: slug попадает в filepath.Join.
// Пробелы разрешены (каталоги вида "Crossroads 2007"); опасное — слеши,
// "..", ведущие точка/пробел — отсечено регэкспом и проверкой ниже.
func validSlug(slug string) bool {
	return reSlug.MatchString(slug) &&
		!reDotDot.MatchString(slug) &&
		filepath.Base(slug) == slug
}

// StartPlace запускает сессию плейса. Блокируется до готовности RCC (обычно
// ~20–30 с у v463; у v347 READY-строки нет — ждём таймаут и принимаем живой
// RCC), но /stop и /kill в это время продолжают работать.
func (s *Supervisor) StartPlace(slug string) error {
	if !validSlug(slug) {
		return fmt.Errorf("invalid place slug %q", slug)
	}
	conf := filepath.Join(s.cfg.PlacesDir, slug, "GameConfig.toml")
	if _, err := os.Stat(conf); err != nil {
		return fmt.Errorf("place %q has no GameConfig.toml: %w", slug, err)
	}
	reply := make(chan error, 1)
	s.cmds <- cmd{action: "start", slug: slug, reply: reply}
	return <-reply
}

func (s *Supervisor) StopCurrentPlace() error {
	reply := make(chan error, 1)
	s.cmds <- cmd{action: "stop", reply: reply}
	return <-reply
}

// KillCurrentPlace — как Stop, но без кулдауна: убивает и RCC, и веб сразу.
func (s *Supervisor) KillCurrentPlace() error {
	reply := make(chan error, 1)
	s.cmds <- cmd{action: "kill", reply: reply}
	return <-reply
}

// Shutdown — daemon умирает: без кулдауна, всё убить (включая веб).
func (s *Supervisor) Shutdown() {
	reply := make(chan error, 1)
	s.cmds <- cmd{action: "shutdown", reply: reply}
	<-reply
}

func (s *Supervisor) GetStatus() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{
		Place:   s.place,
		State:   s.state,
		RccPort: s.rccPort,
		WebPort: s.webPort,
	}
}

func (s *Supervisor) GetLogs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	if s.web != nil {
		for _, l := range s.web.GetLogs() {
			out = append(out, "[web] "+l)
		}
	}
	if s.rcc != nil {
		for _, l := range s.rcc.GetLogs() {
			out = append(out, "[rcc] "+l)
		}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// ---- петля-оркестратор ----

func (s *Supervisor) loop() {
	for c := range s.cmds {
		switch c.action {
		case "start":
			s.handleStart(c)
		case "stop":
			s.handleStop(c)
		case "kill":
			s.handleStop(c)
			s.stopCooldown()
			if s.web != nil {
				s.web.Stop()
				s.web = nil
				s.webPort = 0
				log.Println("[Supervisor] kill: session web killed (no cooldown)")
			}
		case "shutdown":
			s.handleStop(c)
			// При смерти демона кулдауна не бывает: дети-сироты никому не нужны.
			if s.web != nil {
				s.web.Stop()
				s.web = nil
				s.webPort = 0
				log.Println("[Supervisor] daemon shutdown: session web killed (no cooldown)")
			}
		case "web_up":
			s.handleWebUp(c)
		case "web_fail":
			s.handleStartFailed(c, c.err)
		case "rcc_spawned":
			s.handleRccSpawned(c)
		case "rcc_ready":
			s.handleRccReady(c)
		case "start_failed":
			s.handleStartFailed(c, c.err)
		case "rcc_exited":
			s.handleRccExited(c)
		case "web_exited":
			s.handleWebExited(c)
		case "web_respawned":
			s.handleWebRespawned(c)
		case "cooldown_expired":
			s.handleCooldownExpired()
		}
	}
}

func (s *Supervisor) handleStart(c cmd) {
	// Уже играем в этот плейс — no-op.
	if s.state == StateRunning && s.place == c.slug {
		c.reply <- nil
		return
	}

	// Прерываем незавершённый старт и текущую сессию.
	if s.pending != nil {
		old := s.pending
		s.pending = nil
		old.cancel()
		old.deliver(errors.New("start superseded by another /start"))
	}
	if s.rcc != nil {
		s.rcc.Stop()
		s.rcc = nil
		s.rccPort = 0
	}
	// Веб другого плейса не переиспользуем — конфиг у него чужой.
	if s.web != nil && s.webPlace != c.slug {
		s.web.Stop()
		s.web = nil
		s.stopCooldown()
	}

	s.gen++
	gen := s.gen
	conf := filepath.Join(s.cfg.PlacesDir, c.slug, "GameConfig.toml")
	p := &pendingStart{
		slug: c.slug, gen: gen, conf: conf, reply: c.reply,
		cancelCh: make(chan struct{}),
	}
	s.pending = p
	s.state = StateStarting
	s.place = c.slug
	logSup("gen %d: starting place %q", gen, c.slug)

	// Живой веб того же плейса (кулдаун ещё не истёк) — переиспользуем.
	if s.web != nil && s.webPlace == c.slug && s.web.Alive() {
		s.stopCooldown()
		logSup("reusing warm web on port %d", s.webPort)
		p.webPort = s.webPort
		go s.startRCCAsync(p, s.web, s.webPort)
		return
	}
	go s.startWebAsync(p)
}

func (s *Supervisor) handleStop(c cmd) {
	// Нечего останавливать (нет сессии и старт не идёт) — no-op без шума:
	// «тёплый» веб в кулдауне не трогаем, его убивают /kill или таймер.
	if s.pending == nil && s.rcc == nil {
		logSup("/stop: nothing running (no-op)")
		c.reply <- nil
		return
	}
	if s.pending != nil {
		p := s.pending
		s.pending = nil
		p.cancel()
		p.deliver(errors.New("start cancelled by /stop"))
	}
	if s.rcc != nil {
		s.rcc.Stop()
		s.rcc = nil
		s.rccPort = 0
	}
	s.gen++ // инвалидация всех watcher'ов текущей сессии
	s.scheduleWebCooldown()
	s.state = StateIdle
	s.place = ""
	s.sess.SetPresence(nil)
	logSup("session stopped (web stays for cooldown=%s)", s.cfg.WebCooldown)
	c.reply <- nil
}

func (s *Supervisor) handleWebUp(c cmd) {
	if s.pending == nil || s.pending.gen != c.gen || s.state != StateStarting {
		if c.proc != nil {
			c.proc.Stop() // осиротевший веб — убираем
		}
		return
	}
	if c.proc != nil {
		s.web = c.proc
		s.webPlace = s.pending.slug
		s.watchExit(c.proc, c.gen, "web_exited")
	}
	s.webPort = c.webPort
	s.pending.webPort = c.webPort
	go s.startRCCAsync(s.pending, c.proc, c.webPort)
}

// handleRccSpawned — RCC только что заспавнен (ещё не готов). Хэндл сразу
// кладётся в s.rcc, чтобы /stop и /kill могли убить его и во время Starting:
// раньше он жил только в горутине старта, и /stop его не видел (v347-баг
// «stop не убивает, только btop»).
func (s *Supervisor) handleRccSpawned(c cmd) {
	if s.pending == nil || s.pending.gen != c.gen || s.state != StateStarting {
		if c.proc != nil {
			c.proc.Stop() // осиротевший RCC
		}
		return
	}
	s.rcc = c.proc
	s.rccPort = c.rccPort
}

func (s *Supervisor) handleRccReady(c cmd) {
	if s.pending == nil || s.pending.gen != c.gen || s.state != StateStarting {
		if c.proc != nil {
			c.proc.Stop()
		}
		return
	}
	s.rcc = c.proc
	s.rccPort = c.rccPort
	s.state = StateRunning
	s.watchExit(c.proc, c.gen, "rcc_exited")
	logSup("gen %d: RUNNING place %q (web %d, rcc %d UDP)",
		c.gen, s.place, s.webPort, s.rccPort)
	go s.presenceLoop(c.gen, s.webPort)
	s.pending.deliver(nil)
	s.pending = nil
}

func (s *Supervisor) handleStartFailed(c cmd, err error) {
	if s.pending == nil || s.pending.gen != c.gen {
		return
	}
	p := s.pending
	s.pending = nil
	if s.rcc != nil {
		s.rcc.Stop()
		s.rcc = nil
		s.rccPort = 0
	}
	s.gen++
	// Веб (если успел подняться) не убиваем — пусть остывает для быстрого ретрая.
	s.scheduleWebCooldown()
	s.state = StateIdle
	s.place = ""
	s.sess.SetPresence(nil)
	logSup("gen %d: start failed: %v", c.gen, err)
	p.deliver(err)
}

func (s *Supervisor) handleRccExited(c cmd) {
	if c.gen != s.gen {
		return // устаревшее событие от старой сессии
	}
	switch s.state {
	case StateRunning:
		logSup("gen %d: RCC died unexpectedly, ending session", c.gen)
		s.rcc = nil
		s.rccPort = 0
		s.gen++
		s.scheduleWebCooldown()
		s.state = StateIdle
		s.place = ""
		s.sess.SetPresence(nil)
	case StateStarting:
		// Гонка со стартовым таймаутом: стартовая goroutine сама рапортует
		// start_failed; здесь ничего делать не нужно.
	}
}

func (s *Supervisor) handleWebExited(c cmd) {
	if c.gen != s.gen {
		return // веб старой сессии — уже неважно
	}
	if s.state == StateIdle {
		return // веб кулдауна умер сам; следующий start поднимет новый
	}
	// Веб умер посреди сессии/старта — респавним на ТОМ ЖЕ порту:
	// GameServer.json RCC уже указывает на этот порт.
	conf := filepath.Join(s.cfg.PlacesDir, s.webPlace, "GameConfig.toml")
	port := s.webPort
	logSup("gen %d: session web died, respawning on port %d", c.gen, port)
	go s.respawnWebAsync(c.gen, conf, port)
}

func (s *Supervisor) handleWebRespawned(c cmd) {
	if c.gen != s.gen || c.proc == nil {
		if c.proc != nil {
			c.proc.Stop()
		}
		return
	}
	s.web = c.proc
	s.watchExit(c.proc, c.gen, "web_exited")
	logSup("gen %d: session web respawned on port %d", c.gen, s.webPort)
}

func (s *Supervisor) handleCooldownExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateIdle || s.pending != nil || s.web == nil {
		return
	}
	logSup("web cooldown expired, stopping session web (place %q)", s.webPlace)
	s.web.Stop()
	s.web = nil
	s.webPort = 0
}

// ---- вспомогательные ----

func (s *Supervisor) stopCooldown() {
	if s.cooldown != nil {
		s.cooldown.Stop()
		s.cooldown = nil
	}
}

func (s *Supervisor) scheduleWebCooldown() {
	s.stopCooldown()
	if s.web == nil || !s.web.Alive() {
		s.web = nil
		return
	}
	s.cooldown = time.AfterFunc(s.cfg.WebCooldown, func() {
		s.cmds <- cmd{action: "cooldown_expired"}
	})
}

func (s *Supervisor) watchExit(proc *rfdproc.Process, gen uint64, action string) {
	go func() {
		<-proc.Exited()
		s.cmds <- cmd{action: action, gen: gen}
	}()
}

// presenceLoop — пока сессия Running, опрашивает `/rfd/presence` веба сессии
// (rbxd — источник правды: заполняется на join-турникете, чистится Lua-хуком
// PlayerRemoving внутри RCC). Выходит, когда сессия кончилась (state/port
// сменились); при ошибке poll'а просто пропускает тик — сессия не рвётся.
func (s *Supervisor) presenceLoop(gen uint64, webPort int) {
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	url := fmt.Sprintf("https://127.0.0.1:%d/rfd/presence", webPort)
	ticker := time.NewTicker(presencePollInterval)
	defer ticker.Stop()

	for {
		st := s.GetStatus()
		if st.State != StateRunning || st.WebPort != webPort {
			return
		}

		var players []session.PresencePlayer
		resp, err := client.Get(url)
		if err == nil {
			err = json.NewDecoder(resp.Body).Decode(&players)
			resp.Body.Close()
		}
		if err != nil {
			logSup("presence poll failed: %v", err)
		} else {
			s.sess.SetPresence(players)
		}

		<-ticker.C
	}
}

// ---- запускающие goroutine (никогда не трогают состояние напрямую) ----

func (s *Supervisor) startWebAsync(p *pendingStart) {
	for attempt := 1; attempt <= webStartRetries; attempt++ {
		if p.isDropped() {
			s.cmds <- cmd{action: "start_failed", gen: p.gen, err: errors.New("cancelled")}
			return
		}
		port, err := GetFreePort()
		if err != nil {
			continue
		}
		proc := newSessionWebProcess(s.cfg, p.conf, port)
		if err := proc.Start(); err != nil {
			logSup("web spawn attempt %d failed: %v", attempt, err)
			time.Sleep(time.Second)
			continue
		}
		if err := rfdproc.WaitForHTTP(port, "/rfd/status", webReadyTimeout); err == nil {
			s.cmds <- cmd{action: "web_up", gen: p.gen, proc: proc, webPort: port}
			return
		}
		logSup("web readiness attempt %d failed on port %d, retrying", attempt, port)
		proc.Stop()
		<-proc.Exited()
	}
	s.cmds <- cmd{action: "web_fail", gen: p.gen, err: errors.New("session webserver failed to start after retries")}
}

func (s *Supervisor) startRCCAsync(p *pendingStart, web *rfdproc.Process, webPort int) {
	if p.isDropped() {
		s.cmds <- cmd{action: "start_failed", gen: p.gen, err: errors.New("cancelled")}
		return
	}
	rccPort, err := GetFreePort()
	if err != nil {
		s.cmds <- cmd{action: "start_failed", gen: p.gen, err: fmt.Errorf("port alloc: %w", err)}
		return
	}
	proc := newRCCProcess(s.cfg, p.conf, rccPort, webPort)

	ready := make(chan struct{})
	var once sync.Once
	proc.SetLineHook(func(line string) {
		if len(line) >= len(rccReadyLinePrefix) && line[:len(rccReadyLinePrefix)] == rccReadyLinePrefix {
			once.Do(func() { close(ready) })
		}
	})

	if err := proc.Start(); err != nil {
		s.cmds <- cmd{action: "start_failed", gen: p.gen, err: fmt.Errorf("RCC spawn: %w", err)}
		return
	}
	// Хэндл сразу наружу: /stop и /kill должны уметь убить RCC во время Starting.
	s.cmds <- cmd{action: "rcc_spawned", gen: p.gen, proc: proc, rccPort: rccPort}

	select {
	case <-ready:
		s.cmds <- cmd{action: "rcc_ready", gen: p.gen, proc: proc, rccPort: rccPort}
	case <-p.droppedChan():
		proc.Stop()
		// reply уже доставлен handleStop; только убираем процесс.
	case <-proc.Exited():
		s.cmds <- cmd{action: "start_failed", gen: p.gen, err: errors.New("RCC exited during startup (см. логи)")}
	case <-time.After(rccReadyTimeout):
		if proc.Alive() {
			// v347 (2018) не печатает RFD_RCC_READY вообще — это нормально.
			// Живой RCC после таймаута принимаем как готовый: веб отвечает,
			// процесс работает — играть можно. Умрёт позже → rcc_exited.
			logSup("gen %d: RFD_RCC_READY not seen in %s (typical for v347), accepting alive RCC",
				p.gen, rccReadyTimeout)
			s.cmds <- cmd{action: "rcc_ready", gen: p.gen, proc: proc, rccPort: rccPort}
		} else {
			s.cmds <- cmd{action: "start_failed", gen: p.gen, err: errors.New("RCC readiness timeout")}
		}
	}
}

func (s *Supervisor) respawnWebAsync(gen uint64, conf string, port int) {
	for attempt := 1; attempt <= webStartRetries; attempt++ {
		proc := newSessionWebProcess(s.cfg, conf, port)
		if err := proc.Start(); err != nil {
			logSup("web respawn attempt %d failed: %v", attempt, err)
			time.Sleep(time.Second)
			continue
		}
		if err := rfdproc.WaitForHTTP(port, "/rfd/status", webReadyTimeout); err == nil {
			s.cmds <- cmd{action: "web_respawned", gen: gen, proc: proc, webPort: port}
			return
		}
		proc.Stop()
	}
	logSup("gen %d: web respawn failed; session continues degraded (joins unavailable)", gen)
	s.cmds <- cmd{action: "web_exited", gen: gen} // попробовать ещё раз по следующему событию
}

// ---- сборщики команд детей ----

func newSessionWebProcess(cfg *config.Config, confPath string, webPort int) *rfdproc.Process {
	mainPy := filepath.Join(cfg.RfdDir, "Source", "_main.py")
	args := []string{
		mainPy, "webserver",
		"--config", confPath,
		"--web_port", itoa(webPort),
		"--ipv4-only",
	}
	// Веб — чистый Python: без Wine, без WINEPREFIX, без backend.
	return rfdproc.NewProcess("python3", args, cfg.RfdDir, os.Environ(), "web")
}

func newRCCProcess(cfg *config.Config, confPath string, rccPort, webPort int) *rfdproc.Process {
	mainPy := filepath.Join(cfg.RfdDir, "Source", "_main.py")
	args := []string{
		mainPy, "server",
		"--config", confPath,
		"--skip_web",                // веб уже поднят отдельным процессом
		"--web_port", itoa(webPort), // RCC подключается к нему по HTTPS
		"--port", itoa(rccPort), // UDP game-порт
		"--ipv4-only",
		"--backend", "wine",
	}

	winePrefix := os.Getenv("WINEPREFIX")
	if winePrefix == "" {
		winePrefix = filepath.Join(cfg.DataDir, "wine", ".wine-rfd")
	}
	// Wine падает, если префикс не существует заранее.
	if err := os.MkdirAll(winePrefix, 0755); err != nil {
		logSup("warning: cannot create WINEPREFIX %s: %v", winePrefix, err)
	}

	env := append(os.Environ(),
		"WINEPREFIX="+winePrefix,
		"WINEDEBUG=-all",
	)
	if cfg.Test {
		env = append(env, "RFD_NO_CAGE=1")
	}
	return rfdproc.NewProcess("python3", args, cfg.RfdDir, env, "rcc")
}
