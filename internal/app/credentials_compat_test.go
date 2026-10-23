package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBilibiliFileCredentialSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	a, err := New(dir, t.TempDir(), "yt-dlp")
	if err != nil {
		t.Fatal(err)
	}
	key, err := credentialFileKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, aead.NonceSize())
	plain := []byte("# Netscape HTTP Cookie File\n.bilibili.com\tTRUE\t/\tTRUE\t0\tSESSDATA\texisting-login-fixture\n")
	sealed := "v2f:" + base64.RawStdEncoding.EncodeToString(aead.Seal(nonce, nonce, plain, []byte("BiliFetch credential v1:bilibili")))
	if err := a.transaction(func() error { a.state.Credentials = map[PlatformID]string{PlatformBilibili: sealed}; return nil }); err != nil {
		t.Fatal(err)
	}
	a.Close()
	b, err := New(dir, t.TempDir(), "yt-dlp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	requests := 0
	b.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.Header.Get("Cookie") != "SESSDATA=existing-login-fixture" {
			t.Fatal("重启后应复用原有登录凭据")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"isLogin":true,"mid":123,"uname":"已登录账号"}}`))}, nil
	})}
	if got := b.BilibiliAccount(context.Background()); got.State != "logged_in" || got.Name != "已登录账号" {
		t.Fatalf("已保存的登录状态应能恢复: %+v", got)
	}
	if requests != 1 || b.state.Credentials[PlatformBilibili] != sealed {
		t.Fatal("读取账号不应覆盖现有密文或要求重新登录")
	}
}
