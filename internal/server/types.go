package server

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

	"github.com/gorilla/websocket"

	"termirai/internal/proto"
)

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// TermState 服务端视角的终端（注册表持久化，缓冲区为运行时状态）。
type TermState struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"created_at"`
	Closed    bool   `json:"closed"` // shell 已退出或 holder 消亡

	bufMu sync.Mutex
	buf   []byte // 服务端侧滚动缓冲（重启后为空，从 agent 回放补齐）
}

const termBufCap = 256 * 1024

func (t *TermState) appendBuf(p []byte) {
	t.bufMu.Lock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > termBufCap {
		t.buf = t.buf[len(t.buf)-termBufCap:]
	}
	t.bufMu.Unlock()
}

func (t *TermState) setBufIfEmpty(p []byte) {
	t.bufMu.Lock()
	if len(t.buf) == 0 {
		t.buf = append([]byte(nil), p...)
		if len(t.buf) > termBufCap {
			t.buf = t.buf[len(t.buf)-termBufCap:]
		}
	}
	t.bufMu.Unlock()
}

func (t *TermState) snapshot() []byte {
	t.bufMu.Lock()
	defer t.bufMu.Unlock()
	return append([]byte(nil), t.buf...)
}

// Client 已注册客户端。
type Client struct {
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Alias     string                `json:"alias,omitempty"` // 用户起的别名（展示优先）
	Host      string                `json:"host"`
	Version   string                `json:"version"`
	LastSeen  int64                 `json:"last_seen"`
	Shells    []proto.ShellInfo     `json:"shells,omitempty"` // 客户端上报的可用 shell（新建终端可选）
	Terminals map[string]*TermState `json:"terminals"`

	Online bool `json:"online"` // 仅运行时展示，不落盘（重启后由重连置真）

	ac      *agentConn                 `json:"-"`
	pending map[string]chan *proto.Message `json:"-"`
	pmu     sync.Mutex                 `json:"-"`
}

func (c *Client) initRuntime() {
	c.Online = false
	c.pending = map[string]chan *proto.Message{}
	if c.Terminals == nil {
		c.Terminals = map[string]*TermState{}
	}
}

// browserConn 一个浏览器 WebSocket 连接。
type browserConn struct {
	conn *websocket.Conn
	mu   sync.Mutex // 写锁
	hub  *Hub

	subsMu sync.Mutex
	subs   map[string]bool // "clientID/termID"
	aiSubs map[string]bool // sessionID
	closed bool
}

func (b *browserConn) send(v any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.conn.WriteJSON(v)
}

func subKey(clientID, termID string) string { return clientID + "/" + termID }
