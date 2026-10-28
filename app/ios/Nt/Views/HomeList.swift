// HomeList is the Mac sidebar on the phone: Today, Todos, Calendar, Weekly and Agents, the
// pinned folders, then the folders, and the sync state at its foot; a tap there, or pulling the
// list down, syncs. Search finds titles first, then lines. On an iPhone a folder opens as a
// screen of its own; in the iPad's sidebar it opens in place, as on the Mac.
import SwiftUI

struct HomeList: View {
    @Environment(Workspace.self) private var ws
    /// stacked: on the iPhone's stack; else the iPad's sidebar, which selects.
    let stacked: Bool
    @State private var query = ""
    @State private var lines: [Workspace.SearchHit] = []

    var body: some View {
        Group {
            if stacked {
                List { content }
                    .listStyle(.insetGrouped)
            } else {
                List(selection: Binding(get: { ws.selection }, set: { ws.select($0) })) { content }
                    .listStyle(.sidebar)
            }
        }
        .scrollContentBackground(.hidden)
        .background(stacked ? Brand.ground : Brand.sunk)
        .navigationTitle("Notes")
        .searchable(text: $query, prompt: "Search notes")
        .task(id: query) {
            try? await Task.sleep(for: .milliseconds(250))
            guard !Task.isCancelled else { return }
            lines = await ws.search(query)
        }
        .refreshable { await ws.syncNow() }
        .safeAreaInset(edge: .bottom) {
            SyncBadgeView()
                .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
                .padding(.horizontal, 14)
                .background(Brand.ground)
                .overlay(alignment: .top) { Rectangle().fill(Brand.line).frame(height: 1) }
        }
    }

    @ViewBuilder
    private var content: some View {
        if query.trimmingCharacters(in: .whitespaces).isEmpty {
            views
            if !ws.tree.pinned.isEmpty {
                Section("Pinned") { folders(ws.tree.pinned, counts: true) }
            }
            Section("Folders") {
                folders(ws.tree.rest)
                folders(ws.tree.rootNotes)
            }
        } else {
            results
        }
    }

    // MARK: The views

    @ViewBuilder
    private var views: some View {
        let open = ws.todos.filter { !$0.done }
        let late = open.filter(\.late).count
        let agents = ws.reports(on: ws.today).count
        Section {
            row(.today, Route.note(ws.todayPath)) {
                ViewLabel(title: "Today", symbol: "sun.max", detail: Text(Self.short(ws.today)))
            }
            row(.todos, .todos) {
                ViewLabel(title: "Todos", symbol: "checkmark.circle",
                          detail: open.isEmpty ? Text("") : late > 0 ? Text("\(open.count) · \(Text("\(late) late").foregroundStyle(Brand.warn))") : Text("\(open.count)"))
            }
            row(.calendar, .calendar) { ViewLabel(title: "Calendar", symbol: "calendar", detail: Text("")) }
            row(.weekly, ws.weeklyPath.map { Route.note($0) } ?? .weekly) {
                ViewLabel(title: "Weekly", symbol: "text.alignleft", detail: Text(Self.week(ws.weeklyPath)))
            }
            row(.agents, .agents) {
                ViewLabel(title: "Agents", symbol: "apple.terminal", detail: Text(agents > 0 ? "\(agents) today" : ""))
            }
        }
        .listRowBackground(stacked ? Brand.surface : nil)
    }

    /// row is a view's row: a screen on the iPhone; a selection in the sidebar. Today is today's
    /// journal, which nt makes when it is missing, so it goes through nt first on the iPhone.
    @ViewBuilder
    private func row(_ sel: Workspace.Selection, _ route: Route, @ViewBuilder label: () -> some View) -> some View {
        if !stacked {
            label().tag(sel)
        } else if sel == .today {
            Button { Task { await ws.goToday() } } label: { label() }
                .foregroundStyle(Brand.ink)
        } else {
            NavigationLink(value: route) { label() }
        }
    }

