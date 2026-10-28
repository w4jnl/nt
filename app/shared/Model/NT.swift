// NT runs the nt command line tool. nt is the one that knows the repository format and writes
// to the repository (docs/format.md §10); the app asks it, with --json, and reads files itself.
// On the Mac nt is a process; on iPhone and iPad it is nt's Go code in the app (Core), asked the
// same command lines and answering the same way.
import Foundation

struct NT: Sendable {
    #if os(macOS)
    let executable: URL
    let environment: [String: String]
    #else
    let core: Core
    #endif

    /// What a run printed and how it ended.
    struct Output: Sendable {
        let status: Int32
        let stdout: Data
        let stderr: String
    }

    #if os(macOS)
    /// The nt this app bundles, else $NT_BIN, else one installed by hand or by Homebrew.
    static func locate() -> NT? {
        let env = ProcessInfo.processInfo.environment
        let home = FileManager.default.homeDirectoryForCurrentUser
        var candidates: [URL] = []
        if let p = env["NT_BIN"], !p.isEmpty { candidates.append(URL(filePath: p)) }
        candidates.append(Bundle.main.bundleURL.appending(path: "Contents/Helpers/nt"))
        candidates += [
            home.appending(path: ".local/bin/nt"),
            URL(filePath: "/opt/homebrew/bin/nt"),
            URL(filePath: "/usr/local/bin/nt"),
        ]
        guard let exe = candidates.first(where: { FileManager.default.isExecutableFile(atPath: $0.path) }) else {
            return nil
        }
        return NT(executable: exe, environment: environment(env, home: home))
    }

    /// An app opened from the Finder starts with a short PATH; nt runs git and rg.
    nonisolated static func environment(_ env: [String: String], home: URL) -> [String: String] {
        var e = env
        let have = (env["PATH"] ?? "").split(separator: ":").map(String.init)
        let extra = ["/opt/homebrew/bin", "/usr/local/bin", home.appending(path: ".local/bin").path, "/usr/bin", "/bin", "/usr/sbin", "/sbin"]
        e["PATH"] = (have + extra.filter { !have.contains($0) }).joined(separator: ":")
        e["NO_COLOR"] = "1"
        return e
    }

    /// run runs nt with args, input on its standard input, and waits for it, off the main thread.
    func run(_ args: [String], input: Data? = nil) async throws -> Output {
        let exe = executable
        let env = environment
        return try await withCheckedThrowingContinuation { cont in
            DispatchQueue.global(qos: .userInitiated).async {
                let p = Process()
                p.executableURL = exe
                p.arguments = args
                p.environment = env
                p.currentDirectoryURL = FileManager.default.homeDirectoryForCurrentUser
                let out = Pipe()
                let err = Pipe()
                let stdin = Pipe()
                p.standardOutput = out
                p.standardError = err
                p.standardInput = input == nil ? FileHandle.nullDevice : stdin
                do {
                    try p.run()
                } catch {
                    cont.resume(throwing: NTError.cannotRun(exe.path, error.localizedDescription))
                    return
                }
                if let input {
                    // nt reads all of its input before it prints anything
                    try? stdin.fileHandleForWriting.write(contentsOf: input)
                    try? stdin.fileHandleForWriting.close()
                }
                let stdout = out.fileHandleForReading.readDataToEndOfFile()
                let stderr = err.fileHandleForReading.readDataToEndOfFile()
                p.waitUntilExit()
                cont.resume(returning: Output(status: p.terminationStatus, stdout: stdout, stderr: String(decoding: stderr, as: UTF8.self)))
            }
        }
    }

    #else
    /// run asks the core, off the main thread; what it says when it fails reads as nt's stderr.
    func run(_ args: [String], input: Data? = nil) async throws -> Output {
        let out = await core.run(args, input: input)
        return Output(status: Int32(out.status), stdout: out.stdout, stderr: out.stderr.isEmpty ? "" : "nt: " + out.stderr)
    }

    #endif

    /// json runs nt and decodes what it printed; a failed run throws what nt said.
    func json<T: Decodable>(_ type: T.Type, _ args: [String], input: Data? = nil) async throws -> T {
        let out = try await run(args, input: input)
        guard out.status == 0 else { throw NTError.failed(NTError.message(out.stderr)) }
        do {
            return try JSONDecoder().decode(T.self, from: out.stdout)
        } catch {
            throw NTError.failed("nt \(args.first ?? "") printed something this app cannot read: \(error.localizedDescription). Is nt older than the app?")
        }
    }
}

#if os(macOS)
/// Hold is `nt _editor hold` running: .git/nt-edit held (shared) while a note has unsaved
/// changes, so sync commits but does not merge under them. Closing its input lets it go; so does
/// the app ending.
final class Hold: @unchecked Sendable {
    private let process = Process()
    private let input = Pipe()

    init?(_ nt: NT) {
        process.executableURL = nt.executable
        process.arguments = ["_editor", "hold"]
        process.environment = nt.environment
        process.standardInput = input
        process.standardOutput = FileHandle.nullDevice
        process.standardError = FileHandle.nullDevice
        do { try process.run() } catch { return nil }
    }

    func release() {
        try? input.fileHandleForWriting.close()
    }

    deinit { release() }
}
#else
/// Hold is the core holding .git/nt-edit (shared) while a note has unsaved changes, so sync
/// commits but does not merge under them, as `nt _editor hold` does on the Mac.
final class Hold: @unchecked Sendable {
    private let core: Core

    init?(_ nt: NT) {
        core = nt.core
        guard core.hold() else { return nil }
    }

    func release() {
        core.unhold()
    }

    deinit { release() }
}
#endif

enum NTError: LocalizedError, Equatable {
    case cannotRun(String, String)
    case failed(String)

    var errorDescription: String? {
        switch self {
        case .cannotRun(let path, let why): "Cannot run \(path): \(why)"
        case .failed(let msg): msg
        }
    }

    /// message is nt's error line without its "nt: " prefix.
    nonisolated static func message(_ stderr: String) -> String {
        let line = stderr.split(separator: "\n").last.map(String.init) ?? ""
        let msg = line.hasPrefix("nt: ") ? String(line.dropFirst(4)) : line
        return msg.isEmpty ? "nt failed without saying why" : msg
    }
}
