package agent

import "errors"

// codedError 携带稳定错误码的错误，随 ack.ErrCode/ErrParams 透传给服务端与前端做本地化。
type codedError struct {
	code   string
	params map[string]any
	msg    string
}

func (e *codedError) Error() string { return e.msg }

// errCodeOf 取出错误携带的错误码与插值参数（非 codedError 时返回空）。
func errCodeOf(err error) (string, map[string]any) {
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.code, ce.params
	}
	return "", nil
}
