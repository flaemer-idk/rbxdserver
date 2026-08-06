package api

import (
	"net/http"
)

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>RBXD Server Status</title>
    <style>
        :root { --bg: #0d1117; --panel: #161b22; --text: #c9d1d9; --accent: #2ea043; --error: #f85149; }
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; background: var(--bg); color: var(--text); margin: 0; padding: 20px; display: flex; justify-content: center; align-items: center; min-height: 90vh; }
        .container { background: var(--panel); max-width: 550px; width: 100%; padding: 25px; border-radius: 12px; box-shadow: 0 8px 24px rgba(0,0,0,0.5); border: 1px solid #30363d; }
        h1 { margin-top: 0; color: var(--text); text-align: center; font-size: 22px; border-bottom: 1px solid #30363d; padding-bottom: 15px; }
        .row { display: flex; justify-content: space-between; padding: 12px 0; border-bottom: 1px solid #21262d; align-items: center; }
        .row:last-child { border: none; }
        .label { font-weight: 600; color: #8b949e; font-size: 14px; }
        .value { font-family: ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", Menlo, monospace; font-size: 15px; background: rgba(110,118,129,0.1); padding: 2px 6px; border-radius: 6px; }
        .status-running { color: var(--accent); }
        .status-idle { color: #8b949e; }
        .players-list { margin-top: 10px; background: rgba(110,118,129,0.1); padding: 12px; border-radius: 8px; min-height: 24px; display: flex; flex-wrap: wrap; gap: 8px; }
        .player { background: #1f2428; border: 1px solid #30363d; padding: 4px 10px; border-radius: 12px; font-size: 13px; font-family: monospace; }
        #auth-section { display: none; margin-bottom: 20px; text-align: center; }
        input { padding: 10px; border-radius: 6px; border: 1px solid #30363d; background: #0d1117; color: white; width: calc(100% - 90px); box-sizing: border-box; }
        button { padding: 10px 16px; border: 1px solid rgba(240,246,252,0.1); border-radius: 6px; background: var(--accent); color: #fff; cursor: pointer; font-weight: 600; transition: .2s; }
        button:hover { background: #3fb950; }
        .error { color: var(--error); margin-top: 10px; text-align: center; display: none; font-size: 14px; }
        .command-box { display: flex; align-items: center; gap: 10px; margin-top: 10px; background: #0d1117; padding: 10px; border-radius: 8px; border: 1px solid #30363d; }
        code { flex-grow: 1; font-family: monospace; font-size: 12px; color: #a5d6ff; word-break: break-all; }
        .copy-btn { padding: 6px 12px; font-size: 12px; background: #21262d; border: 1px solid #30363d; color: #c9d1d9; }
        .copy-btn:hover { background: #30363d; }
    </style>
</head>
<body>
    <div class="container">
        <h1>RBXD Control Panel</h1>

        <div id="auth-section">
            <input type="password" id="token" placeholder="Enter API Token">
            <button onclick="saveToken()">Login</button>
        </div>
        <div id="error-msg" class="error"></div>

        <div id="status-panel">
            <div class="row"><span class="label">Server IP:</span> <span class="value" id="val-ip">...</span></div>
            <div class="row"><span class="label">Status:</span> <span class="value status-idle" id="val-state">Waiting...</span></div>
            <div class="row"><span class="label">Running Place:</span> <span class="value" id="val-place">-</span></div>
            <div class="row"><span class="label">RCC Port:</span> <span class="value" id="val-rcc">-</span></div>
            <div class="row"><span class="label">Web Port:</span> <span class="value" id="val-web">-</span></div>
            <div class="row"><span class="label">Players Online:</span> <span class="value" id="val-players">0</span></div>

            <div class="label" style="margin-top:20px; display:block;">Player List:</div>
            <div class="players-list" id="val-player-list"><span style="color:#8b949e;font-size:13px;">Empty</span></div>

            <div class="label" style="margin-top:20px; display:block;">Join Command:</div>
            <div class="command-box">
                <code id="val-cmd">Waiting for server to start...</code>
                <button class="copy-btn" id="btn-copy" onclick="copyCmd()">Copy</button>
            </div>
        </div>
    </div>

    <script>
        let token = localStorage.getItem('rbxd_token') || '';
        
        const randomUser = "player" + Math.floor(1000 + Math.random() * 9000);
        
        const ui = {
            ip: document.getElementById('val-ip'), state: document.getElementById('val-state'),
            place: document.getElementById('val-place'), rcc: document.getElementById('val-rcc'),
            web: document.getElementById('val-web'), players: document.getElementById('val-players'),
            playerList: document.getElementById('val-player-list'), auth: document.getElementById('auth-section'),
            err: document.getElementById('error-msg'), cmd: document.getElementById('val-cmd')
        };
        
        let currentPlayers = [];
        let currentCmd = "";
        
        const hostIp = window.location.hostname;
        ui.ip.textContent = hostIp;

        function saveToken() {
            token = document.getElementById('token').value;
            localStorage.setItem('rbxd_token', token);
            ui.auth.style.display = 'none';
            update();
        }

        function copyCmd() {
            if (!currentCmd) return;
            navigator.clipboard.writeText(currentCmd).then(() => {
                const btn = document.getElementById('btn-copy');
                const oldText = btn.textContent;
                btn.textContent = "Copied!";
                btn.style.color = "#2ea043";
                setTimeout(() => {
                    btn.textContent = oldText;
                    btn.style.color = "#c9d1d9";
                }, 1500);
            });
        }

        async function update() {
            try {
                const headers = {};
                if (token) headers['Authorization'] = 'Bearer ' + token;

                const res = await fetch('/status', { headers });

                if (res.status === 401) {
                    ui.auth.style.display = 'block';
                    ui.err.style.display = 'block';
                    ui.err.textContent = 'Authorization token required (Unauthorized)';
                    return;
                }

                if (!res.ok) throw new Error('Server error');
                
                ui.err.style.display = 'none';
                ui.auth.style.display = 'none';
                
                const d = await res.json();
                
                if (ui.state.textContent !== d.state) {
                    ui.state.textContent = d.state || 'Idle';
                    ui.state.className = d.state === 'Running' ? 'value status-running' : 'value status-idle';
                }
                if (ui.place.textContent !== d.place) ui.place.textContent = d.place || '-';
                if (ui.rcc.textContent !== String(d.rcc_port)) ui.rcc.textContent = d.rcc_port || '-';
                if (ui.web.textContent !== String(d.web_port)) ui.web.textContent = d.web_port || '-';
                if (ui.players.textContent !== String(d.players)) ui.players.textContent = d.players;

                if (d.state === 'Running' && d.rcc_port && d.web_port) {
                    const newCmd = "rfd player --host " + hostIp + " --port " + d.rcc_port + " --web_host " + hostIp + " --web_port " + d.web_port + " -u " + randomUser + " --backend proton";
                    if (currentCmd !== newCmd) {
                        currentCmd = newCmd;
                        ui.cmd.textContent = currentCmd;
                    }
                } else {
                    currentCmd = "";
                    ui.cmd.textContent = "Start a place to generate join command...";
                }

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