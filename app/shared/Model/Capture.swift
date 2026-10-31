// Capture is what the capture panel adds on the Mac (docs/design.md §8, Menu bar and capture),
// and the capture sheet on iPhone and iPad (§9): one line to today's journal through nt a, or a
// todo through nt todo add. On the Mac Return adds it the panel's way, Shift-Return the other,
// and Tab picks the folder a todo goes to; the phone has a switch.
import Foundation
import Observation

@MainActor
@Observable
final class Capture {
    enum Mode: Equatable, Sendable {
        case today // a line in today's journal, under Log
        case todo // a todo at the end of Inbox in the root's todo.md, or folder's
    }

    /// Added is what the panel says once the text is in: where it went, and when or when due.
    struct Added: Equatable, Sendable {
        let place: String // Tuesday 28 October, todo.md
        let detail: String // at 11:42; due fri
        let section: String // Log, Inbox

        /// said is the line the phone says it with: the capture sheet's flash, a Shortcut's answer.
        var said: String { "Added to \(place) · \(section)" + (detail.isEmpty ? "" : ", \(detail)") }
    }

    var mode = Mode.today
    var text = ""
    var folder = "" // the folder whose todo.md a todo goes to; "" the root's
    var picking = false // the folder list is open, query typed into it, pick chosen
    var query = ""
    var pick = 0
    var added: Added?
    var problem = ""
    private(set) var busy = false
    private(set) var shown = 0 // counts the times the panel opened, so its field takes the keys again

    @ObservationIgnored private unowned let ws: Workspace
    #if os(macOS)
    @ObservationIgnored weak var panel: CapturePanel?
    #endif

    init(workspace: Workspace) {
        ws = workspace
    }

    #if os(macOS)
    /// open shows the panel, from the menu bar or the shortcut.
    func open(_ mode: Mode) {
        panel?.show(mode)
    }
    #endif

    func began(_ mode: Mode) {
        self.mode = mode
        folder = ""
        closePlaces()
        added = nil
        problem = ""
        shown += 1
    }

    /// target is the file and section a line goes to with Return; while the folder list is
    /// open, the todo.md of the folder chosen in it.
    var target: String {
        if picking {
            let p = places
            return p.indices.contains(pick) ? "\(p[pick].isEmpty ? "" : p[pick] + "/")todo.md · Inbox" : ""
        }
        return mode == .today ? "\(ws.journalPath(ws.today)) · Log" : "\(folder.isEmpty ? "" : folder + "/")todo.md · Inbox"
    }

    // MARK: The folder a todo goes to

    /// places are the folders the list offers for query: while nothing is typed the root first,
    /// then every folder that takes todos; else the folders that match, the best first.
    var places: [String] {
        Self.places(query, in: (ws.index?.dirs ?? []).filter(ws.placeable))
    }

    nonisolated static func places(_ query: String, in dirs: [String]) -> [String] {
        let q = query.trimmingCharacters(in: .whitespaces)
        if q.isEmpty { return [""] + dirs }
        let hits = dirs.compactMap { d in Fuzzy.match(q, d).map { (d, $0.score) } }
        return hits.sorted { $0.1 != $1.1 ? $0.1 > $1.1 : $0.0 < $1.0 }.map(\.0)
    }

    /// open is how many todos are open in a folder's todo.md, the root's for "".
    func open(in place: String) -> Int {
        let file = (place.isEmpty ? "" : place + "/") + "todo.md"
        return ws.todos.count { !$0.done && $0.rel == file }
    }

    /// openPlaces opens the folder list at the folder chosen so far.
    func openPlaces() {
        query = ""
        pick = places.firstIndex(of: folder) ?? 0
        picking = true
    }

    /// type narrows the folder list with what was typed; deleteBack takes the last letter off.
    func type(_ s: String) {
        query += s
        pick = 0
    }

    func deleteBack() {
        query = String(query.dropLast())
        pick = 0
    }

    /// movePick moves the choice in the folder list by one row up or down.
    func movePick(_ by: Int) {
        pick = min(max(0, pick + by), max(0, places.count - 1))
    }

    /// choose takes the folder chosen and closes the list: the text is a todo for its todo.md.
    func choose() {
        let p = places
        if p.indices.contains(pick) {
            folder = p[pick]
            mode = .todo
        }
        closePlaces()
    }

    /// closePlaces closes the folder list; the folder stays as it was.
    func closePlaces() {
        picking = false
        query = ""
        pick = 0
    }

    /// submit adds the text: the panel's way, or the other with alternate (Shift-Return). It
    /// reports whether it did; when nt cannot, problem says why and the text stays.
    func submit(alternate: Bool) async -> Bool {
        let line = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !line.isEmpty, !busy, let nt = ws.nt else { return false }
        let todo = (mode == .todo) != alternate
        busy = true
        defer { busy = false }
        do {
            if todo {
                struct Made: Decodable { let rel: String; let when: String? }
                let m = try await nt.json(Made.self, ["todo", "add", "--json"] + (folder.isEmpty ? [] : ["--in", folder]) + ["--", line])
                let when = m.when ?? ""
                added = Added(place: m.rel, detail: when.isEmpty ? "" : "due \(when)", section: "Inbox")
                await ws.loadTodos()
            } else {
                struct Made: Decodable { let rel: String }
                let m = try await nt.json(Made.self, ["a", "--json", "--", line])
                let day = String((m.rel as NSString).lastPathComponent.prefix(10))
                added = Added(place: Agenda.long(day, thisYear: String(ws.today.prefix(4))), detail: "at \(Self.clock(.now))", section: "Log")
            }
            text = ""
            problem = ""
            #if !os(macOS)
            await ws.wrote()
            #endif
            return true
        } catch {
            problem = error.localizedDescription
            return false
        }
    }

    nonisolated static func clock(_ d: Date) -> String {
        let f = DateFormatter()
        f.dateFormat = "HH:mm"
        return f.string(from: d)
    }
}
