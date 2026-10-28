// Writing end to end: the editor page in a web view, offscreen, the nt bundled in the app and a
// scratch repository. What the person writes is saved under the rules of docs/format.md §10:
// .git/nt-edit held while unsaved, the write compared with the file the editor read, a change
// another writer made laid under the person's, a clash kept as a conflict copy.
import Foundation
import Testing
import WebKit
@testable import Nt

@MainActor
@Suite(.serialized)
struct WritingTests {
    let root: URL
    let ws: Workspace
    let note = "homelab/2026-10-21-nas.md"

    init() throws {
        let base = FileManager.default.temporaryDirectory.appending(path: "nt-writing-\(UUID().uuidString)")
        root = base.appending(path: "notes")
        try FileManager.default.createDirectory(at: root.appending(path: ".git"), withIntermediateDirectories: true)
        try FileManager.default.createDirectory(at: root.appending(path: "homelab"), withIntermediateDirectories: true)
        let config = base.appending(path: "config.toml")
        try "repo = \"\(root.path)\"\nhost = \"mbp\"\n[sync]\nauto = false\n".write(to: config, atomically: true, encoding: .utf8)
        let bundled = try #require(NT.locate(), "the app bundles nt")
        var env = bundled.environment
        env["NT_CONFIG"] = config.path
        env["NT_STATE"] = base.appending(path: "state").path
        env["NT_REPO"] = root.path
        ws = Workspace(nt: NT(executable: bundled.executable, environment: env))
    }

    // MARK: Helpers

    func write(_ text: String, to path: String? = nil) throws {
        try Data(text.utf8).write(to: root.appending(path: path ?? note), options: .atomic)
    }

    func read(_ path: String? = nil) -> String {
        (try? String(contentsOf: root.appending(path: path ?? note), encoding: .utf8)) ?? ""
    }

    /// held says whether something holds .git/nt-edit, as sync asks it.
    func held() -> Bool {
        let fd = open(root.appending(path: ".git/nt-edit").path, O_RDWR | O_CREAT, 0o644)
        defer { close(fd) }
        if flock(fd, LOCK_EX | LOCK_NB) == 0 {
            flock(fd, LOCK_UN)
            return false
        }
        return true
    }

    func until(_ what: String, _ ok: () async -> Bool) async throws {
        let end = ContinuousClock.now + .seconds(15)
        while ContinuousClock.now < end {
            if await ok() { return }
            try await Task.sleep(for: .milliseconds(50))
        }
        Issue.record("timed out waiting for \(what)")
        throw CancellationError()
    }

    func page(_ js: String, _ args: [String: Any] = [:]) async throws {
        _ = try await ws.editor?.web.callAsyncJavaScript(js, arguments: args, contentWorld: .page)
    }

    /// opened writes the note, opens it in the editor and waits for the page.
    func opened(_ text: String) async throws -> NoteEditor {
        try write(text)
        #expect(await ws.prepare())
        let editor = try #require(ws.editor)
        try await until("the page") { editor.ready }
        await ws.open(note)
        #expect(ws.note?.sha == sha256(Data(text.utf8)))
        return editor
    }

    // MARK: Tests

    @Test func typingIsSavedWhileNtEditIsHeld() async throws {
        let editor = try await opened("# NAS\n\nFour disks.\n")
        // a line typed after "Four disks."
        try await page("window.nt.insertImage(p, l)", ["p": 15, "l": "Nightly backups."])
        try await until("unsaved changes") { editor.dirty }
        try await until("nt to hold .git/nt-edit") { held() || !editor.dirty }
        #expect(held())
        try await until("the save") { read() == "# NAS\n\nFour disks.\nNightly backups.\n" }
        try await until("the hold to go") { !editor.dirty && !held() }
        #expect(ws.note?.sha == sha256(Data(read().utf8)))
        let left = try FileManager.default.contentsOfDirectory(atPath: root.appending(path: "homelab").path)
        #expect(!left.contains { $0.hasPrefix(".nt-tmp-") })
    }

    @Test func aLineAddedOnDiskIsKeptWithTheTyping() async throws {
        _ = try await opened("# NAS\n\nFour disks.\n")
        try await page("window.nt.insertImage(p, l)", ["p": 0, "l": "Mine."])
        // nt a, or a sync, adds a line before the editor saves
        try write("# NAS\n\nFour disks.\n- 09:12 fan\n")
        // nt writes the file before the app has its reply, so wait for both
        try await until("the merge") { read() == "# NAS\nMine.\n\nFour disks.\n- 09:12 fan\n" && ws.note?.sha == sha256(Data(read().utf8)) }
        #expect(ws.flash.isEmpty)
    }

    @Test func theSameLineChangedOnDiskBecomesAConflictCopy() async throws {
        _ = try await opened("- [ ] fan\n- [ ] disks\n")
        try await page("window.nt.toggle(n, d)", ["n": 1, "d": "2026-10-28"])
        try write("- [ ] fan in bay 2\n- [ ] disks\n")
        try await until("the person's version") { read() == "- [x] fan done:2026-10-28\n- [ ] disks\n" && !ws.flash.isEmpty }
        let names = try FileManager.default.contentsOfDirectory(atPath: root.appending(path: "homelab").path)
        let copy = try #require(names.first { $0.hasPrefix("2026-10-21-nas.conflict-mbp-") })
        #expect(read("homelab/" + copy) == "- [ ] fan in bay 2\n- [ ] disks\n")
        #expect(ws.flash.contains(copy))
    }

