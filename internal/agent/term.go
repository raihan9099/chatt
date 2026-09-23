package agent

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"path/filepath"
	"sync"
	"time"

	"termirai/internal/proto"
)

// Term 一个托管终端：到 holder 进程的一条控制连接（unix socket / Windows 命名管道）。
type Term struct {
	ID    string
	Title string

	mu     sync.Mutex
	conn   net.Conn
	closed bool
}

// termManager 管理本机所有终端（平台无关；平台差异见 platform_*.go）。
type termManager struct {
	dir    string // holder 地址目录（unix socket 文件目录；Windows 仅作命名空间标识）
	st     *state
	shells []proto.ShellInfo // 本机可用 shell（启动时探测，注册时上报）
	mu     sync.Mutex
	terms  map[string]*Term

	onOutput func(termID string, data []byte, replay bool)
	onClosed func(termID string, reason string)
}

func newTermManager(dataDir string, st *state) *termManager {
	m := &termManager{dir: filepath.Join(dataDir, "holders"), st: st, terms: map[string]*Term{}}
	m.shells = detectShells()
	log.Printf("[agent] 可用 shell: %v", m.shells)
	return m
}

// shellAllowed 校验用户指定的 shell 是否在探测到的可用列表中。
func (m *termManager) shellAllowed(shell string) bool {
	if shell == "" {
		return true
	}
	for _, s := range m.shells {
		if sameShell(s.Path, shell) {
			return true
		}
	}
	return false
}

// recover 在 agent 启动时调用：对状态里登记的每个终端，若 holder 仍存活则重新接入。
func (m *termManager) recover() {
	for _, ti := range m.st.termInfos() {
		if !holderExists(holderAddr(m.dir, ti.ID)) {
			m.st.delTerm(ti.ID) // holder 已随机器重启消亡
			continue
		}
		if _, err := m.open(ti.ID, ti.Title, 0, 0, ""); err != nil {
			log.Printf("[agent] 恢复终端 %s 失败: %v", ti.ID, err)
			m.st.delTerm(ti.ID)
		} else {
			log.Printf("[agent] 已恢复终端 %s (%s)", ti.ID, ti.Title)
		}
	}
}

// open 打开或 reattach 终端。id 为空则新建；shell 仅新建时生效（空=默认），必须在探测到的可用列表中。
func (m *termManager) open(id, title string, cols, rows int, shell string) (*Term, error) {
	m.mu.Lock()
	if t, ok := m.terms[id]; ok && !t.closed {
		m.mu.Unlock()
		return t, nil
	}
	m.mu.Unlock()

	if !m.shellAllowed(shell) {
		return nil, &codedError{code: "shell_not_allowed", params: map[string]any{"shell": shell}, msg: fmt.Sprintf("shell 不在可用列表: %s", shell)}
	}
	if id == "" {
		id = randID()
	}
	addr := holderAddr(m.dir, id)
	// 已有存活 holder 则直接接入，否则启动新 holder
	c, err := dialHolder(addr, 2*time.Second)
	if err != nil {
		if err := startHolder(addr, cols, rows, shell); err != nil {
			return nil, err
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			c, err = dialHolder(addr, time.Second)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				return nil, &codedError{code: "holder_timeout", msg: "等待 holder 就绪超时"}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	if title == "" {
		title = "terminal"
	}
	t := &Term{ID: id, Title: title, conn: c}
	m.mu.Lock()
	m.terms[id] = t
	m.mu.Unlock()
	m.st.addTerm(id, title)
	go m.readLoop(t)
	return t, nil
}

// readLoop 读 holder 帧：数据帧转发给服务端；holder 报告退出则关闭终端。
func (m *termManager) readLoop(t *Term) {
	hdr := make([]byte, 5)
	for {
		if _, err := io.ReadFull(t.conn, hdr); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(hdr[:4])
		payload := make([]byte, n)
		if _, err := io.ReadFull(t.conn, payload); err != nil {
			return
		}
		switch hdr[4] {
		case frameData:
			if m.onOutput != nil {
				m.onOutput(t.ID, payload, false)
			}
		case frameReplayData:
			if m.onOutput != nil {
				m.onOutput(t.ID, payload, true)
			}
		case frameExit:
			t.closed = true
			m.mu.Lock()
			delete(m.terms, t.ID)
			m.mu.Unlock()
			m.st.delTerm(t.ID)
			if m.onClosed != nil {
				m.onClosed(t.ID, string(payload))
			}
			return
		}
	}
}

func (t *Term) writeFrame(kind byte, payload []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return &codedError{code: "term_closed", msg: "终端已关闭"}
	}
	hdr := make([]byte, 5)
	binary.BigEndian.PutUint32(hdr[:4], uint32(len(payload)))
	hdr[4] = kind
	if _, err := t.conn.Write(hdr); err != nil {
		return err
	}
	_, err := t.conn.Write(payload)
	return err
}

func (m *termManager) input(id string, data []byte) error {
	t := m.get(id)
	if t == nil {
		return &codedError{code: "term_not_found", params: map[string]any{"id": id}, msg: fmt.Sprintf("终端不存在: %s", id)}
	}
	return t.writeFrame(frameData, data)
}

// requestReplay 让 holder 重发回放缓冲（服务端重启后恢复现场用）。
func (m *termManager) requestReplay(id string) {
	if t := m.get(id); t != nil {
		t.writeFrame(frameReplayReq, nil)
	}
}

func (m *termManager) resize(id string, cols, rows int) error {
	t := m.get(id)
	if t == nil {
		return &codedError{code: "term_not_found", params: map[string]any{"id": id}, msg: fmt.Sprintf("终端不存在: %s", id)}
	}
	return t.writeFrame(frameResize, []byte(fmt.Sprintf("%d,%d", cols, rows)))
}

func (m *termManager) close(id string) error {
	t := m.get(id)
	if t == nil {
		return nil
	}
	t.closed = true
	t.writeFrame(frameKill, nil)
	t.mu.Lock()
	if t.conn != nil {
		t.conn.Close()
	}
	t.mu.Unlock()
	m.mu.Lock()
	delete(m.terms, id)
	m.mu.Unlock()
	m.st.delTerm(id)
	removeHolderAddr(holderAddr(m.dir, id))
	return nil
}

func (m *termManager) get(id string) *Term {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.terms[id]
}

// rename 更新终端标题并持久化（holder 不关心标题，纯元数据）。
func (m *termManager) rename(id, title string) {
	if t := m.get(id); t != nil {
		t.Title = title
	}
	m.st.renameTerm(id, title)
}

func (m *termManager) aliveInfos() []proto.TermInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]proto.TermInfo, 0, len(m.terms))
	for id, t := range m.terms {
		if !t.closed {
			out = append(out, proto.TermInfo{ID: id, Title: t.Title})
		}
	}
	return out
}
