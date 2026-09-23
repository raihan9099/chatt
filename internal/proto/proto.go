// Package proto 定义服务端与客户端 agent 之间的 WebSocket 消息协议。
// 所有消息均为 JSON，带 type 字段。二进制数据（终端 IO、文件内容）一律 base64。
package proto

// 消息类型
const (
	// agent -> server
	TypeRegister    = "register"     // 注册（携带密钥与存活终端列表）
	TypeRegisterAck = "register_ack" // 注册应答
	TypeTermOutput  = "term_output"  // 终端输出流
	TypeTermClosed  = "term_closed"  // 终端进程退出（shell 结束）
	TypeTermAck     = "term_ack"     // term_open/term_close 应答
	TypeFileAck     = "file_ack"     // 文件操作应答
	TypePong        = "pong"

	// server -> agent
	TypeTermOpen   = "term_open"   // 打开/reattach 终端（不存在则创建）
	TypeTermInput  = "term_input"  // 终端输入
	TypeTermSize   = "term_resize"
	TypeTermClose  = "term_close"
	TypeTermRename = "term_rename" // 重命名终端（Name 为新标题）
	TypeFileOp     = "file_op"
	TypePing       = "ping"
)

// Message 是线上统一信封。按需使用字段。
type Message struct {
	Type string `json:"type"`

	// 注册
	Key      string      `json:"key,omitempty"`
	ClientID string      `json:"client_id,omitempty"`
	Name     string      `json:"name,omitempty"`
	Host     string      `json:"host,omitempty"`
	Version  string      `json:"version,omitempty"`
	Alive    []TermInfo  `json:"alive,omitempty"`  // 注册时上报的存活终端
	Shells   []ShellInfo `json:"shells,omitempty"` // 注册时上报的本机可用 shell 列表

	// 相关应答
	ReqID     string         `json:"req_id,omitempty"`
	OK        bool           `json:"ok,omitempty"`
	Error     string         `json:"error,omitempty"`                          // 人读错误文本（中文兜底）
	ErrCode   string         `json:"err_code,omitempty"`                       // 稳定错误码，前端/调用方按码本地化
	ErrParams map[string]any `json:"err_params,omitempty"`                     // 错误码插值参数

	// 终端
	TermID string `json:"term_id,omitempty"`
	Cols   int    `json:"cols,omitempty"`
	Rows   int    `json:"rows,omitempty"`
	Shell  string `json:"shell,omitempty"` // term_open 指定的 shell 路径（空=默认）
	Data   string `json:"data,omitempty"`  // base64
	Reason string `json:"reason,omitempty"`
	Replay bool   `json:"replay,omitempty"` // term_output 是否为历史回放

	// 文件操作
	Op      string     `json:"op,omitempty"` // list/read/write/delete/rename/mkdir/stat
	Path    string     `json:"path,omitempty"`
	NewPath string     `json:"new_path,omitempty"`
	Entries []FileInfo `json:"entries,omitempty"`
}

// TermInfo 终端元数据。
type TermInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"created_at"`
}

// ShellInfo 一个可用的 shell（新建终端时可选）。
type ShellInfo struct {
	Name string `json:"name"` // 展示名，如 bash / PowerShell
	Path string `json:"path"` // 可执行文件路径
}

// FileInfo 文件条目。
type FileInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size"`
	Mode  string `json:"mode"`
	Mtime int64  `json:"mtime"`
}

// 文件操作 op 取值
const (
	OpList   = "list"
	OpRead   = "read"
	OpWrite  = "write"
	OpDelete = "delete"
	OpRename = "rename"
	OpMkdir  = "mkdir"
	OpStat   = "stat"
)
