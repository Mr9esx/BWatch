package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCookieParserErrorDoesNotExposeCredentials(t *testing.T) {
	trace := "CookieLoadError: failed to load cookies\nERROR: invalid Netscape format cookies file 'data/.yt-cookies-fixture': '.xiaohongshu.com\\ttrue\\t/\\tfalse\\t0\\tweb_session\\tprivate-session-fixture'"
	message := commandError(errors.New("exit status 1"), trace)
	state := State{Tasks: []Task{{Error: trace}}, Events: []Event{{Message: trace}}}
	migrateState(&state)
	for _, got := range []string{message, state.Tasks[0].Error, state.Events[0].Message} {
		if !strings.Contains(got, "Cookie") || strings.Contains(got, "private-session-fixture") || strings.Contains(got, "yt-cookies-fixture") {
			t.Fatal("Cookie parser diagnostics must not expose credentials or temporary paths")
		}
	}
}

func TestLegacyCookiesRemainReadableByYtDlp(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python cookiejar is required to verify yt-dlp cookie compatibility")
	}
	a := &App{statePath: filepath.Join(t.TempDir(), "state.json")}
	path, cleanup, err := a.writeTemporaryCookieFile([]byte("# Netscape HTTP Cookie File\n.xiaohongshu.com\ttrue\t/\ttrue\t0\ttrue\ttrue\n"))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	// yt-dlp uses MozillaCookieJar. This also proves that normalizing boolean
	// columns does not change a Cookie whose name or value happens to be true.
	command := exec.Command(python, "-c", "import http.cookiejar,sys; jar=http.cookiejar.MozillaCookieJar(sys.argv[1]); jar.load(ignore_discard=True, ignore_expires=True); cookies=list(jar); assert len(cookies)==1 and cookies[0].name=='true' and cookies[0].value=='true' and cookies[0].secure", path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("yt-dlp's Cookie parser rejected the file: %v %s", err, output)
	}
}

func TestAccountDisplayDoesNotLaunchBrowser(t *testing.T) {
	for _, saved := range []string{"", "invalid-ciphertext"} {
		t.Run(saved, func(t *testing.T) {
			a := &App{xhsBrowser: newXHSBrowser(t.TempDir()), platforms: newPlatformRegistry()}
			a.state.Credentials = map[PlatformID]string{PlatformXiaohongshu: saved}
			handler := a.Handler(context.Background(), "", "")
			wantState := "logged_out"
			if saved != "" {
				wantState = "unavailable"
			}
			for _, path := range []string{"/api/xiaohongshu/account", "/api/platforms/xiaohongshu/account"} {
				for i := 0; i < 3; i++ {
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
					var account XHSAccount
					if err := json.Unmarshal(response.Body.Bytes(), &account); err != nil {
						t.Fatal(err)
					}
					if response.Code != 200 || account.State != wantState {
						t.Fatalf("%s: status %d, state %s", path, response.Code, account.State)
					}
				}
			}
			if a.xhsBrowser.browser != nil {
				t.Fatal("reading saved credentials launched a browser")
			}
			if _, err := os.Stat(a.xhsBrowser.profileDir); !os.IsNotExist(err) {
				t.Fatalf("reading saved credentials touched the browser profile: %v", err)
			}
		})
	}
}
