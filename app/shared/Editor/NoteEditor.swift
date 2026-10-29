// NoteEditor is the note's editor page (Resources/editor, CodeMirror 6 with nt's live preview) in
// a web view, and what keeps what the person writes saved. It lives as long as the app, so
// closing the window loses nothing.
//
// Saving follows docs/format.md §10 as the nvim plugin does. While the note has unsaved changes
// nt holds .git/nt-edit (Hold), so sync commits but does not merge under them. A second after the
// typing stops, before another note opens, when the app goes to the background and before it
// quits, the page's text goes to `nt _editor save` with the SHA-256 of the file it was read
// from: nt writes it under the repository lock through a temp file, or, when another writer
// changed the file since, hands it back for the page to lay under the person's changes. When
// both changed the same line, the person's text is saved and the other version kept as a
// conflict copy. Once saved the hold goes, and half a minute later, or when the person leaves
// the app, `nt _editor kick` starts a sync.
import CryptoKit
import SwiftUI
import UniformTypeIdentifiers
import WebKit

@MainActor
final class NoteEditor: NSObject, WKScriptMessageHandler, WKNavigationDelegate {
    let web: NoteWebView
    private unowned let ws: Workspace
    private(set) var ready = false
    private var waiting: [() -> Void] = [] // calls made before the page was ready

    private(set) var dirty = false
    private var hold: Hold?
    private var saving = false
    private var again = false
    private var keep = false // the next save keeps the version on disk as a conflict copy
    private var waiters: [CheckedContinuation<Void, Never>] = []
    private var saveSoon: Task<Void, Never>?
    private var kickSoon: Task<Void, Never>?
    private var wrote = false // saved since the last kick

    init(workspace: Workspace) {
        ws = workspace
        let config = WKWebViewConfiguration()
        config.setURLSchemeHandler(AssetHandler(workspace: workspace), forURLScheme: "nt-asset")
        web = NoteWebView(frame: .zero, configuration: config)
        super.init()
        config.userContentController.add(WeakHandler(self), name: "nt")
        web.navigationDelegate = self
        #if os(macOS)
        web.setValue(false, forKey: "drawsBackground")
        web.setAccessibilityLabel("Note")
        #else
        web.isOpaque = false
        web.backgroundColor = .clear
        web.scrollView.backgroundColor = .clear
        web.accessibilityLabel = "Note"
        web.setKeyboardBar(KeyboardBar.host(workspace))
        #endif
        if let page = Bundle.main.url(forResource: "index", withExtension: "html", subdirectory: "editor"),
           let resources = Bundle.main.resourceURL {
            web.loadFileURL(page, allowingReadAccessTo: resources)
        }
    }

    // MARK: Swift → page

    /// load shows a note: its text, where it is, who links to it; end puts the cursor at its end
    /// and gives it the keyboard, as for a note just made.
    func load(_ note: Workspace.OpenNote, text: String, anchor: String = "", line: Int = 0, end: Bool = false) {
        let payload: [String: Any] = [
            "doc": text,
            "path": note.path,
            "readOnly": note.readOnly,
            "anchor": anchor,
            "line": line,
            "end": end,
            "backlinks": Self.backlinks(note.backlinks),
        ]
        page("window.nt.load(p)", ["p": payload])
    }

    func setBacklinks(_ list: [Workspace.Backlink]) {
        page("window.nt.backlinks(l)", ["l": Self.backlinks(list)])
    }

    func scroll(to anchor: String) {
        page("window.nt.anchor(a)", ["a": anchor])
    }

    func go(to line: Int) {
        page("window.nt.line(n)", ["n": line])
    }

    /// zoom sets the note's text size, the page's zoom.
    func zoom(_ z: Double) {
        web.pageZoom = z
    }

    func focusMode(_ on: Bool) {
        page("window.nt.focusMode(o)", ["o": on])
    }

    func focus() {
        #if os(macOS)
        web.window?.makeFirstResponder(web)
        #else
        web.becomeFirstResponder()
        #endif
        page("window.nt.focus()")
    }

    /// external hands the page the note as another writer left it on disk. It returns whether the
    /// page took it; when the person changed the same lines, the next save sorts it out.
    func external(_ text: String) async -> Bool {
        guard ready, !saving else { return false }
        return (try? await call("return window.nt.external(t)", ["t": text])) as? Bool ?? false
    }

    private static func backlinks(_ list: [Workspace.Backlink]) -> [[String: String]] {
        list.map { ["path": $0.path, "title": $0.title, "folder": $0.folder] }
    }

