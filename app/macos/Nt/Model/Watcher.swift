// RepoWatcher tells the app which files changed under some folders (FSEvents): a sync merging,
// an agent writing a report, nvim saving a note, nt sync writing status.json.
import CoreServices
import Foundation

final class RepoWatcher: @unchecked Sendable {
    private var stream: FSEventStreamRef?
    private let handler: @Sendable ([String]) -> Void
    private let queue = DispatchQueue(label: "nl.w4j.nt.watch")

    init(paths: [String], handler: @escaping @Sendable ([String]) -> Void) {
        self.handler = handler
        var ctx = FSEventStreamContext(version: 0, info: Unmanaged.passUnretained(self).toOpaque(), retain: nil, release: nil, copyDescription: nil)
        let callback: FSEventStreamCallback = { _, info, _, paths, _, _ in
            guard let info else { return }
            let me = Unmanaged<RepoWatcher>.fromOpaque(info).takeUnretainedValue()
            let list = unsafeBitCast(paths, to: NSArray.self) as? [String] ?? []
            me.handler(list)
        }
        let flags = FSEventStreamCreateFlags(kFSEventStreamCreateFlagUseCFTypes | kFSEventStreamCreateFlagFileEvents | kFSEventStreamCreateFlagNoDefer)
        guard let s = FSEventStreamCreate(nil, callback, &ctx, paths as CFArray, FSEventStreamEventId(kFSEventStreamEventIdSinceNow), 0.3, flags) else {
            return
        }
        stream = s
        FSEventStreamSetDispatchQueue(s, queue)
        FSEventStreamStart(s)
    }

    deinit {
        guard let s = stream else { return }
        FSEventStreamStop(s)
        FSEventStreamInvalidate(s)
        FSEventStreamRelease(s)
    }

    /// relevant says whether a changed path matters to the notes: not git's own files, not nt's
    /// temp files mid-write, not hidden ones.
    nonisolated static func relevant(_ path: String, root: String) -> Bool {
        guard path.hasPrefix(root + "/") else { return false }
        let rel = path.dropFirst(root.count + 1)
        return !rel.split(separator: "/").contains { $0.hasPrefix(".") }
    }
}
