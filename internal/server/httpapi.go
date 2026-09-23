package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"termirai/internal/proto"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// writeErrL 输出带稳定错误码的错误（errCode 为空时退化为 writeErr），供前端本地化。
func writeErrL(w http.ResponseWriter, code int, msg, errCode string, params map[string]any) {
	if errCode == "" {
		writeErr(w, code, msg)
		return
	}
	v := map[string]any{"error": msg, "code": errCode}
	if params != nil {
		v["params"] = params
	}
	writeJSON(w, code, v)
}

// writeErrE 输出错误，自动提取 codedError 携带的错误码。
func writeErrE(w http.ResponseWriter, code int, err error) {
	c, params := errCodeOf(err)
	writeErrL(w, code, err.Error(), c, params)
}

// ---- 认证 ----

func (h *Hub) handleCaptcha(w http.ResponseWriter, r *http.Request) {
	id, img, err := newCaptcha()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"captcha_id": id, "image": img})
}

type loginReq struct {
	User      string `json:"user"`
	Password  string `json:"password"`
	CaptchaID string `json:"captcha_id"`
	Captcha   string `json:"captcha"`
}

func (h *Hub) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrL(w, 400, "请求格式错误", "bad_request", nil)
		return
	}
	if ok, remain := h.checkLoginAllowed(); !ok {
		writeJSON(w, 429, map[string]any{"error": "登录已锁定", "locked_seconds": remain})
		return
	}
	if !verifyCaptcha(req.CaptchaID, req.Captcha) {
		h.recordLoginFail()
		writeErrL(w, 401, "验证码错误或已过期", "captcha_invalid", nil)
		return
	}
	if req.User != h.cfg.AdminUser || req.Password != h.cfg.AdminPass {
		h.recordLoginFail()
		writeErrL(w, 401, "用户名或密码错误", "bad_credentials", nil)
		return
	}
	h.recordLoginSuccess()
	token, err := h.issueToken()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"token": token, "expires_in": int64(tokenTTL / time.Second)})
}

// ---- 客户端 ----

func (h *Hub) handleClients(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"clients": h.listClients()})
}

func (h *Hub) handleDeleteClient(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.deleteClient(id); err != nil {
		writeErrE(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// handleSetAlias 设置客户端别名（REST，供 App 端使用；网页端走 WS client_alias）。
func (h *Hub) handleSetAlias(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Alias string `json:"alias"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	c := h.getClient(id)
	if c == nil {
		writeErrL(w, 404, "客户端不存在", "client_not_found", nil)
		return
	}
	h.store.mu.Lock()
	c.Alias = body.Alias
	disp := clientDisplayName(c)
	for _, s := range h.store.st.AISessions {
		for i, tg := range s.Targets {
			if tg.ClientID == id {
				s.Targets[i].ClientName = disp
			}
		}
	}
	h.store.mu.Unlock()
	h.store.markDirty()
	h.broadcastClients()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---- AI 配置（Web 端可改，持久化覆盖 .env） ----

func maskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return "****"
	}
	return k[:3] + "****" + k[len(k)-4:]
}

func (h *Hub) handleAIConfigGet(w http.ResponseWriter, r *http.Request) {
	baseURL, apiKey, model, defMode := h.aiConf()
	writeJSON(w, 200, map[string]any{
		"base_url":     baseURL,
		"api_key_mask": maskKey(apiKey),
		"api_key_set":  apiKey != "",
		"model":        model,
		"default_mode": defMode,
	})
}

func (h *Hub) handleAIConfigPost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BaseURL     string `json:"base_url"`
		APIKey      string `json:"api_key"` // 空 = 保持现状
		Model       string `json:"model"`
		DefaultMode string `json:"default_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrL(w, 400, "请求格式错误", "bad_request", nil)
		return
	}
	if body.DefaultMode != "" && body.DefaultMode != "auto" && body.DefaultMode != "approval" {
		writeErrL(w, 400, "default_mode 只能是 auto 或 approval", "bad_default_mode", nil)
		return
	}
	h.store.mu.Lock()
	a := h.store.st.AI
	if a == nil {
		a = &aiSettings{}
		h.store.st.AI = a
	}
	a.BaseURL = strings.TrimSpace(body.BaseURL)
	a.Model = strings.TrimSpace(body.Model)
	a.DefaultMode = body.DefaultMode
	if k := strings.TrimSpace(body.APIKey); k != "" {
		a.APIKey = k
	}
	h.store.mu.Unlock()
	h.store.markDirty()

	baseURL, apiKey, model, defMode := h.aiConf()
	writeJSON(w, 200, map[string]any{
		"ok":          true,
		"ai_enabled":  baseURL != "" && apiKey != "",
		"ai_default_mode": defMode,
		"base_url":    baseURL,
		"model":       model,
		"api_key_mask": maskKey(apiKey),
	})
}

// handleAITest 用生效配置发一条最小请求，验证连通性。
func (h *Hub) handleAITest(w http.ResponseWriter, r *http.Request) {
	baseURL, apiKey, model, _ := h.aiConf()
	if baseURL == "" || apiKey == "" || model == "" {
		writeErrL(w, 400, "AI 未配置完整（base_url/api_key/model）", "ai_not_configured", nil)
		return
	}
	payload := map[string]any{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "ping - reply with pong only"}},
		"max_tokens": 128,
	}
	body, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	req, err := httpNewRequestWithContext(ctx, "POST", strings.TrimRight(baseURL, "/")+"/chat/completions", body)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpDefaultClient.Do(req)
	if err != nil {
		writeErr(w, 502, "连接失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	var parsed struct {
		Error   *struct{ Message string `json:"message"` } `json:"error"`
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				Reasoning string `json:"reasoning_content"` // 推理模型
			} `json:"message"`
		} `json:"choices"`
	}
	json.NewDecoder(resp.Body).Decode(&parsed)
	if parsed.Error != nil {
		writeErr(w, 502, "模型返回错误: "+parsed.Error.Message)
		return
	}
	if len(parsed.Choices) == 0 {
		writeErrL(w, 502, "模型无响应内容", "ai_empty_response", nil)
		return
	}
	reply := parsed.Choices[0].Message.Content
	if reply == "" {
		reply = parsed.Choices[0].Message.Reasoning
	}
	writeJSON(w, 200, map[string]string{"ok": "true", "reply": reply})
}

