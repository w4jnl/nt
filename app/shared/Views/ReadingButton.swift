// ReadingButton is the note's switch between reading and writing, in the bar above it: a book
// while the note is for reading, a pencil while it is for writing. A digest, which nt writes, is
// always for reading. On the Mac the View menu's Reading Mode carries ⌘E.
import SwiftUI

struct ReadingButton: View {
    @Environment(Workspace.self) private var ws

    var body: some View {
        let fixed = ws.note?.readOnly ?? true
        let reading = fixed || ws.reading
        Button {
            ws.toggleReading()
        } label: {
            Image(systemName: reading ? "book" : "pencil")
        }
        .disabled(fixed)
        #if os(macOS)
        .help(fixed ? "A digest: nt writes it" : reading ? "Reading: click to write  ⌘E" : "Writing: click to read  ⌘E")
        #else
        .keyboardShortcut("e")
        #endif
        .accessibilityLabel(reading ? "Reading" : "Writing")
        .accessibilityHint(fixed ? "" : reading ? "Writes in the note" : "Reads the note")
    }
}
