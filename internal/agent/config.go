package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config 客户端配置，全部来自 .env / 环境变量。
type Config struct {
	ServerURL string // 如 ws://host:8080 或 wss://host
	Key       string // 注册密钥
	Name      string // 客户端名称，默认主机名
	DataDir   string // 本地状态目录（client.json、holder socket）
	Shell     string // 终端 shell，默认 /bin/bash
}

func loadEnvFile(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

// LoadConfig 读取 .env 与环境变量。
func LoadConfig() (*Config, error) {
	loadEnvFile(".env")
	c := &Config{
		ServerURL: os.Getenv("SERVER_URL"),
		Key:       os.Getenv("AGENT_KEY"),
		Name:      os.Getenv("AGENT_NAME"),
		DataDir:   os.Getenv("DATA_DIR"),
		Shell:     os.Getenv("SHELL"),
	}
	if c.ServerURL == "" || c.Key == "" {
		return nil, fmt.Errorf("必须在 .env 配置 SERVER_URL 和 AGENT_KEY")
	}
	if !strings.HasPrefix(c.ServerURL, "ws://") && !strings.HasPrefix(c.ServerURL, "wss://") {
		c.ServerURL = "ws://" + c.ServerURL
	}
	if c.Name == "" {
		c.Name, _ = os.Hostname()
	}
	if c.DataDir == "" {
		exe, _ := os.Executable()
		c.DataDir = filepath.Join(filepath.Dir(exe), "termirai-data")
	}
	if c.Shell == "" {
		c.Shell = "/bin/bash"
	}
	return c, nil
}
