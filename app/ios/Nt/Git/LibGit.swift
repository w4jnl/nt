// LibGit is the notes repository's plumbing on libgit2: the phone's git, in place of the git
// command line the Mac runs. nt's Go code (NtCore) asks for it through mobile.Git; the Git
// interface in internal/gitsync/git.go says what each call does. The policy stays in Go: this
// only does what it is asked. Lists cross to Go as NUL-separated strings. A fetch or push that
// cannot reach the remote, or whose key the remote refuses, fails with a message that starts
// "offline: "; a push the remote turns down as behind it, "rejected: ".
import Clibgit2
import Foundation
import NtCore

final class LibGit: NSObject, MobileGitProtocol, Sendable {
    /// root is the clone's worktree.
    let root: URL
    /// author makes the commits when the clone's config names nobody: the device's name, the
    /// host in nt's commit messages.
    let author: String
    let remote: Remote

    init(root: URL, author: String, remote: Remote) {
        _ = Self.started
        self.root = root
        self.author = author
        self.remote = remote
    }

    private static let started: Bool = git_libgit2_init() > 0

    // MARK: Refs

    func branch(_ err: NSErrorPointer) -> String {
        answer(err) { repo in
            var head: OpaquePointer?
            try check(git_reference_lookup(&head, repo, "HEAD"), "HEAD")
            defer { git_reference_free(head) }
            guard git_reference_type(head) == GIT_REFERENCE_SYMBOLIC, let target = git_reference_symbolic_target(head),
                  let b = String(cString: target).cut("refs/heads/")
            else { throw Self.failure("HEAD is detached") }
            return b
        }
    }

    func remoteBranch() -> String {
        (try? with { repo in
            var ref: OpaquePointer?
            guard git_reference_lookup(&ref, repo, "refs/remotes/origin/HEAD") == 0 else { return "" }
            defer { git_reference_free(ref) }
            guard let target = git_reference_symbolic_target(ref) else { return "" }
            return String(cString: target).cut("refs/remotes/origin/") ?? ""
        }) ?? ""
    }

    func hasRemote() -> Bool {
        (try? with { repo in
            var r: OpaquePointer?
            defer { git_remote_free(r) }
            return git_remote_lookup(&r, repo, "origin") == 0
        }) ?? false
    }

    func upstream(_ branch: String?) -> Bool {
        (try? with { repo in
            var ref: OpaquePointer?
            defer { git_reference_free(ref) }
            return git_reference_lookup(&ref, repo, "refs/remotes/origin/\(branch ?? "")") == 0
        }) ?? false
    }

    func count(_ rng: String?) -> Int {
        (try? with { repo in
            var walk: OpaquePointer?
            guard git_revwalk_new(&walk, repo) == 0 else { return 0 }
            defer { git_revwalk_free(walk) }
            let r = rng ?? ""
            guard (r.contains("..") ? git_revwalk_push_range(walk, r) : git_revwalk_push_ref(walk, r)) == 0 else { return 0 }
            var n = 0
            var id = git_oid()
            while git_revwalk_next(&id, walk) == 0 { n += 1 }
            return n
        }) ?? 0
    }

    // MARK: The index and the worktree

    func status(_ err: NSErrorPointer) -> String {
        answer(err) { repo in
            var seen = Set<String>()
            return try changes(repo).map(\.path).filter { seen.insert($0).inserted }.joined(separator: "\0")
        }
    }

    func stage(_ exclude: String?) throws {
        let skip = Set(Self.split(exclude))
        let worktree = [GIT_STATUS_WT_NEW, GIT_STATUS_WT_MODIFIED, GIT_STATUS_WT_DELETED, GIT_STATUS_WT_TYPECHANGE,
                        GIT_STATUS_WT_RENAMED, GIT_STATUS_CONFLICTED].reduce(0) { $0 | $1.rawValue }
        try with { repo in
            let idx = try index(repo)
            defer { git_index_free(idx) }
            for c in try changes(repo) where c.flags & worktree != 0 && !skip.contains(c.path) {
                if (c.path as NSString).lastPathComponent.hasPrefix(".nt-tmp-") { continue }
                try put(idx, c.path)
            }
            try check(git_index_write(idx), "write the index")
        }
    }

    func staged(_ err: NSErrorPointer) -> String {
        answer(err) { repo in
            let idx = try index(repo)
            defer { git_index_free(idx) }
            let head = try headTree(repo)
            defer { git_object_free(head) }
            var diff: OpaquePointer?
            try check(git_diff_tree_to_index(&diff, repo, head, idx, nil), "diff the index")
            defer { git_diff_free(diff) }
            var paths: [String] = []
            for i in 0..<git_diff_num_deltas(diff) {
                guard let d = git_diff_get_delta(diff, i)?.pointee, let p = d.new_file.path ?? d.old_file.path else { continue }
                paths.append(String(cString: p))
            }
            return paths.joined(separator: "\0")
        }
    }

