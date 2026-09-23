package agent

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

// holder 帧格式：4 字节大端长度 + 1 字节 kind + payload（平台无关）。
// holder 是独立于 agent 主进程的 PTY 托管进程：agent 崩溃/重启后 holder 与其 shell 仍然存活，
// agent 重启后重新连接（unix socket / Windows 命名管道）即可恢复终端现场。
const (
	frameData       byte = 0x01 // 双向：原始终端字节流（holder->agent 为实时输出）
	frameResize     byte = 0x02 // agent->holder："cols,rows"
	frameKill       byte = 0x03 // agent->holder：销毁终端
	frameExit       byte = 0x04 // holder->agent：shell 已退出，payload 为原因
	frameReplayReq  byte = 0x05 // agent->holder：请求重放回放缓冲
	frameReplayData byte = 0x06 // holder->agent：回放数据（新连接或响应 0x05）
)

const scrollbackCap = 256 * 1024 // 环形回放缓冲区上限

// ptyIO 平台抽象的伪终端（unix: creack/pty；windows: ConPTY）。
type ptyIO interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
	Kill() error  // 杀掉 shell 进程
	Wait() error  // 阻塞至 shell 退出；非正常退出返回错误
}

func writeFrame(c net.Conn, kind byte, payload []byte) error {
	hdr := make([]byte, 5)
	binary.BigEndian.PutUint32(hdr[:4], uint32(len(payload)))
	hdr[4] = kind
	if _, err := c.Write(hdr); err != nil {
		return err
	}
	_, err := c.Write(payload)
	return err
}

type holderState struct {
	ptmx   ptyIO
	mu     sync.Mutex
	conn   net.Conn // 当前唯一控制连接，后连上的顶掉旧的
	buf    []byte   // 环形回放缓冲
	exited bool
}

func (h *holderState) pump() {
	tmp := make([]byte, 32*1024)
	for {
		n, err := h.ptmx.Read(tmp)
		if n > 0 {
			chunk := tmp[:n]
			h.mu.Lock()
			h.buf = append(h.buf, chunk...)
			if len(h.buf) > scrollbackCap {
				h.buf = h.buf[len(h.buf)-scrollbackCap:]
			}
			c := h.conn
			h.mu.Unlock()
			if c != nil {
				writeFrame(c, frameData, chunk)
			}
		}
		if err != nil {
			return
		}
	}
}

func (h *holderState) serve(c net.Conn) {
	h.mu.Lock()
	old := h.conn
	h.conn = c
	replay := append([]byte(nil), h.buf...)
	exited := h.exited
	h.mu.Unlock()
	if old != nil {
		old.Close()
	}
	// 新连接先收到全量回放（以回放帧发送，agent 转发时标记 replay）
	for len(replay) > 0 {
		n := len(replay)
		if n > 32*1024 {
			n = 32 * 1024
		}
		if writeFrame(c, frameReplayData, replay[:n]) != nil {
			break
		}
		replay = replay[n:]
	}
	if exited {
		writeFrame(c, frameExit, []byte("exited"))
	}
	defer func() {
		h.mu.Lock()
		if h.conn == c {
			h.conn = nil
		}
		h.mu.Unlock()
		c.Close()
	}()
	hdr := make([]byte, 5)
	for {
		if _, err := io.ReadFull(c, hdr); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(hdr[:4])
		if n > 4*1024*1024 {
			return
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(c, payload); err != nil {
			return
		}
		switch hdr[4] {
		case frameData:
			h.ptmx.Write(payload)
		case frameResize:
			var cols, rows int
			fmt.Sscanf(string(payload), "%d,%d", &cols, &rows)
			if cols > 0 && rows > 0 {
				h.ptmx.Resize(cols, rows)
			}
		case frameKill:
			h.ptmx.Kill()
			return
		case frameReplayReq:
			h.mu.Lock()
			buf := append([]byte(nil), h.buf...)
			h.mu.Unlock()
			for len(buf) > 0 {
				n := len(buf)
				if n > 32*1024 {
					n = 32 * 1024
				}
				if writeFrame(c, frameReplayData, buf[:n]) != nil {
					return
				}
				buf = buf[n:]
			}
		}
	}
}

// runHolderBase 是各平台 RunHolder 的共享主体：监听 + 服务循环。
func runHolderBase(ln net.Listener, ptmx ptyIO) error {
	h := &holderState{ptmx: ptmx}
	// 持续把 PTY 输出写入环形缓冲 + 当前连接
	go h.pump()
	// shell 退出后向当前连接发退出事件
	go func() {
		err := ptmx.Wait()
		reason := "exited"
		if err != nil {
			reason = err.Error()
		}
		h.mu.Lock()
		h.exited = true
		c := h.conn
		h.mu.Unlock()
		if c != nil {
			writeFrame(c, frameExit, []byte(reason))
		}
	}()
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go h.serve(c)
	}
}
