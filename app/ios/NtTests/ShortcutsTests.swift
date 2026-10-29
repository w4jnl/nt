// The Shortcuts' way in (Shortcuts.swift): Phone.add starts nt on the clone when the app has not,
// and writes as the capture sheet does, a line to today's Log or a todo to todo.md's Inbox; with
// no clone yet it says so. In LibGitTests' suite, which runs one at a time: nt in this process
// takes its paths from the environment.
import Foundation
import Testing
@testable import Nt

extension LibGitTests {
    @Test @MainActor func aShortcutAddsALineOrATodoThroughThePhonesNt() async throws {
        let place = Place(base: base.appending(path: "phone"))
        let none = Phone(place: place)
        do {
            _ = try await none.add("Called the ISP", todo: false)
            Issue.record("a phone with no clone added a line")
        } catch {
            #expect(error.localizedDescription.contains("clone them first"))
        }

        try place.configure(host: "iphone")
        try LibGit.create(at: place.notes, bare: false)
        let phone = Phone(place: place)
        let line = try await phone.add("Called the ISP", todo: false)
        #expect(line.section == "Log" && line.said.hasPrefix("Added to ") && line.said.contains(" · Log, at "))
        let journals = try FileManager.default.subpathsOfDirectory(atPath: place.notes.path).filter { $0.hasSuffix(".md") }
        #expect(journals.contains { read(place.notes, $0).contains("Called the ISP") })

        let todo = try await phone.add("Renew the domain due:fri", todo: true)
        #expect(todo.place == "todo.md" && todo.section == "Inbox" && todo.detail.hasPrefix("due "))
        #expect(read(place.notes, "todo.md").contains("Renew the domain"))

        // nothing to add is said, not written
        do {
            _ = try await phone.add("  ", todo: false)
            Issue.record("an empty line was added")
        } catch {
            #expect(error.localizedDescription == "Nothing to add: the text is empty.")
        }
        // the sync that wrote waits half a minute for, now, so none is left for the next test
        await phone.workspace?.settle()
    }
}
