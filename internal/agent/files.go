package agent

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"termirai/internal/proto"
)

const maxFileSize = 8 << 20 // 读/写文件上限 8MB（Monaco 文本编辑场景）

// handleFileOp 执行服务端下发的文件操作并返回应答消息。
func handleFileOp(m *proto.Message) *proto.Message {
	ack := &proto.Message{Type: proto.TypeFileAck, ReqID: m.ReqID}
	switch m.Op {
	case proto.OpList:
		path := m.Path
		if path == "" {
			path = "/"
		}
		ents, err := os.ReadDir(path)
		if err != nil {
			ack.Error = err.Error()
			return ack
		}
		for _, e := range ents {
			fi, err := e.Info()
			if err != nil {
				continue
			}
			ack.Entries = append(ack.Entries, proto.FileInfo{
				Name:  e.Name(),
				Path:  filepath.Join(path, e.Name()),
				Dir:   e.IsDir(),
				Size:  fi.Size(),
				Mode:  fi.Mode().String(),
				Mtime: fi.ModTime().Unix(),
			})
		}
		sort.Slice(ack.Entries, func(i, j int) bool {
			if ack.Entries[i].Dir != ack.Entries[j].Dir {
				return ack.Entries[i].Dir
			}
			return ack.Entries[i].Name < ack.Entries[j].Name
		})
		ack.OK = true
	case proto.OpRead:
		fi, err := os.Stat(m.Path)
		if err != nil {
			ack.Error = err.Error()
			return ack
		}
		if fi.Size() > maxFileSize {
			ack.Error = fmt.Sprintf("文件超过 %dMB 上限", maxFileSize>>20)
			ack.ErrCode = "file_too_large"
			ack.ErrParams = map[string]any{"max": maxFileSize >> 20}
			return ack
		}
		b, err := os.ReadFile(m.Path)
		if err != nil {
			ack.Error = err.Error()
			return ack
		}
		ack.Data = base64.StdEncoding.EncodeToString(b)
		ack.OK = true
	case proto.OpWrite:
		b, err := base64.StdEncoding.DecodeString(m.Data)
		if err != nil {
			ack.Error = "base64 解码失败"
			ack.ErrCode = "bad_base64"
			return ack
		}
		if len(b) > maxFileSize {
			ack.Error = fmt.Sprintf("文件超过 %dMB 上限", maxFileSize>>20)
			ack.ErrCode = "file_too_large"
			ack.ErrParams = map[string]any{"max": maxFileSize >> 20}
			return ack
		}
		if err := os.WriteFile(m.Path, b, 0o644); err != nil {
			ack.Error = err.Error()
			return ack
		}
		ack.OK = true
	case proto.OpDelete:
		if err := os.RemoveAll(m.Path); err != nil {
			ack.Error = err.Error()
			return ack
		}
		ack.OK = true
	case proto.OpRename:
		if err := os.Rename(m.Path, m.NewPath); err != nil {
			ack.Error = err.Error()
			return ack
		}
		ack.OK = true
	case proto.OpMkdir:
		if err := os.MkdirAll(m.Path, 0o755); err != nil {
			ack.Error = err.Error()
			return ack
		}
		ack.OK = true
	case proto.OpStat:
		fi, err := os.Stat(m.Path)
		if err != nil {
			ack.Error = err.Error()
			return ack
		}
		ack.Entries = []proto.FileInfo{{
			Name: fi.Name(), Path: m.Path, Dir: fi.IsDir(),
			Size: fi.Size(), Mode: fi.Mode().String(), Mtime: fi.ModTime().Unix(),
		}}
		ack.OK = true
	default:
		ack.Error = "未知文件操作: " + m.Op
		ack.ErrCode = "unknown_file_op"
	}
	return ack
}
