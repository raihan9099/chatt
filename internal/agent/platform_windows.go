//go:build windows

package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/UserExistsError/conpty"
	"github.com/Microsoft/go-winio"

	"termirai/internal/proto"
)

// ---- 平台钩子：Windows 用命名管道连接 holder（Win10 1703+ 全支持） ----

func holderAddr(dir, id string) string { return `\\.\pipe\termirai-holder-` + id }

func dialHolder(addr string, timeout time.Duration) (net.Conn, error) {
	return winio.DialPipe(addr, &timeout)
}

// holderExists 通过快速探测连接判断 holder 是否存活（管道无文件可 stat）。
func holderExists(addr string) bool {
	c, err := dialHolder(addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func removeHolderAddr(addr string) {} // 命名管道随 holder 进程消亡，无需清理

// sameShell 判断两个 shell 路径是否相同（Windows 不区分大小写）。
func sameShell(a, b string) bool { return strings.EqualFold(a, b) }

// defaultShell 默认 shell：%SHELL%，否则 powershell.exe（再退 cmd.exe）。
func defaultShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	if p, err := exec.LookPath("powershell.exe"); err == nil {
		return p
	}
	return "cmd.exe"
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
		if r, err := filepath.EvalSymlinks(path); err == nil {
			key = r
		}
		key = strings.ToLower(key)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, proto.ShellInfo{Name: name, Path: path})
	}
	if s := os.Getenv("SHELL"); s != "" {
		name := filepath.Base(s)
		name = strings.TrimSuffix(name, filepath.Ext(name)) // bash.exe -> bash
		add(name, s)
	}
	for _, c := range []struct{ name, bin string }{
		{"PowerShell", "powershell.exe"},
		{"PowerShell 7", "pwsh.exe"},
		{"CMD", "cmd.exe"},
		{"Git Bash", "bash.exe"},
	} {
		if p, err := exec.LookPath(c.bin); err == nil {
			add(c.name, p)
		}
	}
	if len(out) == 0 {
		add("CMD", "cmd.exe")
	}
	return out
}

// winPty 适配 ConPTY 到 ptyIO。
type winPty struct {
	c *conpty.ConPty
}

func (p *winPty) Read(b []byte) (int, error)  { return p.c.Read(b) }
func (p *winPty) Write(b []byte) (int, error) { return p.c.Write(b) }
func (p *winPty) Close() error                { return p.c.Close() }
func (p *winPty) Resize(cols, rows int) error { return p.c.Resize(cols, rows) }
func (p *winPty) Kill() error                 { return p.c.Close() } // 关闭伪控制台即终止 shell
func (p *winPty) Wait() error {
	code, err := p.c.Wait(context.Background())
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("exit code %d", code)
	}
	return nil
}
