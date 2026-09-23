package server

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mojocn/base64Captcha"
)

// ---- 验证码 ----

var captchaStore = struct {
	sync.Mutex
	m map[string]captchaEntry
}{m: map[string]captchaEntry{}}

type captchaEntry struct {
	answer string
	exp    time.Time
}

var captchaDriver = base64Captcha.NewDriverDigit(60, 160, 4, 0.7, 80)

// newCaptcha 生成图形验证码，答案内存保存、5 分钟有效、一次性。
func newCaptcha() (id, imageB64 string, err error) {
	id = randHex(8)
	_, content, answer := captchaDriver.GenerateIdQuestionAnswer()
	item, err := captchaDriver.DrawCaptcha(content)
	if err != nil {
		return "", "", err
	}
	b64 := item.EncodeB64string()
	captchaStore.Lock()
	captchaStore.m[id] = captchaEntry{answer: answer, exp: time.Now().Add(5 * time.Minute)}
	captchaStore.Unlock()
	return id, b64, nil
}

// verifyCaptcha 校验并销毁验证码。
// 若设置了环境变量 CAPTCHA_TEST_CODE（仅用于自动化测试），该固定答案也可通过（仍需有效 captcha_id）。
func verifyCaptcha(id, answer string) bool {
	captchaStore.Lock()
	defer captchaStore.Unlock()
	e, ok := captchaStore.m[id]
	if !ok || time.Now().After(e.exp) {
		delete(captchaStore.m, id)
		return false
	}
	delete(captchaStore.m, id) // 一次性
	if tc := os.Getenv("CAPTCHA_TEST_CODE"); tc != "" && answer == tc {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(answer)), []byte(strings.ToLower(e.answer))) == 1
}

// ---- 登录防爆破 ----

const (
	maxLoginFails = 5
	lockDuration  = 10 * time.Minute
)

// checkLoginAllowed 返回是否被锁定及剩余秒数。
func (h *Hub) checkLoginAllowed() (bool, int64) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	until := h.store.st.Lockout.LockedUntil
	if until > time.Now().Unix() {
		return false, until - time.Now().Unix()
	}
	return true, 0
}

func (h *Hub) recordLoginFail() {
	h.store.mu.Lock()
	l := h.store.st.Lockout
	l.Fails++
	locked := false
	if l.Fails >= maxLoginFails {
		l.LockedUntil = time.Now().Add(lockDuration).Unix()
		l.Fails = 0
		locked = true
	}
	h.store.mu.Unlock()
	if locked {
		h.logf("连续登录失败 %d 次，锁定 %v", maxLoginFails, lockDuration)
	}
	h.store.markDirty()
}

func (h *Hub) recordLoginSuccess() {
	h.store.mu.Lock()
	h.store.st.Lockout.Fails = 0
	h.store.st.Lockout.LockedUntil = 0
	h.store.mu.Unlock()
	h.store.markDirty()
}

// ---- JWT ----

const tokenTTL = 72 * time.Hour

func (h *Hub) issueToken() (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   h.cfg.AdminUser,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.store.st.JWTSecret))
}

func (h *Hub) validToken(tokenStr string) bool {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		return []byte(h.store.st.JWTSecret), nil
	})
	return err == nil && token.Valid
}

// authMiddleware 校验 Authorization: Bearer <jwt>。
func (h *Hub) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "" || !h.validToken(token) {
			http.Error(w, `{"error":"未认证","code":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
