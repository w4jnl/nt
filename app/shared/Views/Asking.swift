// The questions before nt writes a note or a folder: a name for a new one or a new name, the
// folder to move to, and whether to delete, saying what goes along and how many notes link to
// it. ItemMenu is what can be done to a note or a folder, for its menus.
import SwiftUI

struct Asking: ViewModifier {
    @Environment(Workspace.self) private var ws
    @State private var name = ""
    @State private var removal: Workspace.Removal?

    func body(content: Content) -> some View {
        content
            .alert(title, isPresented: Binding(get: { showsAlert }, set: { if !$0 { ws.ask = nil } })) {
                switch ws.ask {
                case .newNote, .newFolder, .rename:
                    TextField(placeholder, text: $name)
                    Button(action) { answer() }
                    Button("Cancel", role: .cancel) {}
                case .delete:
                    Button("Delete", role: .destructive) { answer() }
                    Button("Cancel", role: .cancel) {}
                default:
                    EmptyView()
                }
            } message: {
                Text(message)
            }
            .sheet(isPresented: Binding(get: { if case .move = ws.ask { true } else { false } }, set: { if !$0 { ws.ask = nil } })) {
                if case .move(let path) = ws.ask {
                    MoveSheet(path: path)
                }
            }
            .task(id: ws.ask) {
                removal = nil
                switch ws.ask {
                case .rename(let path):
                    name = path.hasSuffix(".md") ? ws.title(path) : (path as NSString).lastPathComponent
                case .delete(let path):
                    removal = await ws.removal(path)
                    if removal == nil { ws.ask = nil }
                default:
                    name = ""
                }
            }
    }

    /// showsAlert: a name is asked for, or a delete once what it takes is known.
    private var showsAlert: Bool {
        switch ws.ask {
        case .newNote, .newFolder, .rename: true
        case .delete: removal != nil
        default: false
        }
    }

    private var title: String {
        switch ws.ask {
        case .newNote(let f): f.isEmpty ? "New note" : "New note in \(f)"
        case .newFolder(let f): f.isEmpty ? "New folder" : "New folder in \(f)"
        case .rename(let p): p.hasSuffix(".md") ? "Rename the note" : "Rename \((p as NSString).lastPathComponent)"
        case .delete(let p): "Delete \(removal?.title.map { "“\($0)”" } ?? (p as NSString).lastPathComponent)?"
        default: ""
        }
    }

    private var placeholder: String {
        if case .rename(let p) = ws.ask, !p.hasSuffix(".md") { return "Name" }
        if case .newFolder = ws.ask { return "Name" }
        return "Title"
    }

    private var action: String {
        if case .rename = ws.ask { return "Rename" }
        return "Create"
    }

    private var message: String {
        switch ws.ask {
        case .newNote: return "Its title; the file is named after it, with today's date."
        case .rename(let p): return p.hasSuffix(".md") ? "The title and the file name change; the links to it are fixed." : "The links into it are fixed."
        case .delete:
            guard let r = removal else { return "" }
            var parts: [String] = []
            if let n = r.images?.count, n > 0 { parts.append(n == 1 ? "1 image only it uses goes with it." : "\(n) images only it uses go with it.") }
            if r.links > 0 { parts.append(r.links == 1 ? "1 note links to it." : "\(r.links) notes link to it.") }
            parts.append("Sync commits it, and git keeps the files.")
            return parts.joined(separator: " ")
        default: return ""
        }
    }

    /// answer does what was asked. The question and the name are taken at once: closing the
    /// alert clears the question before a task would run.
    private func answer() {
        let ask = ws.ask
        let name = name
        ws.ask = nil
        Task {
            switch ask {
            case .newNote(let f): await ws.create(name, in: f)
            case .newFolder(let f): await ws.makeFolder(name, in: f)
            case .rename(let p): if !name.trimmingCharacters(in: .whitespaces).isEmpty { await ws.rename(p, to: name) }
            case .delete(let p): await ws.remove(p)
            default: break
            }
        }
    }
}

