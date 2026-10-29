import SwiftUI

/// TodosView is every todo in the notes, as the terminal browser's todo list shows them: the open
/// ones overdue first with their dates in words, a field that adds one to todo.md, and the done
/// ones below. "from every note" in the summary is a menu that keeps to one top-level folder's
/// todos, as `nt todo <folder>` does; a todo added then goes to that folder's todo.md. A click on ○ checks a todo; arrows move, space checks, d sets the due date and
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
    @State private var scope = "" // the top-level folder whose todos show; "" is every note
    @State private var frozen: [Todo]? // the list as it stood at the first tick, while the ticks settle
    @State private var ticked: [String: Bool] = [:] // a todo just ticked → the state it was given
    @State private var after: [String: String] = [:] // a todo just ticked → the open one below it
    @State private var settle: Task<Void, Never>?
    @State private var writes: Task<Void, Never>?
    @FocusState private var focus: Field?
    @Namespace private var rows
    #if os(iOS)
    @Environment(\.horizontalSizeClass) private var size
    #endif

    /// compact: an iPhone's width, where a todo's note goes under its text.
    private var compact: Bool {
        #if os(iOS)
        size == .compact
        #else
        false
        #endif
    }

    enum Field: Hashable { case add, list, due }

    /// shown is the list on screen: as it stood at the first tick while ticks settle, so a row
    /// just ticked stays in its place, else as nt lists it; in the scope.
    private var shown: [Todo] {
        let all = frozen ?? ws.todos
        return scope.isEmpty ? all : all.filter { $0.folder == scope }
    }

    private func isDone(_ t: Todo) -> Bool { ticked[t.id] ?? t.done }

    var body: some View {
        let open = shown.filter { !$0.done }
        let done = shown.filter(\.done)
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                ViewTitle(text: "Todos", below: 4)
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
            .viewInsets(bottom: 96)
            .frame(maxWidth: .infinity)
        }
        .background(Brand.ground)
        .focusable()
        .focused($focus, equals: .list)
        .focusEffectDisabled()
        .onKeyPress(.downArrow) { move(1, open + (showDone ? done : [])); return .handled }
        .onKeyPress(.upArrow) { move(-1, open + (showDone ? done : [])); return .handled }
        .onKeyPress(.space) { act { tick($0) } }
        .onKeyPress(.return) { act { t in ws.go(t.rel, line: t.line) } }
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
        s += AttributedString(" · ")
        return HStack(spacing: 0) {
            Text(s)
            scopeMenu
        }
        .font(.system(size: 13))
        .foregroundStyle(Brand.muted)
    }

    /// scopeMenu is "from every note", or the folder kept to, as a menu of every note and the
    /// top-level folders with open todos.
    private var scopeMenu: some View {
        let all = frozen ?? ws.todos
        return Menu {
            Picker("Todos from", selection: $scope) {
                Text("every note · \(all.filter { !$0.done }.count) open").tag("")
                ForEach(Todo.scopes(all, keep: scope), id: \.folder) { s in
                    Text("\(s.folder) · \(s.open) open").tag(s.folder)
                }
            }
            .pickerStyle(.inline)
        } label: {
            HStack(spacing: 3) {
                Text(scope.isEmpty ? "from every note" : "from \(scope)")
                Image(systemName: "chevron.down")
                    .font(.system(size: 8, weight: .semibold))
            }
            .contentShape(Rectangle())
        }
        .menuIndicator(.hidden)
        #if os(macOS)
        .menuStyle(.borderlessButton)
        #else
        .buttonStyle(.plain)
        #endif
        .fixedSize()
        .onChange(of: scope) { selected = nil }
        .help("Keep to one folder's todos")
        .accessibilityLabel(scope.isEmpty ? "Todos from every note" : "Todos from \(scope)")
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
            Text(scope.isEmpty ? "↵ todo.md" : "↵ \(scope)/todo.md")
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
        let isSelected = selected == t.id
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
                            CubicKeyframe(1.4, duration: 0.08)
                            SpringKeyframe(1.0, duration: 0.24, spring: .bouncy)
                        }
                    }
            }
            .buttonStyle(.plain)
            .help(done ? "Open it again" : "Mark it done")
            .accessibilityLabel(done ? "Done; open it again" : "Open; mark it done")
            let text = Text(t.text)
                .font(.system(size: 15))
                .foregroundStyle(done ? Brand.muted : Brand.ink)
                .lineLimit(2)
            let from = Button(source(t)) { ws.go(t.rel, line: t.line) }
                .buttonStyle(.plain)
                .font(.system(size: compact ? 13 : 12))
                .foregroundStyle(Brand.muted)
                .lineLimit(1)
                .help("Open \(t.rel) at the todo")
            if compact {
                VStack(alignment: .leading, spacing: 2) {
                    text
                    from
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            } else {
                text.frame(maxWidth: .infinity, alignment: .leading)
                from
            }
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
                    .fixedSize()
                    .frame(minWidth: 64, alignment: .trailing)
            }
        }
        .padding(.horizontal, 10)
        .frame(minHeight: compact ? 52 : 40)
        .background(isSelected ? Brand.accentSoft : .clear, in: RoundedRectangle(cornerRadius: 8))
        .overlay(alignment: .leading) {
            if isSelected {
                RoundedRectangle(cornerRadius: 2).fill(Brand.accent).frame(width: 3, height: 16)
            }
        }
        .animation(reduceMotion ? nil : .easeOut(duration: 0.15), value: done)
        .matchedGeometryEffect(id: t.id, in: rows)
        .contentShape(Rectangle())
        .onTapGesture {
            selected = t.id
            focus = .list
        }
        .onTapGesture(count: 2) { ws.go(t.rel, line: t.line) }
        .contextMenu {
            Button(done ? "Open Again" : "Done") { tick(t) }
            if !done { Button("Set Due Date…") { startDue(t) } }
            Button("Open Note") { ws.go(t.rel, line: t.line) }
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
        withAnimation(reduceMotion ? nil : .snappy(duration: 0.15)) {
            ticked[t.id] = now == t.done ? nil : now
        }
        let before = writes
        writes = Task {
            await before?.value
            await ws.setDone(t, now)
        }
        settle?.cancel()
        settle = Task {
            try? await Task.sleep(for: .milliseconds(700))
            await writes?.value
            if !Task.isCancelled { land() }
        }
    }

    /// land lets the ticked rows go where they belong now; the selection moves from a row that
    /// left the open ones to the one that was below it.
    private func land() {
        if let s = selected, ticked[s] == true, let next = after[s] { selected = next }
        withAnimation(reduceMotion ? nil : .smooth(duration: 0.3)) {
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
            if await ws.addTodo(text, in: scope) { newText = "" }
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
    #if os(macOS)
    /// viewToolbar is the toolbar of a view in the note's place: its name, and Search.
    func viewToolbar(_ title: String, large: Bool = true) -> some View {
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
    #else
    /// viewToolbar names a view in the navigation bar: large, or small for a view that heads
    /// itself (the calendar's month). Search is on the home list.
    func viewToolbar(_ title: String, large: Bool = true) -> some View {
        navigationTitle(title)
            .navigationBarTitleDisplayMode(large ? .large : .inline)
    }
    #endif

    /// viewInsets are a view's margins: narrower on an iPhone.
    func viewInsets(bottom: CGFloat) -> some View { modifier(ViewInsets(bottom: bottom)) }
}

struct ViewInsets: ViewModifier {
    let bottom: CGFloat
    #if os(iOS)
    @Environment(\.horizontalSizeClass) private var size
    #endif

    func body(content: Content) -> some View {
        #if os(iOS)
        if size == .compact {
            return content.padding(EdgeInsets(top: 4, leading: 20, bottom: bottom, trailing: 20))
        }
        #endif
        return content.padding(EdgeInsets(top: 22, leading: 40, bottom: bottom, trailing: 40))
    }
}

/// ViewTitle is a view's name above it on the Mac; on iPhone and iPad the navigation bar's large
/// title says it.
struct ViewTitle: View {
    let text: String
    var below: CGFloat = 0

    var body: some View {
        #if os(macOS)
        Text(text)
            .font(.system(size: 28, weight: .semibold))
            .kerning(-0.3)
            .foregroundStyle(Brand.ink)
            .padding(.bottom, below)
        #endif
    }
}

#if os(macOS)
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
#endif
