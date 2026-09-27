# AGENTS.md — rbxdserver

> Headless-микросервис (Go) — оркестратор rbxd на серверной машине. Сам Roblox не
> запускает: дочерние процессы rbxd (`python3 <rfd>/Source/_main.py …`) — веб-части
> и RCC. Отдаёт HTTP/WS API и presence-сессии.
> Соседние файлы: `API.md` (контракт API + ченжлог для клиентов), `DESIGN.md`
> (зачем всё это), `ISSUES.md` (история дефектов старого кода), `../rbxd/INTEGRATION.md`.
> Реврайт 2026-09: архитектура «CDN-веб + сессия из двух процессов».

## Модель (mental model)

```
rbxdserver (Go, всегда 1 процесс, БЕЗ auth — доверенная LAN, один юзер)
│
├── cdnweb: постоянный CDN-веб — стартует с демоном, живёт всегда
│     python3 _main.py webserver --ipv4-only --web_port <cdn-port=8090>
│     (rbxd-мод webserver БЕЗ --config: синтетический v347-конфиг,
│      общий пул data/Assets, скины, thumbnails — «CDN» для клиентов;
│      чистый Python, Wine не нужен)
│
└── supervisor: сессия плейса (одна активная, слаг валидируется)
      ├── веб сессии:  python3 _main.py webserver --config <плейс>/GameConfig.toml
      │                   --web_port <эфемерный> --ipv4-only        (без Wine)
      └── RCC:         python3 _main.py server  --config <плейс>/GameConfig.toml
                          --skip_web --web_port <тот же> --port <эфемерный UDP>
                          --ipv4-only --backend wine
      Порядок: веб → HTTP-готовность (/rfd/status) → RCC → строка
      RFD_RCC_READY в stdout → Running.
      Конец сессии: RCC умирает сразу; веб живёт ещё --web-cooldown (4 мин)
      и переиспользуется, если тот же плейс стартовали снова. Веб другого
      плейса в кулдауне убивается немедленно.
      Падение веба посреди сессии → респавн на ТОМ ЖЕ порту (GameServer.json
      RCC уже указывает на него), сессия живёт.
```

## Глоссарий

- **Плейс** — каталог `<places-dir>/<slug>/` с `GameConfig.toml` (+ `info.json`, иконки).
- **CDN-веб** — постоянный безплейсовый `webserver`-процесс rbxd (v347); раздаёт
  ассеты/скины/превью клиентам независимо от сессий.
- **Веб сессии** — `webserver --config …` конкретного плейса; RCC подключается к нему
  по HTTPS (`--skip_web --web_port W`).
- **Generation (поколение)** — счётчик переходов супервизора; watcher'ы рапортуют
  события со своим поколением, устаревшие отбрасываются (чинка ABA-гонки crash_check).
- **pendingStart** — незавершённый старт: держит reply-канал `/start`, отменяется
  `/stop`/`/kill`/новым `/start` (cancelCh закрывается → стартовая горутина убивает
  ребёнка). Хэндл RCC публикуется событием `rcc_spawned` сразу после спавна —
  иначе `/stop` во время Starting его не видит (v347-баг «убить только через btop»).
- **Готовность RCC** — строка `RFD_RCC_READY` в stdout; у v347 (2018) её нет,
  поэтому по таймауту (60 c) живой RCC принимается как готовый (fallback).
- **Кулдаун веба** — веб сессии переживает конец сессии на `--web-cooldown`;
  `scheduleWebCooldown()` ставит таймер, `handleCooldownExpired()` убивает.
  `/kill` убивает веб сразу, без кулдауна.
- **Presence** — кто реально в игре; источник правды rbxd (`/rfd/presence`:
  заполняется на join-турникете, чистится Lua-хуком `PlayerRemoving` внутри RCC).
  rbxdserver опрашивает его раз в 2 c (`presenceLoop`) и кормит `session.Manager`.
- **Empty-таймер** — никого в игре `--empty-timeout` (5 мин) подряд → `onEmpty()`
  → автостоп сессии. Зашёл один — таймер снят, даже в последние секунды.
- **WS /session** — легаси от старого клиента: только счётчик `connections`,
  на списки и автостоп НЕ влияет.

## Структура

