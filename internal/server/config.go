package server

import (
	"fmt"
	"os"
	"strconv"
)

// Config 服务端配置，全部来自 .env / 环境变量。
type Config struct {
	Port       int
	AdminUser  string
	AdminPass  string
	AgentKey   string
	DataDir    string
	AIBaseURL  string
	AIAPIKey   string
	AIModel    string
	AIDefMode  string // auto | approval
	StaticDir  string // 前端产物目录；为空则用内嵌
}

func LoadConfig() (*Config, error) {
	loadEnvFile(".env")
	port, _ := strconv.Atoi(getenv("PORT", "8080"))
	c := &Config{
		Port:      port,
		AdminUser: getenv("ADMIN_USER", "admin"),
		AdminPass: os.Getenv("ADMIN_PASSWORD"),
		AgentKey:  os.Getenv("AGENT_KEY"),
		DataDir:   getenv("DATA_DIR", "./termirai-data"),
		AIBaseURL: os.Getenv("AI_BASE_URL"),
		AIAPIKey:  os.Getenv("AI_API_KEY"),
		AIModel:   os.Getenv("AI_MODEL"),
		AIDefMode: getenv("AI_DEFAULT_MODE", "approval"),
		StaticDir: os.Getenv("STATIC_DIR"),
	}
	if c.AdminPass == "" || c.AgentKey == "" {
		return nil, fmt.Errorf("必须在 .env 配置 ADMIN_PASSWORD 和 AGENT_KEY")
	}
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
