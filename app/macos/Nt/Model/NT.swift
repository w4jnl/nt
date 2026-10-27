// NT runs the nt command line tool. nt is the one that knows the repository format and writes
// to the repository (docs/format.md §10); the app asks it, with --json, and reads files itself.
import Foundation

struct NT: Sendable {
    let executable: URL
    let environment: [String: String]

    /// What a run printed and how it ended.
    struct Output: Sendable {
        let status: Int32
        let stdout: Data
        let stderr: String
    }

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

    /// run runs nt with args and waits for it, off the main thread.
    func run(_ args: [String]) async throws -> Output {
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
                p.standardOutput = out
                p.standardError = err
                p.standardInput = FileHandle.nullDevice
                do {
                    try p.run()
                } catch {
                    cont.resume(throwing: NTError.cannotRun(exe.path, error.localizedDescription))
                    return
                }
                let stdout = out.fileHandleForReading.readDataToEndOfFile()
                let stderr = err.fileHandleForReading.readDataToEndOfFile()
                p.waitUntilExit()
                cont.resume(returning: Output(status: p.terminationStatus, stdout: stdout, stderr: String(decoding: stderr, as: UTF8.self)))
            }
        }
    }

    /// json runs nt and decodes what it printed; a failed run throws what nt said.
    func json<T: Decodable>(_ type: T.Type, _ args: [String]) async throws -> T {
        let out = try await run(args)
        guard out.status == 0 else { throw NTError.failed(NTError.message(out.stderr)) }
        do {
            return try JSONDecoder().decode(T.self, from: out.stdout)
        } catch {
            throw NTError.failed("nt \(args.first ?? "") printed something this app cannot read: \(error.localizedDescription). Is nt older than the app?")
        }
    }
}

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
