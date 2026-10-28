// Where a link in a note points (docs/format.md §4, Links): relative to the note, with the
// extension, percent-encoded where needed, an optional #anchor.
import Foundation

enum Links {
    /// Target is what following a link does.
    enum Target: Equatable, Sendable {
        case note(String, anchor: String) // a note in the repository
        case file(String) // another file in the repository: an image, a PDF
        case anchor(String) // a heading in the same note
        case web(URL) // outside the repository
        case missing(String) // a repository path that is not there
        case invalid
    }

    /// resolve is the repository path dest points to from the note at from, without the anchor.
    nonisolated static func resolve(from: String, _ dest: String) -> String? {
        var d = dest
        if d.hasPrefix("<"), d.hasSuffix(">") { d = String(d.dropFirst().dropLast()) }
        if isExternal(d) { return nil }
        var p = String(d.split(separator: "#", maxSplits: 1, omittingEmptySubsequences: false).first ?? "")
        if p.isEmpty { return nil }
        p = p.removingPercentEncoding ?? p
        var parts = from.split(separator: "/").map(String.init).dropLast()
        for seg in p.split(separator: "/").map(String.init) {
            switch seg {
            case "", ".": continue
            case "..":
                if parts.isEmpty { return nil }
                parts.removeLast()
            default: parts.append(seg)
            }
        }
        return parts.joined(separator: "/")
    }

    nonisolated static func isExternal(_ dest: String) -> Bool {
        if dest.hasPrefix("/") { return true }
        guard let colon = dest.firstIndex(of: ":") else { return false }
        let scheme = dest[..<colon]
        return !scheme.isEmpty && scheme.first!.isLetter && scheme.allSatisfy { $0.isLetter || $0.isNumber || "+.-".contains($0) }
    }

    /// target says what following dest from the note at from does; exists checks a repository path.
    nonisolated static func target(from: String, _ dest: String, exists: (String) -> Bool) -> Target {
        if dest.hasPrefix("#") { return .anchor(String(dest.dropFirst())) }
        if isExternal(dest) {
            guard let url = URL(string: dest), let s = url.scheme?.lowercased(), ["http", "https", "mailto"].contains(s) else { return .invalid }
            return .web(url)
        }
        guard let path = resolve(from: from, dest) else { return .invalid }
        let anchor = dest.split(separator: "#", maxSplits: 1).dropFirst().first.map(String.init) ?? ""
        guard exists(path) else { return .missing(path) }
        return path.hasSuffix(".md") ? .note(path, anchor: anchor) : .file(path)
    }
}
