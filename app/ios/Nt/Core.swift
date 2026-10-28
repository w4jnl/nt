// Core is nt in this process: NtCore, the mobile package, answering the command lines the Mac
// app runs nt with (`sync --json`, `a --json -- …`, `_editor index`, …) with the same output, on
// the clone, with LibGit as its git. One command at a time, off the main thread: a sync can
// wait on the network.
import Foundation
import NtCore

/// Place is where nt keeps things on this phone: in the app's Application Support, the clone,
/// nt's config file and state folder, and the ssh hosts trusted on first use. Not in iCloud
/// and not in a shared container (docs/design.md §9).
struct Place: Sendable {
    let base: URL
    var notes: URL { base.appending(path: "notes") }
    var config: URL { base.appending(path: "config.toml") }
    var state: URL { base.appending(path: "state") }
    var knownHosts: URL { base.appending(path: "known_hosts.json") }

    static let app = Place(base: URL.applicationSupportDirectory.appending(path: "nt"))

    /// cloned is whether the notes are here.
    var cloned: Bool { FileManager.default.fileExists(atPath: notes.appending(path: ".git").path) }

    /// configure writes nt's config for this phone: its host, and no digests, which a phone never
    /// compiles. The clone is NT_REPO, since the app's folder moves between installs.
    func configure(host: String) throws {
        try FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
        try "host = \"\(host)\"\ndigest = false\nweekly = false\n".write(to: config, atomically: true, encoding: .utf8)
    }
}

/// Core is unchecked Sendable: its MobileCore runs commands on its queue only, and Go keeps the
/// hold to itself.
final class Core: @unchecked Sendable {
    /// Output is what a command printed and how it ended, as NT.Output is on the Mac.
    struct Output: Sendable {
        let status: Int
        let stdout: Data
        let stderr: String
    }

    private let core: MobileCore
    private let queue = DispatchQueue(label: "nl.w4j.nt.core")

    /// init points nt at place and takes git from LibGit. One Core per process: nt reads its
    /// paths from the environment.
    init(place: Place, git: LibGit) throws {
        guard let core = MobileNewCore(place.config.path, place.state.path, place.notes.path, git) else {
            throw LibGit.failure("nt did not start")
        }
        self.core = core
    }

    /// run runs one command line, with input as its standard input.
    func run(_ args: [String], input: Data? = nil) async -> Output {
        let line = String(decoding: (try? JSONEncoder().encode(args)) ?? Data("[]".utf8), as: UTF8.self)
        return await withCheckedContinuation { cont in
            queue.async {
                let r = self.core.run(line, input: input ?? Data())
                cont.resume(returning: Output(status: r?.status ?? 1, stdout: r?.output ?? Data(), stderr: r?.message ?? "nt gave no answer"))
            }
        }
    }

    /// hold holds .git/nt-edit for a note with unsaved changes, until unhold; it does not wait
    /// for a command running on the queue.
    func hold() -> Bool { (try? core.hold()) != nil }

    func unhold() { core.unhold() }

    /// json runs a command line and decodes what it printed; a failed run throws what nt said.
    func json<T: Decodable>(_ type: T.Type, _ args: [String], input: Data? = nil) async throws -> T {
        let out = await run(args, input: input)
        guard out.status == 0 else { throw LibGit.failure(out.stderr.isEmpty ? "nt \(args.first ?? "") failed" : out.stderr) }
        return try JSONDecoder().decode(T.self, from: out.stdout)
    }
}
