// Remote is how the phone reaches origin over ssh: the key it signs in with and the hosts it
// trusts. GitHub's host keys are pinned; any other host is trusted as it was the first time
// (kept in known_hosts.json beside the clone), and a key that changes since is refused.
import Clibgit2
import Foundation

struct Remote: Sendable {
    /// key signs in; nil until the phone has made one.
    var key: DeployKey?
    /// comment ends the public key line: nt@<host>.
    var comment: String
    /// knownHosts is the file of the hosts trusted on first use.
    var knownHosts: URL

    /// github are the SHA-256 fingerprints of GitHub's ssh host keys, Ed25519, ECDSA and RSA,
    /// as GitHub publishes them (docs.github.com, GitHub's SSH key fingerprints).
    static let github: Set<String> = [
        "+DiY3wvvV6TuJJhbpZisF/zLDA0zPMSvHdkr4UvCOqU",
        "p2QAMXNIC1TJYWeIOttrVc98/R1BUFWu3/LiyKgUfQM",
        "uNiVztksCsDhcc0u9e8BujQXVUpKZIDTMczCvj3tD2s",
    ]

    /// trust answers whether host's key, by its SHA-256 fingerprint, is the one to talk to: nil
    /// when it is, else why not.
    func trust(host: String, fingerprint: String) -> String? {
        if host == "github.com" || host == "ssh.github.com" {
            return Self.github.contains(fingerprint) ? nil
                : "\(host) showed a host key that is not GitHub's (SHA256:\(fingerprint)): not connecting"
        }
        var known = (try? JSONDecoder().decode([String: String].self, from: Data(contentsOf: knownHosts))) ?? [:]
        if let was = known[host] {
            return was == fingerprint ? nil
                : "the host key of \(host) changed (SHA256:\(fingerprint), was SHA256:\(was)): not connecting; if the change is right, remove \(host) from known_hosts.json"
        }
        known[host] = fingerprint
        if let data = try? JSONEncoder().encode(known) { try? data.write(to: knownHosts, options: .atomic) }
        return nil
    }
}

/// Transfer is one fetch, push or clone: the callbacks libgit2 calls on the way, with what they
/// learn for the error that follows.
final class Transfer {
    let remote: Remote
    private var tries = 0
    /// problem is why the callbacks stopped the transfer, in place of libgit2's message.
    var problem: String?
    /// refused is a ref the remote turned down on a push, and why.
    var refused: String?

    init(_ remote: Remote) { self.remote = remote }

    /// callbacks are libgit2's remote callbacks with this transfer as their payload; keep the
    /// transfer alive while they are used.
    func callbacks() -> git_remote_callbacks {
        var cb = git_remote_callbacks()
        git_remote_init_callbacks(&cb, UInt32(GIT_REMOTE_CALLBACKS_VERSION))
        cb.payload = Unmanaged.passUnretained(self).toOpaque()
        cb.credentials = { out, _, user, _, payload in
            guard let payload else { return -1 }
            return Unmanaged<Transfer>.fromOpaque(payload).takeUnretainedValue().credential(out, user)
        }
        cb.certificate_check = { cert, _, host, payload in
            guard let payload else { return GIT_ECERTIFICATE.rawValue }
            return Unmanaged<Transfer>.fromOpaque(payload).takeUnretainedValue().check(cert, host)
        }
        cb.push_update_reference = { ref, status, payload in
            if let payload, let status {
                let t = Unmanaged<Transfer>.fromOpaque(payload).takeUnretainedValue()
                t.refused = "\(ref.map { String(cString: $0) } ?? "the branch"): \(String(cString: status))"
            }
            return 0
        }
        return cb
    }

    /// credential signs in with the phone's key, once: libgit2 asks again after a refusal, and
    /// the same key would be refused again.
    private func credential(_ out: UnsafeMutablePointer<UnsafeMutablePointer<git_credential>?>?, _ user: UnsafePointer<CChar>?) -> Int32 {
        tries += 1
        guard let key = remote.key else {
            problem = "this phone has no ssh key yet"
            return GIT_EAUTH.rawValue
        }
        guard tries == 1 else {
            problem = "the remote refused this phone's key (\(key.fingerprint)); add it to the notes repository as a deploy key with write access"
            return GIT_EAUTH.rawValue
        }
        let name = user.map { String(cString: $0) } ?? "git"
        return git_credential_ssh_key_memory_new(out, name, key.publicLine(remote.comment), key.privateOpenSSH(remote.comment), nil)
    }

    /// check lets the transfer go on only with a host the remote trusts.
    private func check(_ cert: UnsafeMutablePointer<git_cert>?, _ host: UnsafePointer<CChar>?) -> Int32 {
        guard let cert, cert.pointee.cert_type == GIT_CERT_HOSTKEY_LIBSSH2 else {
            problem = "the remote is not an ssh host: nt on the phone talks ssh only"
            return GIT_ECERTIFICATE.rawValue
        }
        let hk = UnsafeRawPointer(cert).assumingMemoryBound(to: git_cert_hostkey.self).pointee
        guard hk.type.rawValue & GIT_CERT_SSH_SHA256.rawValue != 0 else {
            problem = "the remote's host key has no SHA-256 fingerprint"
            return GIT_ECERTIFICATE.rawValue
        }
        var hash = hk.hash_sha256
        let fingerprint = withUnsafeBytes(of: &hash) { Data($0) }.base64EncodedString().trimmingCharacters(in: ["="])
        if let why = remote.trust(host: host.map { String(cString: $0) } ?? "", fingerprint: fingerprint) {
            problem = "host key verification failed: " + why
            return GIT_ECERTIFICATE.rawValue
        }
        return 0
    }
}
