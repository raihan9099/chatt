package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ---- 数据模型 ----

// AITarget AI 会话的操作目标：某客户端的某终端。
type AITarget struct {
	ClientID   string `json:"client_id"`
	TermID     string `json:"term_id"`
	ClientName string `json:"client_name"`
	Title      string `json:"title"`
}

// AIMessage 对话消息（结构对齐 OpenAI chat 格式，可直接回放给模型）。
type AIMessage struct {
	Role       string       `json:"role"` // system/user/assistant/tool
	Content    string       `json:"content"`
	ToolCalls  []AIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
	Name       string       `json:"name,omitempty"`
	Time       int64        `json:"time"`
}

// AIToolCall 模型发起的工具调用。
type AIToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
}

// AISession AI 会话（持久化）。
type AISession struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Targets   []AITarget  `json:"targets"`
	Mode      string      `json:"mode"` // auto | approval
	Messages  []AIMessage `json:"messages"`
	Allowlist []string    `json:"allowlist"` // 已批准的命令模式（首词匹配）
	Status    string      `json:"status"`    // idle | running | interrupted
	CreatedAt int64       `json:"created_at"`
	UpdatedAt int64       `json:"updated_at"`
}

// aiTask 一次运行中的 AI 任务。
type aiTask struct {
	ctx    context.Context
	cancel context.CancelFunc

	pmu      sync.Mutex
	pendings map[string]chan string // approvalID -> decision
}

// ---- 会话管理（浏览器消息处理） ----

func (h *Hub) listAISessions() []*AISession {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	out := make([]*AISession, 0, len(h.store.st.AISessions))
	for _, s := range h.store.st.AISessions {
		out = append(out, s)
	}
	return out
}

func (h *Hub) getAISession(id string) *AISession {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	return h.store.st.AISessions[id]
}

func (h *Hub) broadcastAI(v map[string]any) {
	v["type"] = "ai_event"
	h.broadcast(v)
}

func (h *Hub) bmAICreate(b *browserConn, m *browserMsg) {
	mode := m.Mode
	if mode != "auto" {
		mode = "approval"
	}
	name := m.Name
	if name == "" {
		name = "Session " + time.Now().Format("01-02 15:04")
	}
	s := &AISession{
		ID: randHex(8), Name: name, Targets: m.Targets, Mode: mode,
		Status: "idle", CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
	}
	h.store.mu.Lock()
	h.store.st.AISessions[s.ID] = s
	h.store.mu.Unlock()
	h.store.markDirty()
	h.broadcastAI(map[string]any{"kind": "session", "session": s})
}

func (h *Hub) bmAIDelete(b *browserConn, m *browserMsg) {
	h.mu.Lock()
	if t := h.aiRunning[m.SessionID]; t != nil {
		t.cancel()
		delete(h.aiRunning, m.SessionID)
	}
	h.mu.Unlock()
	h.store.mu.Lock()
	delete(h.store.st.AISessions, m.SessionID)
	h.store.mu.Unlock()
	h.store.markDirty()
	h.broadcastAI(map[string]any{"kind": "session_deleted", "session_id": m.SessionID})
}

func (h *Hub) bmAISetMode(b *browserConn, m *browserMsg) {
	s := h.getAISession(m.SessionID)
	if s == nil {
		return
	}
	if m.Mode == "auto" || m.Mode == "approval" {
		h.store.mu.Lock()
		s.Mode = m.Mode
		h.store.mu.Unlock()
		h.store.markDirty()
		h.broadcastAI(map[string]any{"kind": "session", "session": s})
	}
}

func (h *Hub) bmAISetTargets(b *browserConn, m *browserMsg) {
	s := h.getAISession(m.SessionID)
	if s == nil {
		return
	}
	h.store.mu.Lock()
	s.Targets = m.Targets
	h.store.mu.Unlock()
	h.store.markDirty()
	h.broadcastAI(map[string]any{"kind": "session", "session": s})
}

func (h *Hub) bmAIInterrupt(b *browserConn, m *browserMsg) {
	h.mu.Lock()
	t := h.aiRunning[m.SessionID]
	h.mu.Unlock()
	if t != nil {
		t.cancel()
	}
}

func (h *Hub) bmAIApprove(b *browserConn, m *browserMsg) {
	h.mu.Lock()
	var ch chan string
	if t := h.aiRunning[m.SessionID]; t != nil {
		t.pmu.Lock()
		ch = t.pendings[m.ApprovalID]
		t.pmu.Unlock()
	}
	if ch == nil { // 兼容未带 session_id 的调用方：全局查找
		for _, t := range h.aiRunning {
			t.pmu.Lock()
			if c, ok := t.pendings[m.ApprovalID]; ok {
				ch = c
			}
			t.pmu.Unlock()
		}
	}
	h.mu.Unlock()
	if ch != nil {
		select {
		case ch <- m.Decision:
		default:
		}
	}
}

