package server

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"termirai/web"
)

// Run 启动服务端。
func Run() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	st, err := openStore(cfg.DataDir)
	if err != nil {
		return err
	}
	hub := NewHub(cfg, st)

	// 服务端重启恢复：运行中的 AI 任务标记为已中断
	for _, s := range st.st.AISessions {
		if s.Status == "running" {
			s.Status = "interrupted"
		}
	}
	st.markDirty()

	mux := http.NewServeMux()

	// 公开端点
	mux.HandleFunc("POST /api/v1/login", hub.handleLogin)
	mux.HandleFunc("GET /api/v1/captcha", hub.handleCaptcha)
	// agent 接入（密钥鉴权）
	mux.HandleFunc("/api/v1/agent/ws", hub.handleAgentWS)
	// 浏览器 WS（token 鉴权）
	mux.HandleFunc("/api/v1/ws", hub.handleBrowserWS)

	// 受保护 REST
	api := http.NewServeMux()
	api.HandleFunc("GET /api/v1/clients", hub.handleClients)
	api.HandleFunc("DELETE /api/v1/clients/{id}", hub.handleDeleteClient)
	api.HandleFunc("POST /api/v1/clients/{id}/alias", hub.handleSetAlias)
	api.HandleFunc("GET /api/v1/ai/config", hub.handleAIConfigGet)
	api.HandleFunc("POST /api/v1/ai/config", hub.handleAIConfigPost)
	api.HandleFunc("POST /api/v1/ai/test", hub.handleAITest)
	api.HandleFunc("GET /api/v1/clients/{id}/files", hub.handleFileList)
	api.HandleFunc("GET /api/v1/clients/{id}/file", hub.handleFileRead)
	api.HandleFunc("POST /api/v1/clients/{id}/file", hub.handleFileWrite)
	api.HandleFunc("DELETE /api/v1/clients/{id}/file", hub.handleFileDelete)
	api.HandleFunc("POST /api/v1/clients/{id}/file/rename", hub.handleFileRename)
	api.HandleFunc("POST /api/v1/clients/{id}/file/mkdir", hub.handleFileMkdir)
	mux.Handle("/api/v1/clients", hub.authMiddleware(api))
	mux.Handle("/api/v1/clients/", hub.authMiddleware(api))
	mux.Handle("/api/v1/ai/", hub.authMiddleware(api))

	// 前端静态资源（embed，可选外部目录覆盖）
	var staticFS http.FileSystem
	if cfg.StaticDir != "" {
		staticFS = http.FS(os.DirFS(cfg.StaticDir))
	} else if sub, err := fs.Sub(web.Dist, "dist"); err == nil {
		staticFS = http.FS(sub)
	}
	if staticFS != nil {
		fileServer := http.FileServer(staticFS)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// SPA 路由回退到 index.html
			p := strings.TrimPrefix(r.URL.Path, "/")
			if p != "" {
				if f, err := staticFS.Open(p); err == nil {
					f.Close()
					fileServer.ServeHTTP(w, r)
					return
				}
			}
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
		})
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "Termirai server 运行中（未找到前端产物，请先构建 web/）")
		})
	}

	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: mux}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("[server] 退出前落盘状态…")
		st.Flush()
		os.Exit(0)
	}()

	log.Printf("[server] Termirai 服务端启动: http://0.0.0.0:%d (数据目录 %s)", cfg.Port, cfg.DataDir)
	return srv.ListenAndServe()
}