/// MoveSheet picks the folder a note or a folder moves to; a name that is no folder yet makes one.
struct MoveSheet: View {
    let path: String
    @Environment(Workspace.self) private var ws
    @State private var query = ""

    var body: some View {
        NavigationStack {
            List {
                let q = query.trimmingCharacters(in: CharacterSet(charactersIn: " /"))
                let list = folders.filter { q.isEmpty || $0.localizedCaseInsensitiveContains(q) }
                if !q.isEmpty && !folders.contains(q) {
                    Button { go(q) } label: {
                        Label("New folder \(q)", systemImage: "folder.badge.plus")
                    }
                }
                ForEach(list, id: \.self) { f in
                    Button { go(f) } label: {
                        Label(f.isEmpty ? "/ the root" : f, systemImage: "folder")
                            .foregroundStyle(Brand.ink)
                    }
                }
            }
            .searchable(text: $query, prompt: "Folder")
            .navigationTitle("Move \(path.hasSuffix(".md") ? ws.title(path) : (path as NSString).lastPathComponent)")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { ws.ask = nil } }
            }
        }
        #if os(macOS)
        .frame(minWidth: 380, minHeight: 440)
        #endif
    }

    /// folders are where it can go: not the folder it is in, not itself or inside itself, not
    /// assets or hidden folders, and not the journals', agents' or weekly folders.
    private var folders: [String] {
        let here = (path as NSString).deletingLastPathComponent
        let dirs = [""] + (ws.index?.dirs ?? [])
        return dirs.filter { d in
            d != here && d != path && !d.hasPrefix(path + "/") && !ws.fixed(d)
                && !d.split(separator: "/").contains { $0 == "assets" || $0.hasPrefix(".") }
        }
    }

    private func go(_ folder: String) {
        ws.ask = nil
        Task { await ws.move(path, to: folder) }
    }
}

/// ItemMenu is what can be done to a note or a folder: in a folder, a new note or folder; and
/// rename, move and delete where nt allows them.
struct ItemMenu: View {
    @Environment(Workspace.self) private var ws
    let path: String
    let folder: Bool

    var body: some View {
        if folder {
            Button("New Note…") { ws.ask = .newNote(path) }
            Button("New Folder…") { ws.ask = .newFolder(path) }
        }
        if !ws.fixed(path) {
            Button("Rename…") { ws.ask = .rename(path) }
            Button("Move…") { ws.ask = .move(path) }
        }
        if ws.deletable(path) {
            Divider()
            Button("Delete…", role: .destructive) { ws.ask = .delete(path) }
        }
    }
}

extension View {
    /// itemActions are what can be done to a row's note or folder: a right-click or a long press
    /// has the menu; on iPhone and iPad a swipe to the right moves or renames, as Mail's rows
    /// mark, and a swipe to the left deletes (after the question), a full swipe the first of each.
    func itemActions(_ path: String, folder: Bool) -> some View {
        modifier(ItemActions(path: path, folder: folder))
    }
}

struct ItemActions: ViewModifier {
    @Environment(Workspace.self) private var ws
    let path: String
    let folder: Bool

    func body(content: Content) -> some View {
        content
            .contextMenu { ItemMenu(path: path, folder: folder) }
            #if os(iOS)
            .swipeActions(edge: .leading) {
                if !ws.fixed(path) {
                    Button { ws.ask = .move(path) } label: { Label("Move", systemImage: "folder") }
                        .tint(Brand.accent)
                    Button { ws.ask = .rename(path) } label: { Label("Rename", systemImage: "pencil") }
                        .tint(Brand.muted)
                }
            }
            .swipeActions(edge: .trailing) {
                // no destructive role: that takes the row away before the question is answered
                if ws.deletable(path) {
                    Button { ws.ask = .delete(path) } label: { Label("Delete", systemImage: "trash") }
                        .tint(Brand.alert)
                }
            }
            #endif
    }
}
