import CoreGraphics
import Foundation
// usage: keys text <string> | keys code <keycode> [cmd] [shift]
let a = CommandLine.arguments
let src = CGEventSource(stateID: .hidSystemState)
func post(_ code: CGKeyCode, _ flags: CGEventFlags, _ text: String?) {
  for down in [true, false] {
    let e = CGEvent(keyboardEventSource: src, virtualKey: code, keyDown: down)!
    e.flags = flags
    if let t = text { let u = Array(t.utf16); e.keyboardSetUnicodeString(stringLength: u.count, unicodeString: u) }
    e.post(tap: .cghidEventTap)
    usleep(20000)
  }
}
if a[1] == "text" { for ch in a[2] { post(0, [], String(ch)) } }
else {
  var f: CGEventFlags = []
  if a.contains("cmd") { f.insert(.maskCommand) }
  if a.contains("shift") { f.insert(.maskShift) }
  if a.contains("ctrl") { f.insert(.maskControl) }
  if a.contains("alt") { f.insert(.maskAlternate) }
  post(CGKeyCode(Int(a[2])!), f, nil)
  if !f.isEmpty {
    // Posted flags stay in the session's modifier state: release them.
    let r = CGEvent(keyboardEventSource: src, virtualKey: 0x37, keyDown: false)!
    r.type = .flagsChanged; r.flags = []; r.post(tap: .cghidEventTap)
  }
}
