package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Label is the Daemon's launchd label (R-TERM-001).
const Label = "dev.sviniabanditka.notch-lab.daemon"

// BinaryPath returns the notch-labd beside the app: in the bundle's
// Contents/Resources/bin, else next to the executable.
func BinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	for _, p := range []string{
		filepath.Join(filepath.Dir(exe), "..", "Resources", "bin", "notch-labd"),
		filepath.Join(filepath.Dir(exe), "notch-labd"),
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return filepath.Clean(p), nil
		}
	}
	return "", errors.New("notch-labd is not in the app bundle")
}

// Connect dials the Daemon of a home, and starts it when it does not
// answer, then retries every 200 ms for 5 s (R-TERM-004). With dev true
// (NOTCH_LAB_HOME is set) the Daemon starts as a detached process of its
// own, else as a launchd agent (R-TERM-001, R-TERM-005).
func Connect(home, version string, dev bool) (*Client, error) {
	if c, err := Dial(home, version); err == nil {
		return c, nil
	} else if e := (*Error)(nil); errors.As(err, &e) {
		return nil, err // a Daemon of another protocol answered
	}
	bin, err := BinaryPath()
	if err != nil {
		return nil, err
	}
	if dev {
		err = startDetached(bin, home)
	} else {
		err = startLaunchd(bin)
	}
	if err != nil {
		return nil, err
	}
	var last error
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		c, err := Dial(home, version)
		if err == nil {
			return c, nil
		}
		last = err
	}
	return nil, fmt.Errorf("the terminal daemon did not start: %w", last)
}

func startDetached(bin, home string) error {
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "NOTCH_LAB_HOME="+home)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func plistPath() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Library", "LaunchAgents", Label+".plist")
}

// startLaunchd writes the agent's plist when it is missing or names
// another binary, bootstraps it, and kicks it (R-TERM-001, R-TERM-002, R-TERM-004).
func startLaunchd(bin string) error {
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key><array><string>%s</string></array>
	<key>EnvironmentVariables</key><dict><key>NOTCH_LAB_LAUNCHD</key><string>1</string></dict>
	<key>KeepAlive</key><true/>
	<key>RunAtLoad</key><true/>
	<key>ProcessType</key><string>Interactive</string>
</dict>
</plist>
`, Label, bin)
	old, _ := os.ReadFile(plistPath())
	if string(old) != plist {
		_ = exec.Command("/bin/launchctl", "bootout", domain+"/"+Label).Run()
		if err := os.MkdirAll(filepath.Dir(plistPath()), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(plistPath(), []byte(plist), 0o644); err != nil {
			return err
		}
	}
	if out, err := exec.Command("/bin/launchctl", "print", domain+"/"+Label).CombinedOutput(); err != nil || !strings.Contains(string(out), Label) {
		if out, err := exec.Command("/bin/launchctl", "bootstrap", domain, plistPath()).CombinedOutput(); err != nil {
			return fmt.Errorf("launchctl bootstrap: %v: %s", err, out)
		}
	}
	return exec.Command("/bin/launchctl", "kickstart", "-k", domain+"/"+Label).Run()
}

// Restart stops the Daemon so that launchd starts it again (a protocol
// mismatch, R-TERM-011). In dev it kills the detached Daemon of the home.
func Restart(home string, dev bool) {
	if dev {
		// Only the Daemon of this home: the one holding its lock.
		out, _ := exec.Command("/usr/sbin/lsof", "-t", filepath.Join(home, "daemon.lock")).Output()
		for _, pid := range strings.Fields(string(out)) {
			_ = exec.Command("/bin/kill", pid).Run()
		}
		time.Sleep(300 * time.Millisecond)
		return
	}
	_ = exec.Command("/bin/launchctl", "kickstart", "-k", fmt.Sprintf("gui/%d/%s", os.Getuid(), Label)).Run()
}
