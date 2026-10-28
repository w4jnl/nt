// The repository as nt _editor index prints it, and the sidebar's tree built from it.
import Foundation

struct RepoIndex: Codable, Sendable, Equatable {
    struct Note: Codable, Sendable, Hashable {
        let path: String
        let title: String
    }

    struct Folders: Codable, Sendable, Equatable {
        let daily: String
        let agents: String
        let weekly: String
    }

    let root: String
    let state: String
    let day: String
    let pins: [String]
    let folders: Folders
    let dirs: [String]
    let notes: [Note]
    let back: [String: [String]]

    /// title is a note's title, or its file name for a path the index does not hold.
    func title(_ path: String) -> String {
        titles[path] ?? (path as NSString).lastPathComponent
    }

    var titles: [String: String] {
        Dictionary(notes.map { ($0.path, $0.title) }, uniquingKeysWith: { a, _ in a })
    }
}

/// One row of the sidebar's folder tree.
struct TreeNode: Identifiable, Hashable, Sendable {
    let id: String // the repository path
    let name: String // a folder's name, or a note's title
    let date: String // a dated note's day, short: "21 Oct"; "" otherwise
    var children: [TreeNode]? // nil for a note

    var isFolder: Bool { children != nil }
}

/// The sidebar's folders: the pinned ones first, in their order, then the rest, then the notes at
/// the root.
struct SidebarTree: Equatable, Sendable {
    var pinned: [TreeNode] = []
    var rest: [TreeNode] = []
    var rootNotes: [TreeNode] = []

    /// build lays out the index. assets folders are attachments and hidden; dated names sort
    /// newest first, so a folder's newest note comes first and journals' years run backwards.
    nonisolated static func build(_ ix: RepoIndex, thisYear: String) -> SidebarTree {
        let dirs = ix.dirs.filter { !$0.split(separator: "/").contains("assets") }
        var notesIn: [String: [RepoIndex.Note]] = [:]
        for n in ix.notes {
            let dir = (n.path as NSString).deletingLastPathComponent
            notesIn[dir, default: []].append(n)
        }
        var subdirs: [String: [String]] = [:]
        for d in dirs {
            subdirs[(d as NSString).deletingLastPathComponent, default: []].append(d)
        }

        func node(_ dir: String) -> TreeNode {
            let folders = (subdirs[dir] ?? []).sorted { a, b in
                let na = (a as NSString).lastPathComponent, nb = (b as NSString).lastPathComponent
                return dated(na) && dated(nb) ? na > nb : na.localizedStandardCompare(nb) == .orderedAscending
            }
            let notes = (notesIn[dir] ?? []).sorted { $0.path > $1.path }.map { noteNode($0, ix: ix, thisYear: thisYear) }
            return TreeNode(id: dir, name: (dir as NSString).lastPathComponent, date: "", children: folders.map(node) + notes)
        }

        let top = (subdirs[""] ?? [])
        var tree = SidebarTree()
        tree.pinned = ix.pins.filter { top.contains($0) }.map(node)
        tree.rest = top.filter { !ix.pins.contains($0) }.sorted { $0.localizedStandardCompare($1) == .orderedAscending }.map(node)
        tree.rootNotes = (notesIn[""] ?? []).sorted { $0.path < $1.path }.map { noteNode($0, ix: ix, thisYear: thisYear) }
        return tree
    }

    nonisolated static func noteNode(_ n: RepoIndex.Note, ix: RepoIndex, thisYear: String) -> TreeNode {
        let journal = n.path.hasPrefix(ix.folders.daily + "/")
        return TreeNode(id: n.path, name: n.title, date: journal ? "" : shortDate(n.path, thisYear: thisYear), children: nil)
    }

    /// shortDate is the day in a dated file name, "21 Oct", with the year when it is not this one.
    nonisolated static func shortDate(_ path: String, thisYear: String) -> String {
        let name = (path as NSString).lastPathComponent
        guard dated(name) else { return "" }
        let parts = name.prefix(10).split(separator: "-").map(String.init)
        let months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]
        guard parts.count == 3, let m = Int(parts[1]), (1...12).contains(m), let d = Int(parts[2]) else { return "" }
        return parts[0] == thisYear ? "\(d) \(months[m - 1])" : "\(months[m - 1]) \(parts[0])"
    }

    /// dated says whether a name starts with a date or a year, as journals' folders do.
    nonisolated static func dated(_ name: String) -> Bool {
        let c = Array(name.utf8)
        func digits(_ r: Range<Int>) -> Bool { r.allSatisfy { $0 < c.count && (48...57).contains(c[$0]) } }
        if c.count == 4 { return digits(0..<4) }
        return c.count >= 10 && digits(0..<4) && c[4] == 45 && digits(5..<7) && c[7] == 45 && digits(8..<10)
    }

    /// ancestors are the folders that hold path, outermost first.
    nonisolated static func ancestors(_ path: String) -> [String] {
        var out: [String] = []
        var dir = (path as NSString).deletingLastPathComponent
        while !dir.isEmpty {
            out.insert(dir, at: 0)
            dir = (dir as NSString).deletingLastPathComponent
        }
        return out
    }
}
