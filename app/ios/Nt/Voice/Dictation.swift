// Dictation is the keyboard bar's mic (docs/design.md §9, Voice): it listens in the language
// last chosen and puts each settled piece in at the cursor, the words the model may still change
// shown faintly there. Letting the keyboard go, or the app, stops it, the words heard kept.
import Foundation

extension Workspace {
    /// dictate starts listening at the cursor, or stops.
    func dictate() async {
        let listener = dictation
        if listener.busy { return await listener.stop() }
        listener.lines = false
        listener.words = { [weak self] w in self?.editor?.dictate(w) }
        listener.changed = { [weak self] w in self?.editor?.hearing(w) }
        await listener.start(Language.remembered)
        if case .failed(let why) = listener.state { say(why) }
    }

    /// dictate in language: the mic's menu chose it, and it is remembered. Listening goes on in it.
    func dictate(in language: Language) async {
        Language.remembered = language
        if dictation.busy { await dictation.stop() }
        await dictate()
    }

    /// stopDictating stops listening, keeping the words heard.
    func stopDictating() async {
        await dictation.stop()
    }

    /// keyboardWent stops listening once the note has let the keyboard go; a moment's loss of
    /// focus, as a bar key's tap can make, does not.
    func keyboardWent() {
        guard dictation.busy else { return }
        Task {
            try? await Task.sleep(for: .milliseconds(600))
            if !editing { await stopDictating() }
        }
    }
}
