// Command termirai-agent 是运行在受管 Linux/Unix 机器上的客户端 agent。
// 特殊用法：termirai-agent holder <sock> <cols> <rows> —— PTY 托管子进程模式（内部使用）。
package main

import (
	"log"
	"os"

	"termirai/internal/agent"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "holder" {
		if err := agent.RunHolder(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := agent.Run(); err != nil {
		log.Fatal(err)
	}
}
