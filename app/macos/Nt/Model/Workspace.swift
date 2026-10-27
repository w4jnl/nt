// Workspace is the app's state: the repository nt works on, its index, the note on screen, the
// sync state. Files are read here; everything that writes goes through nt.
import AppKit
import Foundation
import Observation

@MainActor
@Observable
final class Workspace {
    enum Phase: Equatable {
        case loading
        case ready
        case problem(String)
    }

    enum Selection: Hashable {
        case today
        case note(String)
    }

    enum PaletteMode: Equatable {
        case open // titles, then text
        case text // text only
    }

    struct Backlink: Equatable, Sendable {
        let path: String
        let title: String
        let folder: String
    }

    /// The note on screen. version changes whenever the editor must load it again.
    struct OpenNote: Equatable {
        let path: String
        var text: String
        var backlinks: [Backlink]
        var anchor: String
        var line: Int // a line to scroll to, from a search; 0 for none
        var version: Int
    }

    struct SearchHit: Decodable, Equatable, Sendable {
        let path: String
        let line: Int
        let text: String
    }

    var phase: Phase = .loading
    var index: RepoIndex?
    var tree = SidebarTree()
    var selection: Selection?
    var note: OpenNote?
    var todayPath = ""
    var status: SyncStatus?
    var syncing = false
    var palette: PaletteMode?
    var flash = "" // one line for the person: what went wrong, or where a link pointed
    var words = 0
    var anchorRequest = (anchor: "", seq: 0)
    var expanded: Set<String> = Set(UserDefaults.standard.stringArray(forKey: "expandedFolders") ?? []) {
        didSet { UserDefaults.standard.set(Array(expanded), forKey: "expandedFolders") }
    }

    @ObservationIgnored let nt: NT?
    @ObservationIgnored private var watcher: RepoWatcher?
    @ObservationIgnored private var reindexTask: Task<Void, Never>?
    @ObservationIgnored private var flashTask: Task<Void, Never>?
    @ObservationIgnored private var version = 0

    init(nt: NT? = NT.locate()) {
        self.nt = nt
    }

    var root: URL? { index.map { URL(filePath: $0.root) } }