    /// page runs a call on the page now, or once it is ready.
    private func page(_ js: String, _ args: [String: Any] = [:]) {
        let run: () -> Void = { [weak self] in
            self?.web.callAsyncJavaScript(js, arguments: args, in: nil, in: .page, completionHandler: nil)
        }
        if ready { run() } else { waiting.append(run) }
    }

    private func call(_ js: String, _ args: [String: Any] = [:]) async throws -> Any? {
        try await web.callAsyncJavaScript(js, arguments: args, contentWorld: .page)
    }

    // MARK: Saving

    /// save writes what the page has not saved yet and returns whether everything is saved. One
    /// save runs at a time; a call during one waits for a round after it.
    @discardableResult
    func save() async -> Bool {
        saveSoon?.cancel()
        guard ready else { return true }
        if saving {
            again = true
            await withCheckedContinuation { waiters.append($0) }
            return !dirty
        }
        saving = true
        repeat {
            again = false
            await saveOnce()
        } while again
        saving = false
        let w = waiters
        waiters = []
        for c in w { c.resume() }
        return !dirty
    }

    private func saveOnce() async {
        guard let nt = ws.nt, let note = ws.note, let root = ws.root else { return }
        guard let snap = (try? await call("return window.nt.beginSave()")) as? [String: Any],
              let seq = snap["seq"] as? Int, let text = snap["text"] as? String else {
            if dirty { clean() } // the text is the file's again
            return
        }
        var args = ["_editor", "save", root.appending(path: note.path).path, "--base", note.sha]
        if keep { args.append("--keep") }
        do {
            let r = try await nt.json(SaveReply.self, args, input: Data(text.utf8))
            if r.saved {
                keep = false
                ws.saved(note.path, sha: r.sha)
                if let copy = r.copy {
                    ws.say("\(ws.title(note.path)) changed on disk as well: that version is in \(copy)")
                }
                let more = (try? await call("return window.nt.saved(s)", ["s": seq])) as? Bool ?? true
                if !more { clean() }
            } else {
                let how = (try? await call("return window.nt.conflict(s, t)", ["s": seq, "t": r.text ?? ""])) as? String
                if how == "merged" {
                    ws.saved(note.path, sha: r.sha)
                } else {
                    keep = true
                }
                again = true
            }
        } catch {
            _ = try? await call("window.nt.failed(s)", ["s": seq])
            ws.say("\(ws.title(note.path)) is not saved: \(error.localizedDescription)")
            schedule(after: .seconds(10))
        }
    }

    struct SaveReply: Decodable, Equatable {
        let saved: Bool
        let sha: String
        var text: String?
        var copy: String?
    }

    private func edited() {
        if !dirty {
            dirty = true
            if hold == nil, let nt = ws.nt { hold = Hold(nt) }
        }
        kickSoon?.cancel()
        schedule(after: .seconds(1))
    }

    private func schedule(after delay: Duration) {
        saveSoon?.cancel()
        saveSoon = Task { [weak self] in
            try? await Task.sleep(for: delay)
            guard !Task.isCancelled else { return }
            await self?.save()
        }
    }

    /// clean: everything is saved. The hold goes, so sync may merge again, and a sync follows.
    private func clean() {
        dirty = false
        hold?.release()
        hold = nil
        wrote = true
        kickSoon?.cancel()
        kickSoon = Task { [weak self] in
            try? await Task.sleep(for: .seconds(30))
            guard !Task.isCancelled else { return }
            self?.kick()
        }
    }

    /// kick starts a sync now when something was saved since the last one: nt _editor kick on
    /// the Mac, the app's own sync on iPhone and iPad.
    func kick() {
        kickSoon?.cancel()
        guard wrote, !dirty, let nt = ws.nt else { return }
        wrote = false
        #if os(macOS)
        Task { _ = try? await nt.run(["_editor", "kick"]) }
        #else
        _ = nt
        Task { await ws.syncNow() }
        #endif
    }

    // MARK: The keyboard bar (iPhone and iPad)

    func task() { page("window.nt.task()") }
    func heading() { page("window.nt.heading()") }
    func link() { page("window.nt.link()") }
    func undo() { page("window.nt.undo()") }
    func redo() { page("window.nt.redo()") }

    /// done lets the keyboard go.
    func done() { page("window.nt.blur()") }

    /// cursor is where the cursor is in the note, for a photo.
    func cursor() async -> Int {
        guard ready else { return 0 }
        return (try? await call("return window.nt.cursor()")) as? Int ?? 0
    }

