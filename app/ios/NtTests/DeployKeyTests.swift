// The phone's key as GitHub and libssh2 read it: the public line, the fingerprint, and the
// private key in OpenSSH's format holding the same key.
import CryptoKit
import Foundation
import Security
import Testing
@testable import Nt

struct DeployKeyTests {
    let key = DeployKey.make()

    /// Reader reads the ssh wire encoding.
    struct Reader {
        var d: Data.SubSequence

        mutating func uint32() -> UInt32 {
            let n = d.prefix(4).reduce(UInt32(0)) { $0 << 8 | UInt32($1) }
            d = d.dropFirst(4)
            return n
        }

        mutating func string() -> Data {
            let n = Int(uint32())
            defer { d = d.dropFirst(n) }
            return Data(d.prefix(n))
        }

        mutating func text() -> String { String(decoding: string(), as: UTF8.self) }
    }

    var point: Data { (try? P256.Signing.PrivateKey(rawRepresentation: key.raw))?.publicKey.x963Representation ?? Data() }

    @Test func thePublicLineIsWhatGitHubTakes() throws {
        let parts = key.publicLine("nt@iphone").split(separator: " ")
        #expect(parts.count == 3 && parts[0] == "ecdsa-sha2-nistp256" && parts[2] == "nt@iphone")
        var r = Reader(d: try #require(Data(base64Encoded: String(parts[1])))[...])
        let (type, curve, q) = (r.text(), r.text(), r.string())
        #expect(type == "ecdsa-sha2-nistp256" && curve == "nistp256")
        #expect(q == point && point.count == 65)
        #expect(r.d.isEmpty)
        #expect(key.fingerprint.hasPrefix("SHA256:") && key.fingerprint.count == 50)
    }

    @Test func thePrivateKeyIsTheSameKeyInOpenSSHsFormat() throws {
        let pem = key.privateOpenSSH("nt@iphone")
        #expect(pem.hasPrefix("-----BEGIN OPENSSH PRIVATE KEY-----\n") && pem.hasSuffix("\n-----END OPENSSH PRIVATE KEY-----\n"))
        #expect(pem.split(separator: "\n").allSatisfy { $0.count <= 70 })
        let data = try #require(Data(base64Encoded: pem.split(separator: "\n").dropFirst().dropLast().joined()))
        let magic = Data("openssh-key-v1".utf8) + Data([0])
        #expect(data.prefix(magic.count) == magic)
        var r = Reader(d: data.dropFirst(magic.count))
        let (cipher, kdf, options, count, blob, secret) = (r.text(), r.text(), r.string(), r.uint32(), r.string(), r.string())
        #expect(cipher == "none" && kdf == "none" && options.isEmpty && count == 1)
        #expect(blob == key.blob)
        #expect(r.d.isEmpty)
        #expect(secret.count % 8 == 0)

        var s = Reader(d: secret[...])
        let (check1, check2, type, curve, q, d, comment) = (s.uint32(), s.uint32(), s.text(), s.text(), s.string(), s.string(), s.text())
        #expect(check1 == check2, "the check numbers match")
        #expect(type == "ecdsa-sha2-nistp256" && curve == "nistp256" && q == point)
        // the scalar as an mpint: a leading zero only when its top bit is set
        #expect(Data(d.drop { $0 == 0 }) == Data(key.raw.drop { $0 == 0 }))
        #expect(d.first.map { $0 != 0 || d.count > 1 && d[d.startIndex + 1] & 0x80 != 0 } ?? false)
        #expect(comment == "nt@iphone")
        #expect(Array(s.d) == (0..<s.d.count).map { UInt8($0 + 1) }, "padded 1, 2, 3, …")
    }

    @Test func theKeychainKeepsTheKey() throws {
        let before = DeployKey.load()
        defer {
            if let before {
                try? before.save()
            } else {
                SecItemDelete([kSecClass: kSecClassGenericPassword, kSecAttrService: "nl.w4j.nt"] as CFDictionary)
            }
        }
        try key.save()
        #expect(DeployKey.load() == key)
    }
}
