import SwiftUI

/// SidebarView is the left column: the sync state, Open, Today and the folders, pinned first.
struct SidebarView: View {
    @Environment(Workspace.self) private var ws

    var body: some View {
        List(selection: Binding(get: { ws.selection }, set: { ws.select($0) })) {
            Label("Today", systemImage: "sun.max")
                .tag(Workspace.Selection.today)
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
        .toolbar {
            ToolbarItem {
                SyncBadgeView()
            }
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
    private static let spinner = ["◐", "◓", "◑", "◒"]

    var body: some View {
        TimelineView(.periodic(from: .now, by: ws.syncing ? 0.15 : 30)) { ctx in
            let b = Badge.of(ws.status, syncing: ws.syncing, now: ctx.date)
            let frame = Int(ctx.date.timeIntervalSinceReferenceDate / 0.15) % Self.spinner.count
            Button {
                Task { await ws.syncNow() }
            } label: {
                HStack(spacing: 5) {
                    Text(ws.syncing ? Self.spinner[frame] : b.glyph)
                        .foregroundStyle(Brand.tone(b.tone))
                    Text(b.word)
                        .foregroundStyle(Brand.muted)
                }
                .font(.system(size: 12))
            }
            .buttonStyle(.plain)
            .help(b.detail.isEmpty ? "Sync now" : b.detail + ". Click to sync now.")
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
