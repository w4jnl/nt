// LibGit on repositories on disk (libgit2's local transport), with nt's own sync deciding, as on
// the phone: a note changed on both sides keeps the remote version and a conflict copy, the
// journals merge line by line, a pull with nothing local fast-forwards, a push behind the remote
// is rejected, and a remote out of reach is offline.
import Foundation
import Testing
@testable import Nt

@Suite(.serialized)
struct LibGitTests {
    let base: URL
    let remote: URL
    let plain: Remote

    init() throws {
        base = FileManager.default.temporaryDirectory.appending(path: "nt-libgit-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
        remote = base.appending(path: "remote.git")
        plain = Remote(key: nil, comment: "nt@test", knownHosts: base.appending(path: "known_hosts.json"))
    }

    // MARK: Helpers

    func write(_ root: URL, _ path: String, _ text: String) throws {
        let url = root.appending(path: path)
        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data(text.utf8).write(to: url)
    }

    func read(_ root: URL, _ path: String) -> String {
        (try? String(contentsOf: root.appending(path: path), encoding: .utf8)) ?? ""
    }

    func commit(_ git: LibGit, _ msg: String) throws {
        try git.stage("")
        try git.commit(msg)
    }

    /// seed makes the remote, and the Mac's repository with a note, a journal and the journals'
    /// merge rule, pushed to it.
    func seed() throws -> LibGit {
        try LibGit.create(at: remote, bare: true)
        let dir = base.appending(path: "mac")
        try LibGit.create(at: dir, bare: false)
        let mac = LibGit(root: dir, author: "mac", remote: plain)
        try mac.setOrigin(remote.path)
        try write(dir, ".gitattributes", "daily/** merge=union\n")
        try write(dir, "tech/nas.md", "# NAS\n\nbase\n")
        try write(dir, "daily/2026/2026-10-28.md", "# Day\n\n## Log\n\n- 09:00 start\n")
        try commit(mac, "seed")
        try mac.push("main", first: true)
        return mac
    }

    func clone(_ dir: URL, author: String) throws -> LibGit {
        try LibGit.clone(remote.path, to: dir, remote: plain)
        return LibGit(root: dir, author: author, remote: plain)
    }

    // MARK: Tests

    @Test func syncKeepsTheRemoteNoteAndMergesTheJournals() async throws {
        let mac = try seed()
        #expect(mac.upstream("main") && mac.count("HEAD") == 1)
        let place = Place(base: base.appending(path: "phone"))
        try place.configure(host: "iphone")
        let phone = try clone(place.notes, author: "iphone")
        #expect(phone.branch(nil) == "main" && phone.remoteBranch() == "main" && phone.hasRemote())
        #expect(LibGit.split(phone.files(nil)).sorted() == [".gitattributes", "daily/2026/2026-10-28.md", "tech/nas.md"])

        try write(mac.root, "tech/nas.md", "# NAS\n\nfrom the mac\n")
        try write(mac.root, "daily/2026/2026-10-28.md", "# Day\n\n## Log\n\n- 09:00 start\n- 10:00 from the mac\n")
        try commit(mac, "mac")
        try mac.push("main", first: false)
        try write(place.notes, "tech/nas.md", "# NAS\n\nfrom the phone\n")
        try write(place.notes, "daily/2026/2026-10-28.md", "# Day\n\n## Log\n\n- 09:00 start\n- 10:05 from the phone\n")

        let core = try Core(place: place, git: phone)
        let out = await core.run(["sync", "--json"])
        struct Status: Decodable {
            let state: String
            let ahead: Int
            let new_conflicts: [String]?
        }
        let st = try JSONDecoder().decode(Status.self, from: out.stdout)
        #expect(st.state == "conflict" && st.ahead == 0, "\(String(decoding: out.stdout, as: UTF8.self)) \(out.stderr)")
        let copy = try #require(st.new_conflicts?.first)
        #expect(copy.hasPrefix("tech/nas.conflict-iphone-"))
        #expect(read(place.notes, "tech/nas.md") == "# NAS\n\nfrom the mac\n", "the remote version stays")
        #expect(read(place.notes, copy) == "# NAS\n\nfrom the phone\n", "the phone's is kept beside it")
        let journal = read(place.notes, "daily/2026/2026-10-28.md")
        #expect(journal.contains("10:00 from the mac") && journal.contains("10:05 from the phone") && !journal.contains("<<<<<<<"))
        #expect(LibGit.split(phone.status(nil)).isEmpty && LibGit.split(phone.unmerged(nil)).isEmpty)

        // the Mac, with nothing of its own, fast-forwards to all of it
        try mac.fetch()
        try mac.merge("main")
        #expect(read(mac.root, "daily/2026/2026-10-28.md") == journal)
        #expect(read(mac.root, copy) == "# NAS\n\nfrom the phone\n")
        #expect(mac.count("HEAD..origin/main") == 0 && mac.count("origin/main..HEAD") == 0)
        #expect(LibGit.split(mac.status(nil)).isEmpty)
    }

    @Test func aPullFastForwardsAndAPushBehindIsRejected() throws {
        let mac = try seed()
        let phone = try clone(base.appending(path: "phone"), author: "iphone")
        try write(mac.root, "tech/nas.md", "# NAS\n\nnew fan\n")
        try commit(mac, "mac")
        try mac.push("main", first: false)

        try phone.fetch()
        #expect(phone.count("HEAD..origin/main") == 1)
        try phone.merge("main")
        #expect(phone.count("HEAD..origin/main") == 0 && phone.count("HEAD") == 2, "no merge commit")
        #expect(read(phone.root, "tech/nas.md") == "# NAS\n\nnew fan\n")
        #expect(LibGit.split(phone.status(nil)).isEmpty)

        // the Mac pushes again, and the phone has not fetched since
        try write(mac.root, "work/q4.md", "# Q4\n")
        try commit(mac, "mac")
        try mac.push("main", first: false)
        try write(phone.root, "work/net.md", "# Network\n")
        try phone.stage("")
        #expect(LibGit.split(phone.staged(nil)) == ["work/net.md"])
        try phone.commit("phone")
        let err = #expect(throws: (any Error).self) { try phone.push("main", first: false) }
        #expect(err?.localizedDescription.hasPrefix("rejected: ") == true, "\(err?.localizedDescription ?? "")")
    }

    @Test func stagingLeavesTempFilesAndTheExcludedOut() throws {
        let mac = try seed()
        try write(mac.root, "work/.nt-tmp-123", "half")
        try write(mac.root, "work/big.bin", "big")
        try write(mac.root, "work/plan.md", "# Plan\n")
        try FileManager.default.removeItem(at: mac.root.appending(path: "tech/nas.md"))
        #expect(Set(LibGit.split(mac.status(nil))) == ["work/.nt-tmp-123", "work/big.bin", "work/plan.md", "tech/nas.md"])
        try mac.stage("work/big.bin")
        #expect(Set(LibGit.split(mac.staged(nil))) == ["work/plan.md", "tech/nas.md"])
        try mac.forget("work/plan.md")
        #expect(LibGit.split(mac.staged(nil)) == ["tech/nas.md"])
    }

    @Test func aRemoteOutOfReachIsOffline() throws {
        _ = try seed()
        let phone = try clone(base.appending(path: "phone"), author: "iphone")
        try phone.setOrigin("ssh://git@127.0.0.1:1/notes.git")
        let err = #expect(throws: (any Error).self) { try phone.fetch() }
        #expect(err?.localizedDescription.hasPrefix("offline: ") == true, "\(err?.localizedDescription ?? "")")
    }

    @Test func theAppWritesThroughTheCore() async throws {
        let place = Place(base: base.appending(path: "phone"))
        try place.configure(host: "iphone")
        try LibGit.create(at: place.notes, bare: false)
        try write(place.notes, "tech/nas.md", "# NAS\n\nbase\n")
        let nt = NT(core: try Core(place: place, git: LibGit(root: place.notes, author: "iphone", remote: plain)))
        struct Reply: Decodable {
            let saved: Bool
            let sha: String
        }
        // a save takes the place of the version it was read from, and one from an older version does not
        let url = place.notes.appending(path: "tech/nas.md")
        let sha = sha256(try Data(contentsOf: url))
        let r = try await nt.json(Reply.self, ["_editor", "save", url.path, "--base", sha], input: Data("# NAS\n\nfrom the phone\n".utf8))
        #expect(r.saved && read(place.notes, "tech/nas.md") == "# NAS\n\nfrom the phone\n")
        let stale = try await nt.json(Reply.self, ["_editor", "save", url.path, "--base", sha], input: Data("# NAS\n\nstale\n".utf8))
        #expect(!stale.saved && read(place.notes, "tech/nas.md") == "# NAS\n\nfrom the phone\n")
        // the hold is nt's edit lock, as on the Mac
        let hold = try #require(Hold(nt))
        hold.release()
        // a line added to today, and a command that fails, reading as nt's error
        struct Added: Decodable { let rel: String }
        let a = try await nt.json(Added.self, ["a", "--json", "--", "Called the ISP"])
        #expect(read(place.notes, a.rel).contains("Called the ISP"))
        let out = try await nt.run(["todo", "done", "nothing like this"])
        #expect(out.status != 0 && !NTError.message(out.stderr).isEmpty)
    }

    /// Over ssh with a key the phone made, when a test sshd is up (scripts/ios-ssh-test.sh):
    /// NT_SSH_URL is a bare repository behind it, and the key goes in NT_SSH_KEYS, its
    /// authorized_keys. The host is trusted the first time and refused when its key changes; a
    /// key the server does not know is refused once, as offline.
    @Test(.enabled(if: ProcessInfo.processInfo.environment["NT_SSH_URL"] != nil))
    func overSSHWithThePhonesKey() throws {
        let env = ProcessInfo.processInfo.environment
        let url = try #require(env["NT_SSH_URL"])
        let keys = URL(filePath: try #require(env["NT_SSH_KEYS"]))
        let key = DeployKey.make()
        try Data((key.publicLine("nt@test") + "\n").utf8).write(to: keys)
        let r = Remote(key: key, comment: "nt@test", knownHosts: base.appending(path: "known_hosts.json"))
        let dir = base.appending(path: "phone")
        try LibGit.clone(url, to: dir, remote: r)
        let phone = LibGit(root: dir, author: "iphone", remote: r)
        #expect(read(dir, "tech/nas.md") == "# NAS\n\nbase\n")
        try write(dir, "tech/nas.md", "# NAS\n\nfrom the phone over ssh\n")
        try commit(phone, "phone")
        try phone.push("main", first: false)
        try phone.fetch()
        #expect(phone.count("origin/main..HEAD") == 0, "pushed")
        let known = try JSONDecoder().decode([String: String].self, from: Data(contentsOf: r.knownHosts))
        #expect(known.count == 1, "the host, trusted on first use")

        let stranger = LibGit(root: dir, author: "iphone", remote: Remote(key: .make(), comment: "nt@test", knownHosts: r.knownHosts))
        let refused = #expect(throws: (any Error).self) { try stranger.fetch() }
        #expect(refused?.localizedDescription.hasPrefix("offline: the remote refused this phone's key") == true, "\(refused?.localizedDescription ?? "")")

        let host = try #require(known.keys.first)
        try JSONEncoder().encode([host: "AAAAnotTheKeyAAAA"]).write(to: r.knownHosts)
        let changed = #expect(throws: (any Error).self) { try phone.fetch() }
        #expect(changed?.localizedDescription.hasPrefix("offline: host key verification failed: the host key of \(host) changed") == true, "\(changed?.localizedDescription ?? "")")
    }
}
