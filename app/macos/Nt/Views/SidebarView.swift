import SwiftUI

/// SidebarView is the left column: Open, Today, the views (Todos, Calendar, Weekly, Agents) and the
/// folders, pinned first, with the sync state at its foot. The sync state stays out of the
/// toolbar, which would draw it as a glass button and fold it into the overflow menu when the
/// sidebar is hidden, and out of the list, whose rows would scroll under it.
struct SidebarView: View {
    @Environment(Workspace.self) private var ws

    var body: some View {
        VStack(spacing: 0) {
            list
            SyncBadgeView()
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 10)
                .padding(.top, 4)
                .padding(.bottom, 8)
        }
    }

    private var list: some View {
        List(selection: Binding(get: { ws.selection }, set: { ws.select($0) })) {
            Label("Today", systemImage: "sun.max")
                .tag(Workspace.Selection.today)
            HStack {
                Label("Todos", systemImage: "checkmark.circle")
                Spacer()
                let open = ws.todos.count { !$0.done }
                if open > 0 {
                    Text("\(open)")
                        .font(Brand.mono(11))
                        .foregroundStyle(Brand.muted)
                        .accessibilityLabel("\(open) open")
                }
            }
            .tag(Workspace.Selection.todos)
            Label("Calendar", systemImage: "calendar")
                .tag(Workspace.Selection.calendar)
            Label("Weekly", systemImage: "text.alignleft")
                .tag(Workspace.Selection.weekly)
            Label("Agents", systemImage: "apple.terminal")
                .tag(Workspace.Selection.agents)
            Section {
                TreeRows(nodes: ws.tree.pinned)
                if !ws.tree.pinned.isEmpty && !ws.tree.rest.isEmpty {
                    Divider()
                }
                TreeRows(nodes: ws.tree.rest)
                TreeRows(nodes: ws.tree.rootNotes)
            } header: {
                Text("Folders")
                    .contextMenu {
                        Button("New Note…") { ws.ask = .newNote("") }
                        Button("New Folder…") { ws.ask = .newFolder("") }
                        Button("New Todo…") { ws.ask = .newTodo("") }
                    }
            }
        }
        .listStyle(.sidebar)
        .safeAreaInset(edge: .top) {
            OpenField()
                .padding(.horizontal, 10)
                .padding(.bottom, 6)
        }
    }
}

/// OpenField looks like a search field and opens the Open palette (⌘P).
struct OpenField: View {
    @Environment(Workspace.self) private var ws

    var body: some View {
        Button {
            ws.palette = .open
        } label: {
            HStack(spacing: 8) {
                Image(systemName: "magnifyingglass")
                    .font(.system(size: 12))
                Text("Open a note")
                Spacer()
                Text("⌘P")
                    .font(Brand.mono(11))
            }
            .foregroundStyle(Brand.muted)
            .padding(.horizontal, 10)
            .frame(height: 28)
            .background(Brand.sunk, in: RoundedRectangle(cornerRadius: 8))
            .overlay(RoundedRectangle(cornerRadius: 8).stroke(Brand.line))
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Open a note")
    }
}
