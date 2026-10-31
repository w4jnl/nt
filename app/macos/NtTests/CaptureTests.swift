// Step 4 without a screen: the capture shortcut and the keys macOS keeps, the menu bar's mark,
// the notes opened last, nt config as the app reads it; and, through the bundled nt on a scratch
// repository, the capture panel adding a line and a todo, and Settings changing a setting.
import AppKit
import Carbon.HIToolbox
import Foundation
import Testing
@testable import Nt

struct ShortcutTests {
    @Test func theStandardShortcutReadsAsMacOSWritesIt() {
        #expect(Shortcut.standard.display == "⌥⌘Space")
        #expect(Shortcut.standard.carbon == UInt32(cmdKey | optionKey))
        #expect(Shortcut.standard.keyEquivalent == .space)
    }

    @Test func aShortcutNeedsCommandOptionOrControl() {
        #expect(Shortcut.pressed(keyCode: UInt16(kVK_ANSI_N), flags: [.shift], characters: "n") == nil)
        let s = Shortcut.pressed(keyCode: UInt16(kVK_ANSI_N), flags: [.control, .option, .capsLock], characters: "n")
        #expect(s?.display == "⌃⌥N" && s?.keyEquivalent == "n")
        #expect(Shortcut.pressed(keyCode: UInt16(kVK_F5), flags: [.command], characters: "")?.display == "⌘F5")
    }

    @Test func theKeysMacOSKeeps() {
        // nothing changed: ⌘Space is Spotlight's, ⌥⌘Space the Finder search window's
        #expect(Shortcut.taken(.standard, hotkeys: nil) == "the Finder search window")
        let spotlight = Shortcut(keyCode: UInt32(kVK_Space), flags: NSEvent.ModifierFlags.command.rawValue, key: "Space")
        #expect(Shortcut.taken(spotlight, hotkeys: [:]) == "Spotlight")
        // turned off in System Settings
        let off: [String: Any] = ["65": ["enabled": NSNumber(value: false), "value": ["parameters": [32, 49, 1_572_864], "type": "standard"]]]
        #expect(Shortcut.taken(.standard, hotkeys: off) == nil)
        // moved to other keys
        let moved: [String: Any] = ["65": ["enabled": NSNumber(value: true), "value": ["parameters": [102, 3, 1_572_864], "type": "standard"]]]
        #expect(Shortcut.taken(.standard, hotkeys: moved) == nil)
        let other = Shortcut(keyCode: UInt32(kVK_ANSI_N), flags: NSEvent.ModifierFlags([.control, .option]).rawValue, key: "N")
        #expect(Shortcut.taken(other, hotkeys: nil) == nil)
    }

    @Test func theShortcutIsKeptOnThisMac() throws {
        let d = try #require(UserDefaults(suiteName: "nt-tests-\(UUID().uuidString)"))
        #expect(Shortcut.load(d) == .standard)
        let s = Shortcut(keyCode: UInt32(kVK_ANSI_J), flags: NSEvent.ModifierFlags([.control, .command]).rawValue, key: "J")
        s.save(d)
        #expect(Shortcut.load(d) == s)
    }
}

struct MenuBarTests {
    @Test func theMarkIsATemplateImage() throws {
        for alert in [false, true] {
            let image = MarkImage.mark(alert: alert)
            #expect(image.isTemplate && image.size == NSSize(width: 18, height: 18))
        }
        let a = try #require(MarkImage.mark(alert: false).tiffRepresentation)
        let b = try #require(MarkImage.mark(alert: true).tiffRepresentation)
        #expect(a != b, "the alert mark is the solid one")
    }

    @Test func notesOpenedLastNewestFirst() {
        var list: [String] = []
        for p in ["a.md", "b.md", "a.md", "c.md"] { list = Workspace.remember(p, in: list, max: 3) }
        #expect(list == ["c.md", "a.md", "b.md"])
        list = Workspace.remember("d.md", in: list, max: 3)
        #expect(list == ["d.md", "c.md", "a.md"])
    }

    @Test func settingsDecodeAsNtPrintsThem() throws {
        let json = #"{"settings": [{"key": "weekly", "kind": "bool", "choices": ["true", "false"], "value": "true", "help": "this machine compiles the weekly digest", "file": "machine", "path": "/c/config.toml"}, {"key": "pins", "kind": "list", "choices": null, "value": "homelab, work", "help": "folders on keys 1-9", "file": "repo", "path": "/r/.nt/config.toml"}]}"#
        let list = try JSONDecoder().decode(ConfigSetting.List.self, from: Data(json.utf8)).settings
        #expect(list.map(\.id) == ["machine:weekly", "repo:pins"])
        #expect(list[1].choices == nil && list[1].value == "homelab, work")
        #expect(ConfigSetting.effect(of: "pins") == .reindex)
        #expect(ConfigSetting.effect(of: "repo") == .restart)
        #expect(ConfigSetting.effect(of: "summary.model") == .none)
    }
}