```
cmd/rbxdserver/main.go              — main: ParseFlags → cdn/supervisor/session →
                                      http.Server (таймауты) → сигнал → graceful shutdown
internal/config/config.go           — флаги: --port --cdn-port --rfd --places
                                      --data-dir --skins-dir --web-cooldown --empty-timeout --test
internal/rfdproc/process.go         — общий Process (Setpgid, stdout+stderr в одну
                                      трубу, LogBuffer, lineHook, Exited/Stop),
                                      WaitForHTTP (готовность = HTTP, не TCP)
internal/cdnweb/cdnweb.go           — менеджер CDN-веба: старт с демоном, авторестарт
                                      с backoff, Stop на выходе демона
internal/supervisor/supervisor.go   — автомат Idle/Starting/Running + кулдаун веба;
                                      все переходы в одной петле cmds; ожидания — в
                                      отдельных goroutine (start/stop не блокируют друг друга)
internal/supervisor/portwait.go     — GetFreePort (TOCTOU документирован)
internal/session/session.go         — Manager: map[user]bool, grace 15 c, onEmpty БЕЗ лока
internal/placesindex/placesindex.go — Scan(): каталоги с GameConfig.toml + info.json;
                                      roblox_version регэкспом из TOML (офлайн-список)
internal/api/router.go              — маршруты (auth НЕТ)
internal/api/handlers.go            — /start /stop /status /logs /places /places/<slug>/<asset>
                                      /favorites* /session (WS + read deadline 60 c)
internal/api/skins.go               — /skins: список/чтение/запись skins/*.json
internal/api/web.go                 — тестовая HTML-панель: статус, кнопки из /places,
                                      join-команды (запуск без rbxdclient)
module.nix / package.nix / shell.nix — NixOS-модуль (services.boblox), Go-сборка, dev-shell
API.md                              — контракт API и ченжлог (клиенты читают это)
```

## Конечный автомат супервизора

```
     ┌────────────────────────────────────────────────────────────┐
     │                                                            │
 ┌───▼───┐  /start   ┌──────────┐  веб готов → RCC RFD_RCC_READY ┌─────────┐
 │ Idle  │ ────────► │ Starting │ ─────────────────────────────► │ Running │
 └───┬───┘           └────┬─────┘                                └────┬────┘
     │                    │ fail/timeout/cancel                      │
     │◄───────────────────┘                                          │
     │  /stop | onEmpty (15 c) | RCC умер (rcc_exited, gen совпал)   │
     │  → RCC убит сразу; веб → кулдаун (--web-cooldown)             │
     ├──► Idle (веб живёт в кулдауне; повторный /start того же плейса│
     │    переиспользует его; cooldown_expired → веб убит)           │
     │                                                               │
     │  веб умер в Starting/Running → респавн на том же порту        │
     └───────────────────────────────────────────────────────────────┘
```

- Все переходы — в **одной** петле `for c := range s.cmds`; ожидания (HTTP-ready,
  RFD_RCC_READY, ретраи) живут в отдельных goroutine и рапортуют событиями —
  `/stop` никогда не заблокирован стартом.
- `gen` инкрементируется на каждом переходе; события с чужим gen игнорируются.
- `/start` того же slug при `Running` — no-op; другого slug — сессия останавливается.
- Веб-процессы отличать: `s.web` (сессии, может жить в кулдауне) vs CDN (его
  cdnweb.Manager не трогает).

## API — точный контракт

Актуальная таблица эндпойнтов и **ченжлог** — в `API.md`. Коротко:

| Метод | Путь | Тело/Query | Ответ |
|---|---|---|---|
| `POST` | `/start` | `?place=<slug>` | `{status,rcc_port,web_port,roblox_version}`; блокируется до готовности RCC (~20–30 c у v463; ~60 c у v347 через fallback) |
| `POST` | `/stop` | — | `OK`; RCC умирает сразу, веб доживает кулдаун |
| `POST` | `/kill` | — | `OK`; RCC **и** веб умирают немедленно, без кулдауна |
| `GET` | `/status` | — | `{place,state,players,player_list,players_detail,connections,rcc_port,web_port,cdn_port,roblox_version}` |
| `GET` | `/logs` | — | `["[web] …", "[rcc] …"]` (склеенные, до 1000+1000) |
| `GET` | `/places` | — | `[{slug,name,…,roblox_version}]` (всегда массив) |
| `GET` | `/places/<slug>/<asset>` | asset ∈ icon/banner.{png,jpg}, place-icon.png, place-thumbnail.png | бинарник (slug валидируется, симлинки резолвятся) |
| `GET/PUT` | `/skins`, `/skins/<name>.json` | тело — JSON ≤1 МБ | каталог скинов rbxd; запись подхватывается rbxd вживую |
| `WS` | `/session` | `?user=&place=` | легаси: только счётчик `connections`; read deadline 60 c |
| `GET` | `/` | — | тестовая HTML-панель (запущенный плейс подсвечен зелёным) |

Auth **нет** по решению владельца (один пользователь, LAN). Сервис **никогда**
не должен оказаться на публичном адресе — см. DESIGN.md.

## Запуск и окружение детей

```sh
go run ./cmd/rbxdserver --rfd <rbxd> --places <dir> --data-dir <dir> \
  [--port 8080] [--cdn-port 8090] [--web-cooldown 4m] [--empty-timeout 5m] [--skins-dir …] [--test]
```

