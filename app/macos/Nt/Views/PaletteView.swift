import SwiftUI

/// PaletteView is Open (⌘P): one field that finds notes by title, then by text, and makes a new
/// note when the person picks that; Search (⇧⌘F), the same field for text only; and New note
/// (⌘N), the same field for a new note's title. Arrows move, Return opens, Escape closes.
struct PaletteView: View {
    let mode: Workspace.PaletteMode
    @Environment(Workspace.self) private var ws
    @State private var query = ""
    @State private var sel = 0
    @State private var hits: [Workspace.SearchHit] = []
    @FocusState private var focused: Bool

    struct Row: Identifiable, Equatable {
        enum Kind { case title, text, create }
        let kind: Kind
        let path: String
        let title: String
        let at: [Int]
        let snippet: String
        let line: Int
        let meta: String
        var id: String { "\(kind)·\(path)·\(line)" }
    }

    var body: some View {
        let rows = rows()
        ZStack(alignment: .top) {
            Color.black.opacity(0.18)
                .ignoresSafeArea()
                .onTapGesture { close() }
                .accessibilityHidden(true)
            VStack(spacing: 0) {
                HStack(spacing: 12) {
                    Image(systemName: "magnifyingglass")
                        .font(.system(size: 15))
                        .foregroundStyle(Brand.muted)
                    TextField(placeholder, text: $query)
                        .textFieldStyle(.plain)
                        .font(.system(size: 18))
                        .focused($focused)
                        .onSubmit { open(rows) }
                        .onKeyPress(.downArrow) { move(1, rows.count); return .handled }
                        .onKeyPress(.upArrow) { move(-1, rows.count); return .handled }
                        .onKeyPress(.escape) { close(); return .handled }
                    Text("esc")
                        .font(Brand.mono(11))
                        .foregroundStyle(Brand.muted)
                }
                .padding(.horizontal, 18)
                .frame(height: 56)
                if mode != .new && (!rows.isEmpty || !query.isEmpty) {
                    Rectangle().fill(Brand.line).frame(height: 1)
                    results(rows)
                }
                Rectangle().fill(Brand.line).frame(height: 1)
                hints
            }
            .frame(width: 600)
            .background(Brand.surface, in: RoundedRectangle(cornerRadius: 14))
            .overlay(RoundedRectangle(cornerRadius: 14).stroke(Brand.line))
            .shadow(color: .black.opacity(0.28), radius: 28, y: 12)
            .padding(.top, 64)
        }
        .onAppear {
            focused = true
            query = ws.searchAsked ?? UserDefaults.standard.string(forKey: "NTQuery") ?? ""
            ws.searchAsked = nil
        }
        .onChange(of: query) { sel = 0 }
        .task(id: query) {
            let q = query.trimmingCharacters(in: .whitespaces)
            guard q.count >= 2, mode != .new else {
                hits = []
                return
            }
            try? await Task.sleep(for: .milliseconds(180))
            guard !Task.isCancelled else { return }
            let found = await ws.search(q)
            if !Task.isCancelled { hits = found }
        }
    }

