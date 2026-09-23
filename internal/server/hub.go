package server

import (
	"encoding/base64"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"termirai/internal/proto"
)

// Hub 服务端中枢：客户端注册表、浏览器连接、消息路由、AI 会话。
type Hub struct {
	cfg   *Config
	store *store

	mu        sync.RWMutex
	browsers  map[*browserConn]bool
	aiRunning map[string]*aiTask // sessionID -> 运行中任务
}

func NewHub(cfg *Config, st *store) *Hub {
	h := &Hub{
		cfg:       cfg,
		store:     st,
		browsers:  map[*browserConn]bool{},
		aiRunning: map[string]*aiTask{},
	}
	for _, c := range st.st.Clients {
		c.initRuntime()
		c.Online = false // 重启后等待 agent 重连
	}
	return h
}

// ---- 客户端注册表 ----

func (h *Hub) listClients() []*Client {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	out := make([]*Client, 0, len(h.store.st.Clients))
	for _, c := range h.store.st.Clients {
		out = append(out, c)
	}
	return out
}

func (h *Hub) getClient(id string) *Client {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	return h.store.st.Clients[id]
}

// deleteClient 删除离线客户端（在线拒绝）。
func (h *Hub) deleteClient(id string) error {
	h.store.mu.Lock()
	c := h.store.st.Clients[id]
	if c == nil {
		h.store.mu.Unlock()
		return &codedError{code: "client_not_found", err: errors.New("客户端不存在")}
	}
	if c.Online {
		h.store.mu.Unlock()
		return &codedError{code: "client_online_cannot_delete", err: errors.New("客户端在线，不能删除")}
	}
	delete(h.store.st.Clients, id)
	h.store.mu.Unlock()
	h.store.markDirty()
	h.broadcastClients()
	return nil
}

// clientDisplayName 客户端展示名：别名优先。
func clientDisplayName(c *Client) string {
	if c.Alias != "" {
		return c.Alias
	}
	return c.Name
}

// aiConf 生效的 AI 配置：Web 端持久化配置优先，空字段回落 .env。
func (h *Hub) aiConf() (baseURL, apiKey, model, defMode string) {
	baseURL, apiKey, model, defMode = h.cfg.AIBaseURL, h.cfg.AIAPIKey, h.cfg.AIModel, h.cfg.AIDefMode
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	if a := h.store.st.AI; a != nil {
		if a.BaseURL != "" {
			baseURL = a.BaseURL
		}
		if a.APIKey != "" {
			apiKey = a.APIKey
		}
		if a.Model != "" {
			model = a.Model
		}
		if a.DefaultMode != "" {
			defMode = a.DefaultMode
		}
	}
	return
}

// bmClientAlias 设置/清除客户端别名（空串为清除），同步 AI 会话目标里的显示名并广播。
func (h *Hub) bmClientAlias(b *browserConn, m *browserMsg) {
	alias := strings.TrimSpace(m.Name)
	c := h.getClient(m.ClientID)
	if c == nil {
		b.send(map[string]any{"type": "error", "error": "客户端不存在", "code": "client_not_found"})
		return
	}
	h.store.mu.Lock()
	c.Alias = alias
	disp := clientDisplayName(c)
	for _, s := range h.store.st.AISessions {
		for i, tg := range s.Targets {
			if tg.ClientID == m.ClientID {
				s.Targets[i].ClientName = disp
			}
		}
	}
	h.store.mu.Unlock()
	h.store.markDirty()
	h.broadcastClients()
}

// bmAIRename 重命名 AI 会话。
func (h *Hub) bmAIRename(b *browserConn, m *browserMsg) {
	name := strings.TrimSpace(m.Name)
	if name == "" {
		return
	}
	s := h.getAISession(m.SessionID)
	if s == nil {
		return
	}
	h.store.mu.Lock()
	s.Name = name
	h.store.mu.Unlock()
	h.store.markDirty()
	h.broadcastAI(map[string]any{"kind": "session", "session": s})
}

