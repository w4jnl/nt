// The two layouts. On an iPhone one stack: the home list, then a folder, a note or a view, each a
// screen of its own, and a link followed in a note is a new screen, so Back returns. On an iPad
// the sidebar beside the note, as on the Mac: the note on screen changes in place.
import SwiftUI

/// Route is a screen on the iPhone's stack.
enum Route: Hashable {
    case todos, calendar, weekly, agents, settings
    case folder(String)
    case note(String, anchor: String = "", line: Int = 0, end: Bool = false, find: String = "") // end: a new note, the cursor at its end; find: a search's words, marked

    /// moved is the route after from moved to to: a note or folder in what moved follows it.
    func moved(_ from: String, _ to: String) -> Route {
        func new(_ p: String) -> String { p == from ? to : p.hasPrefix(from + "/") ? to + p.dropFirst(from.count) : p }
        switch self {
        case .note(let p, let a, let l, let e, let f): return .note(new(p), anchor: a, line: l, end: e, find: f)
        case .folder(let p): return .folder(new(p))
        default: return self
        }
    }

    /// names says whether the route shows path or something in it.
    func names(_ path: String) -> Bool {
        switch self {
        case .note(let p, _, _, _, _), .folder(let p): return p == path || p.hasPrefix(path + "/")
        default: return false
        }
    }
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
                    case .settings: SettingsScreen()
                    case .folder(let id): FolderScreen(id: id)
                    case .note(let path, let anchor, let line, let end, let find): NoteScreen(path: path, anchor: anchor, line: line, end: end, find: find)
                    }
                }
        }
        .onAppear {
            let r = router
            ws.navigate = { path, anchor, line, end, find in r.path.append(.note(path, anchor: anchor, line: line, end: end, find: find)) }
            ws.relocated = { from, to in r.path = r.path.map { $0.moved(from, to) } }
            ws.removed = { path in r.path.removeAll { $0.names(path) } }
            ws.home = { r.path.removeAll() }
            // for screenshots, as Workspace.start does on the iPad: -NTView todos, calendar,
            // weekly, agents or settings shows a view, -NTFolder <path> a folder, -NTOpen <path>
            // a note
            let d = UserDefaults.standard
            switch d.string(forKey: "NTView") {
            case "todos": r.path.append(.todos)
            case "calendar": r.path.append(.calendar)
            case "agents": r.path.append(.agents)
            case "settings": r.path.append(.settings)
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
                Group {
                    if child.isFolder {
                        NavigationLink(value: Route.folder(child.id)) { FolderLabel(node: child) }
                    } else {
                        NavigationLink(value: Route.note(child.id)) { NoteRow(node: child) }
                    }
                }
                .itemActions(child.id, folder: child.isFolder)
            }
            .listRowBackground(Brand.surface)
        }
        .scrollContentBackground(.hidden)
        .background(Brand.ground)
        .navigationTitle(node?.name ?? (id as NSString).lastPathComponent)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    ItemMenu(path: id, folder: true)
                } label: {
                    Image(systemName: "plus")
                }
                .accessibilityLabel("New note, folder or todo, and more")
            }
        }
    }
}

/// NoteScreen is a note on the iPhone. The one editor moves to the screen on top, and a screen
/// the stack returns to opens its note again.
struct NoteScreen: View {
    @Environment(Workspace.self) private var ws
    let path: String
    let anchor: String
    let line: Int
    var end = false
    var find = ""
    @State private var opened = false

    var body: some View {
        Group {
            if let editor = ws.editor {
                EditorView(editor: editor)
                    .ignoresSafeArea(edges: .bottom)
            }
        }
        .background(Brand.ground)
        .safeAreaInset(edge: .top) { if ws.finding { FindBar() } }
        .navigationTitle(ws.title(path))
        .navigationSubtitle(ws.changedLine(path) ?? "")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar { NoteMenu(path: path, find: { ws.openFind() }) }
        .modifier(PhotoPicking())
        .task(id: path) {
            // a new note opens with the cursor at its end, the first time only
            await ws.open(path, anchor: anchor, line: line, end: end && !opened, find: find)
            opened = true
        }
    }
}

struct SplitLayout: View {
    @Environment(Workspace.self) private var ws
    @State private var columns = NavigationSplitViewVisibility.automatic

    var body: some View {
        NavigationSplitView(columnVisibility: $columns) {
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
        .onAppear {
            ws.navigate = nil
            ws.home = nil
        }
        .onChange(of: ws.searchAsked) {
            if ws.searchAsked != nil { columns = .all } // the sidebar, with the search a tag asked for
        }
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
        .navigationSubtitle(ws.note.map { n in
            let folders = SidebarTree.ancestors(n.path).map { ($0 as NSString).lastPathComponent }.joined(separator: " › ")
            return [folders, ws.changedLine(n.path) ?? ""].filter { !$0.isEmpty }.joined(separator: " · ")
        } ?? "")
        .navigationBarTitleDisplayMode(.inline)
        .safeAreaInset(edge: .top) { if ws.finding { FindBar() } }
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
                Button("Back", systemImage: "chevron.left") { Task { await ws.back() } }
                    .disabled(!ws.canGoBack)
                    .keyboardShortcut("[")
                    .accessibilityLabel("Back to the note the link was followed from")
            }
            if let n = ws.note { NoteMenu(path: n.path, find: { ws.openFind() }) }
        }
        .modifier(PhotoPicking())
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

/// NoteMenu is the note's ⋯: rename, move and delete.
struct NoteMenu: ToolbarContent {
    let path: String
    let find: () -> Void // shows the find bar

    var body: some ToolbarContent {
        ToolbarItem(placement: .topBarTrailing) {
            Menu {
                Button("Find in Note…", systemImage: "magnifyingglass", action: find)
                Divider()
                ItemMenu(path: path, folder: false)
            } label: {
                Image(systemName: "ellipsis")
            }
            .accessibilityLabel("Rename, move or delete the note")
        }
    }
}