    /// start finds the repository and opens today. Under tests it does nothing: the app must not
    /// touch the person's notes there.
    func start() async {
        if ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] != nil { return }
        guard let nt else {
            phase = .problem("nt is not installed. Install it with brew install w4jnl/tap/nt, or set NT_BIN to its path.")
            return
        }
        do {
            try await reindex(nt)
        } catch {
            phase = .problem(error.localizedDescription)
            return
        }
        phase = .ready
        watch()
        await openToday()
        // For screenshots: -NTOpen <repository path> opens a note after today, -NTPalette open
        // or text opens the palette (and -NTQuery types into it).
        if let path = UserDefaults.standard.string(forKey: "NTOpen") { show(path) }
        if let mode = UserDefaults.standard.string(forKey: "NTPalette") { palette = mode == "text" ? .text : .open }
    }

    func reindex(_ nt: NT) async throws {
        let ix = try await nt.json(RepoIndex.self, ["_editor", "index"])
        index = ix
        tree = SidebarTree.build(ix, thisYear: String(ix.day.prefix(4)))
        status = SyncStatus.read(stateDir: ix.state)
        if var n = note {
            let back = backlinks(n.path)
            if back != n.backlinks {
                n.backlinks = back
                n.version = next()
                note = n
            }
        }
    }

    // MARK: Opening notes

    func select(_ sel: Selection?) {
        guard let sel, sel != selection else { return }
        switch sel {
        case .today: Task { await openToday() }
        case .note(let path): show(path)
        }
    }

    func openToday() async {
        guard let nt else { return }
        struct Today: Decodable { let rel: String }
        do {
            let t = try await nt.json(Today.self, ["t", "--json"])
            todayPath = t.rel
            show(t.rel)
        } catch {
            say(error.localizedDescription)
        }
    }

    /// show puts the note at path on screen, scrolled to a heading's anchor or to a line.
    func show(_ path: String, anchor: String = "", line: Int = 0) {
        guard let root else { return }
        let url = root.appending(path: path)
        guard let text = try? String(contentsOf: url, encoding: .utf8) else {
            say("\(path) cannot be read: is it still there?")
            return
        }
        note = OpenNote(path: path, text: text, backlinks: backlinks(path), anchor: anchor, line: line, version: next())
        selection = path == todayPath ? .today : .note(path)
        for dir in SidebarTree.ancestors(path) where !expanded.contains(dir) {
            expanded.insert(dir)
        }
    }

    func backlinks(_ path: String) -> [Backlink] {
        guard let ix = index else { return [] }
        return (ix.back[path] ?? []).map { from in
            let dir = (from as NSString).deletingLastPathComponent
            return Backlink(path: from, title: ix.title(from), folder: dir.isEmpty ? "/" : dir)
        }
    }

    /// follow follows a link the person clicked in the note on screen.
    func follow(_ href: String) {
        guard let n = note, let root else { return }
        let target = Links.target(from: n.path, href) { FileManager.default.fileExists(atPath: root.appending(path: $0).path) }
        switch target {
        case .note(let path, let anchor):
            if path == n.path, !anchor.isEmpty {
                scroll(to: anchor)
            } else {
                show(path, anchor: anchor)
            }
        case .anchor(let a): scroll(to: a)
        case .file(let path): NSWorkspace.shared.open(root.appending(path: path))
        case .web(let url): NSWorkspace.shared.open(url)
        case .missing(let path): say("\(path) is not in the repository")
        case .invalid: say("\(href) is not a link nt can follow")
        }
    }

    func scroll(to anchor: String) {
        anchorRequest = (anchor, anchorRequest.seq + 1)
    }

    /// broken are the link destinations of the note on screen that point at nothing.
    func broken(_ dests: [String]) -> [String] {
        guard let n = note, let root else { return [] }
        return dests.filter { d in
            guard let p = Links.resolve(from: n.path, d) else { return false }
            return !FileManager.default.fileExists(atPath: root.appending(path: p).path)
        }
    }

    // MARK: Search

    func search(_ query: String) async -> [SearchHit] {
        guard let nt, !query.trimmingCharacters(in: .whitespaces).isEmpty else { return [] }
        guard let out = try? await nt.run(["s", "--json", query]), out.status == 0 else { return [] }
        return (try? JSONDecoder().decode([SearchHit].self, from: out.stdout)) ?? []
    }

    // MARK: Sync

    func syncNow() async {
        guard let nt, !syncing else { return }
        syncing = true
        let out = try? await nt.run(["sync", "--json"])
        syncing = false
        if let ix = index { status = SyncStatus.read(stateDir: ix.state) }
        if let out, out.status != 0 { say(NTError.message(out.stderr)) }
    }

    // MARK: Changes on disk

    private func watch() {
        guard let ix = index else { return }
        let root = ix.root, state = ix.state
        watcher = RepoWatcher(paths: [root, state]) { [weak self] paths in
            Task { @MainActor in self?.changed(paths, root: root, state: state) }
        }
    }

    private func changed(_ paths: [String], root: String, state: String) {
        if paths.contains(where: { $0.hasPrefix(state + "/") }) {
            status = SyncStatus.read(stateDir: state)
        }
        guard paths.contains(where: { RepoWatcher.relevant($0, root: root) }) else { return }
        reindexTask?.cancel()
        reindexTask = Task {
            try? await Task.sleep(for: .milliseconds(250))
            guard !Task.isCancelled, let nt else { return }
            try? await reindex(nt)
            reloadIfChanged()
        }
    }

    /// reloadIfChanged shows the note on screen again when its file changed under it.
    private func reloadIfChanged() {
        guard var n = note, let root else { return }
        guard let text = try? String(contentsOf: root.appending(path: n.path), encoding: .utf8) else {
            say("\(n.path) was moved or deleted")
            return
        }
        guard text != n.text else { return }
        n.text = text
        n.anchor = ""
        n.line = 0
        n.version = next()
        note = n
    }

    // MARK: Messages

    func say(_ msg: String) {
        flash = msg
        flashTask?.cancel()
        flashTask = Task {
            try? await Task.sleep(for: .seconds(4))
            if !Task.isCancelled { flash = "" }
        }
    }

    private func next() -> Int {
        version += 1
        return version
    }
}
