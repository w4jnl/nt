import SwiftUI

/// AgentsView is the agent sessions of the last days, newest day first, from every machine:
/// each as its state glyph, project and summary, the machine and the state in words. A click
/// opens the report; the day's digest is a click on its heading.
struct AgentsView: View {
    @Environment(Workspace.self) private var ws
    static let days = 14

    var body: some View {
        let days = Array(ws.agentDays.prefix(Self.days))
        let today = ws.reports(on: ws.today)
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                Text("Agents")
                    .font(.system(size: 28, weight: .semibold))
                    .kerning(-0.3)
                    .foregroundStyle(Brand.ink)
                    .padding(.bottom, 4)
                Text(summary(today))
                    .font(.system(size: 13))
                    .foregroundStyle(Brand.muted)
                    .padding(.bottom, 26)
                if days.isEmpty {
                    Text("No agent sessions yet. nt install --hooks reports Claude Code's and Copilot CLI's.")
                        .font(.system(size: 14))
                        .foregroundStyle(Brand.muted)
                }
                ForEach(days, id: \.self) { day in
                    let reports = ws.reports(on: day)
                    if !reports.isEmpty {
                        heading(day)
                        VStack(alignment: .leading, spacing: 2) {
                            ForEach(reports) { ReportRow(report: $0) }
                        }
                        .padding(.bottom, 26)
                    }
                }
            }
            .frame(maxWidth: 640, alignment: .leading)
            .padding(EdgeInsets(top: 22, leading: 40, bottom: 64, trailing: 40))
            .frame(maxWidth: .infinity)
        }
        .background(Brand.ground)
        .viewToolbar("Agents")
    }

    private func summary(_ today: [AgentReport]) -> String {
        let running = today.filter { $0.status == "in-progress" }.count
        let n = today.isEmpty ? "No sessions today" : today.count == 1 ? "1 session today" : "\(today.count) sessions today"
        return running > 0 ? "\(n) · \(running) in progress · from every machine" : "\(n) · from every machine"
    }

    /// heading is a day's name; it opens the day's digest when there is one.
    @ViewBuilder
    private func heading(_ day: String) -> some View {
        let word = day == ws.today ? "Today" : day == Agenda.shift(ws.today, days: -1) ? "Yesterday" : Agenda.long(day, thisYear: String(ws.today.prefix(4)))
        let digest = "\(ws.index?.folders.agents ?? "agents")/\(day.prefix(4))/\(day)/README.md"
        let has = ws.index?.notes.contains { $0.path == digest } ?? false
        HStack(spacing: 8) {
            Text(word)
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(Brand.muted)
            if has {
                Button("digest") { ws.show(digest) }
                    .buttonStyle(.plain)
                    .font(Brand.mono(11))
                    .foregroundStyle(Brand.accent)
                    .help("Open the day's digest")
            }
        }
        .padding(.bottom, 6)
    }
}

/// WeeklyEmptyView says there is no weekly digest yet, and how one is made.
struct WeeklyEmptyView: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Weekly")
                .font(.system(size: 28, weight: .semibold))
                .kerning(-0.3)
                .foregroundStyle(Brand.ink)
            Text("No weekly digest yet. The machine with weekly = true writes one when it syncs: nt config set weekly true on one machine.")
                .font(.system(size: 14))
                .foregroundStyle(Brand.muted)
                .textSelection(.enabled)
        }
        .frame(maxWidth: 640, alignment: .leading)
        .padding(EdgeInsets(top: 22, leading: 40, bottom: 64, trailing: 40))
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .background(Brand.ground)
        .viewToolbar("Weekly")
    }
}