// ---- 文件代理 ----

// fileCall 把文件操作转发给目标客户端。
func (h *Hub) fileCall(w http.ResponseWriter, clientID string, m *proto.Message) {
	c := h.getClient(clientID)
	if c == nil {
		writeErrL(w, 404, "客户端不存在", "client_not_found", nil)
		return
	}
	if !c.Online {
		writeErrL(w, 409, "客户端不在线", "client_offline", nil)
		return
	}
	m.Type = proto.TypeFileOp
	ack, err := h.callAgent(c, m, 30*time.Second)
	if err != nil {
		writeErrE(w, 504, err)
		return
	}
	if !ack.OK {
		writeErrL(w, 400, ack.Error, ack.ErrCode, ack.ErrParams)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "entries": ack.Entries, "data": ack.Data})
}

func (h *Hub) handleFileList(w http.ResponseWriter, r *http.Request) {
	h.fileCall(w, r.PathValue("id"), &proto.Message{Op: proto.OpList, Path: r.URL.Query().Get("path")})
}

func (h *Hub) handleFileRead(w http.ResponseWriter, r *http.Request) {
	h.fileCall(w, r.PathValue("id"), &proto.Message{Op: proto.OpRead, Path: r.URL.Query().Get("path")})
}

type fileWriteReq struct {
	Path string `json:"path"`
	Data string `json:"data"` // base64
}

func (h *Hub) handleFileWrite(w http.ResponseWriter, r *http.Request) {
	var req fileWriteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrL(w, 400, "请求格式错误", "bad_request", nil)
		return
	}
	h.fileCall(w, r.PathValue("id"), &proto.Message{Op: proto.OpWrite, Path: req.Path, Data: req.Data})
}

func (h *Hub) handleFileDelete(w http.ResponseWriter, r *http.Request) {
	h.fileCall(w, r.PathValue("id"), &proto.Message{Op: proto.OpDelete, Path: r.URL.Query().Get("path")})
}

type fileRenameReq struct {
	Path    string `json:"path"`
	NewPath string `json:"new_path"`
}

func (h *Hub) handleFileRename(w http.ResponseWriter, r *http.Request) {
	var req fileRenameReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrL(w, 400, "请求格式错误", "bad_request", nil)
		return
	}
	h.fileCall(w, r.PathValue("id"), &proto.Message{Op: proto.OpRename, Path: req.Path, NewPath: req.NewPath})
}

type fileMkdirReq struct {
	Path string `json:"path"`
}

func (h *Hub) handleFileMkdir(w http.ResponseWriter, r *http.Request) {
	var req fileMkdirReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrL(w, 400, "请求格式错误", "bad_request", nil)
		return
	}
	h.fileCall(w, r.PathValue("id"), &proto.Message{Op: proto.OpMkdir, Path: req.Path})
}
