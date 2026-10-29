// Phone is what the app keeps for this phone (docs/design.md §9): the first run (the repository's
// address, this device's name, the phone's key, the clone), then nt on the clone, whose notes the
// shared Workspace shows as the Mac app's does.
import BackgroundTasks
import Foundation
import Observation
import UIKit

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

    /// keyMade is the day this phone's key was made, as Settings says it.
    var keyMade: Date? { UserDefaults.standard.object(forKey: "keyMade") as? Date }

    /// makeKey makes the phone's key, or a new one in its place: the old one stops working here.
    func makeKey() {
        let k = DeployKey.make()
        do {
            try k.save()
            key = k
            UserDefaults.standard.set(Date.now, forKey: "keyMade")
        } catch {
            problem = error.localizedDescription
        }
    }

    private var gitRemote: Remote { Remote(comment: "nt@\(host)", knownHosts: place.knownHosts) }

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

// MARK: Sync in the background

extension Phone {
    /// refreshID is the background refresh's task, in Info.plist's BGTaskSchedulerPermittedIdentifiers.
    static let refreshID = "nl.w4j.nt.ios.sync"

    /// leave saves the note and syncs as the app goes to the background, in a background task
    /// so iOS lets it finish, and asks for a background refresh later.
    func leave() {
        scheduleRefresh()
        guard let ws = workspace else { return }
        let app = UIApplication.shared
        let box = TaskBox()
        box.id = app.beginBackgroundTask(withName: "nt sync") {
            MainActor.assumeIsolated {
                app.endBackgroundTask(box.id)
                box.id = .invalid
            }
        }
        Task {
            await ws.stopDictating()
            await ws.settle()
            if box.id != .invalid { app.endBackgroundTask(box.id) }
            box.id = .invalid
        }
    }

    /// back syncs as the app comes to the front again.
    func back() {
        guard let ws = workspace else { return }
        Task { await ws.syncNow() }
    }

    /// refresh is a background refresh iOS granted: nt starts on the clone if it has not, and
    /// syncs.
    func refresh(_ task: BGAppRefreshTask) {
        scheduleRefresh()
        nonisolated(unsafe) let t = task
        let work = Task {
            if let ws = workspace { await ws.syncNow() } else { await open() }
            t.setTaskCompleted(success: !Task.isCancelled)
        }
        t.expirationHandler = { work.cancel() }
    }

    /// scheduleRefresh asks iOS for a background refresh in a quarter of an hour at the soonest;
    /// iOS decides when, or whether.
    func scheduleRefresh() {
        guard cloned else { return }
        let r = BGAppRefreshTaskRequest(identifier: Self.refreshID)
        r.earliestBeginDate = Date(timeIntervalSinceNow: 15 * 60)
        try? BGTaskScheduler.shared.submit(r)
    }
}

@MainActor
private final class TaskBox {
    var id = UIBackgroundTaskIdentifier.invalid
}

/// background runs work off the main thread: libgit2 blocks while it talks to the remote.
func background<T: Sendable>(_ work: @escaping @Sendable () throws -> T) async throws -> T {
    try await withCheckedThrowingContinuation { cont in
        DispatchQueue.global(qos: .userInitiated).async {
            cont.resume(with: Result { try work() })
        }
    }
}
