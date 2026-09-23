package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"termirai/internal/proto"
)

// browserMsg 浏览器 WS 消息（复用宽松结构）。
type browserMsg struct {
	Type     string `json:"type"`
	ClientID string `json:"client_id,omitempty"`
	TermID   string `json:"term_id,omitempty"`
	Title    string `json:"title,omitempty"`
	Cols     int    `json:"cols,omitempty"`
	Rows     int    `json:"rows,omitempty"`
	Shell    string `json:"shell,omitempty"` // term_open 指定的 shell 路径（空=客户端默认）
	Data     string `json:"data,omitempty"`

	// UI 状态
	State json.RawMessage `json:"state,omitempty"`

	// AI
	SessionID  string      `json:"session_id,omitempty"`
	Name       string      `json:"name,omitempty"`
	Mode       string      `json:"mode,omitempty"`
	Targets    []AITarget  `json:"targets,omitempty"`
	Content    string      `json:"content,omitempty"`
	ApprovalID string      `json:"approval_id,omitempty"`
	Decision   string      `json:"decision,omitempty"`
}

// handleBrowserWS 浏览器 WebSocket 入口（Token 经 query 传递以便 App 端复用同一协议）。
func (h *Hub) handleBrowserWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" || !h.validToken(token) {
		http.Error(w, "未认证", http.StatusUnauthorized)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	b := &browserConn{conn: conn, hub: h, subs: map[string]bool{}, aiSubs: map[string]bool{}}
	h.addBrowser(b)
	defer func() {
		b.mu.Lock()
		b.closed = true
		b.mu.Unlock()
		h.removeBrowser(b)
		conn.Close()
	}()

	aiBase, aiKey, _, aiDefMode := h.aiConf()
	// 初始全量状态
	b.send(map[string]any{
		"type":        "state",
		"clients":     h.listClients(),
		"ui_state":    h.getUIState(),
		"ai_sessions": h.listAISessions(),
		"ai_default_mode": aiDefMode,
		"ai_enabled":  aiBase != "" && aiKey != "",
	})

	for {
		var m browserMsg
		if err := conn.ReadJSON(&m); err != nil {
			return
		}
		h.handleBrowserMsg(b, &m)
	}
}

func (h *Hub) handleBrowserMsg(b *browserConn, m *browserMsg) {
	switch m.Type {
	case "term_open":
		h.bmTermOpen(b, m)
	case "term_rename":
		h.bmTermRename(b, m)
	case "client_alias":
		h.bmClientAlias(b, m)
	case "term_input":
		data, err := base64.StdEncoding.DecodeString(m.Data)
		if err != nil {
			return
		}
		if c := h.getClient(m.ClientID); c != nil && c.ac != nil {
			c.ac.send(&proto.Message{Type: proto.TypeTermInput, TermID: m.TermID, Data: base64.StdEncoding.EncodeToString(data)})
		}
	case "term_resize":
		if c := h.getClient(m.ClientID); c != nil && c.ac != nil {
			c.ac.send(&proto.Message{Type: proto.TypeTermSize, TermID: m.TermID, Cols: m.Cols, Rows: m.Rows})
		}
	case "term_close":
		h.bmTermClose(b, m)
	case "term_sub":
		b.subsMu.Lock()
		b.subs[subKey(m.ClientID, m.TermID)] = true
		b.subsMu.Unlock()
		// 补发服务端缓冲，保证 reattach 看到现场
		if c := h.getClient(m.ClientID); c != nil {
			if t := c.Terminals[m.TermID]; t != nil {
				if buf := t.snapshot(); len(buf) > 0 {
					b.send(map[string]any{
						"type": "term_output", "client_id": m.ClientID, "term_id": m.TermID,
						"data": base64.StdEncoding.EncodeToString(buf), "replay": true,
					})
				}
			}
		}
	case "term_unsub":
		b.subsMu.Lock()
		delete(b.subs, subKey(m.ClientID, m.TermID))
		b.subsMu.Unlock()
	case "ui_state":
		h.setUIState(m.State)
	case "ai_create":
		h.bmAICreate(b, m)
	case "ai_msg":
		h.bmAIMsg(b, m)
	case "ai_approve":
		h.bmAIApprove(b, m)
	case "ai_interrupt":
		h.bmAIInterrupt(b, m)
	case "ai_set_mode":
		h.bmAISetMode(b, m)
	case "ai_set_targets":
		h.bmAISetTargets(b, m)
	case "ai_delete":
		h.bmAIDelete(b, m)
	case "ai_rename":
		h.bmAIRename(b, m)
	}
}

func (h *Hub) bmTermOpen(b *browserConn, m *browserMsg) {
	c := h.getClient(m.ClientID)
	if c == nil {
		b.send(map[string]any{"type": "error", "error": "客户端不存在", "code": "client_not_found"})
		return
	}
	if !c.Online {
		b.send(map[string]any{"type": "error", "error": "客户端不在线", "code": "client_offline"})
		return
	}
	cols, rows := m.Cols, m.Rows
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	termID, err := h.openTerminal(c, m.TermID, m.Title, cols, rows, m.Shell, false)
	if err != nil {
		em := map[string]any{"type": "error", "error": err.Error()}
		if code, params := errCodeOf(err); code != "" {
			em["code"] = code
			if params != nil {
				em["params"] = params
			}
		}
		b.send(em)
		return
	}
	b.subsMu.Lock()
	b.subs[subKey(m.ClientID, termID)] = true
	b.subsMu.Unlock()
	b.send(map[string]any{
		"type": "term_opened", "client_id": m.ClientID, "term_id": termID,
		"title": m.Title,
	})
	// 已有终端补发现场
	if t := c.Terminals[termID]; t != nil {
		if buf := t.snapshot(); len(buf) > 0 {
			b.send(map[string]any{
				"type": "term_output", "client_id": m.ClientID, "term_id": termID,
				"data": base64.StdEncoding.EncodeToString(buf), "replay": true,
			})
		}
	}
	h.broadcastClients()
}

func (h *Hub) bmTermClose(b *browserConn, m *browserMsg) {
	c := h.getClient(m.ClientID)
	if c == nil {
		return
	}
	if c.ac != nil {
		c.ac.send(&proto.Message{Type: proto.TypeTermClose, TermID: m.TermID})
	}
	b.subsMu.Lock()
	delete(b.subs, subKey(m.ClientID, m.TermID))
	b.subsMu.Unlock()
	h.onTermClosed(c, m.TermID, "用户关闭", "term_closed_by_user") // 统一处理：标关闭+清理 AI 目标+广播
}

// ---- UI 状态 ----

func (h *Hub) getUIState() json.RawMessage {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	if len(h.store.st.UIState) == 0 {
		return json.RawMessage(`{}`)
	}
	return h.store.st.UIState
}

func (h *Hub) setUIState(s json.RawMessage) {
	if len(s) == 0 {
		return
	}
	h.store.mu.Lock()
	h.store.st.UIState = s
	h.store.mu.Unlock()
	h.store.markDirty()
}