// onTermClosed 终端关闭的统一处理：标记关闭、落盘、广播，并从所有 AI 会话的目标集合中剔除。
func (h *Hub) onTermClosed(c *Client, termID, reason, reasonCode string) {
	h.store.mu.Lock()
	t := c.Terminals[termID]
	if t == nil || t.Closed {
		h.store.mu.Unlock()
		return
	}
	t.Closed = true
	changed := h.pruneDeadTargetsLocked()
	h.store.mu.Unlock()
	h.store.markDirty()
	h.broadcastTermEvent(c.ID, termID, "closed", reason, reasonCode)
	for _, s := range changed {
		h.broadcastAI(map[string]any{"kind": "session", "session": s})
	}
	h.broadcastClients()
}

// pruneDeadTargetsLocked 从所有 AI 会话的目标集合中剔除已关闭/已消失的终端。
// 调用方需持有 store.mu；返回目标发生变更的会话（广播在锁外进行）。
func (h *Hub) pruneDeadTargetsLocked() []*AISession {
	var changed []*AISession
	for _, s := range h.store.st.AISessions {
		kept := s.Targets[:0]
		dirty := false
		for _, tg := range s.Targets {
			alive := false
			if cl := h.store.st.Clients[tg.ClientID]; cl != nil {
				if tm := cl.Terminals[tg.TermID]; tm != nil && !tm.Closed {
					alive = true
				}
			}
			if alive {
				kept = append(kept, tg)
			} else {
				dirty = true
			}
		}
		if dirty {
			s.Targets = kept
			changed = append(changed, s)
		}
	}
	return changed
}

// reconcile 注册时对账：agent 上报的存活终端之外的全部标记关闭。
func (h *Hub) reconcile(c *Client, alive []proto.TermInfo) {
	var closed []string
	h.store.mu.Lock()
	aliveSet := map[string]bool{}
	for _, ti := range alive {
		aliveSet[ti.ID] = true
		if t, ok := c.Terminals[ti.ID]; ok {
			if t.Closed {
				t.Closed = false // agent 说还活着，以 agent 为准
			}
		} else {
			c.Terminals[ti.ID] = &TermState{ID: ti.ID, Title: ti.Title, CreatedAt: ti.CreatedAt}
		}
	}
	for id, t := range c.Terminals {
		if !aliveSet[id] && !t.Closed {
			t.Closed = true
			closed = append(closed, id)
		}
	}
	var changed []*AISession
	if len(closed) > 0 {
		changed = h.pruneDeadTargetsLocked()
	}
	h.store.mu.Unlock()
	for _, id := range closed {
		h.broadcastTermEvent(c.ID, id, "closed", "agent 重连后未上报", "term_closed_not_reported")
	}
	for _, s := range changed {
		h.broadcastAI(map[string]any{"kind": "session", "session": s})
	}
}

// ---- 浏览器广播 ----

func (h *Hub) addBrowser(b *browserConn) {
	h.mu.Lock()
	h.browsers[b] = true
	h.mu.Unlock()
}

func (h *Hub) removeBrowser(b *browserConn) {
	h.mu.Lock()
	delete(h.browsers, b)
	h.mu.Unlock()
}

func (h *Hub) broadcast(v any) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for b := range h.browsers {
		b.send(v)
	}
}

func (h *Hub) broadcastClients() {
	h.broadcast(map[string]any{"type": "clients", "clients": h.listClients()})
}

// broadcastTermData 把终端输出推给订阅了的浏览器。
func (h *Hub) broadcastTermData(clientID, termID string, data []byte, replay bool) {
	key := subKey(clientID, termID)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for b := range h.browsers {
		b.subsMu.Lock()
		ok := b.subs[key]
		b.subsMu.Unlock()
		if ok {
			b.send(map[string]any{
				"type": "term_output", "client_id": clientID, "term_id": termID,
				"data": base64.StdEncoding.EncodeToString(data), "replay": replay,
			})
		}
	}
}

