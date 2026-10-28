// The last sync's outcome, as nt sync writes it to status.json in its state folder, and the
// glyph and word the sidebar shows for it (docs/design-system/README.md, Colour).
import Foundation

struct SyncStatus: Codable, Sendable, Equatable {
    let state: String
    var ahead: Int = 0
    var conflicts: [String]? = nil
    var message: String? = nil
    let at: Date

    enum CodingKeys: String, CodingKey { case state, ahead, conflicts, message, at }

    init(state: String, ahead: Int = 0, conflicts: [String]? = nil, message: String? = nil, at: Date) {
        self.state = state
        self.ahead = ahead
        self.conflicts = conflicts
        self.message = message
        self.at = at
    }

    /// Fields an older or newer nt leaves out take their zero value.
    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        state = try c.decode(String.self, forKey: .state)
        ahead = try c.decodeIfPresent(Int.self, forKey: .ahead) ?? 0
        conflicts = try c.decodeIfPresent([String].self, forKey: .conflicts)
        message = try c.decodeIfPresent(String.self, forKey: .message)
        at = try c.decode(Date.self, forKey: .at)
    }

    /// read reads status.json in the state folder; nil when sync has not run yet.
    nonisolated static func read(stateDir: String) -> SyncStatus? {
        let url = URL(filePath: stateDir).appending(path: "status.json")
        guard let data = try? Data(contentsOf: url) else { return nil }
        return decode(data)
    }

    nonisolated static func decode(_ data: Data) -> SyncStatus? {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .custom { dec in
            let s = try dec.singleValueContainer().decode(String.self)
            // Go writes up to nine digits of a second, which ISO8601DateFormatter does not read;
            // the whole second is enough here.
            let whole = s.replacing(/\.\d+/, with: "", maxReplacements: 1)
            let f = ISO8601DateFormatter()
            f.formatOptions = [.withInternetDateTime]
            if let date = f.date(from: whole) { return date }
            throw DecodingError.dataCorrupted(.init(codingPath: dec.codingPath, debugDescription: "not a time: \(s)"))
        }
        return try? d.decode(SyncStatus.self, from: data)
    }
}

/// Badge is a state as the person reads it: a glyph, a word and the token that colours them.
struct Badge: Equatable, Sendable {
    enum Tone: Sendable { case ok, accent, muted, warn, alert, faint }
    let glyph: String
    let word: String
    let tone: Tone
    var detail: String = ""

    /// of is the sidebar's sync badge, as the terminal browser shows it.
    nonisolated static func of(_ st: SyncStatus?, syncing: Bool, now: Date) -> Badge {
        if syncing { return Badge(glyph: "◐", word: "syncing", tone: .accent) }
        guard let st else { return Badge(glyph: "·", word: "not synced yet", tone: .faint) }
        var b: Badge
        switch st.state {
        case "synced": b = Badge(glyph: "●", word: "synced", tone: .ok)
        case "local": b = Badge(glyph: "●", word: "local", tone: .ok)
        case "unpushed": b = Badge(glyph: "↑\(st.ahead)", word: "unpushed", tone: .muted)
        case "offline": b = Badge(glyph: "◌", word: "offline", tone: .warn)
        case "conflict":
            let n = st.conflicts?.count ?? 0
            b = Badge(glyph: "◆", word: n == 1 ? "1 conflict" : "\(n) conflicts", tone: .alert)
        default: b = Badge(glyph: "◆", word: "sync error", tone: .alert)
        }
        let age = now.timeIntervalSince(st.at)
        if age > 600, st.state != "error" { b.detail = ago(age) }
        if let m = st.message, !m.isEmpty { b.detail = b.detail.isEmpty ? m : b.detail + " · " + m }
        return b
    }

    nonisolated static func ago(_ seconds: TimeInterval) -> String {
        switch seconds {
        case ..<3600: "\(Int(seconds / 60))m ago"
        case ..<(48 * 3600): "\(Int(seconds / 3600))h ago"
        default: "\(Int(seconds / 86400))d ago"
        }
    }
}
