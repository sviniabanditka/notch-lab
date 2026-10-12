import AppKit
import CoreGraphics
// Background input: every event goes to one pid with CGEventPostToPid, so the
// app is never activated, the system cursor never moves, and no modifier
// enters the session's flag state.
// usage: bg state | bg front (prints the frontmost app's pid)
//        bg <pid> <window-number> key CODE [cmd shift ctrl alt] | text S | release
//        bg <pid> <window-number> click|rclick|dclick|cmdclick|cclick|move|cmdmove X Y | scroll X Y N | drag X1 Y1 X2 Y2
// Points are window points from the window's top-left, as hg takes them.
let a = CommandLine.arguments
if a.count == 2 && a[1] == "state" {
  let f = NSWorkspace.shared.frontmostApplication
  let p = CGEvent(source: nil)!.location
  print("cursor \(Int(p.x)),\(Int(p.y)) front \(f?.processIdentifier ?? 0) \(f?.localizedName ?? "?") flags session=\(CGEventSource.flagsState(.combinedSessionState).rawValue) hid=\(CGEventSource.flagsState(.hidSystemState).rawValue)")
  exit(0)
}
if a.count == 2 && a[1] == "front" { print(NSWorkspace.shared.frontmostApplication?.processIdentifier ?? 0); exit(0) }
guard a.count >= 4, let pid = pid_t(a[1]), let win = Int64(a[2]) else { fputs("usage: bg <pid> <window-number> OP ARGS | bg state\n", stderr); exit(2) }
// Refuse any pid that is not the dev app nl names in NL_APP_EXE: a wrong pid
// would type into the user's own work.
let want = ProcessInfo.processInfo.environment["NL_APP_EXE"] ?? ""
guard !want.isEmpty, let exe = NSRunningApplication(processIdentifier: pid)?.executableURL?.path, exe == want else {
  fputs("bg: pid \(pid) is not the dev app (NL_APP_EXE); nothing sent\n", stderr); exit(1)
}
let src = CGEventSource(stateID: .privateState)
let op = a[3]
func num(_ i: Int) -> Double { Double(a[i])! }
// Field 51 is the window an event belongs to (what NSEvent.cgEvent writes for
// windowNumber); without it AppKit drops a pid-posted mouseDown.
func target(_ e: CGEvent) { e.setIntegerValueField(CGEventField(rawValue: 51)!, value: win) }
func send(_ e: CGEvent) { e.postToPid(pid); usleep(15000) }
func mods(_ words: ArraySlice<String>) -> CGEventFlags {
  var f: CGEventFlags = []
  if words.contains("cmd") { f.insert(.maskCommand) }
  if words.contains("shift") { f.insert(.maskShift) }
  if words.contains("ctrl") { f.insert(.maskControl) }
  if words.contains("alt") { f.insert(.maskAlternate) }
  return f
}
func key(_ code: CGKeyCode, _ flags: CGEventFlags, _ text: String?) {
  for down in [true, false] {
    let e = CGEvent(keyboardEventSource: src, virtualKey: code, keyDown: down)!
    e.flags = flags
    target(e)
    if let t = text { let u = Array(t.utf16); e.keyboardSetUnicodeString(stringLength: u.count, unicodeString: u) }
    send(e)
  }
}
// AppKit takes a pid-posted mouse event's position in the window from the
// event's window location (WindowServer fills it for routed events, postToPid
// does not); the global position is only kept consistent.
typealias SetWindowLocation = @convention(c) (CGEvent, CGPoint) -> Void
let setWindowLocation = unsafeBitCast(dlsym(UnsafeMutableRawPointer(bitPattern: -2), "CGEventSetWindowLocation")!, to: SetWindowLocation.self)
let origin: CGPoint = {
  let list = CGWindowListCopyWindowInfo([.optionIncludingWindow], CGWindowID(win)) as? [[String: Any]] ?? []
  guard let b = list.first?[kCGWindowBounds as String] as? [String: Double] else { fputs("bg: no window \(win)\n", stderr); exit(1) }
  return CGPoint(x: b["X"]!, y: b["Y"]!)
}()
func located(_ e: CGEvent, _ p: CGPoint) {
  e.location = CGPoint(x: origin.x + p.x, y: origin.y + p.y)
  setWindowLocation(e, p)
  target(e)
}
func mouse(_ type: CGEventType, _ p: CGPoint, _ button: CGMouseButton = .left, _ flags: CGEventFlags = [], clicks: Int64 = 1) {
  let e = CGEvent(mouseEventSource: src, mouseType: type, mouseCursorPosition: .zero, mouseButton: button)!
  e.flags = flags
  e.setIntegerValueField(.mouseEventClickState, value: clicks)
  located(e, p)
  send(e)
}
// A pid-posted mouseMoved does not reach tracking areas of an inactive app
// (mygo's are active-app only), so hover rides on a zero-delta scroll, which
// moves the pointer like any scroll.
func hover(_ p: CGPoint, _ flags: CGEventFlags) {
  mouse(.mouseMoved, p, .left, flags)
  let e = CGEvent(scrollWheelEvent2Source: src, units: .pixel, wheelCount: 1, wheel1: 0, wheel2: 0, wheel3: 0)!
  e.flags = flags
  located(e, p)
  send(e)
}
// Held modifiers reach the app as flagsChanged posted to it alone; the
// session's modifier state never sees them.
func modifiers(_ flags: CGEventFlags) {
  let e = CGEvent(keyboardEventSource: src, virtualKey: 0x37, keyDown: !flags.isEmpty)!
  e.type = .flagsChanged; e.flags = flags; target(e); send(e)
}
func click(_ p: CGPoint, _ down: CGEventType, _ up: CGEventType, _ button: CGMouseButton, _ flags: CGEventFlags, clicks: Int64 = 1) {
  mouse(down, p, button, flags, clicks: clicks); usleep(30000)
  mouse(up, p, button, flags, clicks: clicks)
}
switch op {
case "key": key(CGKeyCode(Int(a[4])!), mods(a[5...]), nil)
case "release": modifiers([])
case "text": for ch in a[4] { key(0, [], String(ch)) }
default:
  let p = CGPoint(x: num(4), y: num(5))
  switch op {
  case "move": hover(p, [])
  case "cmdmove": modifiers(.maskCommand); hover(p, .maskCommand)
  case "click": mouse(.mouseMoved, p); click(p, .leftMouseDown, .leftMouseUp, .left, [])
  case "cmdclick": mouse(.mouseMoved, p, .left, .maskCommand); click(p, .leftMouseDown, .leftMouseUp, .left, .maskCommand)
  case "cclick": mouse(.mouseMoved, p); click(p, .leftMouseDown, .leftMouseUp, .left, .maskControl)
  case "rclick": mouse(.mouseMoved, p); click(p, .rightMouseDown, .rightMouseUp, .right, [])
  case "dclick": mouse(.mouseMoved, p); for n in Int64(1)...2 { click(p, .leftMouseDown, .leftMouseUp, .left, [], clicks: n); usleep(40000) }
  case "scroll":
    mouse(.mouseMoved, p)
    let n = Int32(a[6])!
    for _ in 0..<abs(n) {
      let e = CGEvent(scrollWheelEvent2Source: src, units: .line, wheelCount: 1, wheel1: n > 0 ? 1 : -1, wheel2: 0, wheel3: 0)!
      located(e, p)
      send(e); usleep(20000)
    }
  case "drag":
    let q = CGPoint(x: num(6), y: num(7))
    mouse(.mouseMoved, p); mouse(.leftMouseDown, p)
    for i in 1...10 { let t = Double(i) / 10; mouse(.leftMouseDragged, CGPoint(x: p.x + (q.x - p.x) * t, y: p.y + (q.y - p.y) * t)); usleep(25000) }
    mouse(.leftMouseUp, q)
  default: fputs("bg: unknown op \(op)\n", stderr); exit(2)
  }
}
