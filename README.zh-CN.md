[English](README.md) | **简体中文**

# Termirai

多机终端远程管理与 AI 操作平台。浏览器打开一个 Web 控制台，即可同时操作任意多台客户端（Windows / Linux / macOS）的终端，并可由内置 AI Agent（参照 hermes-agent 的 tool-calling 循环与逐条审批机制）替你操作一台、多台或全部客户端。

Web 界面支持中英双语（按浏览器自动检测，一键切换）。

## 特性

- **终端常驻**：终端由客户端上独立的 *holder* 子进程托管。agent 重启、网络掉线、浏览器关闭、甚至服务端重启都不会杀掉终端；只有显式关闭（或 shell 退出）才销毁。标签支持双击重命名（服务端 + agent 双侧持久化）。
- **终端类型可选**：agent 启动时探测本机可用 shell（Windows：PowerShell / pwsh / CMD / Git Bash；unix：bash / zsh / fish 等）并上报；新建终端时可按客户端实际环境选择类型。
- **状态恢复、无数据库**：终端注册表、AI 会话、页面布局等全部落盘为单个 JSON 文件（`state.json`）。服务端重启后完整恢复；agent 自动重连并回放终端滚动缓冲。
- **文件管理**：浏览远程文件系统，Monaco 编辑器直接编辑客户端上的文本文件。
- **AI Agent**：OpenAI 兼容 tool-calling 循环、多会话、目标终端选择（单/多/全选）、全自动或逐条审批（支持"始终允许" allowlist）、可随时中断；聊天消息完整渲染 Markdown（GFM 表格/代码块/列表等）。
- **安全**：登录 = 用户名 + 密码 + 图形验证码；连续失败 5 次锁定 10 分钟（持久化）；agent 密钥鉴权注册；AI 工具调用限制在会话授权目标内。
- **App 预留**：全部 API 走 `/api/v1` + Header Token，与浏览器解耦。

## 架构

```
浏览器 (React + xterm.js + Monaco)
        │  HTTP / WebSocket（JWT，/api/v1）
        ▼
termirai-server (Go，embed 托管前端，JSON 文件持久化，无数据库)
        │  WebSocket 长连接（密钥鉴权）
        ▼
termirai-agent × N (Go，Windows/Linux/macOS，终端由独立 holder 进程托管)
```

agent 全部**主动出站**连接服务端，可位于 NAT/防火墙之后——只需在服务端开放一个端口。

支持平台：**Windows 10 1809+**（ConPTY + 命名管道 holder）、**Linux**、**macOS** 及其它 unix（PTY + unix socket holder）。Windows 默认 shell：`%SHELL%` → `powershell.exe` → `cmd.exe`；unix：`$SHELL` → `/bin/bash` → `/bin/sh`。

## 构建

依赖：Go 1.26+ 与 Node.js 18+（含 npm）。

```bash
# 1. 前端（产物输出到 web/dist）
cd web && npm install && npx vite build && cd ..

# 2. 服务端（单二进制，前端已内嵌）
go build -o termirai-server ./cmd/server

# 3. 客户端（按目标平台交叉编译）
GOOS=linux   GOARCH=amd64 go build -o termirai-agent-linux-amd64 ./cmd/agent
GOOS=linux   GOARCH=arm64 go build -o termirai-agent-linux-arm64 ./cmd/agent
GOOS=windows GOARCH=amd64 go build -o termirai-agent-windows-amd64.exe ./cmd/agent
GOOS=darwin  GOARCH=amd64 go build -o termirai-agent-darwin-amd64 ./cmd/agent
GOOS=darwin  GOARCH=arm64 go build -o termirai-agent-darwin-arm64 ./cmd/agent

# 服务端同样支持交叉编译，例如：
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o termirai-server ./cmd/server
```

> `web/dist/` 内置一个占位页，保证前端未构建时服务端也能编译。执行 `npx vite build` 后即为真实控制台，重新编译服务端二进制即可生效。

## 配置

所有配置均来自二进制同目录的 `.env` 文件或环境变量（环境变量优先），**不使用任何数据库**。

### 服务端 `.env`

| 项 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `PORT` | 否 | `8080` | Web 服务端口（浏览器与 agent 共用） |
| `ADMIN_USER` | 否 | `admin` | 控制台登录用户名（单管理员） |
| `ADMIN_PASSWORD` | **是** | — | 控制台登录密码 |
| `AGENT_KEY` | **是** | — | 客户端注册密钥（所有 agent 共用） |
| `DATA_DIR` | 否 | `./termirai-data` | 状态目录（`state.json`：终端注册表/AI 会话/页面状态/防爆破计数等） |
| `STATIC_DIR` | 否 | 内嵌 | 前端产物目录；不设置则使用内嵌前端 |
| `AI_BASE_URL` | 否 | — | AI 接口地址（OpenAI 兼容），不配置则 AI 功能不可用 |
| `AI_API_KEY` | 否 | — | AI 接口 Key |
| `AI_MODEL` | 否 | — | 模型名 |
| `AI_DEFAULT_MODE` | 否 | `approval` | 新会话默认授权模式：`approval`（逐条审批）/ `auto`（全自动） |
| `CAPTCHA_TEST_CODE` | 否 | — | **仅自动化测试用**：固定验证码答案。**生产环境严禁设置** |

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

