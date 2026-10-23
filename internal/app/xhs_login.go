package app

import (
	"bufio"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"regexp"
	"time"
)

//go:embed xhs_login.py
var xhsLoginWorker string

type XHSLoginStatus struct {
	ID        string    `json:"id"`
	URL       string    `json:"url,omitempty"`
	State     string    `json:"state"`
	Message   string    `json:"message"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type xhsLoginSession struct {
	status        XHSLoginStatus
	method, phone string
	lastPoll      time.Time
	attempts      int
	input         io.WriteCloser
	output        chan []byte
	cancel        context.CancelFunc
	expiry        *time.Timer
}

type xhsLoginRequest struct {
	Operation string `json:"operation"`
	Proxy     string `json:"proxy,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Code      string `json:"code,omitempty"`
}

var xhsPhonePattern = regexp.MustCompile(`^1[3-9][0-9]{9}$`)
var xhsCodePattern = regexp.MustCompile(`^[0-9]{6}$`)

func (s *xhsLoginSession) close() {
	if s.expiry != nil {
		s.expiry.Stop()
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.input != nil {
		_ = s.input.Close()
		s.input = nil
	}
}

// The caller holds xhsLoginMu. Only the private pipes carry login credentials.
func (s *xhsLoginSession) call(ctx context.Context, input xhsLoginRequest) (string, error) {
	if s.input == nil {
		return "", errors.New("登录会话已关闭，请重新登录")
	}
	if err := json.NewEncoder(s.input).Encode(input); err != nil {
		s.close()
		return "", errors.New("登录会话已中断，请重新登录")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	select {
	case <-ctx.Done():
		s.close()
		return "", errors.New("小红书登录请求超时或已取消，请重试")
	case data, ok := <-s.output:
		var result struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
			Data  struct {
				State   string `json:"state"`
				Message string `json:"message"`
				URL     string `json:"url"`
				Cookies string `json:"cookies"`
			} `json:"data"`
		}
		if !ok || json.Unmarshal(data, &result) != nil {
			s.close()
			return "", errors.New("小红书登录服务已中断，请检查 API 运行环境")
		}
		if !result.OK {
			return "", errors.New(result.Error)
		}
		s.status.State, s.status.Message = result.Data.State, result.Data.Message
		if result.Data.URL != "" {
			s.status.URL = result.Data.URL
		}
		return result.Data.Cookies, nil
	}
}

func (a *App) startXHSLogin(ctx context.Context, method, phone string) (XHSLoginStatus, error) {
	a.xhsLoginMu.Lock()
	defer a.xhsLoginMu.Unlock()
	if method == "sms" && time.Now().Before(a.xhsSMSNext) {
		return XHSLoginStatus{}, errors.New("请等待 60 秒后再获取验证码")
	}
	python, source, err := xhsRuntime()
	if err != nil {
		return XHSLoginStatus{}, err
	}
	proxy := proxyURL(a.Snapshot().Settings)
	if err := validateProxy(proxy); err != nil {
		return XHSLoginStatus{}, err
	}
	if a.xhsLogin != nil {
		a.xhsLogin.close()
		a.xhsLogin = nil
	}
	workerCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	cmd := exec.CommandContext(workerCtx, python, "-u", "-c", xhsLoginWorker, source)
	configureProcess(cmd)
	a.configureXHSPython(cmd, false)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return XHSLoginStatus{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = stdin.Close()
		return XHSLoginStatus{}, err
	}
	if err = cmd.Start(); err != nil {
		cancel()
		_ = stdin.Close()
		_ = stdout.Close()
		return XHSLoginStatus{}, errors.New("无法启动小红书登录服务，请检查 API 运行环境")
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		cancel()
		_ = cmd.Wait()
		return XHSLoginStatus{}, err
	}
	s := &xhsLoginSession{method: method, phone: phone, input: stdin, output: make(chan []byte), cancel: cancel, status: XHSLoginStatus{ID: hex.EncodeToString(id)}}
	go func() {
		defer close(s.output)
		defer cancel()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 2<<20)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			select {
			case s.output <- line:
			case <-workerCtx.Done():
			}
			if workerCtx.Err() != nil {
				break
			}
		}
		cancel()
		_ = cmd.Wait()
	}()
	operation := "qr_start"
	if method == "sms" {
		operation = "sms_send"
		a.xhsSMSNext = time.Now().Add(time.Minute)
	}
	if _, err = s.call(ctx, xhsLoginRequest{Operation: operation, Proxy: proxy, Phone: phone}); err != nil {
		s.close()
		return XHSLoginStatus{}, err
	}
	lifetime := 180 * time.Second
	if method == "sms" {
		lifetime = 5 * time.Minute
	}
	s.status.ExpiresAt = time.Now().Add(lifetime)
	s.expiry = time.AfterFunc(lifetime, cancel)
	a.xhsLogin = s
	return s.status, nil
}

