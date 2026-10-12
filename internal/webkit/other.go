//go:build !darwin

package webkit

func SetDataStore(uintptr, string)                            {}
func AddUserScript(uintptr, string, bool, bool, string)       {}
func AddMessageHandler(uintptr, string, string, func(string)) {}
func SetInspectable(uintptr, bool)                            {}
func Above(uintptr, uintptr)                                  {}
func SetDefaultBrowser(string, func(error))                   {}
func IsDefaultBrowser(string) bool                            { return false }

// Supported reports WKWebExtension; never off macOS.
func Supported() bool { return false }