    func add(_ paths: String?) throws {
        try with { repo in
            let idx = try index(repo)
            defer { git_index_free(idx) }
            for p in Self.split(paths) { try put(idx, p) }
            try check(git_index_write(idx), "write the index")
        }
    }

    func forget(_ path: String?) throws {
        try with { repo in
            let idx = try index(repo)
            defer { git_index_free(idx) }
            let code = git_index_remove_bypath(idx, path ?? "")
            if code != GIT_ENOTFOUND.rawValue { try check(code, "forget \(path ?? "")") }
            try check(git_index_write(idx), "write the index")
        }
    }

    func files(_ err: NSErrorPointer) -> String {
        answer(err) { repo in
            let idx = try index(repo)
            defer { git_index_free(idx) }
            var seen = Set<String>()
            var out: [String] = []
            for i in 0..<git_index_entrycount(idx) {
                guard let p = git_index_get_byindex(idx, i)?.pointee.path else { continue }
                let s = String(cString: p)
                if seen.insert(s).inserted { out.append(s) }
            }
            return out.joined(separator: "\0")
        }
    }

    func commit(_ msg: String?) throws {
        try with { repo in
            let idx = try index(repo)
            defer { git_index_free(idx) }
            var treeID = git_oid()
            try check(git_index_write_tree(&treeID, idx), "write the tree")
            var tree: OpaquePointer?
            try check(git_tree_lookup(&tree, repo, &treeID), "the tree")
            defer { git_tree_free(tree) }

            var parents: [OpaquePointer?] = []
            defer { parents.forEach { git_commit_free($0) } }
            var head = git_oid()
            if git_reference_name_to_id(&head, repo, "HEAD") == 0 {
                parents.append(try lookup(repo, head))
            }
            let merging = mergeHeads(repo)
            for id in merging { parents.append(try lookup(repo, id)) }

            var message = msg ?? ""
            let comments = message.isEmpty
            if message.isEmpty {
                message = (try? String(contentsOf: gitDir(repo).appending(path: "MERGE_MSG"), encoding: .utf8)) ?? "Merge"
            }
            var buf = git_buf()
            defer { git_buf_dispose(&buf) }
            try check(git_message_prettify(&buf, message, comments ? 1 : 0, CChar(UInt8(ascii: "#"))), "the commit message")

            var sig: UnsafeMutablePointer<git_signature>?
            if git_signature_default(&sig, repo) != 0 {
                try check(git_signature_now(&sig, author, "\(author)@nt.invalid"), "the commit's author")
            }
            defer { git_signature_free(sig) }
            var id = git_oid()
            try parents.withUnsafeMutableBufferPointer { p in
                try check(git_commit_create(&id, repo, "HEAD", sig, sig, nil, buf.ptr, tree, p.count, p.baseAddress), "commit")
            }
            if !merging.isEmpty { try check(git_repository_state_cleanup(repo), "finish the merge") }
        }
    }

    // MARK: Merging

    func merge(_ branch: String?) throws {
        let b = branch ?? ""
        let made = try with { repo -> Bool in
            var ref: OpaquePointer?
            try check(git_reference_lookup(&ref, repo, "refs/remotes/origin/\(b)"), "origin/\(b)")
            defer { git_reference_free(ref) }
            var theirs: OpaquePointer?
            try check(git_annotated_commit_from_ref(&theirs, repo, ref), "origin/\(b)")
            defer { git_annotated_commit_free(theirs) }
            var heads: [OpaquePointer?] = [theirs]
            var analysis = git_merge_analysis_t(0)
            var preference = git_merge_preference_t(0)
            try check(git_merge_analysis(&analysis, &preference, repo, &heads, 1), "merge origin/\(b)")
            if analysis.rawValue & GIT_MERGE_ANALYSIS_UP_TO_DATE.rawValue != 0 { return false }
            if analysis.rawValue & (GIT_MERGE_ANALYSIS_FASTFORWARD.rawValue | GIT_MERGE_ANALYSIS_UNBORN.rawValue) != 0 {
                try fastForward(repo, to: git_annotated_commit_id(theirs).pointee, branch: b)
                return false
            }
            // libgit2 merges through .gitattributes, so the journals' merge=union holds
            try check(git_merge(repo, &heads, 1, nil, nil), "merge origin/\(b)")
            try Data("Merge remote-tracking branch 'origin/\(b)'\n".utf8).write(to: gitDir(repo).appending(path: "MERGE_MSG"))
            let idx = try index(repo)
            defer { git_index_free(idx) }
            if git_index_has_conflicts(idx) != 0 { throw Self.failure("merge origin/\(b): conflicts in the index") }
            return true
        }
        // git merge commits a merge that went through; libgit2 leaves that to its caller
        if made { try commit("") }
    }

