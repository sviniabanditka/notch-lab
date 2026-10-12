//go:build !darwin

package webkit

func SetDataStore(uintptr, string)                            {}
func AddUserScript(uintptr, string, bool, bool, string)       {}
func AddMessageHandler(uintptr, string, string, func(string)) {}
func SetInspectable(uintptr, bool)                            {}
func SetDefaultBrowser(string, func(error))                   {}
func IsDefaultBrowser(string) bool                            { return false }
