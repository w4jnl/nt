// Agenda is what the Todos, Calendar, Weekly and Agents views show: the todos as nt lists them
// (`nt todo --all --json`), agent reports and journals read from their files, and a month laid out
// Monday first. Pure functions, so the tests run them without a repository.
import Foundation

/// Todo is one task line, as `nt todo --json` prints it; when is its date in words, as the
/// terminal browser shows it, and late says it is overdue.
struct Todo: Decodable, Identifiable, Equatable, Sendable {
    let rel: String
    let line: Int
    let done: Bool
    let text: String
    var due: String
    var doneOn: String
    var when: String
    var late: Bool

    var id: String { "\(rel):\(line)" }

    enum CodingKeys: String, CodingKey {
        case rel, line, done, text, due, when, late
        case doneOn = "done_on"
    }

    init(from d: any Decoder) throws {
        let c = try d.container(keyedBy: CodingKeys.self)
        rel = try c.decode(String.self, forKey: .rel)
        line = try c.decode(Int.self, forKey: .line)
        done = try c.decode(Bool.self, forKey: .done)
        text = try c.decode(String.self, forKey: .text)
        due = try c.decodeIfPresent(String.self, forKey: .due) ?? ""
        doneOn = try c.decodeIfPresent(String.self, forKey: .doneOn) ?? ""
        when = try c.decodeIfPresent(String.self, forKey: .when) ?? ""
        late = try c.decodeIfPresent(Bool.self, forKey: .late) ?? false
    }

    struct List: Decodable {
        let todos: [Todo]
    }

    /// folder is the top-level folder the todo is in, "" for the root's todo.md.
    var folder: String { rel.firstIndex(of: "/").map { String(rel[..<$0]) } ?? "" }

    /// scopes are what the Todos view can keep to besides every note: the top-level folders with
    /// open todos, with how many, A to Z, as `nt todo <folder>` reads them; keep stays, even with
    /// none left open.
    static func scopes(_ todos: [Todo], keep: String = "") -> [(folder: String, open: Int)] {
        var per: [String: Int] = [:]
        for t in todos where !t.done && !t.folder.isEmpty {
            per[t.folder, default: 0] += 1
        }
        if !keep.isEmpty, per[keep] == nil { per[keep] = 0 }
        return per.keys.sorted { $0.lowercased() < $1.lowercased() }.map { ($0, per[$0] ?? 0) }
    }
}

/// AgentReport is an agent session's report, from its frontmatter (docs/format.md §6).
struct AgentReport: Identifiable, Equatable, Sendable {
    let path: String
    var project = ""
    var summary = ""
    var status = ""
    var host = ""
    var agent = ""
    var created = ""
    var ended = ""
    var id: String { path }

    /// parse reads a report's frontmatter; nil when the file is not an agent report.
    nonisolated static func parse(_ text: String, path: String) -> AgentReport? {
        let fm = Agenda.frontmatter(text)
        guard fm["type"] == "agent-report" else { return nil }
        var r = AgentReport(path: path)
        r.project = fm["project"] ?? ""
        r.summary = fm["summary"] ?? ""
        r.status = fm["status"] ?? ""
        r.host = fm["host"] ?? ""
        r.agent = fm["agent"] ?? ""
        r.created = fm["created"] ?? ""
        r.ended = fm["ended"] ?? ""
        return r
    }

    /// state is the report's status as the design system draws it: glyph, word and tone.
    var state: (glyph: String, word: String, tone: Badge.Tone) {
        switch status {
        case "done": ("✓", ended.isEmpty ? "done" : "done " + Agenda.clock(ended), .ok)
        case "blocked": ("●", "blocked", .warn)
        case "failed": ("◆", "failed", .alert)
        case "abandoned": ("○", "abandoned", .muted)
        default: ("◐", "in progress", .accent)
        }
    }
}

/// LogLine is one line of a journal's ## Log: its time, when it has one, and its text.
struct LogLine: Equatable, Sendable {
    let time: String
    let text: String
}

/// CalendarDay is one cell of a month: its date, and whether it belongs to the month shown.
struct CalendarDay: Identifiable, Equatable, Sendable {
    let date: String // YYYY-MM-DD
    let day: Int
    let inMonth: Bool
    var id: String { date }
}

enum Agenda {
    nonisolated static let months = ["January", "February", "March", "April", "May", "June", "July",
                                     "August", "September", "October", "November", "December"]
    nonisolated static let weekdays = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"]

    /// frontmatter is the flat YAML at the top of a note, key by key, quotes taken off.
    nonisolated static func frontmatter(_ text: String) -> [String: String] {
        var out: [String: String] = [:]
        let lines = text.split(separator: "\n", omittingEmptySubsequences: false)
        guard lines.first?.trimmingCharacters(in: .whitespaces) == "---" else { return out }
        for line in lines.dropFirst() {
            let l = line.trimmingCharacters(in: .whitespaces)
            if l == "---" { break }
            guard let colon = l.firstIndex(of: ":") else { continue }
            let key = String(l[..<colon])
            var value = l[l.index(after: colon)...].trimmingCharacters(in: .whitespaces)
            if value.count >= 2, let f = value.first, let b = value.last, f == b, f == "\"" || f == "'" {
                value = String(value.dropFirst().dropLast())
            }
            out[key] = value
        }
        return out
    }