Дети (в `internal/supervisor/supervisor.go`, сборщики команд):
```
веб:  python3 <rfd>/Source/_main.py webserver --config <conf> --web_port W --ipv4-only
      env: обычный (Wine не нужен)
RCC:  python3 <rfd>/Source/_main.py server --config <conf> --skip_web --web_port W
        --port R --ipv4-only --backend wine
      env: WINEPREFIX=<data-dir>/wine/.wine-rfd (или env; MkdirAll до старта),
           WINEDEBUG=-all, [RFD_NO_CAGE=1 при --test]
      pgid: Setpgid; Stop: SIGTERM → ждём фактического выхода (Exited) до 5 c →
            SIGKILL (PID-reuse закрыт: после Exited килов нет)
```

`RFD_DATA_DIR` **не передаётся** — rbxd его не читает (корень данных фиксирован
`<rfd>/data`, см. rbxd `util/resource.py`); в старом коде это был мёртвый env.
Готовность вебов — HTTP `GET /rfd/status`; готовность RCC — строка
`RFD_RCC_READY` в stdout (печатает rbxd, порт RCC — UDP, TCP-поллинг невозможен).

## NixOS (`module.nix`, `services.boblox`)

Опции: `enable, package, port(8080), cdnPort(8090), webCooldown(4m), placesDir,
rfdDir, dataDir, user/group(boblox), openFirewall, testMode, extraEnv`.
ExecStart: `--port --cdn-port --web-cooldown --empty-timeout --rfd --places --data-dir [--test]`.
`tokenFile`/`--state-dir` удалены. Firewall: API + CDN порты (сессийные — эфемерные).

## Договорённости для агентов

- **Не трогать `rbxd`** без явной просьбы; править только Go-сервис.
- Состояние супервизора меняется **только** в петле `cmds`. Не добавляй блокирующих
  вызовов в петлю (ждать готовности — в goroutine) и под `Manager.mu`.
- Валидация slug обязательна на каждом входе (`validSlug`); пути — через
  `filepath.Base`/`EvalSymlinks` (см. `handlePlaceFile`, `skinPath`).
- Готовность — всегда HTTP, никогда не голый TCP-коннект.
- Регэксп `roblox_version` дублирован в клиенте (Vala/C++). **Он нужен обоим**:
  список плейсов строится офлайн, `/rfd/roblox-version` отдаёт версию только
  активного плейса. Менять синхронно.
- Тестов в репо нет. Минимум: `go vet ./... && go build ./...`; интеграцию гонять
  руками: демон + `curl /status` + `/start` реального плейса (нужен nix-shell с
  python-зависимостями rbxd в PATH, иначе дети-вебы не поднимутся).
- Документацию (`AGENTS.md`, `API.md`, `DESIGN.md`) обновлять вместе с кодом.

## Что сделано в реврайте 2026-09 (был план в этом файле — выполнен)

- [x] Сплит сессии: веб-процесс + RCC-процесс (`--skip_web`), CDN-веб 24/7.
- [x] Готовность по HTTP (`/rfd/status`) + `RFD_RCC_READY` (раньше — TCP-поллинг).
- [x] Гонки: generation-счётчик вместо сравнения slug; петля не блокируется;
  `onEmpty` вне лока; state/rcc/web под одним `mu`.
- [x] Stop: SIGTERM → ожидание фактического выхода → SIGKILL (PID-reuse закрыт).
- [x] slug-валидация + path-traversal закрыт (`/start`, `/places/<slug>/…`, `/skins`).
- [x] WS: read deadline 60 c + pong; favorites без гонки; `/places` без `null`.
- [x] Auth удалена (решение владельца); fail-open дыра исчезла вместе с auth.
- [x] Kулдаун веба с переиспользованием тёплого веба; `/kill` без кулдауна.
- [x] Presence-список игроков из rbxd (`/rfd/presence`, полл раз в 5 c) +
  автостоп по `--empty-timeout`; WS стал легаси-счётчиком.
- [x] v347 (2018): живой RCC после таймаута принимается готовым (строки
  `RFD_RCC_READY` в 2018 нет); `/stop` и `/kill` убивают RCC и во время Starting.
- [ ] Сессии по connection id (сейчас по username — коллизии возможны).
- [ ] Табличные тесты `httptest`; race-тесты сессий.

## Остаточные известные компромиссы

- TOCTOU портов: `GetFreePort` может отдать порт, который тут же заняли; для веба
  это ловится HTTP-ретраем (3 попытки, новый порт), для RCC (UDP) — непроверяемо.
- `/status` при Idle показывает `web_port` тёплого веба в кулдауне — это осознанно
  (клиент может быстро переподключиться), но не путать с «плейс запущен».
- Username-ключ WS-соединений: два одинаковых `user_code` = одна запись (на
  присутствие не влияет — там ключ `id_num`).
- Presence-поллер живёт, пока `state == Running` и порт веба тот же; ошибки
  poll'а не рвут сессию (просто пропускаются) — если веб умер надолго, список
  замораживается до респавна веба.
- Ошибки `/start` отдаются как plain text (не типизированные JSON-ошибки).