    private func results(_ rows: [Row]) -> some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 2) {
                    if rows.isEmpty {
                        Text(query.count < 2 && mode == .text ? "Type two letters or more." : "No notes match.")
                            .font(.system(size: 13))
                            .foregroundStyle(Brand.muted)
                            .padding(.horizontal, 12)
                            .padding(.vertical, 10)
                    }
                    ForEach(Array(rows.enumerated()), id: \.element.id) { i, row in
                        if i == 0 || rows[i - 1].kind != row.kind {
                            Text(row.kind == .title ? "Notes" : row.kind == .text ? "In text" : "New")
                                .font(.system(size: 11, weight: .semibold))
                                .foregroundStyle(Brand.muted)
                                .padding(.horizontal, 12)
                                .padding(.top, i == 0 ? 4 : 10)
                                .padding(.bottom, 2)
                        }
                        Button {
                            sel = i
                            open(rows)
                        } label: {
                            rowView(row, selected: i == sel)
                        }
                        .buttonStyle(.plain)
                        .id(row.id)
                    }
                }
                .padding(8)
            }
            .frame(maxHeight: 380)
            .onChange(of: sel) {
                if rows.indices.contains(sel) { proxy.scrollTo(rows[sel].id) }
            }
        }
    }

    private func rowView(_ row: Row, selected: Bool) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 12) {
                Text(highlightTitle(row))
                    .font(.system(size: 15))
                    .lineLimit(1)
                Spacer(minLength: 8)
                Text(row.meta)
                    .font(Brand.mono(11))
                    .foregroundStyle(Brand.muted)
                    .lineLimit(1)
            }
            if row.kind == .text {
                Text(highlightSnippet(row.snippet))
                    .font(.system(size: 13))
                    .foregroundStyle(Brand.muted)
                    .lineLimit(1)
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(selected ? Brand.accentSoft : .clear, in: RoundedRectangle(cornerRadius: 8))
        .overlay(alignment: .leading) {
            if selected {
                RoundedRectangle(cornerRadius: 2).fill(Brand.accent).frame(width: 3, height: 16)
            }
        }
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(selected ? .isSelected : [])
    }

    private var placeholder: String {
        switch mode {
        case .open: "Open a note"
        case .text: "Search the notes"
        case .new: ws.newFolder.isEmpty ? "New note: its title" : "New note in \(ws.newFolder): its title"
        }
    }

    private var hints: some View {
        HStack(spacing: 18) {
            hint("↵", mode == .new ? "make it" : "open")
            if mode == .open { hint("⇧⌘F", "text only") }
            Spacer()
            hint("esc", "close")
        }
        .font(.system(size: 12))
        .padding(.horizontal, 20)
        .frame(height: 36)
    }

    private func hint(_ key: String, _ verb: String) -> some View {
        HStack(spacing: 4) {
            Text(key).font(Brand.mono(11)).foregroundStyle(Brand.accent)
            Text(verb).foregroundStyle(Brand.muted)
        }
    }

    // MARK: Rows

    private func rows() -> [Row] {
        guard let ix = ws.index else { return [] }
        var out: [Row] = []
        let q = query.trimmingCharacters(in: .whitespaces)
        if mode == .open {
            if q.isEmpty {
                return []
            }
            for hit in Fuzzy.find(q, in: ix.notes, limit: 8) {
                out.append(Row(kind: .title, path: hit.note.path, title: hit.note.title, at: hit.at, snippet: "", line: 0, meta: meta(hit.note.path)))
            }
        }
        if mode == .new { return [] }
        var seen = Set(out.map(\.path))
        for h in hits where !seen.contains(h.path) {
            seen.insert(h.path)
            out.append(Row(kind: .text, path: h.path, title: ix.title(h.path), at: [], snippet: h.text.trimmingCharacters(in: .whitespaces), line: h.line, meta: meta(h.path)))
            if out.count >= 16 { break }
        }
        if mode == .open, !out.contains(where: { $0.kind == .title && $0.title.caseInsensitiveCompare(q) == .orderedSame }) {
            let folder = ws.newFolder
            out.append(Row(kind: .create, path: "", title: "New note “\(q)”", at: [], snippet: "", line: 0, meta: folder.isEmpty ? "" : folder))
        }
        return out
    }

    private func meta(_ path: String) -> String {
        let dir = (path as NSString).deletingLastPathComponent
        let date = SidebarTree.shortDate(path, thisYear: String(ws.index?.day.prefix(4) ?? ""))
        return [dir.isEmpty ? "/" : dir, date].filter { !$0.isEmpty }.joined(separator: " · ")
    }

    private func highlightTitle(_ row: Row) -> AttributedString {
        var s = AttributedString()
        for (i, ch) in row.title.enumerated() {
            var part = AttributedString(String(ch))
            if row.at.contains(i) {
                part.foregroundColor = Brand.accent
                part.font = .system(size: 15, weight: .semibold)
            } else {
                part.foregroundColor = Brand.ink
            }
            s += part
        }
        return s
    }

    private func highlightSnippet(_ text: String) -> AttributedString {
        let line = text.count > 160 ? String(text.prefix(159)) + "…" : text
        var s = AttributedString(line)
        let q = query.trimmingCharacters(in: .whitespaces)
        if !q.isEmpty, let r = s.range(of: q, options: .caseInsensitive) {
            s[r].foregroundColor = Brand.ink
            s[r].backgroundColor = Brand.accentSoft
        }
        return s
    }

    // MARK: Actions

    private func move(_ d: Int, _ count: Int) {
        guard count > 0 else { return }
        sel = min(max(sel + d, 0), count - 1)
    }

    private func open(_ rows: [Row]) {
        if mode == .new {
            let title = query
            close()
            Task { await ws.create(title, in: ws.newFolder) }
            return
        }
        guard rows.indices.contains(sel) else { return }
        let row = rows[sel]
        close()
        if row.kind == .create {
            let title = query.trimmingCharacters(in: .whitespaces)
            Task { await ws.create(title, in: ws.newFolder) }
            return
        }
        ws.show(row.path, line: row.kind == .text ? row.line : 0)
    }

    private func close() {
        ws.palette = nil
    }
}
