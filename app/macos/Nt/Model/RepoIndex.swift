// The repository as nt _editor index prints it, and the sidebar's tree built from it.
import Foundation

struct RepoIndex: Codable, Sendable, Equatable {
    struct Note: Codable, Sendable, Hashable {
        let path: String
        let title: String
        var created: String? = nil // the frontmatter's created: when an agent report's session began
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
    let date: String // a dated note's day, short: "21 Oct", or an agent report's host; "" otherwise
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
    /// newest first, so a folder's newest note comes first and journals' years run backwards,
    /// after the folders named otherwise, A to Z, as nt lists them.
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
                switch (dated(na), dated(nb)) {
                case (true, true): return na > nb
                case (false, false): return na.localizedStandardCompare(nb) == .orderedAscending
                case (let da, _): return !da // named folders before dated ones
                }
            }
            let agents = !ix.folders.agents.isEmpty && (dir == ix.folders.agents || dir.hasPrefix(ix.folders.agents + "/"))
            let notes = order(notesIn[dir] ?? [], agents: agents).map { noteNode($0, ix: ix, thisYear: thisYear) }
            return TreeNode(id: dir, name: (dir as NSString).lastPathComponent, date: "", children: folders.map(node) + notes)
        }

        let top = (subdirs[""] ?? [])
        var tree = SidebarTree()
        tree.pinned = ix.pins.filter { top.contains($0) }.map(node)
        tree.rest = top.filter { !ix.pins.contains($0) }.sorted { $0.localizedStandardCompare($1) == .orderedAscending }.map(node)
        tree.rootNotes = (notesIn[""] ?? []).sorted { $0.path < $1.path }.map { noteNode($0, ix: ix, thisYear: thisYear) }
        return tree
    }

    /// order is a folder's notes as nt lists them: newest first by name; among the agent reports,
    /// a day's digest first, then the sessions by when they began, the latest first.
    nonisolated static func order(_ notes: [RepoIndex.Note], agents: Bool) -> [RepoIndex.Note] {
        guard agents else { return notes.sorted { $0.path > $1.path } }
        let began = notes.map { ($0, Agenda.instant($0.created ?? "") ?? .distantPast) }
        return began.sorted { a, b in
            let da = (a.0.path as NSString).lastPathComponent == "README.md", db = (b.0.path as NSString).lastPathComponent == "README.md"
            if da != db { return da }
            return a.1 != b.1 ? a.1 > b.1 : a.0.path > b.0.path
        }.map(\.0)
    }

    nonisolated static func noteNode(_ n: RepoIndex.Note, ix: RepoIndex, thisYear: String) -> TreeNode {
        let journal = n.path.hasPrefix(ix.folders.daily + "/")
        let meta = journal ? "" : reportHost(n.path, agents: ix.folders.agents) ?? shortDate(n.path, thisYear: thisYear)
        return TreeNode(id: n.path, name: n.title, date: meta, children: nil)
    }

    /// reportHost is the machine an agent report is from, the first part of its name,
    /// <host>--<agent>--<project>--<sid8>.md, so one project's reports from two machines read
    /// apart; nil for any other note.
    nonisolated static func reportHost(_ path: String, agents: String) -> String? {
        guard !agents.isEmpty, path.hasPrefix(agents + "/") else { return nil }
        let name = (path as NSString).lastPathComponent
        guard let m = name.wholeMatch(of: /([a-z0-9]+(?:-[a-z0-9]+)*)--[a-z0-9-]+--[a-z0-9-]+--[a-z0-9-]+\.md/) else { return nil }
        return String(m.1)
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
