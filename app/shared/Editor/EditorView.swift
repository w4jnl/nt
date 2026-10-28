// The note on screen: the editor's web view (NoteEditor), which outlives the window. Its images
// come from the repository through nt-asset:, and nothing else loads.
import SwiftUI
import UniformTypeIdentifiers
import WebKit

#if os(macOS)
struct EditorView: NSViewRepresentable {
    let editor: NoteEditor

    func makeNSView(context: Context) -> NoteWebView { editor.web }

    func updateNSView(_ web: NoteWebView, context: Context) {}
}
#else
/// EditorView on iPhone and iPad: the one web view, moved into whichever screen shows the note
/// now. A screen the stack goes back to takes it back when it is in the window again.
struct EditorView: UIViewRepresentable {
    let editor: NoteEditor

    func makeUIView(context: Context) -> NoteHost { NoteHost(web: editor.web) }

    func updateUIView(_ host: NoteHost, context: Context) { host.adopt() }
}

final class NoteHost: UIView {
    let web: NoteWebView

    init(web: NoteWebView) {
        self.web = web
        super.init(frame: .zero)
        adopt()
    }

    required init?(coder: NSCoder) { nil }

    override func didMoveToWindow() {
        super.didMoveToWindow()
        if window != nil { adopt() }
    }

    func adopt() {
        guard web.superview !== self else { return }
        web.removeFromSuperview()
        web.frame = bounds
        web.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        addSubview(web)
    }
}
#endif

/// WeakHandler keeps the message handler from holding the coordinator for ever: the content
/// controller retains what it is given.
@MainActor
final class WeakHandler: NSObject, WKScriptMessageHandler {
    weak var target: (any WKScriptMessageHandler)?

    init(_ target: any WKScriptMessageHandler) {
        self.target = target
    }

    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
        target?.userContentController(controller, didReceive: message)
    }
}

/// AssetHandler serves nt-asset://repo/<path>: a file in the repository, never one outside it.
@MainActor
final class AssetHandler: NSObject, WKURLSchemeHandler {
    let workspace: Workspace

    init(workspace: Workspace) {
        self.workspace = workspace
    }

    func webView(_ webView: WKWebView, start task: any WKURLSchemeTask) {
        guard let url = task.request.url, let root = workspace.root,
              let file = AssetHandler.file(for: url, root: root),
              let data = try? Data(contentsOf: file) else {
            task.didFailWithError(URLError(.fileDoesNotExist))
            return
        }
        let mime = UTType(filenameExtension: file.pathExtension)?.preferredMIMEType ?? "application/octet-stream"
        task.didReceive(URLResponse(url: url, mimeType: mime, expectedContentLength: data.count, textEncodingName: nil))
        task.didReceive(data)
        task.didFinish()
    }

    func webView(_ webView: WKWebView, stop task: any WKURLSchemeTask) {}

    /// file is the repository file an nt-asset URL names, or nil when it leaves the repository.
    nonisolated static func file(for url: URL, root: URL) -> URL? {
        let rel = url.path(percentEncoded: false).trimmingPrefix("/")
        guard !rel.isEmpty else { return nil }
        let base = root.standardizedFileURL.resolvingSymlinksInPath().path
        let file = root.appending(path: String(rel)).standardizedFileURL.resolvingSymlinksInPath()
        return file.path.hasPrefix(base + "/") ? file : nil
    }
}
