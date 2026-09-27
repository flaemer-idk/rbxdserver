# API.md — HTTP/WS API rbxdserver v2 (ченжлог для клиентов)

> Реврайт 2026-09. Старый rbxdclient (Vala) и будущий C++-клиент должны читать
> это целиком: контракты **ломались осознанно** (разрешение было дано).

## Модель доверия

**Аутентификации нет.** Токен/Bearer убран полностью: сервис для доверенной
LAN и одного пользователя. `--token` / `--token-file` удалены — не передавайте их.

## Флаги демона (изменились)

| Флаг | Статус | Что |
|---|---|---|
| `--port` | как было | API-порт (дефолт 8080) |
| `--cdn-port` | **новый** (дефолт 8090) | фиксированный порт постоянного CDN-веба |
| `--web-cooldown` | **новый** (дефолт 4m) | сколько живёт веб сессии после конца сессии |
| `--empty-timeout` | **новый** (дефолт 5m) | автостоп сессии: никого в игре столько времени подряд |
| `--skins-dir` | **новый** | каталог скинов (дефолт `<rfd>/data/skins`) |
| `--rfd`, `--places`, `--data-dir`, `--test` | как было | — |
| `--token`, `--token-file` | **удалены** | auth убран |
| `--state-dir` | **удалён** | deprecated-алиас `--data-dir` |

## Архитектурное изменение (важно для семантики)

Было: каждое `/start` поднимало **один** процесс rbxd (веб+RCC внутри), веб умирал
вместе с сессией мгновенно. Стало: сессия = **два** независимых процесса rbxd:

- веб сессии (`webserver --config …`) — живёт сессию + кулдаун (4 мин);
- RCC (`server --config … --skip_web`) — живёт сессию, умирает в конце сразу.

Постоянный CDN-веб (`webserver` без конфига, версия v347) поднимается вместе с
демоном на `--cdn-port` и **не зависит от сессий**: с него клиент может брать
ассеты/скины/превью даже когда ничего не запущено.

## Готовность

- `/start` теперь возвращает ответ **только когда RCC реально готов** (строка
  `RFD_RCC_READY` в его stdout, т.е. `Finished initializing game`). Раньше —
  TCP-коннект на веб-порт. Значит `/start` может занять ~20–150 с; кладите его
  в фоновую задачу, статус смотрите в `/status`.
- Готовность любого веба rbxd (CDN и сессионного): `GET /rfd/status` → 200 JSON.
  TCP-поллинг порта больше не нужен и недостаточен.

## Эндпойнты

| Метод | Путь | Изменение |
|---|---|---|
| `GET /` | панель | **без логина**: поля токена нет; кнопки плейсов строятся из `/places` (не захардкожены); добавлен CDN-порт |
| `POST /start?place=<slug>` | запуск | валидация slug (path traversal → 500 «invalid place slug»); ответ та же форма: `{status, rcc_port, web_port, roblox_version}`. Готовность: `RFD_RCC_READY` в stdout RCC, а для v347 (2018) этой строки нет вовсе — после 60 с живой RCC принимается как готовый (fallback) |
| `POST /stop` | остановка | семантика: RCC умирает сразу, веб живёт ещё кулдаун; повторный `/start` того же плейса в кулдаун переиспользует живой веб (быстрый старт) |
| `POST /kill` | **новый** | жёстко убить сессию немедленно: и RCC, и веб, без кулдауна. Для зависших сессий, когда `/stop` не помогает |
| `GET /status` | статус | `players` = кто **реально в игре** (из presence rbxd), `player_list` = ники, `players_detail` = `[{id_num, user_code, username, joined_at}]`, `connections` = легаси-счётчик WS-соединений старого клиента; плюс `place, state, rcc_port, web_port, cdn_port, roblox_version`; `web_port` может быть портом «тёплого» веба в кулдауне при `state: Idle` |
| `GET /logs` | логи | теперь **склеенные** логи веба сессии и RCC с префиксами `[web]`/`[rcc]` |
| `GET /places` | каталог | всегда JSON-массив (раньше мог быть `null`) |
| `GET /places/<slug>/<asset>` | обложки | **новые имена файлов**: `place-icon.png`, `place-thumbnail.png` в дополнение к `icon.png/jpg`, `banner.png/jpg`; path traversal закрыт (slug по регэкспу + резолв симлинков) |
| `GET /favorites`, `POST /favorites/toggle?place=` | избранное | гонка read-modify-write устранена; формат не менялся |
| `GET /skins` | **новый** | список скинов: `[{"name": "default.json", "modified": "RFC3339"}]` |
| `GET /skins/<name>.json` | **новый** | содержимое скина (тот же файл, что читает аватар-эндпойнт rbxd) |
| `PUT/POST /skins/<name>.json` | **новый** | записать скин (тело — валидный JSON ≤1 МБ). Вебсервер rbxd читает скин на каждый запрос — смена подхватывается вживую, без рестарта |
| `WS /session?user=&place=` | presence | **легаси**: не влияет на списки и автостоп, только счётчик `connections` в `/status`; read deadline 60 с |

Имена скинов: `[A-Za-z0-9][A-Za-z0-9 _.-]{0,63}` + обязательно `.json` —
иначе 400. `.json` можно опускать в URL.

## Player list и автостоп (2026-09)

Список игроков берётся **из rbxd**, а не из WS-соединений: вебсервер плейса
заполняет presence на join-турникете (`join.ashx`) и чистит его Lua-хуком
`PlayerRemoving` внутри RCC (`POST /rfd/player-left`). rbxdserver опрашивает
`/rfd/presence` веба сессии раз в 2 с и отдаёт снимок в `/status`.

Автостоп: если в игре **никого** `--empty-timeout` (5 мин) подряд — сессия
останавливается (RCC умирает, веб доживает кулдаун). Зашёл хотя бы один —
таймер снимается, в том числе в последние секунды. Старая логика «WS закрылся
→ через 15 с всё стопится» больше не действует: закрытие WS клиентом не убивает
сессию, пока в игре кто-то есть.

## Примеры

```sh
# статус
curl http://SERVER:8080/status

# каталог
curl http://SERVER:8080/places

# запуск (блокируется до готовности RCC; v347 — ~60 с через fallback)
curl -X POST 'http://SERVER:8080/start?place=f3x'

# жёстко убить сессию без кулдауна (RCC + веб)
curl -X POST http://SERVER:8080/kill

# кто в игре
curl http://SERVER:8080/status | jq '.player_list, .players_detail'

# скины с ноутбука
curl http://SERVER:8080/skins
curl http://SERVER:8080/skins/default.json
curl -X PUT -d '{"type":"R6","items":[107262414],"bundles":[],"scales":{},"colors":{}}' \
  http://SERVER:8080/skins/flaemer.json

# CDN напрямую (мимо rbxdserver API) — как будто это api.roblox.com:
curl -k https://SERVER:8090/rfd/status
curl -k 'https://SERVER:8090/avatar-thumbnail/image?userId=1&x=150&y=150'
```

## Join-команда игрока (не менялась)

```
python Source/_main.py player --host <SERVER_IP> --port <rcc_port> \
  --web_host <SERVER_IP> --web_port <web_port> -u <user_code> --backend <windows|proton|wine>
```
