// The app's logic without a window: the index nt prints, the sidebar's tree, the sync badge,
// Open's matching, links, and the design system's colours.
import Foundation
import Testing

@testable import Nt

/// repo is this checkout, found from this file: app/macos/NtTests/ModelTests.swift.
private let repo = URL(filePath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()

private let indexJSON = """
    {
      "root": "/Users/me/notes",
      "state": "/Users/me/.local/state/nt",
      "day": "2026-10-27",
      "pins": ["homelab", "work"],
      "folders": {"daily": "daily", "agents": "agents", "weekly": "weekly"},
      "dirs": ["agents", "daily", "daily/2025", "daily/2026", "homelab", "homelab/assets", "reading", "work", "work/old"],
      "notes": [
        {"path": "README.md", "title": "Notes"},
        {"path": "daily/2025/2025-12-31.md", "title": "Wednesday 31 December 2025"},
        {"path": "daily/2026/2026-10-27.md", "title": "Tuesday 27 October 2026"},
        {"path": "homelab/2026-10-21-nas-setup.md", "title": "NAS setup"},
        {"path": "homelab/2026-10-24-home-network.md", "title": "Home network"},
        {"path": "work/2025-03-02-q1.md", "title": "Q1 planning"}
      ],
      "back": {"homelab/2026-10-21-nas-setup.md": ["daily/2026/2026-10-27.md", "homelab/2026-10-24-home-network.md"]}
    }
    """

private func index() throws -> RepoIndex {
    try JSONDecoder().decode(RepoIndex.self, from: Data(indexJSON.utf8))
}

@Suite struct IndexTests {
    @Test func decodesWhatNtPrints() throws {
        let ix = try index()
        #expect(ix.title("homelab/2026-10-21-nas-setup.md") == "NAS setup")
        #expect(ix.title("gone/2026-01-01-x.md") == "2026-01-01-x.md")
        #expect(ix.back["homelab/2026-10-21-nas-setup.md"]?.count == 2)
    }

    @Test func agentReportsShowTheirHost() {
        #expect(SidebarTree.reportHost("agents/2026/2026-10-28/work--copilot-cli--vuln-remediation--43fbe292.md", agents: "agents") == "work")
        #expect(SidebarTree.reportHost("agents/2026/2026-10-27/jaromac--claude-code--flok--f0ceca08.md", agents: "agents") == "jaromac")
        #expect(SidebarTree.reportHost("agents/2026/2026-10-28/README.md", agents: "agents") == nil)
        #expect(SidebarTree.reportHost("work/jaromac--claude-code--flok--f0ceca08.md", agents: "agents") == nil)
    }

    @Test func treePinsFirstNewestFirstWithoutAssets() throws {
        let tree = SidebarTree.build(try index(), thisYear: "2026")
        #expect(tree.pinned.map(\.id) == ["homelab", "work"])
        #expect(tree.rest.map(\.id) == ["agents", "daily", "reading"])
        #expect(tree.rootNotes.map(\.name) == ["Notes"])
        let homelab = tree.pinned[0].children ?? []
        #expect(homelab.map(\.id) == ["homelab/2026-10-24-home-network.md", "homelab/2026-10-21-nas-setup.md"])
        #expect(homelab.map(\.date) == ["24 Oct", "21 Oct"])
        let work = tree.pinned[1].children ?? []
        #expect(work.map(\.id) == ["work/old", "work/2025-03-02-q1.md"], "folders before notes")
        #expect(work.last?.date == "Mar 2025")
        let daily = tree.rest[1].children ?? []
        #expect(daily.map(\.id) == ["daily/2026", "daily/2025"], "years newest first")
        #expect(daily[0].children?.first?.date == "", "journals show no date: the title is one")
    }

    @Test func dates() {
        #expect(SidebarTree.dated("2026-10-21-nas.md"))
        #expect(SidebarTree.dated("2026"))
        #expect(!SidebarTree.dated("notes.md"))
        #expect(!SidebarTree.dated("2026-1-21.md"))
        #expect(SidebarTree.shortDate("x/2026-01-05-a.md", thisYear: "2026") == "5 Jan")
        #expect(SidebarTree.shortDate("x/readme.md", thisYear: "2026") == "")
        #expect(SidebarTree.ancestors("a/b/c.md") == ["a", "a/b"])
        #expect(SidebarTree.ancestors("c.md") == [])
    }
}

@Suite struct SyncTests {
    @Test func statusJSONDecodes() throws {
        let a = SyncStatus.decode(Data(#"{"repo":"/r","state":"unpushed","ahead":2,"committed":0,"merged":false,"at":"2026-10-27T09:12:00.123456+01:00"}"#.utf8))
        #expect(a?.state == "unpushed" && a?.ahead == 2)
        let b = SyncStatus.decode(Data(#"{"state":"synced","at":"2026-10-27T09:12:00Z"}"#.utf8))
        #expect(b?.state == "synced")
        #expect(SyncStatus.decode(Data("{}".utf8)) == nil)
    }

    @Test func badgesAreGlyphAndWord() {
        let now = Date(timeIntervalSince1970: 1_800_000_000)
        func st(_ s: String, ahead: Int = 0, conflicts: [String]? = nil, age: TimeInterval = 60) -> SyncStatus {
            SyncStatus(state: s, ahead: ahead, conflicts: conflicts, message: nil, at: now.addingTimeInterval(-age))
        }
        #expect(Badge.of(nil, syncing: false, now: now).word == "not synced yet")
        #expect(Badge.of(st("synced"), syncing: true, now: now).word == "syncing")
        #expect(Badge.of(st("synced"), syncing: false, now: now) == Badge(glyph: "●", word: "synced", tone: .ok))
        #expect(Badge.of(st("unpushed", ahead: 3), syncing: false, now: now).glyph == "↑3")
        #expect(Badge.of(st("offline"), syncing: false, now: now).tone == .warn)
        #expect(Badge.of(st("conflict", conflicts: ["a", "b"]), syncing: false, now: now).word == "2 conflicts")
        #expect(Badge.of(st("error"), syncing: false, now: now).word == "sync error")
        #expect(Badge.of(st("synced", age: 3 * 3600), syncing: false, now: now).detail == "3h ago")
    }
}

@Suite struct FuzzyTests {
    @Test func matchesInOrderAndScoresWordStarts() {
        #expect(Fuzzy.match("net", "Home network")?.at == [5, 6, 7])
        #expect(Fuzzy.match("xyz", "Home network") == nil)
        let start = Fuzzy.match("nas", "NAS setup")!.score
        let middle = Fuzzy.match("nas", "Financial reasons")!.score
        #expect(start > middle)
    }

    @Test func findRanksTitlesAndKeepsNewerOnTies() throws {
        let ix = try index()
        let hits = Fuzzy.find("net", in: ix.notes, limit: 5)
        #expect(hits.first?.note.title == "Home network")
        #expect(Fuzzy.find("q1", in: ix.notes, limit: 5).map(\.note.path) == ["work/2025-03-02-q1.md"])
    }
}

@Suite struct LinkTests {
    @Test func resolvesAgainstTheNote() {
        #expect(Links.resolve(from: "daily/2026/2026-10-27.md", "../../homelab/a%20b.md#setup") == "homelab/a b.md")
        #expect(Links.resolve(from: "n.md", "<my note.md>") == "my note.md")
        #expect(Links.resolve(from: "n.md", "../outside.md") == nil)
        #expect(Links.resolve(from: "n.md", "https://example.com") == nil)
        #expect(Links.resolve(from: "n.md", "/etc/hosts") == nil)
    }

    @Test func targets() {
        let exists: (String) -> Bool = { ["homelab/nas.md", "homelab/assets/a.pdf"].contains($0) }
        #expect(Links.target(from: "homelab/x.md", "nas.md#mounting", exists: exists) == .note("homelab/nas.md", anchor: "mounting"))
        #expect(Links.target(from: "homelab/x.md", "assets/a.pdf", exists: exists) == .file("homelab/assets/a.pdf"))
        #expect(Links.target(from: "homelab/x.md", "gone.md", exists: exists) == .missing("homelab/gone.md"))
        #expect(Links.target(from: "homelab/x.md", "#setup", exists: exists) == .anchor("setup"))
        #expect(Links.target(from: "homelab/x.md", "https://w4j.nl", exists: exists) == .web(URL(string: "https://w4j.nl")!))
        #expect(Links.target(from: "homelab/x.md", "javascript:alert(1)", exists: exists) == .invalid)
    }
}

@Suite struct PlumbingTests {
    @Test func ntErrorsLoseTheirPrefix() {
        #expect(NTError.message("nt: no notes repo at ~/notes: run nt init ~/notes\n") == "no notes repo at ~/notes: run nt init ~/notes")
        #expect(NTError.message("") == "nt failed without saying why")
    }

    @Test func pathHasGitAndRg() {
        let env = NT.environment(["PATH": "/usr/bin:/bin"], home: URL(filePath: "/Users/me"))
        let path = env["PATH"]!.split(separator: ":").map(String.init)
        #expect(path.prefix(2) == ["/usr/bin", "/bin"])
        #expect(path.contains("/opt/homebrew/bin") && path.contains("/Users/me/.local/bin"))
        #expect(env["NO_COLOR"] == "1")
    }

    @Test func assetsStayInTheRepository() {
        let root = URL(filePath: NSTemporaryDirectory()).appending(path: "nt-assets-test")
        #expect(AssetHandler.file(for: URL(string: "nt-asset://repo/homelab/assets/a%20b.png")!, root: root)?.lastPathComponent == "a b.png")
        #expect(AssetHandler.file(for: URL(string: "nt-asset://repo/../../etc/hosts")!, root: root) == nil)
        #expect(AssetHandler.file(for: URL(string: "nt-asset://repo/homelab/%2E%2E/%2E%2E/etc/hosts")!, root: root) == nil)
    }

    @Test func watcherIgnoresGitAndTempFiles() {
        #expect(RepoWatcher.relevant("/n/homelab/a.md", root: "/n"))
        #expect(!RepoWatcher.relevant("/n/.git/index", root: "/n"))
        #expect(!RepoWatcher.relevant("/n/homelab/.nt-tmp-123", root: "/n"))
        #expect(!RepoWatcher.relevant("/elsewhere/a.md", root: "/n"))
    }
}

@Suite struct DesignSystemTests {
    struct Tokens: Decodable {
        struct Color: Decodable {
            struct Token: Decodable {
                let name: String
                let value: [String: String]
            }
            let tokens: [Token]
        }
        let color: Color
    }

    private func hex(_ v: UInt32) -> String { String(format: "#%06X", v) }

    @Test func brandMatchesTokensJSON() throws {
        let data = try Data(contentsOf: repo.appending(path: "docs/design-system/tokens.json"))
        let tokens = try JSONDecoder().decode(Tokens.self, from: data).color.tokens
        for (name, value) in Brand.tokens {
            let t = try #require(tokens.first { $0.name == name }, "\(name) is not in tokens.json")
            #expect(hex(value.light) == t.value["light"]?.uppercased(), "\(name) light")
            #expect(hex(value.dark) == t.value["dark"]?.uppercased(), "\(name) dark")
        }
    }

    @Test func accentColorAssetIsTheAccentToken() throws {
        let url = repo.appending(path: "app/macos/Nt/Assets.xcassets/AccentColor.colorset/Contents.json")
        let json = try String(contentsOf: url, encoding: .utf8).replacingOccurrences(of: " ", with: "")
        let accent = try #require(Brand.tokens["accent"])
        for v in [accent.light, accent.dark] {
            let r = String(format: "\"red\":\"0x%02X\"", v >> 16 & 0xFF)
            let g = String(format: "\"green\":\"0x%02X\"", v >> 8 & 0xFF)
            let b = String(format: "\"blue\":\"0x%02X\"", v & 0xFF)
            #expect(json.contains(r) && json.contains(g) && json.contains(b), "accent \(hex(v))")
        }
    }
}
