package tasks

import (
	"fmt"
	"log"
	"math/rand/v2"
	"strings"

	"github.com/sviniabanditka/notch-lab/internal/session"
)

type form struct {
	host     Host
	stateDir string
	saved    choices

	open bool
	// focusPrompt and focusBranch move the focus in the next frame.
	focusPrompt, focusBranch bool
	// submit is Enter in the prompt, taken as it came.
	submit bool

	paneID, workspaceID, agentName, folder string
	// hex names the task's agent, task-<hex>, and its branch when the
	// prompt has no words to name it by.
	hex string

	prompt, branch string
	// branchEdited stops the branch following the prompt.
	branchEdited bool
	// wantFresh and worktree are the choices as the user left them;
	// fork shows only while the source allows it.
	wantFresh, worktree, skip bool

	src    source
	loaded bool

	// undelivered are tasks whose prompt was not sent, for a toast.
	undelivered []undelivered
	// copy is a prompt to put on the clipboard in the next frame.
	copy string
}

type undelivered struct{ name, paneID, prompt string }

func newView(host Host, stateDir string) *View {
	return &View{form{host: host, stateDir: stateDir, saved: loadChoices(stateDir)}}
}

func focusedAgent(st *session.State) string {
	if st == nil || st.FocusedPaneID == "" {
		return ""
	}
	for _, a := range st.Agents {
		if a.PaneID == st.FocusedPaneID {
			return a.PaneID
		}
	}
	return ""
}

func (f *form) openFor(st *session.State, paneID string) {
	if st == nil {
		return
	}
	var agent *session.Agent
	for i := range st.Agents {
		if st.Agents[i].PaneID == paneID {
			agent = &st.Agents[i]
		}
	}
	if agent == nil {
		return
	}
	*f = form{host: f.host, stateDir: f.stateDir, saved: f.saved, undelivered: f.undelivered, copy: f.copy,
		open: true, focusPrompt: true, paneID: paneID, workspaceID: agent.WorkspaceID, agentName: agent.Name,
		hex: fmt.Sprintf("%04x", rand.N(0x10000)), wantFresh: f.saved.Fresh, worktree: f.saved.Worktree}
	for _, p := range st.Panes {
		if p.ID == paneID {
			f.folder = p.Folder()
		}
	}
	host := f.host
	go func() {
		src := readSource(host, paneID)
		host.Update(func() {
			if f.paneID != paneID {
				return // opened for another agent meanwhile
			}
			f.src, f.loaded = src, true
			if src.info.Folder != "" {
				f.folder = src.info.Folder
			}
		})
	}()
}

// liveStatus is the source agent's status in the latest snapshot.
func (f *form) liveStatus(st *session.State) session.AgentStatus {
	if st != nil {
		for _, a := range st.Agents {
			if a.PaneID == f.paneID {
				return a.Status
			}
		}
	}
	return f.src.info.Status
}

// forkReason is why the form cannot fork now, or "".
func (f *form) forkReason(st *session.State) string {
	switch {
	case !f.loaded:
		return "Reading the agent's session…"
	case f.src.err != nil:
		return "herdr did not say which session this is: " + f.src.err.Error()
	}
	return forkBlocked(f.src.info.Kind, f.src.info.SessionID, f.liveStatus(st))
}

func (f *form) forks(st *session.State) bool { return !f.wantFresh && f.forkReason(st) == "" }

// worktreeReason is why the task cannot have a worktree, or "".
func (f *form) worktreeReason() string {
	if f.loaded && !f.src.isRepo {
		return "The agent's folder is not in a git repository"
	}
	return ""
}

func (f *form) worktreeOn() bool { return f.worktree && f.worktreeReason() == "" }

// followPrompt keeps the branch named after the prompt until edited.
func (f *form) followPrompt() {
	if !f.branchEdited {
		f.branch = branchName(f.prompt, f.hex)
	}
}

func (f *form) start(st *session.State) {
	prompt := strings.TrimSpace(f.prompt)
	if prompt == "" {
		return
	}
	fork := f.forks(st)
	name := "task-" + f.hex
	cmd := session.StartTask{WorkspaceID: f.workspaceID, Cwd: f.folder, Label: tabLabel(prompt),
		AgentName: name, Kind: claudeKind, Args: claudeArgs(fork, f.src.info.SessionID, f.skip), Prompt: prompt}
	if f.worktreeOn() {
		cmd.Branch = strings.TrimSpace(f.branch)
		if cmd.Branch == "" {
			cmd.Branch = branchName(prompt, f.hex)
		}
	}
	host := f.host
	cmd.Undelivered = func(paneID string) {
		host.Update(func() { f.undelivered = append(f.undelivered, undelivered{name, paneID, prompt}) })
	}
	if err := host.Send(cmd); err != nil {
		log.Printf("tasks: %v", err)
		return
	}
	f.open = false
	f.saved = choices{Fresh: f.wantFresh, Worktree: f.worktree}
	saved, dir := f.saved, f.stateDir
	go func() {
		if err := saveChoices(dir, saved); err != nil {
			log.Printf("tasks: save: %v", err)
		}
	}()
}
