# rbxdserver

Go-оркестратор rbxd: держит постоянный **CDN-веб** (ассеты/скины/превью для
клиентов), запускает **сессии плейсов** (веб-процесс + RCC под Wine) и отдаёт
HTTP/WS API + тестовую веб-панель. Без аутентификации — только доверенная LAN.

Документация: `API.md` (контракт + ченжлог), `AGENTS.md` (архитектура),
`DESIGN.md` (зачем), `ISSUES.md` (история дефектов).

## Что потребуется

- **rbxd** (репо `../rbxd`) — движок; в `data/Roblox/` должны быть скачаны
  бинарники (`python3 Source/_main.py download`, ~1 ГБ, один раз).
- **Go** для сборки (`nix-shell -p go`) — либо готовый бинарник.
- **Wine** и python-зависимости rbxd в PATH (NixOS: `nix-shell` из
  `../rbxd/shell.nix` или pythonEnv из `module.nix` — без них дети-вебы не
  поднимутся: у системного python нет `urllib3`/`trustme`/...).
- Каталог **плейсов**: `<places>/<slug>/GameConfig.toml` (+ `Place.rbxl` рядом
  или `rbxl_uri` в конфиге).

## Сборка

```sh
# бинарник
cd rbxdserver
nix-shell -p go --run 'go build -o rbxdserver ./cmd/rbxdserver'

# с race-детектором (для отладки)
nix-shell -p go --run 'go build -race -o rbxdserver-race ./cmd/rbxdserver'
```

## Запуск из сурсов / бинарником

```sh
# из сурсов (первые ~8 c занимает nix eval — это нормально)
nix-shell -p go --run 'go run ./cmd/rbxdserver --rfd ../rbxd --places ~/Places --data-dir ./state'

# бинарником (PATH должен содержать python с зависимостями rbxd!)
nix-shell /path/to/rbxd/shell.nix --run './rbxdserver --rfd /path/to/rbxd --places ~/Places --data-dir ~/boblox-state'
```

Демон сам поднимет CDN-веб на `--cdn-port` и ждёт `/start`.

### Аргументы

| Флаг | Дефолт | Что |
|---|---|---|
| `--port` | 8080 | HTTP/WS API + веб-панель |
| `--cdn-port` | 8090 | постоянный CDN-веб (rbxd `webserver`, v347, без Wine) |
| `--web-cooldown` | 4m | сколько веб сессии живёт после конца сессии |
| `--rfd` | — (обяз.) | каталог rbxd (с `Source/` и `data/`) |
| `--places` | — (обяз.) | каталог плейсов (`<slug>/GameConfig.toml`) |
| `--data-dir` | — (обяз.) | состояние демона: `favorites.json`, `wine/.wine-rfd` |
| `--skins-dir` | `<rfd>/data/skins` | каталог скинов для API `/skins` |
| `--test` | off | `RFD_NO_CAGE=1` детям-RCC (без cage; для отладки) |

### NixOS

`services.boblox` из `module.nix` — те же флаги опциями (`port`, `cdnPort`,
`webCooldown`, `placesDir`, `rfdDir`, `dataDir`, `openFirewall`, `testMode`).
pythonEnv/cage/umu уже в PATH сервиса.

## Первый запуск руками (без NixOS)

```sh
# 1) один раз: инициализировать wine-префикс для RCC
export WINEPREFIX=~/boblox-state/wine/.wine-rfd
wineboot --init && winetricks vcrun2019

# 2) запустить демона (в nix-shell rbxd, чтобы был правильный python3)
nix-shell /path/to/rbxd/shell.nix --run 'rbxdserver --rfd /path/to/rbxd \
  --places /path/to/places --data-dir ~/boblox-state --test'

# 3) проверить
curl http://127.0.0.1:8080/status
curl -k https://127.0.0.1:8090/rfd/status        # CDN готов?
curl -X POST 'http://127.0.0.1:8080/start?place=f3x'   # ждать до ~2.5 мин
```

Дальше — открой `http://<ip-сервера>:8080/` в браузере: тестовая панель со
статусом, кнопками запуска и join-командой для игрока. Клиент с ноутбука —
rbxdclient или join-команда из панели.

## Studio v463 и сертификат

Студия под Wine валидирует HTTPS: без доверенного корня она не грузит ассеты.
Один раз вшей CA вебсервера в префикс (см. `../rbxd/INTEGRATION.md` §12):

```sh
python3 ../rbxd/scripts/install_ca_to_wineprefix.py --prefix $WINEPREFIX
```

## Порты

- API `--port`, CDN `--cdn-port` — фиксированные.
- Веб сессии и RCC — эфемерные на каждую сессию; клиент узнаёт их из ответа
  `/start`. Для доступа с других машин открой API и CDN в firewall
  (`module.nix: openFirewall`); сессийные порты на домашней LAN обычно не
  закрывают.
