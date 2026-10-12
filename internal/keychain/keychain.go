// Package keychain keeps secrets in the login Keychain as generic
// passwords of the service dev.sviniabanditka.notch-lab (R-SHELL-165).
// It talks to /usr/bin/security over stdin, so a value never shows in a
// process's arguments.
package keychain

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

// Service is the Keychain service of every item.
var Service = "dev.sviniabanditka.notch-lab"

// ErrNotFound is a missing item.
var ErrNotFound = errors.New("not in the Keychain")

// quote writes a word for security's interactive mode.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func run(cmd string) (string, error) {
	c := exec.Command("/usr/bin/security", "-i")
	c.Stdin = strings.NewReader(cmd + "\n")
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	if err := c.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 44 { // errSecItemNotFound
			return "", ErrNotFound
		}
		return "", err
	}
	if e := strings.TrimSpace(errb.String()); e != "" {
		if strings.Contains(e, "could not be found") {
			return "", ErrNotFound
		}
		if strings.Contains(e, "has been deleted") {
			return out.String(), nil
		}
		return "", errors.New(e)
	}
	return out.String(), nil
}

// Get reads an item's value.
func Get(account string) (string, error) {
	out, err := run("find-generic-password -s " + quote(Service) + " -a " + quote(account) + " -w")
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimPrefix(out, "security> "), "\n"), nil
}

// Set writes an item, replacing one with the same account.
func Set(account, value string) error {
	_, err := run("add-generic-password -U -s " + quote(Service) + " -a " + quote(account) + " -w " + quote(value))
	return err
}

// Delete removes an item; a missing one is no error.
func Delete(account string) error {
	_, err := run("delete-generic-password -s " + quote(Service) + " -a " + quote(account))
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
