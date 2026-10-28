// DeployKey is this phone's ssh key (docs/design.md §9): ECDSA P-256, made here with CryptoKit,
// its private half kept in the Keychain on this device only. Its public half is added to the
// notes repository on GitHub as a deploy key with write access. libssh2 on mbedTLS signs with it;
// that build has no Ed25519.
import CryptoKit
import Foundation
import Security

struct DeployKey: Sendable, Equatable {
    /// raw is the private scalar, 32 bytes.
    let raw: Data

    static func make() -> DeployKey { DeployKey(raw: P256.Signing.PrivateKey().rawRepresentation) }

    /// point is the public key, uncompressed: 0x04, then x and y.
    var point: Data {
        // raw comes from CryptoKit or the Keychain, where only a valid key is ever written
        (try? P256.Signing.PrivateKey(rawRepresentation: raw))?.publicKey.x963Representation ?? Data()
    }

    /// blob is the public key as ssh encodes it: the key type, the curve and the point.
    var blob: Data { SSH.string("ecdsa-sha2-nistp256") + SSH.string("nistp256") + SSH.string(point) }

    /// publicLine is the public key as GitHub and authorized_keys take it.
    func publicLine(_ comment: String) -> String {
        "ecdsa-sha2-nistp256 \(blob.base64EncodedString()) \(comment)"
    }

    /// fingerprint is "SHA256:…", as GitHub lists deploy keys and ssh-keygen -l prints them.
    var fingerprint: String {
        "SHA256:" + Data(SHA256.hash(data: blob)).base64EncodedString().trimmingCharacters(in: ["="])
    }

    /// privateOpenSSH is the private key in OpenSSH's format, unencrypted: what libssh2 reads
    /// from memory for an ECDSA key (its mbedTLS reader of PEM needs a NUL the caller cannot
    /// pass through libgit2).
    func privateOpenSSH(_ comment: String) -> String {
        let check = UInt32.random(in: .min ... .max)
        var secret = SSH.uint32(check) + SSH.uint32(check)
        secret += SSH.string("ecdsa-sha2-nistp256") + SSH.string("nistp256") + SSH.string(point)
        secret += SSH.mpint(raw) + SSH.string(comment)
        var pad: UInt8 = 1
        while secret.count % 8 != 0 {
            secret.append(pad)
            pad += 1
        }
        var body = Data("openssh-key-v1".utf8) + Data([0])
        body += SSH.string("none") + SSH.string("none") + SSH.string(Data()) + SSH.uint32(1)
        body += SSH.string(blob) + SSH.string(secret)
        let b64 = body.base64EncodedString()
        let lines = stride(from: 0, to: b64.count, by: 70).map { i in
            let start = b64.index(b64.startIndex, offsetBy: i)
            return String(b64[start..<(b64.index(start, offsetBy: 70, limitedBy: b64.endIndex) ?? b64.endIndex)])
        }
        return "-----BEGIN OPENSSH PRIVATE KEY-----\n" + lines.joined(separator: "\n") + "\n-----END OPENSSH PRIVATE KEY-----\n"
    }

    // MARK: The Keychain

    private static var item: [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: "nl.w4j.nt", kSecAttrAccount as String: "deploy-key"]
    }

    /// load is the key in the Keychain; nil when this phone has none yet.
    static func load() -> DeployKey? {
        var q = item
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: CFTypeRef?
        guard SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess, let raw = out as? Data,
              (try? P256.Signing.PrivateKey(rawRepresentation: raw)) != nil
        else { return nil }
        return DeployKey(raw: raw)
    }

    /// save keeps the key in the Keychain in place of the one there: on this device only, and
    /// readable after the first unlock, so a sync in the background can sign with it.
    func save() throws {
        SecItemDelete(Self.item as CFDictionary)
        var q = Self.item
        q[kSecValueData as String] = raw
        q[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        let status = SecItemAdd(q as CFDictionary, nil)
        guard status == errSecSuccess else {
            throw LibGit.failure("the Keychain did not keep the key: \(SecCopyErrorMessageString(status, nil) as String? ?? "status \(status)")")
        }
    }
}

/// SSH writes the wire encoding ssh keys are made of (RFC 4251 §5).
enum SSH {
    static func uint32(_ n: UInt32) -> Data { withUnsafeBytes(of: n.bigEndian) { Data($0) } }
    static func string(_ s: String) -> Data { string(Data(s.utf8)) }
    static func string(_ d: Data) -> Data { uint32(UInt32(d.count)) + d }

    /// mpint is a positive number: no leading zero bytes, but one when the top bit is set.
    static func mpint(_ d: Data) -> Data {
        var n = Data(d.drop { $0 == 0 })
        if let first = n.first, first & 0x80 != 0 { n.insert(0, at: 0) }
        return string(n)
    }
}
