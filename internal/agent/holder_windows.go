//go:build windows

package agent

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/UserExistsError/conpty"
	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// RunHolder 以 ConPTY 托管模式运行：RunHolder([pipeAddr, cols, rows, shell?])。
func RunHolder(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("holder 用法: holder <addr> <cols> <rows> [shell]")
	}
	addr := args[0]
	cols, _ := strconv.Atoi(args[1])
	rows, _ := strconv.Atoi(args[2])
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	if !conpty.IsConPtyAvailable() {
		return fmt.Errorf("当前 Windows 版本不支持 ConPTY（需要 Windows 10 1809+）")
	}
	ln, err := winio.ListenPipe(addr, nil)
	if err != nil {
		return err
	}
	shell := defaultShell()
	if len(args) >= 4 && args[3] != "" {
		shell = args[3]
	}
	cpty, err := conpty.Start(shell,
		conpty.ConPtyDimensions(cols, rows),
		conpty.ConPtyEnv(append(os.Environ(), "TERM=xterm-256color")),
	)
	if err != nil {
		return fmt.Errorf("启动 shell 失败: %w", err)
	}
	log.Printf("[holder] shell %s pid=%d addr=%s", shell, cpty.Pid(), addr)
	return runHolderBase(ln, &winPty{c: cpty})
}

// startHolder 以脱离控制台的方式启动 holder 子进程（agent 退出不影响它）。
// shell 为空则用默认 shell。
func startHolder(addr string, cols, rows int, shell string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "holder", addr, strconv.Itoa(cols), strconv.Itoa(rows), shell)
	// DETACHED_PROCESS：不依附 agent 的控制台，agent 退出后 holder 继续存活
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}
