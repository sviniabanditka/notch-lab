import CoreGraphics
import Foundation
// mouse bounds <pid> (prints: window-number x y w h) | mouse scroll x y lines | mouse drag x1 y1 x2 y2 | mouse click x y | mouse cmdmove|cmdclick x y (Cmd held; cmdmove leaves it held until release) | mouse release
let a = CommandLine.arguments
func f(_ i: Int) -> Double { Double(a[i])! }
// Posted events' flags stay in the session's modifier state: release them,
// or Cmd/Ctrl stays held for every app until a real key clears it.
func release() {
  let e = CGEvent(keyboardEventSource: nil, virtualKey: 0x37, keyDown: false)!; e.type = .flagsChanged; e.flags = []; e.post(tap: .cghidEventTap)
}
switch a[1] {
case "bounds":
  // All windows, not only on-screen ones: a window on another Space is off
  // screen yet takes pid-posted input and window captures.
  let list = CGWindowListCopyWindowInfo([.optionAll], kCGNullWindowID) as! [[String: Any]]
  for w in list where (w[kCGWindowOwnerPID as String] as? Int32) == Int32(a[2])! && (w[kCGWindowLayer as String] as? Int) == 0 {
    let b = w[kCGWindowBounds as String] as! [String: Double]; print(w[kCGWindowNumber as String]!, Int(b["X"]!), Int(b["Y"]!), Int(b["Width"]!), Int(b["Height"]!))
  }
case "scroll":
  CGEvent(mouseEventSource: nil, mouseType: .mouseMoved, mouseCursorPosition: CGPoint(x: f(2), y: f(3)), mouseButton: .left)!.post(tap: .cghidEventTap)
  usleep(50000)
  let n = Int32(a[4])!
  for _ in 0..<abs(n) { CGEvent(scrollWheelEvent2Source: nil, units: .line, wheelCount: 1, wheel1: n > 0 ? 1 : -1, wheel2: 0, wheel3: 0)!.post(tap: .cghidEventTap); usleep(30000) }
case "drag":
  let p1 = CGPoint(x: f(2), y: f(3)), p2 = CGPoint(x: f(4), y: f(5))
  CGEvent(mouseEventSource: nil, mouseType: .mouseMoved, mouseCursorPosition: p1, mouseButton: .left)!.post(tap: .cghidEventTap); usleep(80000)
  CGEvent(mouseEventSource: nil, mouseType: .leftMouseDown, mouseCursorPosition: p1, mouseButton: .left)!.post(tap: .cghidEventTap); usleep(80000)
  for i in 1...10 { let t = Double(i)/10; let p = CGPoint(x: p1.x+(p2.x-p1.x)*t, y: p1.y+(p2.y-p1.y)*t)
    CGEvent(mouseEventSource: nil, mouseType: .leftMouseDragged, mouseCursorPosition: p, mouseButton: .left)!.post(tap: .cghidEventTap); usleep(40000) }
  CGEvent(mouseEventSource: nil, mouseType: .leftMouseUp, mouseCursorPosition: p2, mouseButton: .left)!.post(tap: .cghidEventTap)
case "click":
  let p = CGPoint(x: f(2), y: f(3))
  CGEvent(mouseEventSource: nil, mouseType: .mouseMoved, mouseCursorPosition: p, mouseButton: .left)!.post(tap: .cghidEventTap); usleep(80000)
  CGEvent(mouseEventSource: nil, mouseType: .leftMouseDown, mouseCursorPosition: p, mouseButton: .left)!.post(tap: .cghidEventTap); usleep(50000)
  CGEvent(mouseEventSource: nil, mouseType: .leftMouseUp, mouseCursorPosition: p, mouseButton: .left)!.post(tap: .cghidEventTap)
case "rclick":
  let p = CGPoint(x: f(2), y: f(3))
  CGEvent(mouseEventSource: nil, mouseType: .mouseMoved, mouseCursorPosition: p, mouseButton: .right)!.post(tap: .cghidEventTap); usleep(80000)
  CGEvent(mouseEventSource: nil, mouseType: .rightMouseDown, mouseCursorPosition: p, mouseButton: .right)!.post(tap: .cghidEventTap); usleep(50000)
  CGEvent(mouseEventSource: nil, mouseType: .rightMouseUp, mouseCursorPosition: p, mouseButton: .right)!.post(tap: .cghidEventTap)
case "cclick":
  let p = CGPoint(x: f(2), y: f(3))
  CGEvent(mouseEventSource: nil, mouseType: .mouseMoved, mouseCursorPosition: p, mouseButton: .left)!.post(tap: .cghidEventTap); usleep(80000)
  let d = CGEvent(mouseEventSource: nil, mouseType: .leftMouseDown, mouseCursorPosition: p, mouseButton: .left)!; d.flags = .maskControl; d.post(tap: .cghidEventTap); usleep(50000)
  let u = CGEvent(mouseEventSource: nil, mouseType: .leftMouseUp, mouseCursorPosition: p, mouseButton: .left)!; u.flags = .maskControl; u.post(tap: .cghidEventTap)
  usleep(50000); release()
case "cmdmove":
  let m = CGEvent(mouseEventSource: nil, mouseType: .mouseMoved, mouseCursorPosition: CGPoint(x: f(2), y: f(3)), mouseButton: .left)!; m.flags = .maskCommand; m.post(tap: .cghidEventTap)
case "cmdclick":
  let p = CGPoint(x: f(2), y: f(3))
  let m = CGEvent(mouseEventSource: nil, mouseType: .mouseMoved, mouseCursorPosition: p, mouseButton: .left)!; m.flags = .maskCommand; m.post(tap: .cghidEventTap); usleep(80000)
  let d = CGEvent(mouseEventSource: nil, mouseType: .leftMouseDown, mouseCursorPosition: p, mouseButton: .left)!; d.flags = .maskCommand; d.post(tap: .cghidEventTap); usleep(50000)
  let u = CGEvent(mouseEventSource: nil, mouseType: .leftMouseUp, mouseCursorPosition: p, mouseButton: .left)!; u.flags = .maskCommand; u.post(tap: .cghidEventTap)
  usleep(50000); release()
case "release": release()
case "dclick":
  let p = CGPoint(x: f(2), y: f(3))
  for n in [1, 2] {
    let d = CGEvent(mouseEventSource: nil, mouseType: .leftMouseDown, mouseCursorPosition: p, mouseButton: .left)!; d.setIntegerValueField(.mouseEventClickState, value: Int64(n)); d.post(tap: .cghidEventTap); usleep(30000)
    let u = CGEvent(mouseEventSource: nil, mouseType: .leftMouseUp, mouseCursorPosition: p, mouseButton: .left)!; u.setIntegerValueField(.mouseEventClickState, value: Int64(n)); u.post(tap: .cghidEventTap); usleep(60000)
  }
case "move":
  CGEvent(mouseEventSource: nil, mouseType: .mouseMoved, mouseCursorPosition: CGPoint(x: f(2), y: f(3)), mouseButton: .left)!.post(tap: .cghidEventTap)
default: break
}