@MainActor
@Suite(.serialized)
struct CaptureActionTests {
    let root: URL
    let config: URL
    let ws: Workspace

    init() throws {
        let base = FileManager.default.temporaryDirectory.appending(path: "nt-capture-\(UUID().uuidString)")
        root = base.appending(path: "notes")
        try FileManager.default.createDirectory(at: root.appending(path: ".git"), withIntermediateDirectories: true)
        config = base.appending(path: "config.toml")
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

    @Test func returnAddsToTodayShiftReturnMakesATodo() async throws {
        #expect(await ws.prepare())
        let c = Capture(workspace: ws)
        c.began(.today)
        #expect(c.target == "daily/\(ws.today.prefix(4))/\(ws.today).md · Log")
        c.text = "Call the ISP about the static IP"
        #expect(await c.submit(alternate: false))
        #expect(read(ws.journalPath(ws.today)).contains(" Call the ISP about the static IP"))
        #expect(c.text.isEmpty && c.added?.section == "Log" && c.added?.place == Agenda.long(ws.today, thisYear: String(ws.today.prefix(4))))

        c.began(.today)
        c.text = "Renew the domain due:fri"
        #expect(await c.submit(alternate: true))
        #expect(read("todo.md").contains("- [ ] Renew the domain due:"))
        #expect(c.added?.place == "todo.md" && c.added?.section == "Inbox" && c.added?.detail.hasPrefix("due ") == true)
        #expect(ws.todos.contains { $0.text == "Renew the domain" }, "the todo list knows it at once")

        // in the todo panel Return makes a todo and Shift-Return goes to today
        c.began(.todo)
        #expect(c.target == "todo.md · Inbox")
        c.text = "Order patch cables"
        #expect(await c.submit(alternate: false))
        #expect(read("todo.md").contains("- [ ] Order patch cables"))
        c.text = "   "
        #expect(await !c.submit(alternate: false), "nothing to add")
    }

    @Test func tabPicksTheFolderOfATodo() async throws {
        for (dir, file) in [("homelab/rack", "2026-10-01-ups.md"), ("work", "plan.md"), ("assets", "x.md")] {
            try FileManager.default.createDirectory(at: root.appending(path: dir), withIntermediateDirectories: true)
            try "# \(file)\n".write(to: root.appending(path: "\(dir)/\(file)"), atomically: true, encoding: .utf8)
        }
        #expect(await ws.prepare())
        let c = Capture(workspace: ws)
        c.began(.today)
        #expect(c.places.first == "" && c.places.contains("homelab/rack") && c.places.contains("work"))
        #expect(!c.places.contains("assets"), "no todos among the images")

        // Tab, a few letters, Return: the line is a todo in that folder's todo.md
        c.text = "Order patch cables due:fri"
        c.openPlaces()
        #expect(c.picking && c.pick == 0)
        c.query = "rack"
        #expect(c.target == "homelab/rack/todo.md · Inbox", "the footer names the folder chosen in the list")
        c.choose()
        #expect(!c.picking && c.mode == .todo && c.folder == "homelab/rack")
        #expect(c.target == "homelab/rack/todo.md · Inbox")
        #expect(await c.submit(alternate: false))
        #expect(read("homelab/rack/todo.md").contains("- [ ] Order patch cables due:"))
        #expect(c.added?.place == "homelab/rack/todo.md" && c.open(in: "homelab/rack") == 1)

        // the list opens at the folder chosen, and esc leaves it there
        c.openPlaces()
        #expect(c.places[c.pick] == "homelab/rack")
        c.movePick(-1)
        c.closePlaces()
        #expect(c.folder == "homelab/rack")
        // the next panel starts at the root's todo.md again
        c.began(.todo)
        #expect(c.folder == "" && c.target == "todo.md · Inbox")
    }

    @Test func placesMatchLikeOpen() {
        let dirs = ["homelab", "homelab/rack", "work", "work/q3"]
        #expect(Capture.places("", in: dirs) == ["", "homelab", "homelab/rack", "work", "work/q3"])
        #expect(Capture.places("q3", in: dirs).first == "work/q3")
        #expect(Capture.places("hl", in: dirs).first == "homelab")
        #expect(Capture.places("zz", in: dirs).isEmpty)
    }

    @Test func settingsChangeThroughNtConfig() async throws {
        #expect(await ws.prepare())
        await ws.loadSettings()
        let host = try #require(ws.settings.first { $0.key == "host" })
        #expect(host.value == "mbp" && host.file == "machine")
        #expect(await ws.setSetting(host, to: "studio") == nil)
        #expect(ws.settings.first { $0.key == "host" }?.value == "studio")
        #expect(try String(contentsOf: config, encoding: .utf8).contains("host = \"studio\""))
        // nt says what it will not take, and changes nothing
        let starts = try #require(ws.settings.first { $0.key == "day_starts" })
        let problem = await ws.setSetting(starts, to: "25:99")
        #expect(problem?.isEmpty == false)
        #expect(ws.settings.first { $0.key == "day_starts" }?.value == starts.value)
    }
}
