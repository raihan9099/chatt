package server

import "errors"

// codedError 携带稳定错误码的错误，随 HTTP/WS 错误响应附 code/params 字段，前端按码本地化。
type codedError struct {
	code   string
	params map[string]any
	err    error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// errCodeOf 取出错误码与插值参数（非 codedError 时返回空码）。
func errCodeOf(err error) (string, map[string]any) {
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.code, ce.params
	}
	return "", nil
}

// agentErrEn agent 已知错误码的英文文案：AI 工具结果面向模型与英文界面时使用；
// 未知错误码回退 ack 原文。
func agentErrEn(errCode string, params map[string]any, fallback string) string {
	p := func(k string) string {
		if v, ok := params[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	switch errCode {
	case "term_not_found":
		return "terminal not found: " + p("id")
	case "term_closed":
		return "terminal already closed"
	case "shell_not_allowed":
		return "shell not in the available list: " + p("shell")
	case "holder_timeout":
		return "timed out waiting for terminal holder"
	case "file_too_large":
		return "file exceeds the size limit"
	case "bad_base64":
		return "base64 decode failed"
	case "unknown_file_op":
		return "unknown file operation"
	}
	return fallback
}
