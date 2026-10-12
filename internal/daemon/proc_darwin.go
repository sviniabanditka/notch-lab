package daemon

import (
	"bytes"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

// Process facts of macOS without cgo: the foreground group's processes from
// sysctl, their arguments from kern.procargs2, a process's current folder
// from libproc's proc_pidinfo (R-TERM-030).

type proc struct {
	PID, PPID int
	Name      string
	Args      []string
}

// groupProcs returns the processes of a process group.
func groupProcs(pgid int) []proc {
	list, err := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pgid)
	if err != nil {
		return nil
	}
	out := make([]proc, 0, len(list))
	for _, k := range list {
		comm := k.Proc.P_comm[:]
		if i := bytes.IndexByte(comm, 0); i >= 0 {
			comm = comm[:i]
		}
		name := string(comm)
		out = append(out, proc{PID: int(k.Proc.P_pid), PPID: int(k.Eproc.Ppid), Name: name})
	}
	return out
}

// procArgs returns a process's arguments, or nil.
func procArgs(pid int) []string {
	buf, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(buf) < 4 {
		return nil
	}
	argc := int(*(*int32)(unsafe.Pointer(&buf[0])))
	rest := buf[4:]
	// The executable path, then NULs, then argc NUL-ended strings.
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return nil
	}
	rest = rest[i:]
	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}
	var args []string
	for len(args) < argc && len(rest) > 0 {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			j = len(rest)
		}
		args = append(args, string(rest[:j]))
		rest = rest[min(j+1, len(rest)):]
	}
	return args
}

var libproc struct {
	once    sync.Once
	pidinfo func(pid int32, flavor int32, arg uint64, buf unsafe.Pointer, size int32) int32
}

// procCwd returns a process's current folder, or "".
func procCwd(pid int) string {
	libproc.once.Do(func() {
		lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			purego.RegisterLibFunc(&libproc.pidinfo, lib, "proc_pidinfo")
		}
	})
	if libproc.pidinfo == nil {
		return ""
	}
	// struct proc_vnodepathinfo { vnode_info_path pvi_cdir, pvi_rdir; };
	// vnode_info_path { vnode_info vip_vi (152 bytes); char vip_path[1024]; }
	const flavor, viSize, pathSize = 9, 152, 1024 // PROC_PIDVNODEPATHINFO
	var buf [2 * (viSize + pathSize)]byte
	if n := libproc.pidinfo(int32(pid), flavor, 0, unsafe.Pointer(&buf[0]), int32(len(buf))); n <= 0 {
		return ""
	}
	path := buf[viSize : viSize+pathSize]
	if i := bytes.IndexByte(path, 0); i >= 0 {
		path = path[:i]
	}
	return string(path)
}

// agentKind names the agent a process is, or "" (12-agents.md R-AGENT-001).
// The process name alone is not enough: Claude Code names its process
// after its version ("2.1.295"), so argv[0] decides too, and for a script
// agent the entry point after node or bun.
func agentKind(p proc) string {
	kind := func(a string) string {
		switch b := filepath.Base(a); {
		case b == "claude" || strings.Contains(a, "claude-code"):
			return "claude"
		case b == "codex" || strings.Contains(a, "@openai/codex"):
			return "codex"
		}
		return ""
	}
	if k := kind(p.Name); k != "" {
		return k
	}
	args := procArgs(p.PID)
	for i, a := range args {
		if i > 2 {
			break
		}
		if k := kind(a); k != "" {
			return k
		}
	}
	return ""
}
