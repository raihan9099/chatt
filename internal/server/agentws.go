package server

import (
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"termirai/internal/proto"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// agentConn 一条客户端 agent 的 WebSocket 连接。
type agentConn struct {
	h    *Hub
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *agentConn) send(m *proto.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(m)
}

// handleAgentWS 客户端 agent 接入端点（密钥鉴权在 register 消息中完成）。
func (h *Hub) handleAgentWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ac := &agentConn{h: h, conn: conn}
	defer conn.Close()

	var reg proto.Message
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	if err := conn.ReadJSON(&reg); err != nil || reg.Type != proto.TypeRegister {
		return
	}
	conn.SetReadDeadline(time.Time{})
	if reg.Key != h.cfg.AgentKey {
		ac.send(&proto.Message{Type: proto.TypeRegisterAck, Error: "鉴权密钥错误"})
		h.logf("拒绝注册（密钥错误）: client_id=%s name=%s", reg.ClientID, reg.Name)
		return
	}
	if reg.ClientID == "" {
		ac.send(&proto.Message{Type: proto.TypeRegisterAck, Error: "缺少 client_id"})
		return
	}

	h.store.mu.Lock()
	client, ok := h.store.st.Clients[reg.ClientID]
	if !ok {
		client = &Client{ID: reg.ClientID}
		client.initRuntime()
		h.store.st.Clients[reg.ClientID] = client
	}
	// 顶掉旧连接（同 ID 重复接入）
	if client.ac != nil {
		client.ac.conn.Close()
	}
	client.Name, client.Host, client.Version = reg.Name, reg.Host, reg.Version
	client.Shells = reg.Shells
	client.LastSeen = time.Now().Unix()
	client.Online = true
	client.ac = ac
	h.store.mu.Unlock()
	h.store.markDirty()

	h.reconcile(client, reg.Alive)
	h.store.markDirty()
	h.logf("客户端上线: %s (%s %s)，存活终端 %d", client.Name, client.ID, client.Host, len(reg.Alive))

	ac.send(&proto.Message{Type: proto.TypeRegisterAck, OK: true, ClientID: client.ID})
	h.broadcastClients()

	// 服务端若无某终端的缓冲（如刚重启），向 agent 请求回放
	for _, ti := range reg.Alive {
		if t := client.Terminals[ti.ID]; t != nil && len(t.snapshot()) == 0 {
			ac.send(&proto.Message{Type: proto.TypeTermOpen, TermID: ti.ID, Replay: true})
		}
	}

	defer func() {
		h.store.mu.Lock()
		if client.ac == ac {
			client.Online = false
			client.ac = nil
		}
		h.store.mu.Unlock()
		h.store.markDirty()
		h.logf("客户端离线: %s (%s)", client.Name, client.ID)
		h.broadcastClients()
	}()

	for {
		var m proto.Message
		if err := conn.ReadJSON(&m); err != nil {
			return
		}
		h.handleAgentMessage(client, &m)
	}
}

func (h *Hub) handleAgentMessage(c *Client, m *proto.Message) {
	switch m.Type {
	case proto.TypeTermOutput:
		data, err := base64.StdEncoding.DecodeString(m.Data)
		if err != nil {
			return
		}
		t := c.Terminals[m.TermID]
		if t == nil {
			return
		}
		if m.Replay {
			t.setBufIfEmpty(data)
			h.broadcastTermData(c.ID, m.TermID, data, true)
		} else {
			t.appendBuf(data)
			h.broadcastTermData(c.ID, m.TermID, data, false)
		}
	case proto.TypeTermClosed:
		h.onTermClosed(c, m.TermID, m.Reason, "")
	case proto.TypeTermAck, proto.TypeFileAck:
		c.pmu.Lock()
		ch := c.pending[m.ReqID]
		c.pmu.Unlock()
		if ch != nil {
			select {
			case ch <- m:
			default:
			}
		}
	case proto.TypePong:
		c.LastSeen = time.Now().Unix()
	}
}
