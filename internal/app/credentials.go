package app

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

const credentialService = "BiliFetch"

var credentialKeyCache struct {
	sync.Mutex
	key []byte
}

// SavedPlatformAccount reports local credential availability without contacting
// the platform or starting a browser. Platforms validate sessions when used.
func (a *App) SavedPlatformAccount(platform PlatformID) XHSAccount {
	a.mu.Lock()
	saved := a.state.Credentials[platform] != ""
	a.mu.Unlock()
	if !saved {
		return XHSAccount{State: "logged_out", Message: "未保存登录凭据，请先扫码登录"}
	}
	data, err := a.loadCredential(platform)
	if err != nil || len(data) == 0 {
		return XHSAccount{State: "unavailable", Message: "登录凭据不可读，请重新登录"}
	}
	return XHSAccount{State: "logged_in", Message: "已保存登录凭据"}
}

func credentialKey(dataDir string) ([]byte, string, error) {
	credentialKeyCache.Lock()
	defer credentialKeyCache.Unlock()
	if len(credentialKeyCache.key) == 32 {
		return credentialKeyCache.key, "v2k", nil
	}
	encoded, err := keyring.Get(credentialService, "state-encryption-key")
	if err != nil {
		key := make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, "", err
		}
		if err = keyring.Set(credentialService, "state-encryption-key", base64.RawStdEncoding.EncodeToString(key)); err == nil {
			credentialKeyCache.key = key
			return key, "v2k", nil
		}
		fileKey, fileErr := credentialFileKey(dataDir)
		if fileErr != nil {
			return nil, "", errors.New("无法初始化登录凭据加密密钥")
		}
		return fileKey, "v2f", nil
	}
	key, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(key) != 32 {
		return nil, "", errors.New("系统安全凭据库中的加密密钥无效")
	}
	credentialKeyCache.key = key
	return key, "v2k", nil
}

func credentialFileKey(dataDir string) ([]byte, error) {
	path := filepath.Join(dataDir, ".credential-key")
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			return nil, errors.New("本地凭据密钥无效")
		}
		if err = os.Chmod(path, 0600); err != nil {
			return nil, err
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key = make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) { // Another process initialized it first.
		key, err = os.ReadFile(path)
		if err == nil && len(key) == 32 {
			return key, nil
		}
		return nil, errors.New("本地凭据密钥无效")
	}
	if err != nil {
		return nil, err
	}
	if _, err = file.Write(key); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return key, nil
}

func credentialKeyring() ([]byte, error) {
	credentialKeyCache.Lock()
	defer credentialKeyCache.Unlock()
	if len(credentialKeyCache.key) == 32 {
		return credentialKeyCache.key, nil
	}
	encoded, err := keyring.Get(credentialService, "state-encryption-key")
	if err != nil {
		return nil, err
	}
	key, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(key) != 32 {
		return nil, errors.New("系统安全凭据库中的加密密钥无效")
	}
	credentialKeyCache.key = key
	return key, nil
}

func (a *App) encryptCredential(platform PlatformID, plain []byte) (string, error) {
	dataDir := filepath.Dir(a.statePath)
	key, version, err := credentialKey(dataDir)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, plain, []byte("BiliFetch credential v1:"+string(platform)))
	return version + ":" + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (a *App) decryptCredential(platform PlatformID, encoded string) ([]byte, error) {
	if len(encoded) < 4 {
		return nil, errors.New("登录凭据格式无效，请重新登录")
	}
	version := encoded[:3]
	var key []byte
	var err error
	switch version {
	case "v1:", "v2k":
		key, err = credentialKeyring()
	case "v2f":
		key, err = credentialFileKey(filepath.Dir(a.statePath))
	default:
		return nil, errors.New("登录凭据格式无效，请重新登录")
	}
	if err != nil {
		return nil, err
	}
	sealed, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(encoded[3:], ":"))
	if err != nil {
		return nil, errors.New("登录凭据无法解码，请重新登录")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < aead.NonceSize() {
		return nil, errors.New("登录凭据不完整，请重新登录")
	}
	return aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte("BiliFetch credential v1:"+string(platform)))
}

func (a *App) saveCredential(platform PlatformID, plain []byte) error {
	if existing, err := a.loadCredential(platform); err == nil && bytes.Equal(existing, plain) {
		return nil
	}
	sealed, err := a.encryptCredential(platform, plain)
	if err != nil {
		return err
	}
	return a.transaction(func() error {
		if a.state.Credentials == nil {
			a.state.Credentials = map[PlatformID]string{}
		}
		a.state.Credentials[platform] = sealed
		return nil
	})
}

func (a *App) loadCredential(platform PlatformID) ([]byte, error) {
	a.mu.Lock()
	sealed := a.state.Credentials[platform]
	a.mu.Unlock()
	if sealed == "" {
		return nil, errors.New("请先登录对应平台")
	}
	return a.decryptCredential(platform, sealed)
}

func (a *App) deleteCredential(platform PlatformID) error {
	return a.transaction(func() error {
		delete(a.state.Credentials, platform)
		return nil
	})
}

func (a *App) writeTemporaryCookieFile(data []byte) (string, func(), error) {
	if len(data) > 2<<20 {
		return "", func() {}, errors.New("Cookie 数据超过 2MB")
	}
	// MozillaCookieJar (used by yt-dlp) requires uppercase boolean flags.
	// Normalize previously saved credentials without changing Cookie values.
	lines := bytes.Split(data, []byte("\n"))
	for i, line := range lines {
		fields := bytes.Split(line, []byte("\t"))
		if len(fields) == 7 {
			fields[1], fields[3] = bytes.ToUpper(fields[1]), bytes.ToUpper(fields[3])
			lines[i] = bytes.Join(fields, []byte("\t"))
		}
	}
	data = bytes.Join(lines, []byte("\n"))
	file, err := os.CreateTemp(filepath.Dir(a.statePath), ".yt-cookies-*")
	if err != nil {
		return "", func() {}, err
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}