func (h *Hub) bmAIMsg(b *browserConn, m *browserMsg) {
	s := h.getAISession(m.SessionID)
	if s == nil {
		b.send(map[string]any{"type": "error", "error": "AI session not found"})
		return
	}
	baseURL, apiKey, _, _ := h.aiConf()
	if baseURL == "" || apiKey == "" {
		b.send(map[string]any{"type": "ai_event", "kind": "error", "session_id": s.ID, "error": "AI is not configured on the server (AI_BASE_URL/AI_API_KEY)"})
		return
	}
	h.mu.Lock()
	if _, running := h.aiRunning[s.ID]; running {
		h.mu.Unlock()
		b.send(map[string]any{"type": "ai_event", "kind": "error", "session_id": s.ID, "error": "A task is already running in this session; interrupt it or wait for it to finish"})
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	task := &aiTask{ctx: ctx, cancel: cancel, pendings: map[string]chan string{}}
	h.aiRunning[s.ID] = task
	h.mu.Unlock()

	h.appendAIMsg(s, AIMessage{Role: "user", Content: m.Content, Time: time.Now().Unix()})
	go h.runAI(task, s)
}

func (h *Hub) appendAIMsg(s *AISession, msg AIMessage) {
	h.store.mu.Lock()
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now().Unix()
	h.store.mu.Unlock()
	h.store.markDirty()
	h.broadcastAI(map[string]any{"kind": "msg", "session_id": s.ID, "message": msg})
}

func (h *Hub) setAIStatus(s *AISession, status string) {
	h.store.mu.Lock()
	s.Status = status
	h.store.mu.Unlock()
	h.store.markDirty()
	h.broadcastAI(map[string]any{"kind": "status", "session_id": s.ID, "status": status})
}

// ---- Agent 循环（参照 hermes：LLM tool-calling loop） ----

const aiMaxIters = 30

func (h *Hub) runAI(task *aiTask, s *AISession) {
	defer func() {
		h.mu.Lock()
		delete(h.aiRunning, s.ID)
		h.mu.Unlock()
		if r := recover(); r != nil {
			h.logf("AI 任务 panic: %v", r)
		}
		if s.Status == "running" {
			h.setAIStatus(s, "idle")
		}
	}()
	h.setAIStatus(s, "running")

	for i := 0; i < aiMaxIters; i++ {
		if task.ctx.Err() != nil {
			h.setAIStatus(s, "interrupted")
			h.appendAIMsg(s, AIMessage{Role: "system", Content: "[interrupted by user]", Time: time.Now().Unix()})
			return
		}
		resp, err := h.callLLM(task.ctx, s)
		if err != nil {
			if task.ctx.Err() != nil {
				h.setAIStatus(s, "interrupted")
			} else {
				h.broadcastAI(map[string]any{"kind": "error", "session_id": s.ID, "error": err.Error()})
				h.appendAIMsg(s, AIMessage{Role: "system", Content: "[model call failed] " + err.Error(), Time: time.Now().Unix()})
			}
			return
		}
		assistant := AIMessage{
			Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls, Time: time.Now().Unix(),
		}
		h.appendAIMsg(s, assistant)
		if len(resp.ToolCalls) == 0 {
			return // 任务完成
		}
		// 同一轮的工具调用并行执行（多目标下发）
		results := h.execToolCalls(task, s, resp.ToolCalls)
		for _, r := range results {
			h.appendAIMsg(s, r)
		}
	}
	h.appendAIMsg(s, AIMessage{Role: "system", Content: fmt.Sprintf("[max iterations (%d) reached, stopped]", aiMaxIters), Time: time.Now().Unix()})
}

// execToolCalls 并行执行工具调用，保持结果顺序与调用一致。
func (h *Hub) execToolCalls(task *aiTask, s *AISession, calls []AIToolCall) []AIMessage {
	results := make([]AIMessage, len(calls))
	var wg sync.WaitGroup
	for i, tc := range calls {
		h.broadcastAI(map[string]any{
			"kind": "tool_call", "session_id": s.ID,
			"tool": tc.Name, "args": tc.Arguments, "call_id": tc.ID,
		})
		wg.Add(1)
		go func(i int, tc AIToolCall) {
			defer wg.Done()
			out := h.execOneTool(task, s, tc)
			h.broadcastAI(map[string]any{
				"kind": "tool_result", "session_id": s.ID,
				"tool": tc.Name, "call_id": tc.ID, "result": out,
			})
			results[i] = AIMessage{
				Role: "tool", Content: out, ToolCallID: tc.ID, Name: tc.Name, Time: time.Now().Unix(),
			}
		}(i, tc)
	}
	wg.Wait()
	return results
}

// execOneTool 审批 + 执行单个工具。
func (h *Hub) execOneTool(task *aiTask, s *AISession, tc AIToolCall) string {
	if task.ctx.Err() != nil {
		return "[interrupted]"
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
		return "failed to parse tool arguments: " + err.Error()
	}

	// 授权（approval 模式，参照 hermes：逐条审批 / allow_always 入 allowlist）
	if needsApproval(tc.Name) && s.Mode == "approval" {
		key := approvalKey(tc.Name, args)
		if !h.inAllowlist(s, key) {
			decision := h.waitApproval(task, s, tc, args, key)
			switch decision {
			case "allow_always":
				h.addAllowlist(s, key)
			case "allow":
			default:
				return "[declined by user]"
			}
		}
	}

	out, err := h.runTool(task.ctx, s, tc.Name, args)
	if err != nil {
		return "error: " + err.Error()
	}
	return out
}

// waitApproval 推送审批请求并等待用户决定（超时 5 分钟视为拒绝）。
func (h *Hub) waitApproval(task *aiTask, s *AISession, tc AIToolCall, args map[string]any, key string) string {
	id := randHex(8)
	ch := make(chan string, 1)
	task.pmu.Lock()
	task.pendings[id] = ch
	task.pmu.Unlock()
	defer func() {
		task.pmu.Lock()
		delete(task.pendings, id)
		task.pmu.Unlock()
	}()
	h.broadcastAI(map[string]any{
		"kind": "approval", "session_id": s.ID,
		"approval": map[string]any{
			"id": id, "tool": tc.Name, "args": tc.Arguments, "summary": approvalSummary(tc.Name, args),
		},
	})
	select {
	case d := <-ch:
		return d
	case <-task.ctx.Done():
		return "deny"
	case <-time.After(5 * time.Minute):
		return "deny"
	}
}

func (h *Hub) inAllowlist(s *AISession, key string) bool {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	for _, k := range s.Allowlist {
		if k == key {
			return true
		}
	}
	return false
}

func (h *Hub) addAllowlist(s *AISession, key string) {
	h.store.mu.Lock()
	s.Allowlist = append(s.Allowlist, key)
	h.store.mu.Unlock()
	h.store.markDirty()
}

// needsApproval 只读工具免审批。
func needsApproval(tool string) bool {
	switch tool {
	case "run_command", "send_input", "write_file":
		return true
	}
	return false
}

// approvalKey allowlist 键：命令按首词（如 "ls"、"systemctl"），写文件按路径。
func approvalKey(tool string, args map[string]any) string {
	switch tool {
	case "run_command":
		cmd, _ := args["command"].(string)
		fields := strings.Fields(cmd)
		if len(fields) > 0 {
			return "cmd:" + fields[0]
		}
		return "cmd:"
	case "write_file":
		p, _ := args["path"].(string)
		return "write:" + p
	case "send_input":
		return "send_input"
	}
	return tool
}

func approvalSummary(tool string, args map[string]any) string {
	switch tool {
	case "run_command":
		cmd, _ := args["command"].(string)
		return "Run command: " + cmd
	case "send_input":
		return "Send keystrokes to terminal"
	case "write_file":
		p, _ := args["path"].(string)
		return "Write file: " + p
	}
	return tool
}

// ---- 工具实现 ----

// checkTarget 校验 (client, term) 是否在会话目标集合内。
// termID 为空（文件操作等不针对特定终端）时，只要客户端在目标集合内即放行。
func (s *AISession) checkTarget(clientID, termID string) error {
	if len(s.Targets) == 0 {
		return nil // 未限定目标时允许全部
	}
	for _, t := range s.Targets {
		if t.ClientID != clientID {
			continue
		}
		if termID == "" || t.TermID == "" || t.TermID == termID {
			return nil
		}
	}
	return fmt.Errorf("target %s/%s is outside this session's allowed targets", clientID, termID)
}

func (h *Hub) runTool(ctx context.Context, s *AISession, name string, args map[string]any) (string, error) {
	str := func(k string) string { v, _ := args[k].(string); return v }
	switch name {
	case "list_clients":
		return h.toolListClients()
	case "run_command":
		return h.toolRunCommand(ctx, s, str("client_id"), str("term_id"), str("command"))
	case "read_terminal":
		n := 4000
		if f, ok := args["tail_chars"].(float64); ok && f > 0 {
			n = int(f)
		}
		return h.toolReadTerminal(s, str("client_id"), str("term_id"), n)
	case "send_input":
		return h.toolSendInput(s, str("client_id"), str("term_id"), str("data"))
	case "list_files":
		return h.toolListFiles(s, str("client_id"), str("path"))
	case "read_file":
		return h.toolReadFile(s, str("client_id"), str("path"))
	case "write_file":
		return h.toolWriteFile(s, str("client_id"), str("path"), str("content"))
	}
	return "", errors.New("unknown tool: " + name)
}

var ansiRe = regexp.MustCompile("\x1b(\\[[0-9;?]*[a-zA-Z]|\\][^\x07\x1b]*(\x07|\x1b\\\\)|[()][0-2]|[>=])")

func stripANSI(s string) string {
	s = ansiRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

func capStr(s string, n int) string {
	if len(s) > n {
		return s[:n] + fmt.Sprintf("\n...[truncated, %d chars total]", len(s))
	}
	return s
}