> AI 参数也可在 Web 端「AI Agent → ⚙ 模型设置」中在线修改（含连通性测试），保存后立即生效并持久化；**Web 端保存的非空字段优先于 .env**。

### 客户端 `.env`

| 项 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `SERVER_URL` | **是** | — | 服务端地址，如 `ws://server-host:18080`（无 `ws://` 前缀会自动补；支持 `wss://`） |
| `AGENT_KEY` | **是** | — | 与服务端 `AGENT_KEY` 一致 |
| `AGENT_NAME` | 否 | 主机名 | 客户端显示名（Web 端还可随时设置别名） |
| `DATA_DIR` | 否 | 二进制旁 `termirai-data/` | 本地状态目录（`client.json` 身份、终端元数据、holder socket） |
| `SHELL` | 否 | 见上 | 新建终端的默认 shell |

## 部署

```
浏览器 ──HTTP/WS──▶ 服务端 :PORT ◀──WS（agent 主动出站连接）── 各客户端
```

### 服务端（Linux，systemd）

```bash
sudo mkdir -p /opt/termirai-server && sudo cp termirai-server /opt/termirai-server/
sudo tee /opt/termirai-server/.env <<'EOF'
PORT=18080
ADMIN_USER=admin
ADMIN_PASSWORD=换强密码
AGENT_KEY=换随机长密钥
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

浏览器打开 `http://server-host:18080` 登录，确认 agent 上线即可。

### 客户端（Linux，systemd）

```bash
sudo mkdir -p /opt/termirai && sudo cp termirai-agent-linux-amd64 /opt/termirai/termirai-agent
sudo tee /opt/termirai/.env <<'EOF'
SERVER_URL=ws://192.168.1.10:18080
AGENT_KEY=与服务端一致
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
KillMode=process   # 关键：重启只杀 agent 主进程，holder（终端）存活并被重新接管

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload && sudo systemctl enable --now termirai-agent
```

> **必须设置 `KillMode=process`**：默认的 `control-group` 会在 restart/stop 时杀掉整个进程组（包括 holder），导致终端现场丢失。

### Windows 说明

- 要求 **Windows 10 1809+ / Server 2019+**（终端基于 ConPTY）。
- 在含 `.env` 的目录运行 `termirai-agent.exe`；开机自启用任务计划程序（`schtasks /Create /TN TermiraiAgent /SC ONSTART /RU SYSTEM /TR "D:\termirai\termirai-agent.exe"`，"起始于"设为 `D:\termirai`）或 [NSSM](https://nssm.cc)。
- 无需防火墙入站规则（agent 只做出站连接）。
- 运行中的 exe 被锁定：升级时先重命名旧文件再放入新文件并重启；**勿杀命令行含 `holder` 的进程**——那是终端本体。

### macOS 说明

- 服务端与 agent 均可用 launchd（`RunAtLoad` + `KeepAlive`，`WorkingDirectory` 指向 `.env` 所在目录）；Apple Silicon 用 `darwin-arm64`，Intel 用 `darwin-amd64`。
- 首次运行可能被 Gatekeeper 拦截：系统设置 → 隐私与安全性 → 仍要打开，或 `xattr -d com.apple.quarantine <二进制>`。

### 反向代理 / HTTPS（可选）

```nginx
location / {
    proxy_pass http://127.0.0.1:18080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;   # WebSocket 必需
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_read_timeout 3600s;
}
```

agent 侧把 `SERVER_URL` 改为 `wss://your-domain` 即可。

### 生产安全检查清单

- [ ] `ADMIN_PASSWORD`、`AGENT_KEY` 使用强随机值；`.env` 权限 600
- [ ] **不要**设置 `CAPTCHA_TEST_CODE`（测试专用旁路）
- [ ] 仅对外开放服务端 `PORT`；agent 无需任何入站端口
- [ ] AI 默认模式保持 `approval`（逐条审批），确认可控后再对信任会话切 `auto`
- [ ] 定期备份 `DATA_DIR`——它就是全部状态（服务端与 agent 各自一份，均为自包含 JSON）

## 更多

- [详细需求与设计说明书](docs/requirements.zh-CN.md)

## 许可证

[MIT](LICENSE)