    /// log is the lines of a journal's ## Log, markdown links read as their text.
    nonisolated static func log(_ text: String) -> [LogLine] {
        var out: [LogLine] = []
        var inLog = false
        for line in text.split(separator: "\n", omittingEmptySubsequences: false) {
            if line.hasPrefix("## ") {
                inLog = line.dropFirst(3).trimmingCharacters(in: .whitespaces) == "Log"
                continue
            }
            guard inLog, line.hasPrefix("- ") else { continue }
            let item = String(line.dropFirst(2))
            if let m = item.wholeMatch(of: /(\d{1,2}:\d{2})\s+(.*)/) {
                out.append(LogLine(time: String(m.1), text: plain(String(m.2))))
            } else {
                out.append(LogLine(time: "", text: plain(item)))
            }
        }
        return out
    }

    /// plain is a line of markdown as it reads: links as their text, emphasis marks dropped.
    nonisolated static func plain(_ md: String) -> String {
        var s = md.replacing(/!?\[([^\]]*)\]\([^)]*\)/) { String($0.1) }
        s = s.replacing(/\*\*|__|`/, with: "")
        return s
    }

    /// instant is an RFC 3339 time as nt writes them (created, ended); nil when it is none.
    nonisolated static func instant(_ stamp: String) -> Date? {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f.date(from: stamp.replacing(/\.\d+/, with: "", maxReplacements: 1))
    }

    /// clock is the HH:MM of an RFC 3339 time.
    nonisolated static func clock(_ stamp: String) -> String {
        guard let t = stamp.firstMatch(of: /T(\d{2}:\d{2})/) else { return "" }
        return String(t.1)
    }

    /// month is the days to show for a month, Monday first, in whole weeks.
    nonisolated static func month(_ year: Int, _ month: Int) -> [CalendarDay] {
        var cal = Calendar(identifier: .gregorian)
        cal.firstWeekday = 2
        cal.timeZone = TimeZone(identifier: "UTC")!
        guard let first = cal.date(from: DateComponents(year: year, month: month, day: 1)),
              let count = cal.range(of: .day, in: .month, for: first)?.count else { return [] }
        let lead = (cal.component(.weekday, from: first) + 5) % 7 // days before it, from Monday
        let total = Int((Double(lead + count) / 7).rounded(.up)) * 7
        return (0..<total).compactMap { i in
            guard let d = cal.date(byAdding: .day, value: i - lead, to: first) else { return nil }
            let c = cal.dateComponents([.year, .month, .day], from: d)
            return CalendarDay(date: String(format: "%04d-%02d-%02d", c.year!, c.month!, c.day!), day: c.day!, inMonth: c.month == month)
        }
    }

    /// shift moves a YYYY-MM-DD date by days, or by months to the same day or the month's last.
    nonisolated static func shift(_ date: String, days: Int = 0, months: Int = 0) -> String {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        guard let p = parts(date), let d = cal.date(from: DateComponents(year: p.year, month: p.month, day: p.day)),
              let moved = cal.date(byAdding: DateComponents(month: months, day: days), to: d) else { return date }
        let c = cal.dateComponents([.year, .month, .day], from: moved)
        return String(format: "%04d-%02d-%02d", c.year!, c.month!, c.day!)
    }

    nonisolated static func parts(_ date: String) -> (year: Int, month: Int, day: Int)? {
        let p = date.split(separator: "-").compactMap { Int($0) }
        guard p.count == 3, (1...12).contains(p[1]) else { return nil }
        return (p[0], p[1], p[2])
    }

    /// long is a date as a heading says it: "Tuesday 27 October", with the year when it is not
    /// thisYear's.
    nonisolated static func long(_ date: String, thisYear: String = "") -> String {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        guard let p = parts(date), let d = cal.date(from: DateComponents(year: p.year, month: p.month, day: p.day)) else { return date }
        let s = "\(weekdays[cal.component(.weekday, from: d) - 1]) \(p.day) \(months[p.month - 1])"
        return String(p.year) == thisYear || thisYear.isEmpty ? s : "\(s) \(p.year)"
    }

    /// latestWeekly is the newest weekly digest among paths: <weekly>/GGGG/GGGG-Www.md, not a
    /// machine's summary (-<host>).
    nonisolated static func latestWeekly(_ paths: [String], folder: String) -> String? {
        paths.filter { p in
            p.hasPrefix(folder + "/") && (p as NSString).lastPathComponent.wholeMatch(of: /\d{4}-W\d{2}\.md/) != nil
        }.max()
    }

    /// agentDays are the days that have an agents folder, newest first: agents/YYYY/YYYY-MM-DD.
    nonisolated static func agentDays(_ dirs: [String], folder: String) -> [String] {
        dirs.compactMap { d -> String? in
            guard d.hasPrefix(folder + "/") else { return nil }
            let name = (d as NSString).lastPathComponent
            return name.wholeMatch(of: /\d{4}-\d{2}-\d{2}/) != nil ? name : nil
        }.sorted(by: >)
    }
}
