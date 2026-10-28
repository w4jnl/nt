import SwiftUI

/// TodosView is every todo in the notes, as the terminal browser's todo list shows them: the open
/// ones overdue first with their dates in words, a field that adds one to todo.md, and the done
/// ones below. A click on ○ checks a todo; arrows move, space checks, d sets the due date and
/// Return opens the note at the todo.
///
/// A tick shows at once: the glyph turns, with a small pop, and the text fades. The row stays
/// where it is until the ticks settle, a moment after the last, so the person sees what they did
/// and can take it back; then it moves to the done ones, or back up to the open ones.
struct TodosView: View {
    @Environment(Workspace.self) private var ws
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var newText = ""
    @State private var selected: String?
    @State private var editingDue: String?
    @State private var dueText = ""
    @State private var showDone = true
    @State private var frozen: [Todo]? // the list as it stood at the first tick, while the ticks settle
    @State private var ticked: [String: Bool] = [:] // a todo just ticked → the state it was given
    @State private var after: [String: String] = [:] // a todo just ticked → the open one below it
    @State private var settle: Task<Void, Never>?
    @State private var writes: Task<Void, Never>?
    @FocusState private var focus: Field?
    @Namespace private var rows

    enum Field: Hashable { case add, list, due }

    /// shown is the list on screen: as it stood at the first tick while ticks settle, so a row
    /// just ticked stays in its place, else as nt lists it.
    private var shown: [Todo] { frozen ?? ws.todos }

    private func isDone(_ t: Todo) -> Bool { ticked[t.id] ?? t.done }

