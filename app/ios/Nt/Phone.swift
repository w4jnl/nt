// Phone is the app's state for step 1 of docs/design.md §9: the first run (the repository's
// address, this device's name, the phone's key, the clone), then the notes and their sync.
import Foundation
import Observation

@MainActor @Observable
final class Phone {
    /// remote is the notes repository's ssh address, git@github.com:<you>/notes.git.
    var remote: String { didSet { UserDefaults.standard.set(remote, forKey: "remote") } }
    /// host is this device's name: the host in commits, conflict copies and reports.
    var host: String { didSet { UserDefaults.standard.set(host, forKey: "host") } }
    private(set) var key: DeployKey?
    private(set) var cloned: Bool
    private(set) var cloning = false
    private(set) var syncing = false
    private(set) var notes: [Note] = []
    private(set) var status: SyncStatus?
    /// problem is the last thing that went wrong, as the person reads it.
    var problem: String?

    let place: Place
    private var core: Core?

    struct Note: Decodable, Identifiable, Hashable, Sendable {
        let path: String
        let title: String
        var id: String { path }
    }

    init(place: Place = .app) {
        self.place = place
        remote = UserDefaults.standard.string(forKey: "remote") ?? ""
        host = UserDefaults.standard.string(forKey: "host") ?? "iphone"
        key = DeployKey.load()
        cloned = place.cloned
        status = SyncStatus.read(stateDir: place.state.path)
    }

    /// hostOK is a name nt can put in file names: lowercase letters, digits and hyphens.
    var hostOK: Bool { !host.isEmpty && host.allSatisfy { $0.isASCII && ($0.isLowercase || $0.isNumber || $0 == "-") } }

    var publicKey: String { key?.publicLine("nt@\(host)") ?? "" }

    func makeKey() {
        let k = DeployKey.make()
        do {
            try k.save()
            key = k
        } catch {
            problem = error.localizedDescription
        }
    }

    private var gitRemote: Remote { Remote(key: key, comment: "nt@\(host)", knownHosts: place.knownHosts) }

    /// clone clones the notes into the app's folder and opens them.
    func clone() async {
        let url = remote.trimmingCharacters(in: .whitespaces)
        let (dest, r, place, host) = (self.place.notes, gitRemote, self.place, self.host)
        cloning = true
        problem = nil
        defer { cloning = false }
        do {
            try await background {
                try place.configure(host: host)
                try LibGit.clone(url, to: dest, remote: r)
            }
            cloned = true
            try await open()
            await reload()
        } catch {
            problem = error.localizedDescription
        }
    }

    /// open starts nt on the clone, once.
    func open() async throws {
        guard core == nil, cloned else { return }
        try place.configure(host: host)
        core = try Core(place: place, git: LibGit(root: place.notes, author: host, remote: gitRemote))
    }

    /// reload lists the notes, newest first.
    func reload() async {
        guard let core else { return }
        struct Index: Decodable { let notes: [Note] }
        do {
            let ix = try await core.json(Index.self, ["_editor", "index"])
            notes = ix.notes.sorted { $0.path > $1.path }
        } catch {
            problem = error.localizedDescription
        }
    }

    /// sync runs nt sync and reads where it left the repository.
    func sync() async {
        guard let core, !syncing else { return }
        syncing = true
        defer { syncing = false }
        let out = await core.run(["sync", "--json"])
        status = SyncStatus.decode(out.stdout) ?? status
        problem = out.status == 0 ? nil : (out.stderr.isEmpty ? status?.message : out.stderr)
        await reload()
    }

    /// add adds a line to today's journal, then syncs.
    func add(_ line: String) async {
        guard let core, !line.trimmingCharacters(in: .whitespaces).isEmpty else { return }
        let out = await core.run(["a", "--json", "--", line])
        if out.status != 0 {
            problem = out.stderr
            return
        }
        await sync()
    }

    func text(of note: Note) -> String {
        (try? String(contentsOf: place.notes.appending(path: note.path), encoding: .utf8)) ?? ""
    }
}

/// background runs work off the main thread: libgit2 blocks while it talks to the remote.
func background<T: Sendable>(_ work: @escaping @Sendable () throws -> T) async throws -> T {
    try await withCheckedThrowingContinuation { cont in
        DispatchQueue.global(qos: .userInitiated).async {
            cont.resume(with: Result { try work() })
        }
    }
}