    func unmerged(_ err: NSErrorPointer) -> String {
        answer(err) { repo in
            let idx = try index(repo)
            defer { git_index_free(idx) }
            var it: OpaquePointer?
            try check(git_index_conflict_iterator_new(&it, idx), "the conflicts")
            defer { git_index_conflict_iterator_free(it) }
            var items: [String] = []
            var base, ours, theirs: UnsafePointer<git_index_entry>?
            while git_index_conflict_next(&base, &ours, &theirs, it) == 0 {
                for (stage, e) in [(1, base), (2, ours), (3, theirs)] {
                    guard let e, let p = e.pointee.path else { continue }
                    items.append("\(stage) \(Self.hex(e.pointee.id)) \(String(cString: p))")
                }
            }
            return items.joined(separator: "\0")
        }
    }

    func blob(_ id: String?) throws -> Data {
        try with { repo in
            var oid = git_oid()
            try check(git_oid_fromstr(&oid, id ?? ""), "blob \(id ?? "")")
            return try blob(repo, oid)
        }
    }

    func take(_ path: String?, theirs: Bool) throws {
        let p = path ?? ""
        try with { repo in
            let idx = try index(repo)
            defer { git_index_free(idx) }
            var base, ours, their: UnsafePointer<git_index_entry>?
            try check(git_index_conflict_get(&base, &ours, &their, idx, p), "\(p) is not in conflict")
            guard let side = theirs ? their : ours else { throw Self.failure("\(p) has no \(theirs ? "remote" : "local") version") }
            let url = root.appending(path: p)
            try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
            try blob(repo, side.pointee.id).write(to: url, options: .atomic)
        }
    }

    // MARK: The remote

    func fetch() throws {
        try transfer("fetch") { repo, t in
            let r = try origin(repo)
            defer { git_remote_free(r) }
            var opts = git_fetch_options()
            git_fetch_options_init(&opts, UInt32(GIT_FETCH_OPTIONS_VERSION))
            opts.callbacks = t.callbacks()
            opts.prune = GIT_FETCH_PRUNE
            return git_remote_fetch(r, nil, &opts, nil)
        }
    }

    func push(_ branch: String?, first: Bool) throws {
        let b = branch ?? ""
        try transfer("push") { repo, t in
            let r = try origin(repo)
            defer { git_remote_free(r) }
            var opts = git_push_options()
            git_push_options_init(&opts, UInt32(GIT_PUSH_OPTIONS_VERSION))
            opts.callbacks = t.callbacks()
            let spec = strdup("refs/heads/\(b):refs/heads/\(b)")
            defer { free(spec) }
            var specs: [UnsafeMutablePointer<CChar>?] = [spec]
            return specs.withUnsafeMutableBufferPointer { p in
                var arr = git_strarray(strings: p.baseAddress, count: 1)
                return git_remote_push(r, &arr, &opts)
            }
        }
        guard first else { return }
        try with { repo in
            var ref: OpaquePointer?
            try check(git_branch_lookup(&ref, repo, b, GIT_BRANCH_LOCAL), b)
            defer { git_reference_free(ref) }
            try check(git_branch_set_upstream(ref, "origin/\(b)"), "the upstream of \(b)")
        }
    }

    /// clone clones url into dir over ssh, with remote's key and trusted hosts. A clone that
    /// fails leaves nothing behind, and says why without sync's "offline: ".
    static func clone(_ url: String, to dir: URL, remote: Remote) throws {
        _ = started
        let t = Transfer(remote)
        var opts = git_clone_options()
        git_clone_options_init(&opts, UInt32(GIT_CLONE_OPTIONS_VERSION))
        opts.fetch_opts.callbacks = t.callbacks()
        var repo: OpaquePointer?
        let code = withExtendedLifetime(t) { git_clone(&repo, url, dir.path, &opts) }
        git_repository_free(repo)
        guard code < 0 else { return }
        let why = describe(code, t)
        try? FileManager.default.removeItem(at: dir)
        throw failure(why.message)
    }

