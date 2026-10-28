import SwiftUI

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
