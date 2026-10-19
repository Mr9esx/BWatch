//go:build unix

package app

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// yt-dlp may own an FFmpeg process. Cancel the entire job, including children
// holding stdout open, rather than only terminating its Python parent.
func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 5 * time.Second
}
