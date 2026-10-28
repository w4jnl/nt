import SwiftUI

/// CalendarView is a month, Monday first, beside the day chosen in it: a day with a journal in
/// `ink`, today in `accent`, `•` under a day with open todos due (in `warn` once it has passed).
/// The day shows its journal's Log, its agent sessions and its todos. Arrows move the day, ‹ ›
/// the month; a second click on a day, or Return, opens its journal.
struct CalendarView: View {
    @Environment(Workspace.self) private var ws
    @FocusState private var focused: Bool

    var body: some View {
        let day = ws.calendarDay.isEmpty ? ws.today : ws.calendarDay
        ScrollView {
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .top, spacing: 48) {
                    month(day).frame(width: 420)
                    dayPane(day).frame(minWidth: 300, maxWidth: 420, alignment: .leading)
                }
                VStack(alignment: .leading, spacing: 36) {
                    month(day).frame(maxWidth: 420)
                    dayPane(day)
                }
            }
            .viewInsets(bottom: 64)
            .frame(maxWidth: .infinity)
        }
        .background(Brand.ground)
        .focusable()
        .focused($focused)
        .focusEffectDisabled()
        .onAppear { focused = true }
        .onKeyPress(.leftArrow) { go(day, days: -1) }
        .onKeyPress(.rightArrow) { go(day, days: 1) }
        .onKeyPress(.upArrow) { go(day, days: -7) }
        .onKeyPress(.downArrow) { go(day, days: 7) }
        .onKeyPress(.return) {
            openJournal(day)
            return .handled
        }
        .viewToolbar("Calendar", large: false)
    }

    private func go(_ day: String, days: Int = 0, months: Int = 0) -> KeyPress.Result {
        ws.calendarDay = Agenda.shift(day, days: days, months: months)
        return .handled
    }

    // MARK: The month

    private func month(_ day: String) -> some View {
        let p = Agenda.parts(day) ?? (2026, 1, 1)
        let dueOpen = Dictionary(grouping: ws.todos.filter { !$0.done && !$0.due.isEmpty }, by: \.due)
        return VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 4) {
                Text("\(Agenda.months[p.month - 1]) \(Text(String(p.year)).foregroundStyle(Brand.muted).fontWeight(.regular))")
                    .font(.system(size: 28, weight: .semibold))
                    .kerning(-0.3)
                    .foregroundStyle(Brand.ink)
                Spacer()
                stepButton("chevron.left", "Previous month") { _ = go(day, months: -1) }
                stepButton("chevron.right", "Next month") { _ = go(day, months: 1) }
                Button("Today") { ws.calendarDay = "" }
                    .buttonStyle(.plain)
                    .font(.system(size: 13))
                    .foregroundStyle(Brand.accent)
                    .padding(.horizontal, 10)
                    .frame(height: 28)
            }
            .padding(.bottom, 18)
            let columns = Array(repeating: GridItem(.flexible(minimum: 0), spacing: 2), count: 7)
            LazyVGrid(columns: columns, spacing: 2) {
                ForEach(["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"], id: \.self) {
                    Text($0)
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundStyle(Brand.muted)
                        .padding(.bottom, 4)
                        .accessibilityHidden(true)
                }
                ForEach(Agenda.month(p.year, p.month)) { d in
                    cell(d, selected: d.date == day, due: dueOpen[d.date] ?? [])
                }
            }
            // the key says in words what a colour repeats: bold for a journal, a past date for overdue
            HStack(spacing: 18) {
                Text("\(Text("Bold date").foregroundStyle(Brand.ink).fontWeight(.medium)): a journal")
                HStack(spacing: 6) {
                    dot(Brand.muted)
                    Text("todo due")
                }
                HStack(spacing: 6) {
                    dot(Brand.warn)
                    Text("overdue (a past date)")
                }
            }
            .font(.system(size: 12))
            .foregroundStyle(Brand.muted)
            .padding(.top, 18)
            .accessibilityHidden(true)
        }
    }

    /// dot is the `•` under a date with todos due, drawn large enough for its colour to show.
    private func dot(_ color: Color) -> some View {
        Circle().fill(color).frame(width: 5, height: 5)
    }

    private func stepButton(_ symbol: String, _ label: String, _ action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: symbol)
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(Brand.muted)
                .frame(width: 28, height: 28)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .help(label)
        .accessibilityLabel(label)
    }

    private func cell(_ d: CalendarDay, selected: Bool, due: [Todo]) -> some View {
        let today = d.date == ws.today
        let journal = ws.journalDays.contains(d.date)
        let late = !due.isEmpty && d.date < ws.today
        let color: Color = !d.inMonth ? Brand.faint : today ? Brand.accent : journal ? Brand.ink : Brand.muted
        let weight: Font.Weight = today ? .bold : journal ? .medium : .regular
        var words = [Agenda.long(d.date)]
        if today { words.append("today") }
        if journal { words.append("a journal") }
        if !due.isEmpty { words.append(late ? "a todo overdue" : "a todo due") }
        return Button {
            if selected {
                openJournal(d.date)
            } else {
                ws.calendarDay = d.date
            }
        } label: {
            VStack(spacing: 1) {
                Text("\(d.day)")
                    .font(.system(size: 15, weight: weight))
                    .foregroundStyle(color)
                    .frame(width: 34, height: 34)
                    .background(selected ? Brand.accentSoft : .clear, in: Circle())
                dot(late ? Brand.warn : Brand.muted)
                    .opacity(due.isEmpty ? 0 : 1)
                    .frame(height: 8)
            }
            .frame(maxWidth: .infinity)
            .frame(height: 52)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(words.joined(separator: ", "))
        .accessibilityAddTraits(selected ? .isSelected : [])
    }

    // MARK: The day

    private func dayPane(_ day: String) -> some View {
        let isToday = day == ws.today
        let reports = ws.reports(on: day)
        let todos = ws.todos.filter { $0.done ? $0.doneOn == day : $0.due == day }
        return VStack(alignment: .leading, spacing: 0) {
            Text("\(Agenda.long(day, thisYear: String(ws.today.prefix(4))))\(Text(isToday ? "  today" : "").font(.system(size: 13)).foregroundStyle(Brand.accent))")
                .font(.system(size: 19, weight: .semibold))
                .foregroundStyle(Brand.ink)
                .padding(.bottom, 22)

            label("Journal")
            journal(day, isToday: isToday)
                .padding(.bottom, 28)

            if !reports.isEmpty {
                label("Agents")
                VStack(alignment: .leading, spacing: 2) {
                    ForEach(reports) { ReportRow(report: $0) }
                }
                .padding(.bottom, 28)
            }

            label("Todos")
            if todos.isEmpty {
                nextTodo(after: day, isToday: isToday)
            } else {
                VStack(alignment: .leading, spacing: 2) {
                    ForEach(todos) { t in
                        Button {
                            ws.go(t.rel, line: t.line)
                        } label: {
                            HStack(alignment: .firstTextBaseline, spacing: 10) {
                                Text(t.done ? "✓" : "○")
                                    .foregroundStyle(t.done ? Brand.ok : Brand.muted)
                                    .frame(width: 14)
                                Text(t.text)
                                    .foregroundStyle(t.done ? Brand.muted : Brand.ink)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                            }
                            .font(.system(size: 14))
                            .padding(.vertical, 5)
                            .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                        .help("Open \(t.rel) at the todo")
                    }
                }
            }
        }
    }

    private func label(_ s: String) -> some View {
        Text(s)
            .font(.system(size: 12, weight: .semibold))
            .foregroundStyle(Brand.muted)
            .padding(.bottom, 6)
    }

    @ViewBuilder
    private func journal(_ day: String, isToday: Bool) -> some View {
        if ws.journalDays.contains(day) {
            VStack(alignment: .leading, spacing: 4) {
                Button(ws.title(ws.journalPath(day))) { openJournal(day) }
                    .buttonStyle(.plain)
                    .font(.system(size: 15))
                    .foregroundStyle(Brand.accent)
                    .padding(.bottom, 4)
                ForEach(Array(ws.log(of: day).enumerated()), id: \.offset) { _, l in
                    HStack(alignment: .firstTextBaseline, spacing: 0) {
                        Text(l.time)
                            .font(Brand.mono(12))
                            .foregroundStyle(Brand.muted)
                            .frame(width: 48, alignment: .leading)
                        Text(l.text)
                            .font(.system(size: 13))
                            .foregroundStyle(Brand.ink)
                    }
                }
            }
        } else if isToday {
            Button("Start today's journal") { Task { await ws.goToday() } }
                .buttonStyle(.plain)
                .font(.system(size: 15))
                .foregroundStyle(Brand.accent)
        } else {
            Text("No journal")
                .font(.system(size: 14))
                .foregroundStyle(Brand.muted)
        }
    }

    /// nextTodo says there is nothing due on the day, and names the next todo due after it.
    private func nextTodo(after day: String, isToday: Bool) -> some View {
        let next = ws.todos.filter { !$0.done && $0.due > day }.min { $0.due < $1.due }
        return HStack(spacing: 0) {
            Text(isToday ? "None today." : "None.")
            if let next {
                Text(" Next: ")
                Button(next.text) { ws.go(next.rel, line: next.line) }
                    .buttonStyle(.plain)
                    .foregroundStyle(Brand.accent)
                Text(", \(next.when).")
            }
        }
        .font(.system(size: 14))
        .foregroundStyle(Brand.muted)
        .lineLimit(1)
    }

    private func openJournal(_ day: String) {
        if day == ws.today {
            Task { await ws.goToday() }
        } else if ws.journalDays.contains(day) {
            ws.go(ws.journalPath(day))
        }
    }
}

/// ReportRow is an agent session: its state as glyph, the project and its summary, and on the
/// right the machine it ran on and the state as words. A click opens the report.
struct ReportRow: View {
    let report: AgentReport
    @Environment(Workspace.self) private var ws

    var body: some View {
        let st = report.state
        Button {
            ws.go(report.path)
        } label: {
            HStack(alignment: .firstTextBaseline, spacing: 10) {
                Text(st.glyph)
                    .foregroundStyle(Brand.tone(st.tone))
                    .frame(width: 14)
                Text("\(Text(report.project).fontWeight(.medium).foregroundStyle(Brand.ink))\(Text(report.summary.isEmpty ? "" : "  " + report.summary).foregroundStyle(Brand.muted))")
                    .lineLimit(1)
                    .frame(maxWidth: .infinity, alignment: .leading)
                Text(report.host.isEmpty ? st.word : "\(report.host) · \(st.word)")
                    .font(Brand.mono(11))
                    .foregroundStyle(st.tone == .ok || st.tone == .muted ? Brand.muted : Brand.tone(st.tone))
            }
            .font(.system(size: 14))
            .padding(.vertical, 5)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel("\(report.project), \(report.summary), \(st.word), on \(report.host)")
    }
}
