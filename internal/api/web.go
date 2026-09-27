package api

import (
	"net/http"
)

// Тестовая панель: статус, кто онлайн, кнопки запуска плейсов и join-команды —
// чтобы можно было всё проверить без rbxdclient, напрямую.
const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>rbxdserver</title>
    <style>
        :root { --bg: #0d1117; --panel: #161b22; --text: #c9d1d9; --accent: #2ea043; --btn-blue: #1f6feb; --error: #f85149; }
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; background: var(--bg); color: var(--text); margin: 0; padding: 20px; display: flex; justify-content: center; align-items: flex-start; min-height: 90vh; }
        .container { background: var(--panel); max-width: 620px; width: 100%; padding: 25px; border-radius: 12px; box-shadow: 0 8px 24px rgba(0,0,0,0.5); border: 1px solid #30363d; }
        h1 { margin-top: 0; color: var(--text); text-align: center; font-size: 22px; border-bottom: 1px solid #30363d; padding-bottom: 15px; }
        .row { display: flex; justify-content: space-between; padding: 10px 0; border-bottom: 1px solid #21262d; align-items: center; }
        .row:last-child { border: none; }
        .label { font-weight: 600; color: #8b949e; font-size: 14px; }
        .value { font-family: ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", Menlo, monospace; font-size: 15px; background: rgba(110,118,129,0.1); padding: 2px 6px; border-radius: 6px; }
        .status-running { color: var(--accent); }
        .status-idle { color: #8b949e; }
        .players-list { margin-top: 10px; background: rgba(110,118,129,0.1); padding: 12px; border-radius: 8px; min-height: 24px; display: flex; flex-wrap: wrap; gap: 8px; }
        .player { background: #1f2428; border: 1px solid #30363d; padding: 4px 10px; border-radius: 12px; font-size: 13px; font-family: monospace; }

        .actions-panel { display: flex; gap: 10px; margin: 15px 0; flex-wrap: wrap; }
        .btn-action { padding: 10px 14px; border-radius: 6px; border: 1px solid #30363d; background: #21262d; color: #c9d1d9; font-weight: 600; cursor: pointer; transition: .2s; }
        .btn-action:hover { background: #30363d; color: #fff; border-color: #8b949e; }
        .btn-running { background: rgba(46,160,67,0.2); border-color: var(--accent); color: var(--accent); }
        .btn-running:hover { background: rgba(46,160,67,0.35); color: #fff; border-color: var(--accent); }
        .btn-stop { background: rgba(248,81,73,0.15); border-color: rgba(248,81,73,0.4); color: #ff7b72; }
        .btn-stop:hover { background: var(--error); color: #fff; border-color: var(--error); }
        .btn-kill { background: rgba(248,81,73,0.3); border-color: var(--error); color: #ff7b72; font-weight: 800; }
        .btn-kill:hover { background: var(--error); color: #fff; border-color: var(--error); }

        .error { color: var(--error); margin-top: 10px; text-align: center; display: none; font-size: 14px; }
        .command-box { display: flex; align-items: center; gap: 10px; margin-top: 10px; background: #0d1117; padding: 10px; border-radius: 8px; border: 1px solid #30363d; }
        code { flex-grow: 1; font-family: monospace; font-size: 12px; color: #a5d6ff; word-break: break-all; }
        .copy-btn { padding: 6px 12px; font-size: 12px; background: #21262d; border: 1px solid #30363d; color: #c9d1d9; border-radius: 6px; cursor: pointer; }
        .copy-btn:hover { background: #30363d; }
        .cdn-link { color: #58a6ff; text-decoration: none; }
        .cdn-link:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="container">
        <h1>rbxdserver — test panel</h1>
        <div id="error-msg" class="error"></div>

        <div class="row"><span class="label">Server IP:</span> <span class="value" id="val-ip">...</span></div>
        <div class="row"><span class="label">Status:</span> <span class="value status-idle" id="val-state">Waiting...</span></div>
        <div class="row"><span class="label">Running Place:</span> <span class="value" id="val-place">-</span></div>
        <div class="row"><span class="label">Client Version:</span> <span class="value" id="val-version">-</span></div>
        <div class="row"><span class="label">RCC Port:</span> <span class="value" id="val-rcc">-</span></div>
        <div class="row"><span class="label">Web Port:</span> <span class="value" id="val-web">-</span></div>
        <div class="row"><span class="label">CDN:</span> <span class="value" id="val-cdn">-</span></div>
        <div class="row"><span class="label">Players Online:</span> <span class="value" id="val-players">0</span></div>

        <div class="label" style="margin-top:20px; display:block;">Player List:</div>
        <div class="players-list" id="val-player-list"><span style="color:#8b949e;font-size:13px;">Empty</span></div>

        <div class="label" style="margin-top:20px; display:block;">Places (green = running):</div>
        <div class="actions-panel" id="places-panel"><span style="color:#8b949e;font-size:13px;">Loading places...</span>
            <button class="btn-action btn-stop" onclick="stopPlace()" title="Stop the session: RCC dies now, the session web stays warm for the cooldown">Stop</button>
            <button class="btn-action btn-kill" onclick="killPlace()" title="Kill everything immediately: RCC and the session web, no cooldown">Kill</button>
        </div>

        <div class="label" style="display:block;">Join Command (backend picked by your OS: windows / proton / wine):</div>
        <div class="command-box">
            <code id="val-cmd">Start a place to generate join command...</code>
            <button class="copy-btn" onclick="copyCmd()">Copy</button>
        </div>
    </div>


    <script>
        // Fresh nick on every copy: never repeats within the page
        // and never collides with someone currently in game.
        // NOTE: state declarations come first and the pickUser() call comes
        // last — it reads currentPlayers, and a top-level call before that
        // let-declaration would throw a TDZ ReferenceError and freeze the
        // whole panel.
        let usedNames = new Set();
        let currentPlayers = [];
        let currentCmd = "";
        let lastStatus = null;
        let placesLoaded = false;

        function pickUser() {
            let name;
            do {
                name = "player" + Math.floor(1000 + Math.random() * 900000);
            } while (usedNames.has(name) || currentPlayers.includes(name));
            usedNames.add(name);
            return name;
        }

        let currentUser = pickUser();

        const ui = {
            ip: document.getElementById('val-ip'), state: document.getElementById('val-state'),
            place: document.getElementById('val-place'), version: document.getElementById('val-version'),
            rcc: document.getElementById('val-rcc'), web: document.getElementById('val-web'),
            cdn: document.getElementById('val-cdn'),
            players: document.getElementById('val-players'), playerList: document.getElementById('val-player-list'),
            placesPanel: document.getElementById('places-panel'), err: document.getElementById('error-msg'),
            cmd: document.getElementById('val-cmd')
        };

        const hostIp = window.location.hostname;
        ui.ip.textContent = hostIp;

        // windows on Windows; proton on Linux (not Android); everything else — wine.
        function getBackend() {
            const ua = (navigator.userAgent || navigator.platform || "").toLowerCase();
            if (ua.includes("win")) {
                return "windows";
            } else if (ua.includes("linux") && !ua.includes("android")) {
                return "proton";
            } else {
                return "wine";
            }
        }

        function formatClientVersion(ver) {
            if (!ver) return '-';
            const vLower = ver.toLowerCase();
            if (vLower.includes('2018')) {
                return ver + ' v348';
            } else if (vLower.includes('2021')) {
                return ver + ' v463';
            }
            return ver;
        }

        async function loadPlaces() {
            try {
                const res = await fetch('/places');
                if (!res.ok) throw new Error('places fetch failed');
                const places = await res.json();
                const stopBtn = ui.placesPanel.querySelector('.btn-stop');
                const killBtn = ui.placesPanel.querySelector('.btn-kill');
                ui.placesPanel.innerHTML = '';
                if (places.length === 0) {
                    ui.placesPanel.innerHTML = '<span style="color:#8b949e;font-size:13px;">No places found in places dir</span>';
                } else {
                    places.forEach(p => {
                        const b = document.createElement('button');
                        b.className = 'btn-action';
                        b.textContent = p.name + (p.roblox_version ? ' (' + formatClientVersion(p.roblox_version) + ')' : '');
                        b.title = p.slug;
                        b.dataset.slug = p.slug;
                        b.onclick = () => startPlace(p.slug);
                        ui.placesPanel.appendChild(b);
                    });
                }
                if (stopBtn) ui.placesPanel.appendChild(stopBtn);
                if (killBtn) ui.placesPanel.appendChild(killBtn);
                placesLoaded = true;
            } catch (e) { /* the panel keeps polling /status */ }
        }

        // Highlight the running place in green.
        function highlightRunning(place, state) {
            const active = (state === 'Running' || state === 'Starting');
            ui.placesPanel.querySelectorAll('button[data-slug]').forEach(b => {
                b.classList.toggle('btn-running', active && b.dataset.slug === place);
            });
        }

        async function startPlace(placeSlug) {
            try {
                ui.err.style.display = 'none';
                const res = await fetch('/start?place=' + encodeURIComponent(placeSlug), { method: 'POST' });
                if (!res.ok) {
                    const errTxt = await res.text();
                    throw new Error(errTxt || 'Failed to start place');
                }
                update();
            } catch (e) {
                ui.err.style.display = 'block';
                ui.err.textContent = e.message;
            }
        }

        async function stopPlace() {
            try {
                ui.err.style.display = 'none';
                const res = await fetch('/stop', { method: 'POST' });
                if (!res.ok) {
                    const errTxt = await res.text();
                    throw new Error(errTxt || 'Failed to stop server');
                }
                update();
            } catch (e) {
                ui.err.style.display = 'block';
                ui.err.textContent = e.message;
            }
        }

        async function killPlace() {
            try {
                ui.err.style.display = 'none';
                const res = await fetch('/kill', { method: 'POST' });
                if (!res.ok) {
                    const errTxt = await res.text();
                    throw new Error(errTxt || 'Failed to kill server');
                }
                update();
            } catch (e) {
                ui.err.style.display = 'block';
                ui.err.textContent = e.message;
            }
        }

        // Every copy gets a fresh unique nick: regenerate and re-render
        // the box so the copied text matches what is shown.
        function copyCmd() {
            if (!currentCmd) return;
            currentUser = pickUser();
            buildCmd();
            navigator.clipboard.writeText(currentCmd).then(() => {
                const btn = event.currentTarget || event.target;
                const oldText = btn.textContent;
                btn.textContent = "Copied!";
                btn.style.color = "#2ea043";
                setTimeout(() => {
                    btn.textContent = oldText;
                    btn.style.color = "#c9d1d9";
                }, 1500);
            });
        }

        // Only touch the DOM when the placeholder text actually changed.
        function setCmdBox(el, text) {
            if (el.textContent !== text) el.textContent = text;
        }

        // Player command: only the backend (windows / proton / wine) and
        // user_code change. Rebuilt after every /status and on copy.
        function buildCmd() {
            if (lastStatus && lastStatus.state === 'Running' && lastStatus.rcc_port && lastStatus.web_port) {
                currentCmd = "python Source/_main.py player --host " + hostIp + " --port " + lastStatus.rcc_port + " --web_host " + hostIp + " --web_port " + lastStatus.web_port + " -u " + currentUser + " --backend " + getBackend();
            } else {
                currentCmd = "";
            }
            setCmdBox(ui.cmd, currentCmd || "Start a place to generate join command...");
        }

        async function update() {
            try {
                const res = await fetch('/status');
                if (!res.ok) throw new Error('Server error');

                ui.err.style.display = 'none';
                const d = await res.json();
                lastStatus = d;

                highlightRunning(d.place, d.state);

                if (ui.state.textContent !== d.state) {
                    ui.state.textContent = d.state || 'Idle';
                    ui.state.className = d.state === 'Running' ? 'value status-running' : 'value status-idle';
                }
                if (ui.place.textContent !== d.place) ui.place.textContent = d.place || '-';

                const formattedVer = formatClientVersion(d.roblox_version);
                if (ui.version.textContent !== formattedVer) ui.version.textContent = formattedVer;

                if (ui.rcc.textContent !== String(d.rcc_port || '-')) ui.rcc.textContent = d.rcc_port || '-';
                if (ui.web.textContent !== String(d.web_port || '-')) ui.web.textContent = d.web_port || '-';
                if (ui.players.textContent !== String(d.players)) ui.players.textContent = d.players;

                if (d.cdn_port && ui.cdn.textContent === '-') {
                    ui.cdn.innerHTML = '<a class="cdn-link" target="_blank" href="https://' + hostIp + ':' + d.cdn_port + '/rfd/status">' + d.cdn_port + '</a>';
                }

                buildCmd();

                const pList = d.player_list || [];
                if (JSON.stringify(pList) !== JSON.stringify(currentPlayers)) {
                    ui.playerList.innerHTML = '';
                    if (pList.length === 0) {
                        ui.playerList.innerHTML = '<span style="color:#8b949e;font-size:13px;">Empty</span>';
                    } else {
                        pList.forEach(p => {
                            const el = document.createElement('span');
                            el.className = 'player'; el.textContent = p;
                            ui.playerList.appendChild(el);
                        });
                    }
                    currentPlayers = pList;
                }
            } catch (e) {
                ui.err.style.display = 'block';
                ui.err.textContent = 'Cannot connect to server...';
            }
        }

        loadPlaces();
        update();
        setInterval(update, 2000);
    </script>
</body>
</html>`

func (rt *Router) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(indexHTML))
}
