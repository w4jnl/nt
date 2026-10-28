// The note on screen: the editor page (Resources/editor, CodeMirror 6 with nt's live preview) in
// a web view. Swift hands it the note's text; the page asks back through the "nt" message
// handler: which links point at nothing, follow this link, how many words. The page's images
// come from the repository through nt-asset:, and nothing else loads.
import SwiftUI
import UniformTypeIdentifiers
import WebKit

struct EditorView: NSViewRepresentable {
    let workspace: Workspace
    let note: Workspace.OpenNote?
    let scroll: (anchor: String, seq: Int)

    func makeCoordinator() -> Coordinator { Coordinator(workspace: workspace) }

    func makeNSView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        config.setURLSchemeHandler(AssetHandler(workspace: workspace), forURLScheme: "nt-asset")
        config.userContentController.add(WeakHandler(context.coordinator), name: "nt")
        let web = WKWebView(frame: .zero, configuration: config)
        web.navigationDelegate = context.coordinator
        web.setValue(false, forKey: "drawsBackground")
        web.setAccessibilityLabel("Note")
        context.coordinator.web = web
        if let page = Bundle.main.url(forResource: "index", withExtension: "html", subdirectory: "editor"),
           let resources = Bundle.main.resourceURL {
            web.loadFileURL(page, allowingReadAccessTo: resources)
        }
        return web
    }

    func updateNSView(_ web: WKWebView, context: Context) {
        context.coordinator.show(note)
        context.coordinator.scroll(scroll.anchor, seq: scroll.seq)
    }

    @MainActor
    final class Coordinator: NSObject, WKScriptMessageHandler, WKNavigationDelegate {
        weak var web: WKWebView?
        let workspace: Workspace
        private var ready = false
        private var shown = -1
        private var pending: Workspace.OpenNote?
        private var scrollSeq = 0

        init(workspace: Workspace) {
            self.workspace = workspace
        }

        func show(_ note: Workspace.OpenNote?) {
            guard let note else { return }
            pending = note
            flush()
        }

        private func flush() {
            guard ready, let web, let n = pending, n.version != shown else { return }
            shown = n.version
            let payload: [String: Any] = [
                "doc": n.text,
                "path": n.path,
                "readOnly": true,
                "anchor": n.anchor,
                "line": n.line,
                "backlinks": n.backlinks.map { ["path": $0.path, "title": $0.title, "folder": $0.folder] },
            ]
            web.callAsyncJavaScript("window.nt.load(p)", arguments: ["p": payload], in: nil, in: .page, completionHandler: nil)
        }

        func scroll(_ anchor: String, seq: Int) {
            guard ready, seq != scrollSeq else { return }
            scrollSeq = seq
            web?.callAsyncJavaScript("window.nt.anchor(a)", arguments: ["a": anchor], in: nil, in: .page, completionHandler: nil)
        }

        func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
            guard let body = message.body as? [String: Any], let type = body["type"] as? String else { return }
            switch type {
            case "ready":
                ready = true
                flush()
            case "check":
                let bad = workspace.broken(body["dests"] as? [String] ?? [])
                if !bad.isEmpty {
                    web?.callAsyncJavaScript("window.nt.broken(d)", arguments: ["d": bad], in: nil, in: .page, completionHandler: nil)
                }
            case "open":
                if let href = body["href"] as? String {
                    workspace.follow(href)
                } else if let path = body["path"] as? String {
                    workspace.show(path)
                }
            case "stats":
                workspace.words = body["words"] as? Int ?? 0
            default:
                break
            }
        }

        /// The page is the only thing the web view loads; a link is followed by the app.
        func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction) async -> WKNavigationActionPolicy {
            guard let url = action.request.url, url.isFileURL, url.lastPathComponent == "index.html" else { return .cancel }
            return .allow
        }

        func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
            ready = false
            shown = -1
            webView.reload()
        }
    }
}

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