func (h *Hub) broadcastTermEvent(clientID, termID, event, reason, reasonCode string) {
	h.broadcast(map[string]any{
		"type": "term_event", "client_id": clientID, "term_id": termID,
		"event": event, "reason": reason, "code": reasonCode,
	})
}

// ---- 到 agent 的请求/响应 ----

// callAgent 向客户端发请求并等待带 req_id 的应答。
func (h *Hub) callAgent(c *Client, m *proto.Message, timeout time.Duration) (*proto.Message, error) {
	if c.ac == nil {
		return nil, &codedError{code: "client_offline", err: errors.New("客户端不在线")}
	}
	m.ReqID = randHex(8)
	ch := make(chan *proto.Message, 1)
	c.pmu.Lock()
	c.pending[m.ReqID] = ch
	c.pmu.Unlock()
	defer func() {
		c.pmu.Lock()
		delete(c.pending, m.ReqID)
		c.pmu.Unlock()
	}()
	if err := c.ac.send(m); err != nil {
		return nil, err
	}
	select {
	case ack := <-ch:
		return ack, nil
	case <-time.After(timeout):
		return nil, &codedError{code: "agent_timeout", err: errors.New("等待客户端应答超时")}
	}
}

// openTerminal 在客户端打开/reattach 终端；needReplay 为真时让 agent 补发历史输出。
// shell 仅新建时生效（空=客户端默认 shell）。
func (h *Hub) openTerminal(c *Client, termID, title string, cols, rows int, shell string, needReplay bool) (string, error) {
	ack, err := h.callAgent(c, &proto.Message{
		Type: proto.TypeTermOpen, TermID: termID, Name: title,
		Cols: cols, Rows: rows, Shell: shell, Replay: needReplay,
	}, 10*time.Second)
	if err != nil {
		return "", err
	}
	if !ack.OK {
		return "", &codedError{code: ack.ErrCode, params: ack.ErrParams, err: errors.New(ack.Error)}
	}
	h.store.mu.Lock()
	created := false
	if _, ok := c.Terminals[ack.TermID]; !ok {
		c.Terminals[ack.TermID] = &TermState{ID: ack.TermID, Title: title, CreatedAt: time.Now().Unix()}
		created = true
	}
	h.store.mu.Unlock()
	if created {
		h.store.markDirty()
	}
	return ack.TermID, nil
}

func (h *Hub) logf(format string, args ...any) { log.Printf("[hub] "+format, args...) }

// bmTermRename 重命名终端：更新注册表与 AI 会话目标中的标题，同步 agent 持久化，并广播。
func (h *Hub) bmTermRename(b *browserConn, m *browserMsg) {
	title := strings.TrimSpace(m.Title)
	if title == "" {
		return
	}
	c := h.getClient(m.ClientID)
	if c == nil {
		b.send(map[string]any{"type": "error", "error": "客户端不存在", "code": "client_not_found"})
		return
	}
	h.store.mu.Lock()
	t := c.Terminals[m.TermID]
	if t == nil || t.Closed {
		h.store.mu.Unlock()
		b.send(map[string]any{"type": "error", "error": "终端不存在或已关闭", "code": "term_not_found_or_closed"})
		return
	}
	t.Title = title
	// 同步各 AI 会话目标里的显示标题
	for _, s := range h.store.st.AISessions {
		for i, tg := range s.Targets {
			if tg.ClientID == m.ClientID && tg.TermID == m.TermID {
				s.Targets[i].Title = title
			}
		}
	}
	h.store.mu.Unlock()
	h.store.markDirty()
	if c.ac != nil {
		c.ac.send(&proto.Message{Type: proto.TypeTermRename, TermID: m.TermID, Name: title})
	}
	h.broadcastClients()
}
