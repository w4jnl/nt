// Fuzzy title matching for Open (⌘P), the same scoring as nt's terminal picker (internal/scan,
// Match): every query character in order, more for word starts and runs.
import Foundation

enum Fuzzy {
    /// match scores query against s and gives the matched characters' offsets, or nil.
    nonisolated static func match(_ query: String, _ s: String) -> (score: Int, at: [Int])? {
        let q = Array(query.lowercased().filter { $0 != " " })
        if q.isEmpty { return (0, []) }
        let r = Array(s)
        let lower = r.map { Character($0.lowercased()) }
        var qi = 0, run = 0, score = 0
        var at: [Int] = []
        var i = 0
        while i < lower.count && qi < q.count {
            defer { i += 1 }
            guard lower[i] == q[qi] else {
                run = 0
                continue
            }
            var pts = 1
            if i == 0 || !(r[i - 1].isLetter || r[i - 1].isNumber) || (r[i].isUppercase && r[i - 1].isLowercase) {
                pts += 8
            }
            run += 1
            pts += 4 * (run - 1)
            score += pts
            at.append(i)
            qi += 1
        }
        guard qi == q.count else { return nil }
        return (score - r.count / 8, at)
    }

    struct Hit: Equatable, Sendable {
        let note: RepoIndex.Note
        let at: [Int] // matched offsets in the title; empty when the path matched
    }

    /// find returns the notes whose title or path matches, best first, at most limit.
    nonisolated static func find(_ query: String, in notes: [RepoIndex.Note], limit: Int) -> [Hit] {
        var hits: [(Hit, Int)] = []
        for n in notes {
            let t = match(query, n.title)
            let p = match(query, n.path)
            guard t != nil || p != nil else { continue }
            let st = (t?.score ?? Int.min / 2) + 4
            let sp = p?.score ?? Int.min / 2
            hits.append((Hit(note: n, at: st >= sp ? t?.at ?? [] : []), max(st, sp)))
        }
        hits.sort { a, b in a.1 != b.1 ? a.1 > b.1 : a.0.note.path > b.0.note.path }
        return hits.prefix(limit).map(\.0)
    }
}
