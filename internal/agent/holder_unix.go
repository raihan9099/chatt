//go:build !windows

package agent

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/creack/pty"
)

// RunHolder 以 PTY 托管模式运行：RunHolder([sockPath, cols, rows, shell?])。
func RunHolder(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("holder 用法: holder <addr> <cols> <rows> [shell]")
	}
	sockPath := args[0]
	cols, _ := strconv.Atoi(args[1])
	rows, _ := strconv.Atoi(args[2])
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	shell := defaultShell()
	if len(args) >= 4 && args[3] != "" {
		shell = args[3]
	}
	os.Remove(sockPath)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return err
	}
	defer os.Remove(sockPath)

	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return err
	}
	log.Printf("[holder] shell %s pid=%d addr=%s", shell, cmd.Process.Pid, sockPath)
	return runHolderBase(ln, &unixPty{ptmx: ptmx, cmd: cmd})
}

// startHolder 以脱离会话的方式启动 holder 子进程（agent 退出不影响它）。
// shell 为空则用默认 shell。
func startHolder(addr string, cols, rows int, shell string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(addr), 0o755)
	cmd := exec.Command(exe, "holder", addr, strconv.Itoa(cols), strconv.Itoa(rows), shell)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	devNull, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if devNull != nil {
		cmd.Stdin = devNull
		defer devNull.Close()
	}
	return cmd.Start()
}
