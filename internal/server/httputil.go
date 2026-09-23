package server

import (
	"bytes"
	"context"
	"net/http"
	"time"
)

var httpDefaultClient = &http.Client{Timeout: 120 * time.Second}

func httpNewRequestWithContext(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
}
