package tasks

import (
	"context"
	"os/exec"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/session"
)

// source is what the form learns of the source agent off the main
// thread as it opens: herdr's agent.get, then git in the agent's folder.
type source struct {
	info session.AgentInfo
	err  error
	// isRepo is set when the folder is in a git work tree; isDirty when
	// it has uncommitted or untracked changes, which a worktree lacks.
	isRepo, isDirty bool
}

const gitTimeout = 3 * time.Second

func readSource(host Host, paneID string) source {
	info, err := host.AgentInfo(paneID)
	if err != nil {
		return source{err: err}
	}
	s := source{info: info}
	if info.Folder == "" {
		return s
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", info.Folder, "status", "--porcelain").Output()
	if err == nil {
		s.isRepo, s.isDirty = true, len(out) > 0
	}
	return s
}
