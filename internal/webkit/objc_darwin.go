// Package webkit reaches the WebKit and AppKit calls mygo does not wrap,
// through purego's Objective-C runtime: data stores of Profiles, user
// scripts and message handlers in content worlds, the default browser,
// and (from M4 stage B) WKWebExtension. Main goroutine only, unless noted.
package webkit

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

type id = objc.ID

var (
	selMu  sync.Mutex
	sels   = map[string]objc.SEL{}
	loaded sync.Once
)

func load() {
	loaded.Do(func() {
		for _, p := range []string{
			"/System/Library/Frameworks/Foundation.framework/Foundation",
			"/System/Library/Frameworks/AppKit.framework/AppKit",
			"/System/Library/Frameworks/WebKit.framework/WebKit",
		} {
			_, _ = purego.Dlopen(p, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		}
	})
}

func sel(name string) objc.SEL {
	selMu.Lock()
	defer selMu.Unlock()
	s, ok := sels[name]
	if !ok {
		s = objc.RegisterName(name)
		sels[name] = s
	}
	return s
}

func class(name string) id {
	load()
	return id(objc.GetClass(name))
}

func send(obj id, selector string, args ...any) id {
	if obj == 0 {
		return 0
	}
	return obj.Send(sel(selector), args...)
}

func sendBool(obj id, selector string, args ...any) bool {
	if obj == 0 {
		return false
	}
	return objc.Send[bool](obj, sel(selector), args...)
}

func nsString(s string) id {
	b := append([]byte(s), 0)
	return send(class("NSString"), "stringWithUTF8String:", unsafe.Pointer(&b[0]))
}

func goString(s id) string {
	if s == 0 {
		return ""
	}
	p := objc.Send[*byte](s, sel("UTF8String"))
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

func isKind(obj id, className string) bool {
	return obj != 0 && sendBool(obj, "isKindOfClass:", class(className))
}

// retain and release own an object across calls.
func retain(obj id) id {
	if obj != 0 {
		send(obj, "retain")
	}
	return obj
}

func release(obj id) {
	if obj != 0 {
		send(obj, "release")
	}
}
