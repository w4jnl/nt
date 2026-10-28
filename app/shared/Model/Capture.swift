// Capture is what the capture panel adds on the Mac (docs/design.md §8, Menu bar and capture),
// and the capture sheet on iPhone and iPad (§9): one line to today's journal through nt a, or a
// todo through nt todo add. On the Mac Return adds it the panel's way, Shift-Return the other;
// the phone has a switch.
import Foundation
import Observation

@MainActor
@Observable
final class Capture {
    enum Mode: Equatable, Sendable {
        case today // a line in today's journal, under Log
        case todo // a todo at the end of Inbox in the root's todo.md
    }

    /// Added is what the panel says once the text is in: where it went, and when or when due.
    struct Added: Equatable, Sendable {
        let place: String // Tuesday 28 October, todo.md
        let detail: String // at 11:42; due fri
        let section: String // Log, Inbox
    }

    var mode = Mode.today
    var text = ""
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
        added = nil
        problem = ""
        shown += 1
    }

    /// target is the file and section a line goes to with Return.
    var target: String {
        mode == .today ? "\(ws.journalPath(ws.today)) · Log" : "todo.md · Inbox"
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
                let m = try await nt.json(Made.self, ["todo", "add", "--json", "--", line])
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
