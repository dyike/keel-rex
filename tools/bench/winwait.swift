import CoreGraphics
import Foundation
// winwait <pid> <timeoutSec>: prints epoch seconds when pid has an on-screen window >= 200x200, and its bounds.
let pid = Int32(CommandLine.arguments[1])!
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
