package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"termirai/internal/proto"
)

// ---- 各工具的具体实现 ----

func (h *Hub) toolListClients() (string, error) {
	var sb strings.Builder
	for _, c := range h.listClients() {
		status := "offline"
		if c.Online {
			status = "online"
		}
		fmt.Fprintf(&sb, "client %s (id=%s, %s, %s):\n", clientDisplayName(c), c.ID, c.Host, status)
		for _, t := range c.Terminals {
			state := "running"
			if t.Closed {
				state = "closed"
			}
			fmt.Fprintf(&sb, "  - terminal %s (id=%s, %s)\n", t.Title, t.ID, state)
		}
	}
	return sb.String(), nil
}

func (h *Hub) toolRunCommand(ctx context.Context, s *AISession, clientID, termID, command string) (string, error) {
	if command == "" {
		return "", errors.New("command must not be empty")
	}
	if err := s.checkTarget(clientID, termID); err != nil {
		return "", err
	}
	c := h.getClient(clientID)
	if c == nil || !c.Online || c.ac == nil {
		return "", fmt.Errorf("client %s is offline", clientID)
	}
	t := c.Terminals[termID]
	if t == nil || t.Closed {
		return "", fmt.Errorf("terminal %s not found or closed", termID)
	}

	before := len(t.snapshot())
	// 发送命令（加一个空格回显哨兵，帮助识别命令已开始）
	input := command + "\n"
	if err := c.ac.send(&proto.Message{
		Type:   proto.TypeTermInput,
		TermID: termID,
		Data:   base64.StdEncoding.EncodeToString([]byte(input)),
	}); err != nil {
		return "", err
	}

	// 等待输出静默：300ms 轮询，连续 1.2s 无增长视为完成，最长 25s
	deadline := time.Now().Add(25 * time.Second)
	lastLen := before
	quietSince := time.Now()
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return "[interrupted]", nil
		}
		select {
		case <-ctx.Done():
			return "[interrupted]", nil
		case <-time.After(300 * time.Millisecond):
		}
		cur := len(t.snapshot())
		if cur != lastLen {
			lastLen = cur
			quietSince = time.Now()
		} else if cur > before && time.Since(quietSince) > 1200*time.Millisecond {
			break
		}
	}
	out := t.snapshot()
	if len(out) < before {
		before = 0
	}
	newOut := stripANSI(string(out[before:]))
	return capStr(strings.TrimSpace(newOut), 4000), nil
}

func (h *Hub) toolReadTerminal(s *AISession, clientID, termID string, tailChars int) (string, error) {
	if err := s.checkTarget(clientID, termID); err != nil {
		return "", err
	}
	c := h.getClient(clientID)
	if c == nil {
		return "", fmt.Errorf("client %s not found", clientID)
	}
	t := c.Terminals[termID]
	if t == nil {
		return "", fmt.Errorf("terminal %s not found", termID)
	}
	buf := stripANSI(string(t.snapshot()))
	if len(buf) > tailChars {
		buf = buf[len(buf)-tailChars:]
	}
	return buf, nil
}

func (h *Hub) toolSendInput(s *AISession, clientID, termID, data string) (string, error) {
	if err := s.checkTarget(clientID, termID); err != nil {
		return "", err
	}
	c := h.getClient(clientID)
	if c == nil || !c.Online || c.ac == nil {
		return "", fmt.Errorf("client %s is offline", clientID)
	}
	if t := c.Terminals[termID]; t == nil || t.Closed {
		return "", fmt.Errorf("terminal %s not found or closed", termID)
	}
	// 支持转义写法如 \x03（Ctrl-C）、\n
	raw := decodeEscapes(data)
	if err := c.ac.send(&proto.Message{
		Type:   proto.TypeTermInput,
		TermID: termID,
		Data:   base64.StdEncoding.EncodeToString(raw),
	}); err != nil {
		return "", err
	}
	return "sent", nil
}

func decodeEscapes(s string) []byte {
	var out []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'x':
				if i+3 < len(s) {
					var v int
					fmt.Sscanf(s[i+2:i+4], "%02x", &v)
					out = append(out, byte(v))
					i += 3
					continue
				}
				out = append(out, s[i])
			default:
				out = append(out, s[i+1])
			}
			i++
			continue
		}
		out = append(out, s[i])
	}
	return out
}