func (a *App) StartXHSQRLogin(ctx context.Context) (XHSLoginStatus, error) {
	return a.startXHSLogin(ctx, "qr", "")
}

func (a *App) SendXHSLoginCode(ctx context.Context, phone string) (XHSLoginStatus, error) {
	if !xhsPhonePattern.MatchString(phone) {
		return XHSLoginStatus{}, errors.New("请输入有效的中国大陆手机号")
	}
	return a.startXHSLogin(ctx, "sms", phone)
}

func (a *App) finishXHSLogin(s *xhsLoginSession, cookies string) (XHSLoginStatus, error) {
	if s.status.State == "success" {
		if cookies == "" {
			s.status.State = "expired"
			s.close()
			return XHSLoginStatus{}, errors.New("未收到正式登录凭据，请重新登录")
		}
		if err := a.saveCredential(PlatformXiaohongshu, []byte(cookies)); err != nil {
			s.status.State = "expired"
			s.close()
			return XHSLoginStatus{}, errors.New("加密保存登录凭据失败，请重新登录")
		}
		s.status.URL = ""
		s.close()
	}
	if s.status.State == "expired" {
		s.close()
	}
	return s.status, nil
}

func (a *App) activeXHSLogin(id string) (*xhsLoginSession, error) {
	s := a.xhsLogin
	if s == nil || s.status.ID != id {
		return nil, errors.New("登录会话不存在，请重新登录")
	}
	if s.status.State != "success" && time.Now().After(s.status.ExpiresAt) {
		s.status.State, s.status.Message = "expired", "登录已过期，请重新登录"
		s.close()
	}
	return s, nil
}

func (a *App) PollXHSLogin(ctx context.Context, id string) (XHSLoginStatus, error) {
	a.xhsLoginMu.Lock()
	defer a.xhsLoginMu.Unlock()
	s, err := a.activeXHSLogin(id)
	if err != nil {
		return XHSLoginStatus{}, err
	}
	if s.status.State == "success" || s.status.State == "expired" {
		return s.status, nil
	}
	if s.method != "qr" {
		return XHSLoginStatus{}, errors.New("请提交短信验证码")
	}
	if time.Since(s.lastPoll) < 2*time.Second {
		return s.status, nil
	}
	s.lastPoll = time.Now()
	cookies, err := s.call(ctx, xhsLoginRequest{Operation: "qr_poll"})
	if err != nil {
		s.close()
		return XHSLoginStatus{}, err
	}
	return a.finishXHSLogin(s, cookies)
}

func (a *App) VerifyXHSLoginCode(ctx context.Context, id, phone, code string) (XHSLoginStatus, error) {
	a.xhsLoginMu.Lock()
	defer a.xhsLoginMu.Unlock()
	s, err := a.activeXHSLogin(id)
	if err != nil {
		return XHSLoginStatus{}, err
	}
	if s.status.State == "expired" || s.status.State == "success" {
		return s.status, nil
	}
	if s.method != "sms" || phone != s.phone {
		return XHSLoginStatus{}, errors.New("手机号已变更，请重新获取验证码")
	}
	if !xhsCodePattern.MatchString(code) {
		return XHSLoginStatus{}, errors.New("请输入 6 位短信验证码")
	}
	if s.attempts >= 5 {
		s.close()
		return XHSLoginStatus{}, errors.New("验证码尝试过多，请重新获取")
	}
	s.attempts++
	cookies, err := s.call(ctx, xhsLoginRequest{Operation: "sms_login", Phone: phone, Code: code})
	if err != nil {
		return XHSLoginStatus{}, err
	}
	return a.finishXHSLogin(s, cookies)
}

func (a *App) CancelXHSLogin(id string) {
	a.xhsLoginMu.Lock()
	defer a.xhsLoginMu.Unlock()
	if a.xhsLogin != nil && (id == "" || a.xhsLogin.status.ID == id) {
		a.xhsLogin.close()
		a.xhsLogin = nil
	}
}
