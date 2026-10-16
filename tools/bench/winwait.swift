import CoreGraphics
import Foundation
import AppKit
// winwait <pid> <timeoutSec>: prints epoch seconds when pid has an on-screen window >= 200x200, and its bounds.
let pid = Int32(CommandLine.arguments[1])!
if CommandLine.arguments.count > 2 && CommandLine.arguments[2] == "info" {
  let list = CGWindowListCopyWindowInfo([.optionOnScreenOnly], kCGNullWindowID) as? [[String: Any]] ?? []
  let bounds = list.filter { ($0[kCGWindowOwnerPID as String] as? Int32) == pid }
    .compactMap { $0[kCGWindowBounds as String] as? [String: Double] }
    .filter { ($0["Width"] ?? 0) >= 200 && ($0["Height"] ?? 0) >= 200 }
  let data: [String: Any] = ["frontmost_pid": NSWorkspace.shared.frontmostApplication?.processIdentifier ?? 0,
                             "bounds": bounds, "screen_scales": NSScreen.screens.map { $0.backingScaleFactor }]
  print(String(data: try! JSONSerialization.data(withJSONObject: data), encoding: .utf8)!)
  exit(0)
}
let timeout = Double(CommandLine.arguments[2]) ?? 15
let start = Date()
while Date().timeIntervalSince(start) < timeout {
  if let list = CGWindowListCopyWindowInfo([.optionOnScreenOnly], kCGNullWindowID) as? [[String: Any]] {
    for w in list where (w[kCGWindowOwnerPID as String] as? Int32) == pid {
      if let b = w[kCGWindowBounds as String] as? [String: Double], (b["Width"] ?? 0) >= 200, (b["Height"] ?? 0) >= 200 {
        print(String(format: "%.6f %.0f %.0f", Date().timeIntervalSince1970, b["Width"]!, b["Height"]!)); exit(0)
      }
    }
  }
  usleep(5000)
}
print("timeout"); exit(1)