func (h *Hub) fileToolCall(s *AISession, clientID string, m *proto.Message) (*proto.Message, error) {
	if err := s.checkTarget(clientID, ""); err != nil {
		return nil, err
	}
	c := h.getClient(clientID)
	if c == nil || !c.Online {
		return nil, fmt.Errorf("client %s is offline", clientID)
	}
	m.Type = proto.TypeFileOp
	return h.callAgent(c, m, 30*time.Second)
}

func (h *Hub) toolListFiles(s *AISession, clientID, path string) (string, error) {
	ack, err := h.fileToolCall(s, clientID, &proto.Message{Op: proto.OpList, Path: path})
	if err != nil {
		return "", err
	}
	if !ack.OK {
		return "", errors.New(agentErrEn(ack.ErrCode, ack.ErrParams, ack.Error))
	}
	var sb strings.Builder
	for _, e := range ack.Entries {
		kind := "f"
		if e.Dir {
			kind = "d"
		}
		fmt.Fprintf(&sb, "%s %10d %s\n", kind, e.Size, e.Name)
	}
	return sb.String(), nil
}

func (h *Hub) toolReadFile(s *AISession, clientID, path string) (string, error) {
	ack, err := h.fileToolCall(s, clientID, &proto.Message{Op: proto.OpRead, Path: path})
	if err != nil {
		return "", err
	}
	if !ack.OK {
		return "", errors.New(agentErrEn(ack.ErrCode, ack.ErrParams, ack.Error))
	}
	b, err := base64.StdEncoding.DecodeString(ack.Data)
	if err != nil {
		return "", err
	}
	return capStr(string(b), 8000), nil
}

func (h *Hub) toolWriteFile(s *AISession, clientID, path, content string) (string, error) {
	ack, err := h.fileToolCall(s, clientID, &proto.Message{
		Op:   proto.OpWrite,
		Path: path,
		Data: base64.StdEncoding.EncodeToString([]byte(content)),
	})
	if err != nil {
		return "", err
	}
	if !ack.OK {
		return "", errors.New(agentErrEn(ack.ErrCode, ack.ErrParams, ack.Error))
	}
	return "written: " + path, nil
}

// ---- LLM 调用（OpenAI 兼容接口） ----

type llmResponse struct {
	Content   string
	ToolCalls []AIToolCall
}

type llmToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func (h *Hub) callLLM(ctx context.Context, s *AISession) (*llmResponse, error) {
	baseURL, apiKey, model, _ := h.aiConf()
	msgs := h.buildLLMMessages(s)
	tools := llmToolSchemas()
	payload := map[string]any{
		"model":    model,
		"messages": msgs,
		"tools":    tools,
	}
	body, _ := json.Marshal(payload)
	req, err := httpNewRequestWithContext(ctx, "POST", strings.TrimRight(baseURL, "/")+"/chat/completions", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpDefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var parsed struct {
		Error   *struct{ Message string `json:"message"` } `json:"error"`
		Choices []struct {
			Message struct {
				Content   string        `json:"content"`
				ToolCalls []llmToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Error != nil {
		return nil, errors.New(parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, errors.New("empty model response")
	}
	out := &llmResponse{Content: parsed.Choices[0].Message.Content}
	for _, tc := range parsed.Choices[0].Message.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, AIToolCall{
			ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
		})
	}
	return out, nil
}

// buildLLMMessages 组装上下文：system + 历史；超长时压缩旧工具输出与中间消息。
func (h *Hub) buildLLMMessages(s *AISession) []map[string]any {
	h.store.mu.Lock()
	history := append([]AIMessage(nil), s.Messages...)
	h.store.mu.Unlock()

	msgs := []map[string]any{{"role": "system", "content": h.systemPrompt(s)}}
	// 估算大小（粗略：4 字符 ≈ 1 token，阈值 24000 tokens）
	const maxChars = 24000 * 4
	total := 0
	for i := range history {
		total += len(history[i].Content)
	}
	// 超限则先压缩旧工具结果
	if total > maxChars {
		for i := range history {
			if history[i].Role == "tool" && len(history[i].Content) > 500 && i < len(history)-6 {
				history[i].Content = history[i].Content[:500] + "...[compressed]"
			}
		}
	}
	// 仍超限则丢弃中间消息
	for total > maxChars && len(history) > 12 {
		history = append(history[:2], history[4:]...)
		total = 0
		for i := range history {
			total += len(history[i].Content)
		}
	}
	for _, m := range history {
		entry := map[string]any{"role": m.Role, "content": m.Content}
		if len(m.ToolCalls) > 0 {
			tcs := []map[string]any{}
			for _, tc := range m.ToolCalls {
				tcs = append(tcs, map[string]any{
					"id": tc.ID, "type": "function",
					"function": map[string]any{"name": tc.Name, "arguments": tc.Arguments},
				})
			}
			entry["tool_calls"] = tcs
		}
		if m.ToolCallID != "" {
			entry["tool_call_id"] = m.ToolCallID
			entry["name"] = m.Name
		}
		msgs = append(msgs, entry)
	}
	return msgs
}

func (h *Hub) systemPrompt(s *AISession) string {
	var sb strings.Builder
	sb.WriteString(`You are the Termirai AI ops assistant. You operate remote terminals and files on connected clients (Windows/Linux/macOS) directly through tools.
Workflow (hermes-agent style agent loop):
1. Understand the user's task and plan the steps.
2. Use run_command to execute commands in target terminals, or read/write files.
3. Observe the output and proceed step by step until the task is done.
4. Summarize the results in the same language the user used.

Rules:
- Work step by step; observe each result before deciding the next step.
- Confirm dangerous commands (rm -rf, mkfs, etc.) with the user before running them.
- run_command output is captured during a quiet window; for long-running commands use read_terminal to keep watching.
- send_input can send control characters (e.g. \x03 for Ctrl-C).
`)
	if len(s.Targets) > 0 {
		sb.WriteString("\nTerminals authorized for this session (you may only operate these):\n")
		for _, t := range s.Targets {
			fmt.Fprintf(&sb, "- client %s (client_id=%s) terminal %s (term_id=%s)\n", t.ClientName, t.ClientID, t.Title, t.TermID)
		}
		sb.WriteString("To operate multiple machines, issue multiple tool calls in the same response; they run in parallel.\n")
	} else {
		sb.WriteString("\nThe user has not pinned target terminals. Call list_clients first to see online clients and terminals, then pick suitable ones.\n")
	}
	return sb.String()
}

func llmToolSchemas() []map[string]any {
	obj := func(props map[string]any, required []string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	strP := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	target := map[string]any{
		"client_id": strP("ID of the target client"),
		"term_id":   strP("ID of the target terminal"),
	}
	fn := func(name, desc string, params map[string]any) map[string]any {
		return map[string]any{"type": "function", "function": map[string]any{"name": name, "description": desc, "parameters": params}}
	}
	return []map[string]any{
		fn("list_clients", "List all clients and their terminals (with ids and online status)", obj(map[string]any{}, []string{})),
		fn("run_command", "Run a shell command in the specified terminal and return its output (waits for output to go quiet, up to ~25s)", obj(map[string]any{
			"client_id": target["client_id"], "term_id": target["term_id"],
			"command": strP("Shell command to execute"),
		}, []string{"client_id", "term_id", "command"})),
		fn("read_terminal", "Read the terminal's recent output (for watching long-running commands)", obj(map[string]any{
			"client_id": target["client_id"], "term_id": target["term_id"],
			"tail_chars": map[string]any{"type": "integer", "description": "How many trailing characters to read, default 4000"},
		}, []string{"client_id", "term_id"})),
		fn("send_input", "Send raw input/keystrokes to the terminal (escapes like \\n, \\x03 supported), for interactive programs", obj(map[string]any{
			"client_id": target["client_id"], "term_id": target["term_id"],
			"data": strP("Raw input to send"),
		}, []string{"client_id", "term_id", "data"})),
		fn("list_files", "List files in a directory on a client", obj(map[string]any{
			"client_id": target["client_id"], "path": strP("Directory path"),
		}, []string{"client_id", "path"})),
		fn("read_file", "Read a text file on a client", obj(map[string]any{
			"client_id": target["client_id"], "path": strP("File path"),
		}, []string{"client_id", "path"})),
		fn("write_file", "Write (overwrite) a text file on a client", obj(map[string]any{
			"client_id": target["client_id"], "path": strP("File path"),
			"content": strP("Full file content"),
		}, []string{"client_id", "path", "content"})),
	}
}
