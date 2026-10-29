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
    @ObservationIgnored private var starting: Task<Workspace?, Never>?

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

    /// open is the app on screen: nt runs on the clone, started if a Shortcut has not started it
    /// already, and it syncs.
    func open() async {
        guard let ws = await started() else { return }
        await ws.syncNow()
    }

    /// started is the notes with nt running on the clone, started once: by the app on screen, or
    /// by a Shortcut with the app off screen. Nil when there is no clone yet, or nt cannot start
    /// (problem says why).
    func started() async -> Workspace? {
        guard cloned else { return nil }
        let start = starting ?? Task { await self.start() }
        starting = start
        let ws = await start.value
        if ws == nil { starting = nil } // Try again starts it again
        return ws
    }

    private func start() async -> Workspace? {
        do {
            try place.configure(host: host)
            let core = try Core(place: place, git: LibGit(root: place.notes, author: host, remote: gitRemote))
            let ws = Workspace(nt: NT(core: core))
            workspace = ws
            await ws.start()
            return ws
        } catch {
            problem = error.localizedDescription
            return nil
        }
    }
}

// MARK: Shortcuts and Siri

extension Phone {
    /// add adds text from outside the app (Shortcuts, Siri, Spotlight, the Action button) as the
    /// capture sheet does: a line to today's journal under Log, or a todo to todo.md under
    /// Inbox, due:fri in the text setting its date. It says where the text went, or throws why it
    /// did not.
    func add(_ text: String, todo: Bool) async throws -> Capture.Added {
        guard cloned else { throw LibGit.failure("Your notes are not on this phone yet. Open nt and clone them first.") }
        guard let ws = await started() else { throw LibGit.failure(problem ?? "nt did not start") }
        let capture = Capture(workspace: ws)
        capture.mode = todo ? .todo : .today
        capture.text = text
        guard await capture.submit(alternate: false), let added = capture.added else {
            throw LibGit.failure(capture.problem.isEmpty ? "Nothing to add: the text is empty." : capture.problem)
        }
        return added
    }

    /// syncAdded syncs what add wrote. On screen the app syncs half a minute later, as after a
    /// save; off screen it syncs now, in a background task, since iOS may suspend the app before
    /// then.
    func syncAdded() {
        guard UIApplication.shared.applicationState != .active, let ws = workspace else { return }
        whileAway { await ws.settle() }
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
        whileAway {
            await ws.stopDictating()
            await ws.settle()
        }
    }

    /// whileAway does work in a background task, so iOS lets it finish with the app off screen.
    private func whileAway(_ work: @escaping @MainActor () async -> Void) {
        let app = UIApplication.shared
        let box = TaskBox()
        box.id = app.beginBackgroundTask(withName: "nt sync") {
            MainActor.assumeIsolated {
                app.endBackgroundTask(box.id)
                box.id = .invalid
            }
        }
        Task {
            await work()
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
            await open()
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
