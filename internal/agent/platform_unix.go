//go:build !windows

package agent

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/creack/pty"

	"termirai/internal/proto"
)

// ---- 平台钩子：unix（Linux/macOS/*BSD）用 unix socket 连接 holder ----

func holderAddr(dir, id string) string { return filepath.Join(dir, id+".sock") }

func dialHolder(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("unix", addr, timeout)
}

func holderExists(addr string) bool {
	_, err := os.Stat(addr)
	return err == nil
}

func removeHolderAddr(addr string) { os.Remove(addr) }

// sameShell 判断两个 shell 路径是否相同（unix 区分大小写）。
func sameShell(a, b string) bool { return a == b }

// defaultShell 默认 shell：$SHELL，否则 /bin/bash（再退 /bin/sh）。
func defaultShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	if _, err := os.Stat("/bin/bash"); err == nil {
		return "/bin/bash"
	}
	return "/bin/sh"
}

// detectShells 探测本机可用 shell 列表（注册时上报，供 Web 端新建终端选择）。
func detectShells() []proto.ShellInfo {
	var out []proto.ShellInfo
	seen := map[string]bool{}
	add := func(name, path string) {
		if path == "" {
			return
		}
		key := path
		if r, err := filepath.EvalSymlinks(path); err == nil { // /bin/bash 与 /usr/bin/bash 常为同一文件
			key = r
		}
		if seen[key] {
			return
		}
		if fi, err := os.Stat(path); err != nil || fi.IsDir() {
			return
		}
		seen[key] = true
		out = append(out, proto.ShellInfo{Name: name, Path: path})
	}
	if s := os.Getenv("SHELL"); s != "" {
		add(filepath.Base(s), s)
	}
	for _, name := range []string{"bash", "zsh", "fish", "sh", "dash", "ksh"} {
		if p, err := exec.LookPath(name); err == nil {
			add(name, p)
		}
	}
	if len(out) == 0 {
		add("sh", "/bin/sh")
	}
	return out
}

// unixPty 适配 creack/pty 到 ptyIO。
type unixPty struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

func (p *unixPty) Read(b []byte) (int, error)  { return p.ptmx.Read(b) }
func (p *unixPty) Write(b []byte) (int, error) { return p.ptmx.Write(b) }
func (p *unixPty) Close() error                { return p.ptmx.Close() }
func (p *unixPty) Wait() error                 { return p.cmd.Wait() }
func (p *unixPty) Resize(cols, rows int) error {
	return pty.Setsize(p.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
func (p *unixPty) Kill() error {
	if p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}
