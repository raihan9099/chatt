package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// persistedState 服务端落盘状态（无数据库，JSON 文件）。
type persistedState struct {
	JWTSecret string                  `json:"jwt_secret"`
	Clients   map[string]*Client      `json:"clients"`
	AISessions map[string]*AISession  `json:"ai_sessions"`
	UIState   json.RawMessage         `json:"ui_state"`
	Lockout   *lockoutState           `json:"lockout"`
	AI        *aiSettings             `json:"ai,omitempty"` // Web 端配置的 AI 参数（覆盖 .env）
}

// aiSettings Web 端可配的 AI 参数；空字段表示跟随 .env。
type aiSettings struct {
	BaseURL     string `json:"base_url,omitempty"`
	APIKey      string `json:"api_key,omitempty"`
	Model       string `json:"model,omitempty"`
	DefaultMode string `json:"default_mode,omitempty"`
}

type lockoutState struct {
	Fails       int   `json:"fails"`
	LockedUntil int64 `json:"locked_until"` // unix 秒
}

// store 负责加载/保存，写操作去抖落盘。
// 约定：markDirty 不得在持有 mu 时调用（mu 保护状态，wmu 保护落盘调度）。
type store struct {
	mu   sync.Mutex // 保护 st
	path string
	st   *persistedState

	wmu   sync.Mutex // 保护 dirty/timer
	dirty bool
	timer *time.Timer
}

func openStore(dir string) (*store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &store{path: filepath.Join(dir, "state.json")}
	s.st = &persistedState{
		Clients:    map[string]*Client{},
		AISessions: map[string]*AISession{},
		Lockout:    &lockoutState{},
	}
	if b, err := os.ReadFile(s.path); err == nil {
		json.Unmarshal(b, s.st)
	}
	if s.st.Clients == nil {
		s.st.Clients = map[string]*Client{}
	}
	if s.st.AISessions == nil {
		s.st.AISessions = map[string]*AISession{}
	}
	if s.st.Lockout == nil {
		s.st.Lockout = &lockoutState{}
	}
	if s.st.JWTSecret == "" {
		s.st.JWTSecret = randHex(32)
		s.saveNow()
	}
	return s, nil
}

// markDirty 去抖 500ms 落盘。不得在持有 mu 时调用。
func (s *store) markDirty() {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	s.dirty = true
	if s.timer == nil {
		s.timer = time.AfterFunc(500*time.Millisecond, s.saveNow)
	}
}

func (s *store) saveNow() {
	s.wmu.Lock()
	if !s.dirty {
		s.wmu.Unlock()
		return
	}
	s.dirty = false
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.wmu.Unlock()
	s.mu.Lock()
	b, err := json.Marshal(s.st)
	s.mu.Unlock()
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		os.Rename(tmp, s.path)
	}
}

// Flush 退出前强制落盘。
func (s *store) Flush() {
	s.wmu.Lock()
	s.dirty = true
	s.wmu.Unlock()
	s.saveNow()
}
