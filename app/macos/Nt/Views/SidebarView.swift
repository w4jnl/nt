import SwiftUI

/// SidebarView is the left column: Open, Today, the views (Todos, Calendar, Weekly, Agents) and the
/// folders, pinned first, with the sync state at its foot. The sync state stays out of the
/// toolbar, which would draw it as a glass button and fold it into the overflow menu when the
/// sidebar is hidden.
struct SidebarView: View {
    @Environment(Workspace.self) private var ws

    var body: some View {
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
            Section("Folders") {
                TreeRows(nodes: ws.tree.pinned)
                if !ws.tree.pinned.isEmpty && !ws.tree.rest.isEmpty {
                    Divider()
                }
                TreeRows(nodes: ws.tree.rest)
                TreeRows(nodes: ws.tree.rootNotes)
            }
        }
        .listStyle(.sidebar)
        .safeAreaInset(edge: .top) {
            OpenField()
                .padding(.horizontal, 10)
                .padding(.bottom, 6)
        }
        .safeAreaInset(edge: .bottom) {
            SyncBadgeView()
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 10)
                .padding(.bottom, 8)
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

/// TreeRows are folders, each a disclosure group, and notes, newest first.
struct TreeRows: View {
    let nodes: [TreeNode]
    @Environment(Workspace.self) private var ws

    var body: some View {
        ForEach(nodes) { node in
            if let children = node.children {
                DisclosureGroup(isExpanded: expanded(node.id)) {
                    TreeRows(nodes: children)
                } label: {
                    Label(node.name, systemImage: "folder")
                }
            } else {
                NoteRow(node: node)
                    .tag(Workspace.Selection.note(node.id))
            }
        }
    }

    private func expanded(_ id: String) -> Binding<Bool> {
        Binding(
            get: { ws.expanded.contains(id) },
            set: { open in
                if open { ws.expanded.insert(id) } else { ws.expanded.remove(id) }
            })
    }
}

struct NoteRow: View {
    let node: TreeNode

    var body: some View {
        HStack(spacing: 8) {
            Text(node.name)
                .lineLimit(1)
                .truncationMode(.tail)
            Spacer(minLength: 4)
            if !node.date.isEmpty {
                Text(node.date)
                    .font(Brand.mono(11))
                    .foregroundStyle(Brand.muted)
            }
        }
    }
}

/// SyncBadgeView is the sync state as glyph and word; a click syncs now.
struct SyncBadgeView: View {
    @Environment(Workspace.self) private var ws
    @State private var hover = false
    private static let spinner = ["◐", "◓", "◑", "◒"]

    var body: some View {
        TimelineView(.periodic(from: .now, by: ws.syncing ? 0.15 : 30)) { ctx in
            let b = Badge.of(ws.status, syncing: ws.syncing, now: ctx.date)
            let frame = Int(ctx.date.timeIntervalSinceReferenceDate / 0.15) % Self.spinner.count
            Button {
                Task { await ws.syncNow() }
            } label: {
                HStack(spacing: 6) {
                    Text(ws.syncing ? Self.spinner[frame] : b.glyph)
                        .font(.system(size: 10))
                        .foregroundStyle(Brand.tone(b.tone))
                    Text(b.word)
                        .font(.system(size: 12))
                        .foregroundStyle(Brand.muted)
                }
                .padding(.horizontal, 6)
                .frame(height: 24)
                .background(hover ? Brand.sunk : .clear, in: RoundedRectangle(cornerRadius: 6))
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .onHover { hover = $0 }
            .help(b.detail.isEmpty ? "Sync now  ⌥⌘S" : b.detail + ". Click to sync now  ⌥⌘S")
            .accessibilityLabel("Sync state: \(b.word). Sync now")
        }
    }
}

extension Brand {
    static func tone(_ t: Badge.Tone) -> Color {
        switch t {
        case .ok: ok
        case .accent: accent
        case .muted: muted
        case .warn: warn
        case .alert: alert
        case .faint: faint
        }
    }
}
