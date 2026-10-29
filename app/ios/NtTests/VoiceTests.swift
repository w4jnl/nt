// Voice: where a pause ends a line, and speech in English, Dutch and Slovak turned into words
// by the model the device has for each, from sound macOS spoke (make ios-voice writes it with say
// into build/voice). The simulator has no speech models, so that runs on a device, or on the Mac
// with make ios-voice.
import Foundation
import Testing
@testable import Nt

struct VoiceTests {
    let t0 = Date(timeIntervalSinceReferenceDate: 0)

    @Test func aPauseEndsALineOnceTheWordsAreSettled() {
        var h = Heard()
        h.got("called the", final: false, at: t0)
        #expect(h.line(at: t0 + 3, pause: 1.5) == nil) // still unsettled
        #expect(h.unsettled(at: t0 + 3, pause: 1.5))
        h.got("Called the ISP", final: true, at: t0 + 3)
        h.got("about the", final: false, at: t0 + 3.2)
        #expect(h.line(at: t0 + 3.4, pause: 1.5) == nil)
        h.got("about the static IP.", final: true, at: t0 + 4)
        #expect(h.line(at: t0 + 5, pause: 1.5) == nil) // no pause yet
        #expect(h.line(at: t0 + 5.6, pause: 1.5) == "Called the ISP about the static IP.")
        #expect(h.line(at: t0 + 9, pause: 1.5) == nil) // taken
    }

    @Test func wordsJoinWithOneSpaceAndNoneBeforePunctuation() {
        #expect(Heard.join("", "  Hello  there ") == "Hello there")
        #expect(Heard.join("Hello", "there") == "Hello there")
        #expect(Heard.join("Hello", ", there") == "Hello, there")
        #expect(Heard.join("Hello", "") == "Hello")
        var h = Heard()
        h.got("Five euro", final: true, at: t0)
        h.got("a month", final: false, at: t0)
        #expect(h.rest() == "Five euro a month")
        #expect(h == Heard())
    }

    @Test func englishIsAskedInAnEnglishLocaleAModelHas() {
        for l in Language.allCases {
            #expect(l.locales.last?.language.languageCode?.identifier == l.rawValue)
        }
    }

    @Test(.enabled(if: !simulator, "the simulator has no speech models: make ios-voice"), arguments: [
        (Language.english, ["serial number", "router"]),
        (Language.dutch, ["serienummer", "provider"]),
        (Language.slovak, ["číslo", "adrese"]),
    ])
    @MainActor func speechBecomesWords(language: Language, expected: [String]) async throws {
        let sample = URL(filePath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
            .appending(path: "build/voice/\(language.rawValue).aiff")
        try #require(FileManager.default.fileExists(atPath: sample.path), "make ios-voice writes \(sample.path)")
        let module = await Listener.module(language)
        try #require(module != nil, "no model for \(language.name)")
        let listener = Listener()
        var words: [String] = []
        listener.words = { words.append($0) }
        await listener.play(sample, language)
        if case .failed(let why) = listener.state { Issue.record("\(language.name): \(why)") }
        let text = words.joined(separator: " ").lowercased()
        print("\(language.name) (\(type(of: module!))): \(text)")
        for w in expected {
            #expect(text.contains(w), "\(language.name): \(text)")
        }
    }
}

#if targetEnvironment(simulator)
let simulator = true
#else
let simulator = false
#endif