    var body: some View {
        let open = shown.filter { !$0.done }
        let done = shown.filter(\.done)
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                Text("Todos")
                    .font(.system(size: 28, weight: .semibold))
                    .kerning(-0.3)
                    .foregroundStyle(Brand.ink)
                    .padding(.bottom, 4)
                summary(open)
                    .padding(.bottom, 22)
                addField
                    .padding(.bottom, 18)
                if open.isEmpty {
                    Text("Nothing open. Add a todo above, or write - [ ] in any note.")
                        .font(.system(size: 14))
                        .foregroundStyle(Brand.muted)
                        .padding(.horizontal, 10)
                }
                ForEach(open) { row($0) }
                if !done.isEmpty {
                    Button {
                        showDone.toggle()
                    } label: {
                        HStack(spacing: 8) {
                            Image(systemName: showDone ? "chevron.down" : "chevron.right")
                                .font(.system(size: 9, weight: .bold))
                            Text("Done").fontWeight(.semibold)
                            Text("\(done.count)").font(Brand.mono(11))
                        }
                        .font(.system(size: 12))
                        .foregroundStyle(Brand.muted)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 4)
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .padding(.top, 26)
                    .padding(.bottom, 6)
                    .accessibilityValue(showDone ? "shown" : "hidden")
                    if showDone {
                        ForEach(done) { row($0) }
                    }
                }
            }
            .frame(maxWidth: 640, alignment: .leading)
            .padding(EdgeInsets(top: 22, leading: 40, bottom: 96, trailing: 40))
            .frame(maxWidth: .infinity)
        }
        .background(Brand.ground)
        .focusable()
        .focused($focus, equals: .list)
        .focusEffectDisabled()
        .onKeyPress(.downArrow) { move(1, open + (showDone ? done : [])); return .handled }
        .onKeyPress(.upArrow) { move(-1, open + (showDone ? done : [])); return .handled }
        .onKeyPress(.space) { act { tick($0) } }
        .onKeyPress(.return) { act { t in ws.show(t.rel, line: t.line) } }
        .onKeyPress("d") { act { t in startDue(t) } }
        .viewToolbar("Todos")
    }

    private func summary(_ open: [Todo]) -> some View {
        let late = open.filter(\.late).count
        var s = AttributedString("\(open.count) open")
        if late > 0 {
            s += AttributedString(" · ")
            var l = AttributedString("\(late) late")
            l.foregroundColor = Brand.warn
            s += l
        }
        s += AttributedString(" · from every note")
        return Text(s)
            .font(.system(size: 13))
            .foregroundStyle(Brand.muted)
    }

    private var addField: some View {
        HStack(spacing: 10) {
            Text("+")
                .font(.system(size: 15))
                .foregroundStyle(Brand.muted)
                .frame(width: 20)
                .accessibilityHidden(true)
            TextField("Add a todo, e.g. Renew the certificate due:fri", text: $newText)
                .textFieldStyle(.plain)
                .font(.system(size: 15))
                .focused($focus, equals: .add)
                .onSubmit(add)
                .accessibilityLabel("New todo")
            Text("↵ todo.md")
                .font(Brand.mono(11))
                .foregroundStyle(Brand.muted)
        }
        .padding(.horizontal, 12)
        .frame(height: 40)
        .background(Brand.sunk, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(Brand.line))
    }

    private func row(_ t: Todo) -> some View {
        let done = isDone(t)
        let isSelected = selected == t.id && !t.done
        return HStack(spacing: 12) {
            Button {
                tick(t)
            } label: {
                Text(done ? "✓" : "○")
                    .font(.system(size: 16))
                    .foregroundStyle(done ? Brand.ok : Brand.muted)
                    .frame(width: 20)
                    .keyframeAnimator(initialValue: 1.0, trigger: reduceMotion ? false : done) { glyph, scale in
                        glyph.scaleEffect(scale)
                    } keyframes: { _ in
                        KeyframeTrack {
                            CubicKeyframe(1.45, duration: 0.11)
                            SpringKeyframe(1.0, duration: 0.32, spring: .bouncy)
                        }
                    }
            }
            .buttonStyle(.plain)
            .help(done ? "Open it again" : "Mark it done")
            .accessibilityLabel(done ? "Done; open it again" : "Open; mark it done")
            Text(t.text)
                .font(.system(size: 15))
                .foregroundStyle(done ? Brand.muted : Brand.ink)
                .frame(maxWidth: .infinity, alignment: .leading)
                .lineLimit(2)
            Button(source(t)) { ws.show(t.rel, line: t.line) }
                .buttonStyle(.plain)
                .font(.system(size: 12))
                .foregroundStyle(Brand.muted)
                .lineLimit(1)
                .help("Open \(t.rel) at the todo")
            if editingDue == t.id {
                TextField("fri, +3d, none", text: $dueText)
                    .textFieldStyle(.plain)
                    .font(Brand.mono(12))
                    .frame(width: 110)
                    .focused($focus, equals: .due)
                    .onSubmit { setDue(t) }
                    .onKeyPress(.escape) { editingDue = nil; focus = .list; return .handled }
            } else {
                Text(t.when)
                    .font(Brand.mono(12))
                    .foregroundStyle(t.late ? Brand.warn : Brand.muted)
                    .frame(width: 64, alignment: .trailing)
            }
        }
        .padding(.horizontal, 10)
        .frame(minHeight: 40)
        .background(isSelected ? Brand.accentSoft : .clear, in: RoundedRectangle(cornerRadius: 8))
        .overlay(alignment: .leading) {
            if isSelected {
                RoundedRectangle(cornerRadius: 2).fill(Brand.accent).frame(width: 3, height: 16)
            }
        }
        .animation(reduceMotion ? nil : .easeOut(duration: 0.2), value: done)
        .matchedGeometryEffect(id: t.id, in: rows)
        .contentShape(Rectangle())
        .onTapGesture {
            selected = t.id
            focus = .list
        }
        .onTapGesture(count: 2) { ws.show(t.rel, line: t.line) }
        .contextMenu {
            Button(done ? "Open Again" : "Done") { tick(t) }
            if !done { Button("Set Due Date…") { startDue(t) } }
            Button("Open Note") { ws.show(t.rel, line: t.line) }
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel([t.text, t.when, source(t)].filter { !$0.isEmpty }.joined(separator: ", "))
    }

    /// source is where a todo is: a todo list by its path, any other note by its title.
    private func source(_ t: Todo) -> String {
        (t.rel as NSString).lastPathComponent == "todo.md" ? t.rel : ws.title(t.rel)
    }

    /// tick checks a todo, or opens it again: the glyph turns at once and nt writes it, one
    /// write after the other. The row stays in its place until the ticks settle.
    private func tick(_ t: Todo) {
        let base = shown
        if frozen == nil { frozen = base }
        let now = !isDone(t)
        let open = base.filter { !$0.done }
        if let i = open.firstIndex(where: { $0.id == t.id }) {
            after[t.id] = open.dropFirst(i + 1).first { ticked[$0.id] == nil }?.id
        }
        withAnimation(reduceMotion ? nil : .snappy(duration: 0.2)) {
            ticked[t.id] = now == t.done ? nil : now
        }
        let before = writes
        writes = Task {
            await before?.value
            await ws.setDone(t, now)
        }
        settle?.cancel()
        settle = Task {
            try? await Task.sleep(for: .milliseconds(1100))
            await writes?.value
            if !Task.isCancelled { land() }
        }
    }

    /// land lets the ticked rows go where they belong now; the selection moves from a row that
    /// left the open ones to the one that was below it.
    private func land() {
        if let s = selected, ticked[s] == true, let next = after[s] { selected = next }
        withAnimation(reduceMotion ? nil : .smooth(duration: 0.45)) {
            frozen = nil
            ticked = [:]
        }
        after = [:]
    }

    /// settled lands the ticks now, for a change that rereads the list (a todo added, a due date).
    private func settled() async {
        settle?.cancel()
        await writes?.value
        if frozen != nil { land() }
    }

    private func add() {
        let text = newText.trimmingCharacters(in: .whitespaces)
        guard !text.isEmpty else { return }
        Task {
            await settled()
            if await ws.addTodo(text) { newText = "" }
        }
    }

    private func move(_ d: Int, _ rows: [Todo]) {
        guard !rows.isEmpty else { return }
        let i = rows.firstIndex { $0.id == selected } ?? (d > 0 ? -1 : rows.count)
        selected = rows[min(max(i + d, 0), rows.count - 1)].id
    }

    /// act runs f on the selected todo.
    private func act(_ f: (Todo) -> Void) -> KeyPress.Result {
        guard editingDue == nil, focus == .list, let t = shown.first(where: { $0.id == selected }) else { return .ignored }
        f(t)
        return .handled
    }

    private func startDue(_ t: Todo) {
        guard !isDone(t) else { return }
        selected = t.id
        dueText = t.due
        editingDue = t.id
        focus = .due
    }

    private func setDue(_ t: Todo) {
        let when = dueText.trimmingCharacters(in: .whitespaces)
        editingDue = nil
        focus = .list
        guard !when.isEmpty else { return }
        Task {
            await settled()
            await ws.setDue(t, when)
        }
    }
}

extension View {
    /// viewToolbar is the toolbar of a view in the note's place: its name, and Search.
    func viewToolbar(_ title: String) -> some View {
        // no bar behind the toolbar: the views sit on ground under it, as the note does
        scrollEdgeEffectHidden(true, for: .top)
            .toolbarBackgroundVisibility(.hidden, for: .windowToolbar)
            .navigationTitle("")
            .toolbar {
                ToolbarItem(placement: .navigation) {
                    Text(title)
                        .font(.system(size: 13, weight: .medium))
                        .foregroundStyle(Brand.ink)
                        .padding(.horizontal, 6)
                }
                .sharedBackgroundVisibility(.hidden)
                ToolbarItem(placement: .primaryAction) {
                    SearchButton()
                }
            }
    }
}

/// SearchButton opens Search (⇧⌘F).
struct SearchButton: View {
    @Environment(Workspace.self) private var ws

    var body: some View {
        Button {
            ws.palette = .text
        } label: {
            Image(systemName: "magnifyingglass")
        }
        .help("Search notes  ⇧⌘F")
        .accessibilityLabel("Search notes")
    }
}