    /// create makes an empty repository at dir on branch main, bare or with a worktree.
    static func create(at dir: URL, bare: Bool) throws {
        _ = started
        var opts = git_repository_init_options()
        git_repository_init_options_init(&opts, UInt32(GIT_REPOSITORY_INIT_OPTIONS_VERSION))
        opts.flags = GIT_REPOSITORY_INIT_MKPATH.rawValue | (bare ? GIT_REPOSITORY_INIT_BARE.rawValue : 0)
        var repo: OpaquePointer?
        let code = "main".withCString { head in
            opts.initial_head = head
            return git_repository_init_ext(&repo, dir.path, &opts)
        }
        git_repository_free(repo)
        try check(code, "create \(dir.path)")
    }

    /// setOrigin points origin at url, adding origin when there is none.
    func setOrigin(_ url: String) throws {
        try with { repo in
            var r: OpaquePointer?
            if git_remote_lookup(&r, repo, "origin") == 0 {
                git_remote_free(r)
                try check(git_remote_set_url(repo, "origin", url), "origin")
            } else {
                try check(git_remote_create(&r, repo, "origin", url), "origin")
                git_remote_free(r)
            }
        }
    }

    // MARK: Helpers

    /// with opens the repository for one call: nt writes files between calls, and a handle of
    /// its own reads the index and the refs as they are now.
    private func with<T>(_ body: (OpaquePointer) throws -> T) throws -> T {
        var repo: OpaquePointer?
        try check(git_repository_open(&repo, root.path), "open \(root.path)")
        defer { git_repository_free(repo) }
        return try body(repo!)
    }

    /// answer is with for the calls that hand Go a string and a failure through err.
    private func answer(_ err: NSErrorPointer, _ body: (OpaquePointer) throws -> String) -> String {
        do {
            return try with(body)
        } catch {
            err?.pointee = error as NSError
            return ""
        }
    }

    /// transfer runs a fetch or a push and words its failure as sync reads it.
    private func transfer(_ what: String, _ body: (OpaquePointer, Transfer) throws -> Int32) throws {
        try with { repo in
            let t = Transfer(remote)
            let code = try withExtendedLifetime(t) { try body(repo, t) }
            if let refused = t.refused { throw Self.failure("rejected: \(what): \(refused)") }
            guard code < 0 else { return }
            let why = Self.describe(code, t)
            throw Self.failure((why.kind.map { $0 + ": " } ?? "\(what): ") + why.message)
        }
    }

    /// describe is a failed transfer's kind, offline or rejected or neither, and its message.
    private static func describe(_ code: Int32, _ t: Transfer) -> (kind: String?, message: String) {
        let message = t.problem ?? lastMessage(code)
        if code == GIT_ENONFASTFORWARD.rawValue { return ("rejected", message) }
        let klass = git_error_last()?.pointee.klass ?? 0
        let unreachable = [GIT_EAUTH, GIT_ECERTIFICATE, GIT_TIMEOUT].map(\.rawValue).contains(code)
            // a connection refused is the operating system's error
            || [GIT_ERROR_NET, GIT_ERROR_SSH, GIT_ERROR_OS].map { Int32($0.rawValue) }.contains(klass)
        return (t.problem != nil || unreachable ? "offline" : nil, message)
    }

    /// changes are the paths that differ from HEAD, in the index or the worktree, untracked ones
    /// too, with their status flags.
    private func changes(_ repo: OpaquePointer) throws -> [(path: String, flags: UInt32)] {
        var opts = git_status_options()
        try check(git_status_options_init(&opts, UInt32(GIT_STATUS_OPTIONS_VERSION)), "status")
        opts.show = GIT_STATUS_SHOW_INDEX_AND_WORKDIR
        opts.flags = GIT_STATUS_OPT_INCLUDE_UNTRACKED.rawValue | GIT_STATUS_OPT_RECURSE_UNTRACKED_DIRS.rawValue
        var list: OpaquePointer?
        try check(git_status_list_new(&list, repo, &opts), "status")
        defer { git_status_list_free(list) }
        var out: [(String, UInt32)] = []
        for i in 0..<git_status_list_entrycount(list) {
            guard let e = git_status_byindex(list, i)?.pointee,
                  let d = (e.index_to_workdir ?? e.head_to_index)?.pointee,
                  let p = d.new_file.path ?? d.old_file.path
            else { continue }
            out.append((String(cString: p), e.status.rawValue))
        }
        return out
    }

    /// put stages path as the worktree has it: its content, or its removal.
    private func put(_ idx: OpaquePointer?, _ path: String) throws {
        if FileManager.default.fileExists(atPath: root.appending(path: path).path) {
            try check(git_index_add_bypath(idx, path), "stage \(path)")
        } else {
            try check(git_index_remove_bypath(idx, path), "stage the removal of \(path)")
        }
    }

