// The two layouts. On an iPhone one stack: the home list, then a folder, a note or a view, each a
// screen of its own, and a link followed in a note is a new screen, so Back returns. On an iPad
// the sidebar beside the note, as on the Mac: the note on screen changes in place.
import SwiftUI

/// Route is a screen on the iPhone's stack.
enum Route: Hashable {
    case todos, calendar, weekly, agents
    case folder(String)
    case note(String, anchor: String = "", line: Int = 0)
}

@MainActor @Observable
final class Router {
    var path: [Route] = []
}

struct StackLayout: View {
    @Environment(Workspace.self) private var ws
    @State private var router = Router()

    var body: some View {
        @Bindable var router = router
        NavigationStack(path: $router.path) {
            HomeList(stacked: true)
                .navigationDestination(for: Route.self) { route in
                    switch route {
                    case .todos: TodosView()
                    case .calendar: CalendarView()
                    case .weekly: WeeklyEmptyView()
                    case .agents: AgentsView()
                    case .folder(let id): FolderScreen(id: id)
                    case .note(let path, let anchor, let line): NoteScreen(path: path, anchor: anchor, line: line)
                    }
                }
        }
        .onAppear {
            let r = router
            ws.navigate = { path, anchor, line in r.path.append(.note(path, anchor: anchor, line: line)) }
            // for screenshots, as Workspace.start does on the iPad: -NTView todos, calendar,
            // weekly or agents shows a view, -NTFolder <path> a folder, -NTOpen <path> a note
            let d = UserDefaults.standard
            switch d.string(forKey: "NTView") {
            case "todos": r.path.append(.todos)
            case "calendar": r.path.append(.calendar)
            case "agents": r.path.append(.agents)
            case "weekly": r.path.append(ws.weeklyPath.map { .note($0) } ?? .weekly)
            default: break
            }
            if let f = d.string(forKey: "NTFolder") { r.path.append(.folder(f)) }
            if let p = d.string(forKey: "NTOpen") { r.path.append(.note(p)) }
        }
    }
}

/// FolderScreen is a folder on the iPhone: its folders, then its notes, newest first.
struct FolderScreen: View {
    @Environment(Workspace.self) private var ws
    let id: String

    var body: some View {
        let node = ws.tree.find(id)
        List {
            ForEach(node?.children ?? []) { child in
                if child.isFolder {
                    NavigationLink(value: Route.folder(child.id)) { FolderLabel(node: child) }
                } else {
                    NavigationLink(value: Route.note(child.id)) { NoteRow(node: child) }
                }
            }
            .listRowBackground(Brand.surface)
        }
        .scrollContentBackground(.hidden)
        .background(Brand.ground)
        .navigationTitle(node?.name ?? (id as NSString).lastPathComponent)
    }
}

/// NoteScreen is a note on the iPhone. The one editor moves to the screen on top, and a screen
/// the stack returns to opens its note again.
struct NoteScreen: View {
    @Environment(Workspace.self) private var ws
    let path: String
    let anchor: String
    let line: Int

    var body: some View {
        Group {
            if let editor = ws.editor {
                EditorView(editor: editor)
                    .ignoresSafeArea(edges: .bottom)
            }
        }
        .background(Brand.ground)
        .navigationTitle(ws.title(path))
        .navigationBarTitleDisplayMode(.inline)
        .task(id: path) { await ws.open(path, anchor: anchor, line: line) }
    }
}

struct SplitLayout: View {
    @Environment(Workspace.self) private var ws

    var body: some View {
        NavigationSplitView {
            HomeList(stacked: false)
                .navigationSplitViewColumnWidth(min: 260, ideal: 320, max: 400)
        } detail: {
            NavigationStack {
                switch ws.selection {
                case .todos: TodosView()
                case .calendar: CalendarView()
                case .agents: AgentsView()
                case .weekly where ws.weeklyPath == nil: WeeklyEmptyView()
                default: NoteDetail()
                }
            }
        }
        .onAppear { ws.navigate = nil }
        .task { if ws.note == nil { await ws.openToday() } }
    }
}

/// NoteDetail is the note beside the sidebar, under its title and its folders.
struct NoteDetail: View {
    @Environment(Workspace.self) private var ws

    var body: some View {
        Group {
            if let editor = ws.editor {
                EditorView(editor: editor)
                    .ignoresSafeArea(edges: .bottom)
            }
        }
        .background(Brand.ground)
        .navigationTitle(ws.note.map { ws.title($0.path) } ?? "")
        .navigationSubtitle(ws.note.map { SidebarTree.ancestors($0.path).map { ($0 as NSString).lastPathComponent }.joined(separator: " › ") } ?? "")
        .navigationBarTitleDisplayMode(.inline)
    }
}

extension SidebarTree {
    /// find is the folder or note with this repository path, anywhere in the tree.
    func find(_ id: String) -> TreeNode? {
        func search(_ nodes: [TreeNode]) -> TreeNode? {
            for n in nodes {
                if n.id == id { return n }
                if let c = n.children, let hit = search(c) { return hit }
            }
            return nil
        }
        return search(pinned + rest + rootNotes)
    }
}
