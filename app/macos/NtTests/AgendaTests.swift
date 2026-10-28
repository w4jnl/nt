// The views' data: todos as nt lists them, agent reports and journals read from their files, a
// month laid out Monday first; and, through the bundled nt on a scratch repository, the todo
// actions the Todos view takes.
import Foundation
import Testing
@testable import Nt

struct AgendaTests {
    @Test func todosDecodeAsNtPrintsThem() throws {
        let json = #"{"todos": [{"path": "/r/todo.md", "rel": "todo.md", "line": 4, "done": false, "text": "Replace the UPS battery", "due": "2026-10-25", "done_on": "", "section": "Inbox", "when": "3d late", "late": true}, {"path": "/r/homelab/n.md", "rel": "homelab/n.md", "line": 9, "done": true, "text": "Update the firmware", "done_on": "2026-10-27", "when": "yesterday", "late": false}]}"#
        let list = try JSONDecoder().decode(Todo.List.self, from: Data(json.utf8))
        #expect(list.todos.count == 2)
        #expect(list.todos[0].id == "todo.md:4" && list.todos[0].late && list.todos[0].when == "3d late")
        #expect(list.todos[1].done && list.todos[1].doneOn == "2026-10-27" && list.todos[1].due == "")
        // an older nt without when and late still decodes
        let old = try JSONDecoder().decode(Todo.List.self, from: Data(#"{"todos": [{"rel": "todo.md", "line": 1, "done": false, "text": "x"}]}"#.utf8))
        #expect(old.todos[0].when == "" && !old.todos[0].late)
    }

    @Test func reportsReadTheirFrontmatter() {
        let text = """
        ---
        type: agent-report
        schema: 1
        created: 2026-10-27T09:12:00+01:00
        ended: 2026-10-27T10:40:05+01:00
        agent: claude-code
        host: work
        project: infra
        summary: "Terraform: the new cron host"
        status: done
        ---

        # infra: Terraform: the new cron host
        """
        let r = AgentReport.parse(text, path: "agents/2026/2026-10-27/work--claude-code--infra--1a2b3c4d.md")
        #expect(r?.project == "infra" && r?.host == "work" && r?.summary == "Terraform: the new cron host")
        #expect(r?.state.glyph == "✓" && r?.state.word == "done 10:40")
        #expect(AgentReport.parse("---\ntitle: NAS\n---\n# NAS\n", path: "x.md") == nil)
        var b = AgentReport(path: "x")
        b.status = "blocked"
        #expect(b.state.glyph == "●" && b.state.tone == .warn)
        b.status = "in-progress"
        #expect(b.state.word == "in progress" && b.state.tone == .accent)
    }

    @Test func aJournalsLogReadsAsText() {
        let journal = """
        # Tuesday 27 October 2026

        ## Log

        - 08:58 Stand-up: **billing** migration on track
        - 10:14 Fan in bay 2 is loud again, see [NAS setup](../../homelab/2026-10-21-nas-setup.md)
        - a line without a time

        ## Notes

        - [Q4 planning](../../work/q4.md)
        """
        #expect(Agenda.log(journal) == [
            LogLine(time: "08:58", text: "Stand-up: billing migration on track"),
            LogLine(time: "10:14", text: "Fan in bay 2 is loud again, see NAS setup"),
            LogLine(time: "", text: "a line without a time"),
        ])
    }

    @Test func aMonthStartsOnMonday() {
        let oct = Agenda.month(2026, 10) // 1 October 2026 is a Thursday
        #expect(oct.count == 35)
        #expect(oct.first?.date == "2026-09-28" && oct.first?.inMonth == false)
        #expect(oct[3].date == "2026-10-01" && oct[3].inMonth)
        #expect(oct.last?.date == "2026-11-01")
        let feb = Agenda.month(2027, 2) // 1 February 2027 is a Monday
        #expect(feb.first?.date == "2027-02-01" && feb.count == 28)
    }

    @Test func datesMove() {
        #expect(Agenda.shift("2026-10-28", days: 7) == "2026-11-04")
        #expect(Agenda.shift("2026-01-31", months: 1) == "2026-02-28")
        #expect(Agenda.shift("2026-12-15", months: 1) == "2027-01-15")
        #expect(Agenda.long("2026-10-27") == "Tuesday 27 October")
        #expect(Agenda.long("2025-12-31", thisYear: "2026") == "Wednesday 31 December 2025")
        #expect(Agenda.clock("2026-10-27T10:40:05+01:00") == "10:40")
    }

