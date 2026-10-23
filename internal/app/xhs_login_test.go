package app

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func xhsLoginFixture(t *testing.T) *App {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("需要 Python 3 验证登录管道")
	}
	dir := t.TempDir()
	for _, name := range []string{"venv/bin", "spider-xhs/apis"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(python, filepath.Join(dir, "venv/bin/python")); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"loguru.py":                   "class Logger:\n def remove(self): pass\nlogger = Logger()\n",
		"xhs_utils/xhs_pc/runtime.py": "import subprocess\n_SIGN_JS='/fixture/sign.js'\n",
		"apis/xhs_pc_login_apis.py": `
import os
class XHSLoginApi:
 def __init__(self, **kwargs): self.polls = 0
 def close(self): pass
 def generate_init_cookies(self): return {"a1":"private-a1", "web_session":"visitor-session"}
 def generate_qrcode(self, cookies): return True, "成功", {"cookies":cookies,"qr_id":"qr-id","code":"private-qr-code","qr_url":"https://www.xiaohongshu.com/qr/fixture"}
 def check_qrcode_status(self, qr_id, code, cookies):
  self.polls += 1
  if self.polls > 1 and os.environ.get("XHS_LOGIN_FIXTURE_FAILURE"): raise RuntimeError("private-formal-session private-qr-code 13800138000")
  if self.polls < 3: return False, "请扫描二维码", cookies
  cookies["web_session"] = "private-formal-session"
  return True, "验证成功", cookies
 def ensure_webprofile(self, cookies): pass
 def get_user_info(self, cookies): return True, {"guest":False,"user_id":"5da907190000000001005c5d","nickname":"测试用户"}, cookies
 def send_phone_code(self, phone, cookies): return True, "成功", {}
 def login_by_phone(self, phone, code, cookies):
  if code != "123456": return False, "错误", {}
  cookies["web_session"] = "private-formal-session"
  return True, "成功", {"cookies":cookies}
`,
	}
	for path, body := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, "spider-xhs", path)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "spider-xhs", path), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("BILIFETCH_XHS_RUNTIME", dir)
	a := testApp(t)
	t.Cleanup(a.Close)
	return a
}

func TestXHSHTTPQRLoginNeedsNoBrowserAndKeepsCredentialsPrivate(t *testing.T) {
	a := xhsLoginFixture(t)
	t.Setenv("PATH", "") // 重现没有 google-chrome 的服务环境。
	ctx := context.Background()
	status, err := a.StartXHSQRLogin(ctx)
	if err != nil || status.State != "waiting_scan" || status.URL == "" {
		t.Fatalf("申请二维码: %+v %v", status, err)
	}
	if a.SavedPlatformAccount(PlatformXiaohongshu).State != "logged_out" {
		t.Fatal("扫码前不应保存访客凭据")
	}
	if _, err := a.PollXHSLogin(ctx, "unknown"); err == nil {
		t.Fatal("必须拒绝未知会话")
	}
	status, err = a.PollXHSLogin(ctx, status.ID)
	if err != nil || status.State != "waiting_scan" {
		t.Fatalf("等待扫码: %+v %v", status, err)
	}
	a.xhsLogin.lastPoll = time.Now().Add(-3 * time.Second)
	status, err = a.PollXHSLogin(ctx, status.ID)
	if err != nil || status.State != "success" {
		t.Fatalf("手机确认: %+v %v", status, err)
	}
	data, err := a.loadCredential(PlatformXiaohongshu)
	if err != nil || !strings.Contains(string(data), "web_session\tprivate-formal-session") {
		t.Fatal("未保存正式凭据", err)
	}
	public, _ := json.Marshal(status)
	if strings.Contains(string(public), "private-") || strings.Contains(string(public), "cookies") {
		t.Fatal("登录响应泄漏凭据")
	}
	if a.xhsBrowser.browser != nil {
		t.Fatal("HTTP 登录启动了浏览器")
	}
}

func TestXHSSMSLoginValidationRetryExpiryAndLogout(t *testing.T) {
	a := xhsLoginFixture(t)
	ctx := context.Background()
	if _, err := a.SendXHSLoginCode(ctx, "123"); err == nil {
		t.Fatal("无效手机号不应发短信")
	}
	status, err := a.SendXHSLoginCode(ctx, "13800138000")
	if err != nil || status.State != "waiting_code" {
		t.Fatalf("发送验证码: %+v %v", status, err)
	}
	if _, err := a.SendXHSLoginCode(ctx, "13800138000"); err == nil {
		t.Fatal("必须限制短信重发频率")
	}
	if _, err := a.VerifyXHSLoginCode(ctx, status.ID, "13900139000", "123456"); err == nil {
		t.Fatal("验证码必须绑定原手机号")
	}
	if _, err := a.VerifyXHSLoginCode(ctx, status.ID, "13800138000", "000000"); err == nil {
		t.Fatal("错误验证码不应登录")
	}
	status, err = a.VerifyXHSLoginCode(ctx, status.ID, "13800138000", "123456")
	if err != nil || status.State != "success" {
		t.Fatalf("验证码重试: %+v %v", status, err)
	}
	if err := a.LogoutXHS(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := a.VerifyXHSLoginCode(ctx, status.ID, "13800138000", "123456"); err == nil {
		t.Fatal("退出后旧会话不得重新写入凭据")
	}
	qr, err := a.StartXHSQRLogin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a.xhsLogin.status.ExpiresAt = time.Now().Add(-time.Second)
	qr, err = a.PollXHSLogin(ctx, qr.ID)
	if err != nil || qr.State != "expired" {
		t.Fatalf("过期会话: %+v %v", qr, err)
	}
}

func TestXHSLoginHTTPResponsesArePrivate(t *testing.T) {
	a := xhsLoginFixture(t)
	handler := a.Handler(context.Background(), "", "")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/api/xiaohongshu/login", nil))
	var status XHSLoginStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || status.State != "waiting_scan" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/api/xiaohongshu/login/"+status.ID+"/cancel", nil))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, err := a.PollXHSLogin(context.Background(), status.ID); err == nil {
		t.Fatal("关闭弹窗后会话应失效")
	}
}

func TestXHSLoginUpstreamErrorsAreRedactedAndWorkerStops(t *testing.T) {
	a := xhsLoginFixture(t)
	t.Setenv("XHS_LOGIN_FIXTURE_FAILURE", "1")
	status, err := a.StartXHSQRLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.PollXHSLogin(context.Background(), status.ID)
	if err == nil || strings.Contains(err.Error(), "private-") || strings.Contains(err.Error(), "13800138000") {
		t.Fatalf("上游异常必须脱敏: %v", err)
	}
	select {
	case _, open := <-a.xhsLogin.output:
		if open {
			t.Fatal("失败会话不应继续发送响应")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("登录失败后未终止工作进程")
	}
}
