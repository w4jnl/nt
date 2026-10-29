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
    @State private var sheet: Sheet?
    @Environment(\.verticalSizeClass) private var height
    @State private var settings = false

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
        .toolbar {
            ToolbarItem(placement: .topBarLeading) { Mark() }
                .sharedBackgroundVisibility(.hidden)
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button("New Note…") { ws.ask = .newNote("") }
                    Button("New Folder…") { ws.ask = .newFolder("") }
                } label: {
                    Image(systemName: "plus")
                }
                .accessibilityLabel("New note or folder")
            }
            ToolbarItem(placement: .topBarTrailing) {
                if stacked {
                    NavigationLink(value: Route.settings) { Image(systemName: "gearshape") }
                        .accessibilityLabel("Settings")
                } else {
                    Button { settings = true } label: { Image(systemName: "gearshape") }
                        .accessibilityLabel("Settings")
                }
            }
        }
        .sheet(isPresented: $settings) {
            NavigationStack {
                SettingsScreen()
                    .toolbar { ToolbarItem(placement: .confirmationAction) { Button("Done") { settings = false } } }
            }
        }
        // at the top, under the title, as designed (iOS 26 would put it at the bottom, where a
        // phone on its side has no room for it); on its side it scrolls away with the list
        .searchable(text: $query, placement: stacked ? .navigationBarDrawer(displayMode: upright ? .always : .automatic) : .sidebar, prompt: "Search notes")
        .onChange(of: ws.searchAsked, initial: true) {
            // a tag tapped in a note: its search here
            if let q = ws.searchAsked {
                query = q
                ws.searchAsked = nil
            }
        }
        .task(id: query) {
            try? await Task.sleep(for: .milliseconds(250))
            guard !Task.isCancelled else { return }
            lines = await ws.search(query)
        }
        .refreshable { await ws.syncNow() }
        .safeAreaInset(edge: .bottom) {
            HStack {
                SyncBadgeView()
                Spacer()
                Button { sheet = .capture } label: {
                    Image(systemName: "square.and.pencil")
                        .font(.system(size: 20))
                        .foregroundStyle(Brand.accent)
                        .frame(width: 44, height: 44)
                }
                .accessibilityLabel("Add to today")
            }
            .padding(.leading, 14)
            .padding(.trailing, 6)
            .background(Brand.ground)
            .overlay(alignment: .top) { Rectangle().fill(Brand.line).frame(height: 1) }
        }
        .sheet(item: $sheet) { s in
            switch s {
            case .capture: CaptureSheet(workspace: ws) { sheet = .talk }
            case .talk: TalkSheet(workspace: ws)
            }
        }
        .onAppear {
            // for screenshots, as on the Mac: -NTCapture opens the capture sheet
            if UserDefaults.standard.string(forKey: "NTCapture") != nil { sheet = .capture }
        }
    }

    /// Sheet is what the foot's pencil opens: the capture sheet, from which the mic talks a note.
    enum Sheet: Identifiable {
        case capture, talk
        var id: Self { self }
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
                ViewLabel(title: "Today", symbol: "sun.max", detail: Text(Self.short(ws.today)), selected: on(.today))
            }
            row(.todos, .todos) {
                ViewLabel(title: "Todos", symbol: "checkmark.circle",
                          detail: open.isEmpty ? Text("") : late > 0 && !on(.todos) ? Text("\(open.count) · \(Text("\(late) late").foregroundStyle(Brand.warn))") : Text(late > 0 ? "\(open.count) · \(late) late" : "\(open.count)"),
                          selected: on(.todos))
            }
            row(.calendar, .calendar) { ViewLabel(title: "Calendar", symbol: "calendar", detail: Text(""), selected: on(.calendar)) }
            row(.weekly, ws.weeklyPath.map { Route.note($0) } ?? .weekly) {
                ViewLabel(title: "Weekly", symbol: "text.alignleft", detail: Text(Self.week(ws.weeklyPath)), selected: on(.weekly))
            }
            row(.agents, .agents) {
                ViewLabel(title: "Agents", symbol: "apple.terminal", detail: Text(agents > 0 ? "\(agents) today" : ""), selected: on(.agents))
            }
        }
        .listRowBackground(stacked ? Brand.surface : nil)
    }

    /// upright: a phone held upright, with the height for the search field to stay.
    private var upright: Bool { height != .compact }

    /// on says whether the sidebar has sel selected; its row is then in accent.
    private func on(_ sel: Workspace.Selection) -> Bool { !stacked && ws.selection == sel }

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
                Group {
                    if node.isFolder {
                        NavigationLink(value: Route.folder(node.id)) { FolderLabel(node: node, count: counts) }
                    } else {
                        NavigationLink(value: Route.note(node.id)) { NoteRow(node: node) }
                    }
                }
                .itemActions(node.id, folder: node.isFolder)
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

/// ViewLabel is a view's row: its symbol, its name, and a word about it on the right. Selected
/// in the sidebar, on accent, it takes the row's own colour.
struct ViewLabel: View {
    let title: String
    let symbol: String
    let detail: Text
    var selected = false

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: symbol)
                .foregroundStyle(selected ? AnyShapeStyle(.primary) : AnyShapeStyle(Brand.accent))
                .frame(width: 24)
            Text(title).foregroundStyle(selected ? AnyShapeStyle(.primary) : AnyShapeStyle(Brand.ink))
            Spacer(minLength: 8)
            detail
                .font(Brand.mono(13))
                .foregroundStyle(selected ? AnyShapeStyle(.secondary) : AnyShapeStyle(Brand.muted))
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
