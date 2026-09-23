package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"termirai/internal/proto"
)

// state 客户端本地持久化：固定 client ID + 终端元数据。
type state struct {
	mu        sync.Mutex
	path      string
	ClientID  string            `json:"client_id"`
	Terminals map[string]string `json:"terminals"` // termID -> title
}

func loadState(dir string) (*state, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &state{path: filepath.Join(dir, "client.json"), Terminals: map[string]string{}}
	if b, err := os.ReadFile(s.path); err == nil {
		json.Unmarshal(b, s)
	}
	if s.ClientID == "" {
		s.ClientID = randID()
	}
	if s.Terminals == nil {
		s.Terminals = map[string]string{}
	}
	s.save()
	return s, nil
}

func (s *state) save() {
	b, _ := json.MarshalIndent(s, "", "  ")
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		os.Rename(tmp, s.path)
	}
}

func (s *state) addTerm(id, title string) {
	s.mu.Lock()
	s.Terminals[id] = title
	s.mu.Unlock()
	s.save()
}

func (s *state) delTerm(id string) {
	s.mu.Lock()
	delete(s.Terminals, id)
	s.mu.Unlock()
	s.save()
}

// renameTerm 更新已存在终端的标题并落盘。
func (s *state) renameTerm(id, title string) {
	s.mu.Lock()
	if _, ok := s.Terminals[id]; !ok {
		s.mu.Unlock()
		return
	}
	s.Terminals[id] = title
	s.mu.Unlock()
	s.save()
}

func (s *state) termInfos() []proto.TermInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]proto.TermInfo, 0, len(s.Terminals))
	for id, title := range s.Terminals {
		out = append(out, proto.TermInfo{ID: id, Title: title})
	}
	return out
}
