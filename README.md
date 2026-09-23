# Termirai

**English** | [简体中文](README.zh-CN.md)

Multi-host terminal remote management and AI operations platform. Open one web console in your browser and drive terminals on any number of machines (Windows / Linux / macOS) simultaneously — with a built-in AI agent (hermes-agent-style tool-calling loop with per-command approval) that can operate one, several, or all connected hosts for you.

The web UI is bilingual (English / 简体中文, auto-detected, one-click switch).

## Highlights

- **Persistent terminals** — each terminal lives in a dedicated *holder* subprocess on the client. Agent restarts, network drops, closed browsers, even server restarts do not kill your terminals; only an explicit close (or the shell exiting) destroys one. Tabs can be renamed (double-click), persisted on both server and agent.
- **Shell selection** — agents detect available shells at startup (PowerShell / pwsh / CMD / Git Bash on Windows; bash / zsh / fish … on unix) and report the list; new terminals can pick any of them.
- **Full state recovery, no database** — terminal registry, AI sessions, page layout and more are persisted to a single JSON file (`state.json`). After a server restart everything comes back; agents re-register automatically and terminal scrollback is replayed.
- **File manager** — browse remote filesystems and edit text files in an embedded Monaco editor.
- **AI Agent** — OpenAI-compatible tool-calling loop, multiple concurrent sessions, per-session target selection (one / several / all terminals), fully-automatic or per-command approval mode with an "always allow" allowlist, interruptible at any time, full Markdown rendering (GFM tables, code blocks, lists).
- **Security** — login with username + password + captcha; 5 failed attempts lock the account for 10 minutes (persisted); agents register with a shared key; AI tool calls are confined to the session's authorized targets.
- **App-ready API** — everything speaks versioned `/api/v1` REST + WebSocket with header-token auth, decoupled from the browser.

## Architecture

```
Browser (React + xterm.js + Monaco)
        │  HTTP / WebSocket (JWT, /api/v1)
        ▼
termirai-server (Go, embeds the frontend, JSON file persistence, no database)
        │  WebSocket long connection (key auth)
        ▼
termirai-agent × N (Go, Windows/Linux/macOS, terminals hosted by holder subprocesses)
```

Agents connect **outbound** to the server, so clients may sit behind NAT/firewalls — only the server needs one open port.

Supported platforms: **Windows 10 1809+** (ConPTY + named-pipe holder), **Linux**, **macOS** and other unix-like systems (PTY + unix-socket holder). Default shell: `%SHELL%` → `powershell.exe` → `cmd.exe` on Windows; `$SHELL` → `/bin/bash` → `/bin/sh` on unix.

## Building

Prerequisites: Go 1.26+ and Node.js 18+ (with npm).

```bash
# 1. Frontend (output goes to web/dist)
cd web && npm install && npx vite build && cd ..

# 2. Server (single binary, frontend embedded)
go build -o termirai-server ./cmd/server

# 3. Agent — cross-compile for your targets
GOOS=linux   GOARCH=amd64 go build -o termirai-agent-linux-amd64 ./cmd/agent
GOOS=linux   GOARCH=arm64 go build -o termirai-agent-linux-arm64 ./cmd/agent
GOOS=windows GOARCH=amd64 go build -o termirai-agent-windows-amd64.exe ./cmd/agent
GOOS=darwin  GOARCH=amd64 go build -o termirai-agent-darwin-amd64 ./cmd/agent
GOOS=darwin  GOARCH=arm64 go build -o termirai-agent-darwin-arm64 ./cmd/agent

# Server cross-compiles the same way (GOOS/GOARCH), e.g.:
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o termirai-server ./cmd/server
```

> `web/dist/` ships with a placeholder page so the server compiles before the frontend is built. Running `npx vite build` replaces it with the real console; rebuild the server binary afterwards.

## Configuration

All configuration comes from an `.env` file next to the binary or from environment variables (env vars win). **No database is used anywhere.**

### Server `.env`

| Key | Required | Default | Description |
|---|---|---|---|
| `PORT` | no | `8080` | Web port (browser and agents share it) |
| `ADMIN_USER` | no | `admin` | Console login user (single admin) |
| `ADMIN_PASSWORD` | **yes** | — | Console login password |
| `AGENT_KEY` | **yes** | — | Agent registration key (shared by all agents) |
| `DATA_DIR` | no | `./termirai-data` | State directory (`state.json`: terminal registry / AI sessions / page state / brute-force counters …) |
| `STATIC_DIR` | no | embedded | Frontend directory; unset = use the embedded frontend |
| `AI_BASE_URL` | no | — | OpenAI-compatible API base URL; AI features disabled if unset |
| `AI_API_KEY` | no | — | API key |
| `AI_MODEL` | no | — | Model name |
| `AI_DEFAULT_MODE` | no | `approval` | Default approval mode for new sessions: `approval` / `auto` |
| `CAPTCHA_TEST_CODE` | no | — | **Automated tests only**: fixed captcha answer. **Never set in production.** |

