// winid <pid> [layer]: the id and frame of the largest window of that process at that window
// layer (0, the default, is a normal window; 101 a menu), as JSON for screencapture -l and -R.
// assets/demo/mac.sh compiles and runs it.
import CoreGraphics
import Foundation

let pid = Int32(CommandLine.arguments[1])!
let layer = CommandLine.arguments.count > 2 ? Int(CommandLine.arguments[2])! : 0
let list = CGWindowListCopyWindowInfo([.optionOnScreenOnly], kCGNullWindowID) as! [[String: Any]]
let wins = list.filter { ($0[kCGWindowOwnerPID as String] as? Int32) == pid && ($0[kCGWindowLayer as String] as? Int) == layer }
let best = wins.max { a, b in
    let ra = a[kCGWindowBounds as String] as! [String: Double], rb = b[kCGWindowBounds as String] as! [String: Double]
    return ra["Width"]! * ra["Height"]! < rb["Width"]! * rb["Height"]!
}
guard let w = best else { exit(1) }
let r = w[kCGWindowBounds as String] as! [String: Double]
print("{\"window\": \(w[kCGWindowNumber as String] as! Int), \"x\": \(Int(r["X"]!)), \"y\": \(Int(r["Y"]!)), \"w\": \(Int(r["Width"]!)), \"h\": \(Int(r["Height"]!))}")
