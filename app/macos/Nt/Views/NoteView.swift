import SwiftUI

/// NoteView is the right column: where the note is, the note, and a quiet line at the bottom for
/// the word count and messages.
struct NoteView: View {
    @Environment(Workspace.self) private var ws

    var body: some View {
        ZStack(alignment: .bottom) {
            EditorView(workspace: ws, note: ws.note, scroll: ws.anchorRequest)
            HStack(alignment: .bottom) {
                if !ws.flash.isEmpty {
                    Text(ws.flash)
                        .font(.system(size: 12))
                        .foregroundStyle(Brand.ink)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 6)
                        .background(Brand.surface, in: Capsule())
                        .overlay(Capsule().stroke(Brand.line))
                        .transition(.opacity)
                }
                Spacer()
                if ws.note != nil {
                    Text(ws.words == 1 ? "1 word" : "\(ws.words) words")
                        .font(Brand.mono(11))
                        .foregroundStyle(Brand.muted)
                }
            }
            .padding(.horizontal, 18)
            .padding(.bottom, 10)
            .animation(.easeOut(duration: 0.2), value: ws.flash)
        }
        .background(Brand.ground)
        .navigationTitle("")
        .toolbar {
            ToolbarItem(placement: .navigation) {
                Breadcrumb(path: ws.note?.path ?? "", title: ws.note.map { ws.index?.title($0.path) ?? $0.path } ?? "")
            }
            .sharedBackgroundVisibility(.hidden)
            ToolbarItem(placement: .primaryAction) {
                Button {
                    ws.palette = .text
                } label: {
                    Image(systemName: "magnifyingglass")
                }
                .help("Search notes  ⇧⌘F")
                .accessibilityLabel("Search notes")
            }
        }
    }
}

/// Breadcrumb is the note's folders, then its title.
struct Breadcrumb: View {
    let path: String
    let title: String

    var body: some View {
        let folders = SidebarTree.ancestors(path).map { ($0 as NSString).lastPathComponent }
        HStack(spacing: 6) {
            ForEach(Array(folders.enumerated()), id: \.offset) { _, name in
                Text(name).foregroundStyle(Brand.muted)
                Text("›").foregroundStyle(Brand.faint)
            }
            Text(title)
                .foregroundStyle(Brand.ink)
                .fontWeight(.medium)
                .lineLimit(1)
        }
        .font(.system(size: 13))
        .padding(.horizontal, 6)
        .accessibilityElement(children: .combine)
    }
}