```ini
PORT=18080
ADMIN_USER=admin
ADMIN_PASSWORD=your-strong-password
AGENT_KEY=your-random-agent-key
DATA_DIR=/opt/termirai-server/data
AI_BASE_URL=https://api.deepseek.com/v1
AI_API_KEY=sk-...
AI_MODEL=deepseek-chat
AI_DEFAULT_MODE=approval
```

> AI settings can also be edited at runtime in **AI Agent → ⚙ Model settings** (with a connectivity test); non-empty fields saved there take precedence over `.env`.

### Agent `.env`

| Key | Required | Default | Description |
|---|---|---|---|
| `SERVER_URL` | **yes** | — | Server address, e.g. `ws://server-host:18080` (`ws://` prefix added if missing; `wss://` supported) |
| `AGENT_KEY` | **yes** | — | Must match the server's `AGENT_KEY` |
| `AGENT_NAME` | no | hostname | Display name (aliases can also be set from the web UI) |
| `DATA_DIR` | no | `termirai-data/` beside the binary | Local state (client identity, terminal metadata, holder sockets) |
| `SHELL` | no | see below | Default shell for new terminals |

## Deployment

```
Browser ──HTTP/WS──▶ server :PORT ◀──WS (outbound from agents)── clients
```

### Server (Linux, systemd)

```bash
sudo mkdir -p /opt/termirai-server && sudo cp termirai-server /opt/termirai-server/
sudo tee /opt/termirai-server/.env <<'EOF'
PORT=18080
ADMIN_USER=admin
ADMIN_PASSWORD=change-me
AGENT_KEY=change-me-too
DATA_DIR=/opt/termirai-server/data
EOF
sudo chmod 600 /opt/termirai-server/.env

sudo tee /etc/systemd/system/termirai-server.service <<'EOF'
[Unit]
Description=Termirai Server
After=network-online.target

[Service]
WorkingDirectory=/opt/termirai-server
ExecStart=/opt/termirai-server/termirai-server
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload && sudo systemctl enable --now termirai-server
```

Then open `http://server-host:18080`, log in, and check that agents appear online.

### Agent (Linux, systemd)

```bash
sudo mkdir -p /opt/termirai && sudo cp termirai-agent-linux-amd64 /opt/termirai/termirai-agent
sudo tee /opt/termirai/.env <<'EOF'
SERVER_URL=ws://192.168.1.10:18080
AGENT_KEY=must-match-server
AGENT_NAME=web-01
DATA_DIR=/opt/termirai/data
EOF

sudo tee /etc/systemd/system/termirai-agent.service <<'EOF'
[Unit]
Description=Termirai Agent
After=network-online.target

[Service]
WorkingDirectory=/opt/termirai
ExecStart=/opt/termirai/termirai-agent
Restart=always
RestartSec=5
KillMode=process   # critical: restart must not kill the holders (your terminals)

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload && sudo systemctl enable --now termirai-agent
```

> **`KillMode=process` is required.** The default `control-group` mode kills holders together with the agent, losing live terminals.

### Windows notes

- Requires **Windows 10 1809+ / Server 2019+** (ConPTY).
- Run `termirai-agent.exe` from a folder containing `.env`; for autostart use Task Scheduler (`schtasks /Create /TN TermiraiAgent /SC ONSTART /RU SYSTEM /TR "D:\termirai\termirai-agent.exe"`, set "Start in" to `D:\termirai`) or [NSSM](https://nssm.cc).
- No inbound firewall rules needed — the agent only makes outbound connections.
- A running exe is locked; to upgrade, rename the old binary, drop in the new one, and restart. **Do not kill processes whose command line contains `holder`** — those are your live terminals.

### macOS notes

- Use launchd (`RunAtLoad` + `KeepAlive`, `WorkingDirectory` pointing at the `.env` folder) for both server and agent; pick `darwin-arm64` on Apple Silicon, `darwin-amd64` on Intel.
- First launch may hit Gatekeeper: System Settings → Privacy & Security → *Open Anyway*, or `xattr -d com.apple.quarantine <binary>`.

### Reverse proxy / HTTPS (optional)

```nginx
location / {
    proxy_pass http://127.0.0.1:18080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;   # required for WebSocket
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_read_timeout 3600s;
}
```

Point agents at `wss://your-domain` afterwards.

### Production security checklist

- [ ] Use strong random `ADMIN_PASSWORD` and `AGENT_KEY`; `.env` permissions 600
- [ ] **Do not** set `CAPTCHA_TEST_CODE` (test-only bypass)
- [ ] Expose only the server `PORT`; agents need no inbound ports
- [ ] Keep AI default mode `approval`; switch to `auto` only for trusted sessions
- [ ] Back up `DATA_DIR` — that is the entire state, on both server and agents

## More

- [Detailed requirements & design spec (Chinese)](docs/requirements.zh-CN.md)

## License

[MIT](LICENSE)