    // MARK: Page → Swift

    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
        guard let body = message.body as? [String: Any], let type = body["type"] as? String else { return }
        switch type {
        case "ready":
            ready = true
            let w = waiting
            waiting = []
            for run in w { run() }
        case "edit":
            edited()
        case "check":
            let bad = ws.broken(body["dests"] as? [String] ?? [])
            page("window.nt.broken(d)", ["d": bad])
        case "open":
            if let href = body["href"] as? String {
                ws.follow(href)
            } else if let path = body["path"] as? String {
                ws.go(path)
            }
        case "stats":
            ws.words = body["words"] as? Int ?? 0
        case "focus":
            ws.editing = body["on"] as? Bool ?? false
        case "history":
            ws.undoable = body["undo"] as? Bool ?? false
            ws.redoable = body["redo"] as? Bool ?? false
        case "toggle":
            if let line = body["line"] as? Int {
                Task {
                    let day = await ws.day()
                    page("window.nt.toggle(n, d)", ["n": line, "d": day])
                }
            }
        case "image":
            if let b64 = body["data"] as? String, let data = Data(base64Encoded: b64), let pos = body["pos"] as? Int {
                let name = body["name"] as? String ?? "image"
                let mime = body["mime"] as? String ?? "image/png"
                Task { await paste(name: name, mime: mime, data: data, at: pos) }
            }
        default:
            break
        }
    }

    /// paste saves an image pasted or dropped on the note, or picked from Photos or taken with the
    /// camera, with nt paste, beside the note, and puts the line that shows it on a line of its
    /// own.
    func paste(name: String, mime: String, data: Data, at pos: Int) async {
        guard let nt = ws.nt, let note = ws.note, let root = ws.root else { return }
        let fm = FileManager.default
        let dir = fm.temporaryDirectory.appending(path: "nt-paste-\(UUID().uuidString)")
        defer { try? fm.removeItem(at: dir) }
        let file = dir.appending(path: Self.fileName(name, mime: mime))
        struct Pasted: Decodable { let markdown: String }
        do {
            try fm.createDirectory(at: dir, withIntermediateDirectories: true)
            try data.write(to: file)
            let p = try await nt.json(Pasted.self, ["paste", root.appending(path: note.path).path, "--from", file.path, "--json"])
            guard ws.note?.path == note.path else { return }
            page("window.nt.insertImage(p, l)", ["p": pos, "l": p.markdown])
        } catch {
            ws.say(error.localizedDescription)
        }
    }

    /// fileName is the name a pasted or dropped image is saved under for nt paste, which names
    /// the attachment after it: its own name, with an extension from its type when it has none.
    nonisolated static func fileName(_ name: String, mime: String) -> String {
        var base = (name as NSString).lastPathComponent.replacing("/", with: "-")
        if base.isEmpty || base.hasPrefix(".") { base = "image" + base }
        if (base as NSString).pathExtension.isEmpty, let ext = UTType(mimeType: mime)?.preferredFilenameExtension {
            base += "." + ext
        }
        return base
    }

    // MARK: Navigation

    /// The page is the only thing the web view loads; a link is followed by the app.
    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction) async -> WKNavigationActionPolicy {
        guard let url = action.request.url, url.isFileURL, url.lastPathComponent == "index.html" else { return .cancel }
        return .allow
    }

    func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
        // what was not saved went with the page; show the note as it is on disk
        ready = false
        dirty = false
        hold?.release()
        hold = nil
        webView.reload()
        if let n = ws.note { ws.show(n.path, reload: true) }
    }
}

#if os(macOS)
/// NoteWebView sends Undo and Redo to the editor's own history, not WebKit's.
final class NoteWebView: WKWebView {
    @objc func undo(_ sender: Any?) {
        callAsyncJavaScript("window.nt.undo()", arguments: [:], in: nil, in: .page, completionHandler: nil)
    }

    @objc func redo(_ sender: Any?) {
        callAsyncJavaScript("window.nt.redo()", arguments: [:], in: nil, in: .page, completionHandler: nil)
    }

    override func validateUserInterfaceItem(_ item: any NSValidatedUserInterfaceItem) -> Bool {
        if item.action == #selector(undo(_:)) || item.action == #selector(redo(_:)) { return true }
        return super.validateUserInterfaceItem(item)
    }
}
#else
/// NoteWebView is the note's web view; on iPhone and iPad it moves to the screen that shows the
/// note (NoteHost).
final class NoteWebView: WKWebView {}
#endif

/// sha256 is the hex SHA-256 of data, as nt _editor save takes it.
nonisolated func sha256(_ data: Data) -> String {
    SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
}