    // MARK: Folders

    @ViewBuilder
    private func folders(_ nodes: [TreeNode], counts: Bool = false) -> some View {
        if stacked {
            ForEach(nodes) { node in
                if node.isFolder {
                    NavigationLink(value: Route.folder(node.id)) { FolderLabel(node: node, count: counts) }
                } else {
                    NavigationLink(value: Route.note(node.id)) { NoteRow(node: node) }
                }
            }
            .listRowBackground(Brand.surface)
        } else {
            TreeRows(nodes: nodes)
        }
    }

    // MARK: Search

    @ViewBuilder
    private var results: some View {
        let titles = ws.index.map { Fuzzy.find(query, in: $0.notes, limit: 8) } ?? []
        if !titles.isEmpty {
            Section("Titles") {
                ForEach(titles, id: \.note.path) { hit in
                    result(hit.note.path, line: 0) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(hit.note.title).foregroundStyle(Brand.ink)
                            Text(hit.note.path).font(Brand.mono(12)).foregroundStyle(Brand.muted)
                        }
                    }
                }
            }
            .listRowBackground(stacked ? Brand.surface : nil)
        }
        if !lines.isEmpty {
            Section("Lines") {
                ForEach(lines.prefix(50), id: \.self) { hit in
                    result(hit.path, line: hit.line) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(hit.text.trimmingCharacters(in: .whitespaces)).foregroundStyle(Brand.ink).lineLimit(2)
                            Text("\(ws.title(hit.path)) · \(hit.line)").font(Brand.mono(12)).foregroundStyle(Brand.muted)
                        }
                    }
                }
            }
            .listRowBackground(stacked ? Brand.surface : nil)
        }
        if titles.isEmpty && lines.isEmpty {
            Text("Nothing matches.").foregroundStyle(Brand.muted)
        }
    }

    @ViewBuilder
    private func result(_ path: String, line: Int, @ViewBuilder label: () -> some View) -> some View {
        if stacked {
            NavigationLink(value: Route.note(path, line: line)) { label() }
        } else {
            Button { ws.go(path, line: line) } label: { label() }
        }
    }

    // MARK: Words

    /// short is a day as the Today row says it: "Wed 28 Oct".
    static func short(_ day: String) -> String {
        let words = Agenda.long(day).split(separator: " ")
        guard words.count >= 3 else { return day }
        return "\(words[0].prefix(3)) \(words[1]) \(words[2].prefix(3))"
    }

    /// week is the newest weekly digest as the Weekly row says it: "Week 44".
    static func week(_ path: String?) -> String {
        guard let path, let m = (path as NSString).lastPathComponent.firstMatch(of: /W(\d{2})\.md$/), let n = Int(m.1) else { return "" }
        return "Week \(n)"
    }
}

/// ViewLabel is a view's row: its symbol, its name, and a word about it on the right.
struct ViewLabel: View {
    let title: String
    let symbol: String
    let detail: Text

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: symbol)
                .foregroundStyle(Brand.accent)
                .frame(width: 24)
            Text(title).foregroundStyle(Brand.ink)
            Spacer(minLength: 8)
            detail
                .font(Brand.mono(13))
                .foregroundStyle(Brand.muted)
        }
    }
}

/// FolderLabel is a folder's row, with how many notes are in it for a pinned one.
struct FolderLabel: View {
    let node: TreeNode
    var count = false

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: "folder")
                .foregroundStyle(Brand.muted)
                .frame(width: 24)
            Text(node.name).foregroundStyle(Brand.ink)
            Spacer(minLength: 8)
            if count {
                Text("\(node.notes)")
                    .font(Brand.mono(13))
                    .foregroundStyle(Brand.muted)
            }
        }
    }
}

extension TreeNode {
    /// notes counts the notes in a folder and its folders.
    var notes: Int { children.map { $0.reduce(0) { $0 + ($1.isFolder ? $1.notes : 1) } } ?? 0 }
}
