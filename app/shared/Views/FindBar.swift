import SwiftUI

/// FindBar finds in the note on screen: the words typed are marked as a search's are and the
/// note goes to the first hit from the cursor; Return the next, ⇧Return the one before, Escape
/// closes the bar and takes the marks off. Above the note: on the Mac from Edit › Find in Note
/// (⌘F), on the phone from the note's menu.
struct FindBar: View {
    @Environment(Workspace.self) private var ws
    @FocusState private var focused: Bool

    var body: some View {
        @Bindable var ws = ws
        HStack(spacing: 8) {
            Image(systemName: "magnifyingglass")
                .font(.system(size: 12))
                .foregroundStyle(Brand.muted)
            TextField("Find in note", text: $ws.findText)
                .textFieldStyle(.plain)
                .focused($focused)
                .onSubmit { ws.findStep(1) }
                .onChange(of: ws.findText) { ws.findInNote(ws.findText) }
                .accessibilityLabel("Find in note")
            Text(ws.findCount)
                .font(Brand.mono(11))
                .foregroundStyle(Brand.muted)
            Button { ws.findStep(-1) } label: { Image(systemName: "chevron.up") }
                .accessibilityLabel("The hit before")
            Button { ws.findStep(1) } label: { Image(systemName: "chevron.down") }
                .accessibilityLabel("The next hit")
            Button { ws.closeFind() } label: { Image(systemName: "xmark") }
                .accessibilityLabel("Done")
        }
        .buttonStyle(.plain)
        .font(.system(size: 13))
        .foregroundStyle(Brand.ink)
        .padding(.horizontal, 10)
        .frame(height: 28)
        .background(Brand.sunk, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(Brand.line))
        .padding(.horizontal, 18)
        .padding(.top, 8)
        .onAppear { focused = true }
        .onChange(of: ws.findFocus) { focused = true }
        .onKeyPress(.escape) {
            ws.closeFind()
            return .handled
        }
        .onKeyPress(keys: [.return]) { press in
            guard press.modifiers.contains(.shift) else { return .ignored }
            ws.findStep(-1)
            return .handled
        }
    }
}
