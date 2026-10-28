// Phone is what the app keeps for this phone (docs/design.md §9): the first run (the repository's
// address, this device's name, the phone's key, the clone), then nt on the clone, whose notes the
// shared Workspace shows as the Mac app's does.
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
    /// workspace is the notes once nt runs on the clone.
    private(set) var workspace: Workspace?
    /// problem is the last thing that went wrong, as the person reads it.
    var problem: String?

    let place: Place

    init(place: Place = .app) {
        self.place = place
        remote = UserDefaults.standard.string(forKey: "remote") ?? ""
        host = UserDefaults.standard.string(forKey: "host") ?? "iphone"
        key = DeployKey.load()
        cloned = place.cloned
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
            await open()
        } catch {
            problem = error.localizedDescription
        }
    }

    /// open starts nt on the clone, once, and shows today and the sync state.
    func open() async {
        guard workspace == nil, cloned else { return }
        do {
            try place.configure(host: host)
            let core = try Core(place: place, git: LibGit(root: place.notes, author: host, remote: gitRemote))
            let ws = Workspace(nt: NT(core: core))
            workspace = ws
            await ws.start()
            await ws.syncNow()
        } catch {
            problem = error.localizedDescription
        }
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