    private func index(_ repo: OpaquePointer) throws -> OpaquePointer? {
        var idx: OpaquePointer?
        try check(git_repository_index(&idx, repo), "the index")
        return idx
    }

    private func origin(_ repo: OpaquePointer) throws -> OpaquePointer? {
        var r: OpaquePointer?
        try check(git_remote_lookup(&r, repo, "origin"), "origin")
        return r
    }

    /// headTree is HEAD's tree; nil before the first commit.
    private func headTree(_ repo: OpaquePointer) throws -> OpaquePointer? {
        var obj: OpaquePointer?
        let code = git_revparse_single(&obj, repo, "HEAD^{tree}")
        if code == GIT_ENOTFOUND.rawValue || code == GIT_EUNBORNBRANCH.rawValue { return nil }
        try check(code, "HEAD")
        return obj
    }

    private func lookup(_ repo: OpaquePointer, _ id: git_oid) throws -> OpaquePointer? {
        var id = id
        var c: OpaquePointer?
        try check(git_commit_lookup(&c, repo, &id), "commit \(Self.hex(id))")
        return c
    }

    private func blob(_ repo: OpaquePointer, _ id: git_oid) throws -> Data {
        var id = id
        var b: OpaquePointer?
        try check(git_blob_lookup(&b, repo, &id), "blob \(Self.hex(id))")
        defer { git_blob_free(b) }
        return Data(bytes: git_blob_rawcontent(b), count: Int(git_blob_rawsize(b)))
    }

    private func fastForward(_ repo: OpaquePointer, to id: git_oid, branch: String) throws {
        var target = id
        var c: OpaquePointer?
        try check(git_commit_lookup(&c, repo, &target), "origin/\(branch)")
        defer { git_commit_free(c) }
        var tree: OpaquePointer?
        try check(git_commit_tree(&tree, c), "the tree of origin/\(branch)")
        defer { git_tree_free(tree) }
        var opts = git_checkout_options()
        git_checkout_options_init(&opts, UInt32(GIT_CHECKOUT_OPTIONS_VERSION))
        opts.checkout_strategy = GIT_CHECKOUT_SAFE.rawValue
        try check(git_checkout_tree(repo, tree, &opts), "check out origin/\(branch)")
        var ref: OpaquePointer?
        try check(git_reference_create(&ref, repo, "refs/heads/\(branch)", &target, 1, "nt: fast-forward to origin/\(branch)"), "move \(branch)")
        git_reference_free(ref)
    }

    /// mergeHeads are the commits a merge in progress brings in, from .git/MERGE_HEAD.
    private func mergeHeads(_ repo: OpaquePointer) -> [git_oid] {
        guard let text = try? String(contentsOf: gitDir(repo).appending(path: "MERGE_HEAD"), encoding: .utf8) else { return [] }
        return text.split(whereSeparator: \.isNewline).compactMap { line in
            var id = git_oid()
            return git_oid_fromstr(&id, String(line)) == 0 ? id : nil
        }
    }

    private func gitDir(_ repo: OpaquePointer) -> URL { URL(filePath: String(cString: git_repository_path(repo))) }

    static func hex(_ id: git_oid) -> String {
        var id = id
        var buf = [CChar](repeating: 0, count: 41)
        git_oid_tostr(&buf, buf.count, &id)
        return String(decoding: buf.prefix { $0 != 0 }.map { UInt8(bitPattern: $0) }, as: UTF8.self)
    }

    static func split(_ s: String?) -> [String] {
        guard let s, !s.isEmpty else { return [] }
        return s.split(separator: "\0", omittingEmptySubsequences: true).map(String.init)
    }

    /// failure is an error Go reads by its message.
    static func failure(_ message: String) -> NSError {
        NSError(domain: "nt.git", code: 1, userInfo: [NSLocalizedDescriptionKey: message])
    }

    static func lastMessage(_ code: Int32) -> String {
        if let e = git_error_last(), let m = e.pointee.message {
            let s = String(cString: m).trimmingCharacters(in: .whitespacesAndNewlines)
            if !s.isEmpty { return s }
        }
        return "libgit2 error \(code)"
    }
}

/// check throws libgit2's last error, named by what was being done, when code is a failure.
private func check(_ code: Int32, _ what: String) throws {
    guard code < 0 else { return }
    throw LibGit.failure("\(what): \(LibGit.lastMessage(code))")
}

private extension String {
    /// cut is the string after prefix; nil when it does not start with it.
    func cut(_ prefix: String) -> String? { hasPrefix(prefix) ? String(dropFirst(prefix.count)) : nil }
}
