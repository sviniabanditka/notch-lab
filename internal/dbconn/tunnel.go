package dbconn

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// tunnel is an SSH connection the database dials through (R-DB-009).
type tunnel struct{ client *ssh.Client }

func (t *tunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return t.client.DialContext(ctx, network, addr)
}

func (t *tunnel) Close() { t.client.Close() }

// Signers are the keys to try: the identity file, else every ~/.ssh/id_*
// without a passphrase, then the agent's.
func Signers(identity string) ([]ssh.AuthMethod, error) {
	home, _ := os.UserHomeDir()
	var signers []ssh.Signer
	files := []string{identity}
	if identity == "" {
		files, _ = filepath.Glob(filepath.Join(home, ".ssh", "id_*"))
	}
	for _, f := range files {
		if f == "" || strings.HasSuffix(f, ".pub") {
			continue
		}
		if strings.HasPrefix(f, "~/") {
			f = filepath.Join(home, f[2:])
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if s, err := ssh.ParsePrivateKey(data); err == nil {
			signers = append(signers, s)
		}
	}
	methods := []ssh.AuthMethod{}
	if len(signers) > 0 {
		methods = append(methods, ssh.PublicKeys(signers...))
	}
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if c, err := net.Dial("unix", sock); err == nil {
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(c).Signers))
		}
	}
	if len(methods) == 0 {
		return nil, errors.New("no SSH key: no identity file, no ~/.ssh/id_* and no ssh-agent")
	}
	return methods, nil
}

// HostKeys checks host keys against ~/.ssh/known_hosts.
func HostKeys() (ssh.HostKeyCallback, error) {
	home, _ := os.UserHomeDir()
	return knownhosts.New(filepath.Join(home, ".ssh", "known_hosts"))
}

func openTunnel(spec, identity string) (*tunnel, error) {
	user, host, ok := strings.Cut(spec, "@")
	if !ok {
		user, host = os.Getenv("USER"), spec
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "22")
	}
	auth, err := Signers(identity)
	if err != nil {
		return nil, err
	}
	keys, err := HostKeys()
	if err != nil {
		return nil, err
	}
	c, err := ssh.Dial("tcp", host, &ssh.ClientConfig{User: user, Auth: auth, HostKeyCallback: keys, Timeout: 10e9})
	if err != nil {
		return nil, err
	}
	return &tunnel{client: c}, nil
}
