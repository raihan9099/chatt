// Command termirai-server 是 Termirai 服务端：Web 控制台、客户端管理、AI Agent。
package main

import (
	"log"

	"termirai/internal/server"
)

func main() {
	if err := server.Run(); err != nil {
		log.Fatal(err)
	}
}