    @Test func weeklyAndAgentFolders() {
        let paths = ["weekly/2026/2026-W43.md", "weekly/2026/2026-W44.md", "weekly/2026/2026-W44-mbp.md", "work/2026-W99.md"]
        #expect(Agenda.latestWeekly(paths, folder: "weekly") == "weekly/2026/2026-W44.md")
        #expect(Agenda.latestWeekly(["notes.md"], folder: "weekly") == nil)
        let dirs = ["agents", "agents/2026", "agents/2026/2026-10-27", "agents/2026/2026-10-28", "daily/2026"]
        #expect(Agenda.agentDays(dirs, folder: "agents") == ["2026-10-28", "2026-10-27"])
    }
}

@MainActor
@Suite(.serialized)
struct TodoActionTests {
    let root: URL
    let ws: Workspace

    init() throws {
        let base = FileManager.default.temporaryDirectory.appending(path: "nt-todos-\(UUID().uuidString)")
        root = base.appending(path: "notes")
        let fm = FileManager.default
        for dir in [".git", "homelab", "agents/2026/2026-10-27"] {
            try fm.createDirectory(at: root.appending(path: dir), withIntermediateDirectories: true)
        }
        try "# NAS\n\n- [ ] Replace the fan due:2026-10-25\n".write(to: root.appending(path: "homelab/2026-10-21-nas.md"), atomically: true, encoding: .utf8)
        try "---\ntype: agent-report\nproject: infra\nhost: work\nstatus: blocked\ncreated: 2026-10-27T09:00:00+01:00\n---\n# infra\n"
            .write(to: root.appending(path: "agents/2026/2026-10-27/work--claude-code--infra--1a2b3c4d.md"), atomically: true, encoding: .utf8)
        try "---\ntype: agent-report\nproject: web\nhost: mbp\nstatus: done\ncreated: 2026-10-27T10:30:00+01:00\n---\n# web\n"
            .write(to: root.appending(path: "agents/2026/2026-10-27/mbp--claude-code--web--5e6f7a8b.md"), atomically: true, encoding: .utf8)
        let config = base.appending(path: "config.toml")
        try "repo = \"\(root.path)\"\nhost = \"mbp\"\n[sync]\nauto = false\n".write(to: config, atomically: true, encoding: .utf8)
        let bundled = try #require(NT.locate(), "the app bundles nt")
        var env = bundled.environment
        env["NT_CONFIG"] = config.path
        env["NT_STATE"] = base.appending(path: "state").path
        env["NT_REPO"] = root.path
        ws = Workspace(nt: NT(executable: bundled.executable, environment: env))
    }

    func read(_ rel: String) -> String {
        (try? String(contentsOf: root.appending(path: rel), encoding: .utf8)) ?? ""
    }

    @Test func theTodosViewWritesThroughNt() async throws {
        #expect(await ws.prepare())
        let fan = try #require(ws.todos.first { $0.text == "Replace the fan" })
        #expect(fan.late && fan.when.hasSuffix("late"))

        await ws.toggle(fan)
        #expect(read("homelab/2026-10-21-nas.md").contains("- [x] Replace the fan due:2026-10-25 done:"))
        let done = try #require(ws.todos.first { $0.text == "Replace the fan" })
        #expect(done.done && done.when == "today")
        await ws.toggle(done)
        #expect(read("homelab/2026-10-21-nas.md").contains("- [ ] Replace the fan due:2026-10-25\n"))

        #expect(await ws.addTodo("Renew the domain due:2026-11-06"))
        #expect(read("todo.md").contains("- [ ] Renew the domain due:2026-11-06"))
        let domain = try #require(ws.todos.first { $0.text == "Renew the domain" })
        await ws.setDue(domain, "none")
        #expect(read("todo.md").contains("- [ ] Renew the domain\n"))
        #expect(ws.flash.isEmpty)

        // a todo nt cannot find any more says so, and changes nothing
        try "# NAS\n".write(to: root.appending(path: "homelab/2026-10-21-nas.md"), atomically: true, encoding: .utf8)
        await ws.toggle(fan)
        #expect(!ws.flash.isEmpty)
    }

    @Test func reportsAndJournalsAreReadFromTheirFiles() async throws {
        #expect(await ws.prepare())
        let reports = ws.reports(on: "2026-10-27")
        #expect(reports.map(\.project) == ["web", "infra"], "the latest session first")
        #expect(reports.last?.host == "work")
        #expect(ws.agentDays == ["2026-10-27"])
        #expect(ws.reports(on: "2026-10-26").isEmpty)
        #expect(ws.journalPath("2026-10-27") == "daily/2026/2026-10-27.md")
        #expect(ws.weeklyPath == nil)
    }
}