    @Test func aChangeOnDiskReachesAnUnchangedNote() async throws {
        let editor = try await opened("# NAS\n")
        try write("# NAS\n\nFrom the other Mac.\n")
        await ws.reloadIfChanged()
        #expect(ws.note?.sha == sha256(Data(read().utf8)))
        #expect(!editor.dirty)
        // what is typed next is laid on that version
        try await page("window.nt.insertImage(p, l)", ["p": 0, "l": "Mine."])
        try await until("the save") { read() == "# NAS\nMine.\n\nFrom the other Mac.\n" }
    }

    @Test func openingAnotherNoteSavesFirst() async throws {
        try write("# Other\n", to: "homelab/2026-10-22-other.md")
        _ = try await opened("# NAS\n")
        try await page("window.nt.insertImage(p, l)", ["p": 0, "l": "Mine."])
        await ws.open("homelab/2026-10-22-other.md")
        #expect(read() == "# NAS\nMine.\n")
        #expect(ws.note?.path == "homelab/2026-10-22-other.md")
    }

    @Test func aPastedImageGoesBesideTheNoteOnALineOfItsOwn() async throws {
        _ = try await opened("# NAS\n\nFour disks.\n")
        let png = Data([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A])
        await ws.editor?.paste(name: "Rack front.png", mime: "image/png", data: png, at: 15)
        let day = await ws.day()
        let image = "homelab/assets/\(day)-rack-front.png"
        #expect(FileManager.default.fileExists(atPath: root.appending(path: image).path))
        try await until("the line saved") { read() == "# NAS\n\nFour disks.\n![Rack front](assets/\(day)-rack-front.png)\n" }
    }

    @Test func undoGoesToTheEditorsHistory() async throws {
        let editor = try await opened("# NAS\n")
        try await page("window.nt.insertImage(p, l)", ["p": 0, "l": "Mine."])
        try await until("the save") { read() == "# NAS\nMine.\n" }
        editor.web.undo(nil)
        try await until("the undo saved") { read() == "# NAS\n" }
        editor.web.redo(nil)
        try await until("the redo saved") { read() == "# NAS\nMine.\n" }
    }

    @Test func quittingSavesFirst() async throws {
        _ = try await opened("# NAS\n")
        try await page("window.nt.insertImage(p, l)", ["p": 0, "l": "Mine."])
        #expect(await ws.finish())
        #expect(read() == "# NAS\nMine.\n")
    }

    @Test func aDigestIsReadOnly() async throws {
        let digest = "---\ntitle: Week 44\ntype: weekly\n---\n\n# Week 44\n"
        let editor = try await opened(digest)
        #expect(ws.note?.readOnly == true)
        try await page("window.nt.insertImage(p, l)", ["p": 0, "l": "Mine."])
        try await page("window.nt.toggle(n, d)", ["n": 1, "d": "2026-10-28"])
        try await Task.sleep(for: .milliseconds(300))
        #expect(!editor.dirty)
        #expect(read() == digest)
    }
}

struct WritingModelTests {
    @Test func readOnlyIsForDigests() {
        #expect(Workspace.readOnly("---\ntitle: Agents\ntype: digest\n---\n"))
        #expect(Workspace.readOnly("---\ntype: weekly\n---\n# Week 44\n"))
        #expect(!Workspace.readOnly("---\ntype: weekly-summary\nhost: mbp\n---\n"))
        #expect(!Workspace.readOnly("---\ntitle: NAS\n---\n\ntype: digest\n"))
        #expect(!Workspace.readOnly("# Notes\n\ntype: digest\n"))
    }

    @Test func newNotesGoBesideTheOneOnScreen() {
        let f = RepoIndex.Folders(daily: "daily", agents: "agents", weekly: "weekly")
        #expect(Workspace.newFolder(for: "homelab/2026-10-21-nas.md", folders: f) == "homelab")
        #expect(Workspace.newFolder(for: "work/q3/2026-10-01-plan.md", folders: f) == "work/q3")
        #expect(Workspace.newFolder(for: "daily/2026/2026-10-28.md", folders: f) == "")
        #expect(Workspace.newFolder(for: "agents/2026/2026-10-28/README.md", folders: f) == "")
        #expect(Workspace.newFolder(for: "todo.md", folders: f) == "")
        #expect(Workspace.newFolder(for: "dailyish/2026-10-28-x.md", folders: f) == "dailyish")
    }

    @Test func pastedImagesKeepTheirNames() {
        #expect(NoteEditor.fileName("Rack photo.jpeg", mime: "image/jpeg") == "Rack photo.jpeg")
        #expect(NoteEditor.fileName("image", mime: "image/png") == "image.png")
        #expect(NoteEditor.fileName("../../etc/x.png", mime: "image/png") == "x.png")
        #expect(NoteEditor.fileName("", mime: "image/gif") == "image.gif")
        #expect(NoteEditor.fileName(".png", mime: "image/png") == "image.png")
    }

    @Test func saveRepliesDecode() throws {
        let saved = try JSONDecoder().decode(NoteEditor.SaveReply.self, from: Data(#"{"saved": true, "sha": "ab", "copy": "x.conflict-mbp-1.md"}"#.utf8))
        #expect(saved == NoteEditor.SaveReply(saved: true, sha: "ab", text: nil, copy: "x.conflict-mbp-1.md"))
        let not = try JSONDecoder().decode(NoteEditor.SaveReply.self, from: Data(##"{"saved": false, "sha": "cd", "text": "# NAS\n"}"##.utf8))
        #expect(!not.saved && not.text == "# NAS\n")
    }

    @Test func sha256IsNtsHex() {
        #expect(sha256(Data("abc".utf8)) == "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
    }
}
