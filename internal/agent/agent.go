package agent

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"termirai/internal/proto"
)

const version = "0.1.0"

// Run 启动 agent 主循环：连接服务端、注册、断线重连。
func Run() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	st, err := loadState(cfg.DataDir)
	if err != nil {
		return err
	}
	tm := newTermManager(cfg.DataDir, st)
	tm.recover()

	a := &agentConn{cfg: cfg, st: st, tm: tm}
	tm.onOutput = a.onTermOutput
	tm.onClosed = a.onTermClosed

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("[agent] 收到退出信号（终端由 holder 托管，不受影响）")
		os.Exit(0)
	}()

	backoff := time.Second
	for {
		err := a.connectOnce()
		if err != nil {
			log.Printf("[agent] 连接断开: %v，%v 后重连", err, backoff)
		}
		select {
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
		if err == nil {
			backoff = time.Second
		}
	}
}

type agentConn struct {
	cfg *Config
	st  *state
	tm  *termManager

	mu   sync.Mutex
	conn *websocket.Conn
}

func (a *agentConn) send(m *proto.Message) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn == nil {
		return nil // 掉线期间丢弃输出；reattach 时 holder 会回放
	}
	return a.conn.WriteJSON(m)
}

func (a *agentConn) onTermOutput(termID string, data []byte, replay bool) {
	a.send(&proto.Message{
		Type:   proto.TypeTermOutput,
		TermID: termID,
		Data:   base64.StdEncoding.EncodeToString(data),
		Replay: replay,
	})
}

func (a *agentConn) onTermClosed(termID, reason string) {
	a.send(&proto.Message{Type: proto.TypeTermClosed, TermID: termID, Reason: reason})
}

func (a *agentConn) connectOnce() error {
	u, err := url.Parse(a.cfg.ServerURL)
	if err != nil {
		return err
	}
	u.Path = "/api/v1/agent/ws"
	host, _ := os.Hostname()
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.conn = conn
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.conn = nil
		a.mu.Unlock()
		conn.Close()
	}()

	// 注册：携带密钥、固定 ID、存活终端列表
	reg := &proto.Message{
		Type:     proto.TypeRegister,
		Key:      a.cfg.Key,
		ClientID: a.st.ClientID,
		Name:     a.cfg.Name,
		Host:     host + " (" + runtime.GOOS + "/" + runtime.GOARCH + ")",
		Version:  version,
		Alive:    a.tm.aliveInfos(),
		Shells:   a.tm.shells,
	}
	if err := conn.WriteJSON(reg); err != nil {
		return err
	}
	var ack proto.Message
	if err := conn.ReadJSON(&ack); err != nil {
		return err
	}
	if ack.Type != proto.TypeRegisterAck || !ack.OK {
		log.Printf("[agent] 注册被拒绝: %s", ack.Error)
		time.Sleep(10 * time.Second) // 密钥错误时避免疯狂重连
		return &fatalError{msg: ack.Error}
	}
	log.Printf("[agent] 已注册到服务端 %s (client_id=%s, 存活终端 %d 个)", u.Host, a.st.ClientID, len(reg.Alive))

	// 心跳
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for range t.C {
			if err := a.send(&proto.Message{Type: proto.TypePong}); err != nil {
				return
			}
		}
	}()

	for {
		var m proto.Message
		if err := conn.ReadJSON(&m); err != nil {
			return err
		}
		a.dispatch(&m)
	}
}

type fatalError struct{ msg string }

func (e *fatalError) Error() string { return e.msg }

func (a *agentConn) dispatch(m *proto.Message) {
	switch m.Type {
	case proto.TypeTermOpen:
		t, err := a.tm.open(m.TermID, m.Name, m.Cols, m.Rows, m.Shell)
		ack := &proto.Message{Type: proto.TypeTermAck, ReqID: m.ReqID}
		if err != nil {
			ack.Error = err.Error()
			if code, params := errCodeOf(err); code != "" {
				ack.ErrCode = code
				ack.ErrParams = params
			}
		} else {
			ack.OK = true
			ack.TermID = t.ID
			if m.Replay {
				a.tm.requestReplay(t.ID) // 服务端请求回放（如重启后恢复）
			}
		}
		a.send(ack)
	case proto.TypeTermInput:
		data, err := base64.StdEncoding.DecodeString(m.Data)
		if err == nil {
			a.tm.input(m.TermID, data)
		}
	case proto.TypeTermSize:
		a.tm.resize(m.TermID, m.Cols, m.Rows)
	case proto.TypeTermClose:
		a.tm.close(m.TermID)
	case proto.TypeTermRename:
		a.tm.rename(m.TermID, m.Name)
	case proto.TypeFileOp:
		a.send(handleFileOp(m))
	case proto.TypePing:
		// 服务端心跳，pong 由 ticker 发
	default:
		b, _ := json.Marshal(m)
		log.Printf("[agent] 未知消息: %s", b)
	}
}
