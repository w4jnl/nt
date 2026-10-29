// CaptureSheet adds one line to today's journal, under Log, or with the switch a todo to todo.md;
// due:fri in the text sets the todo's date (docs/design.md §9, Capture). Return adds it. The mic
// talks instead (Talk a note), in the language chosen beside it; what is typed is added first.
import SwiftUI

struct CaptureSheet: View {
    @Environment(Workspace.self) private var ws
    @Environment(\.dismiss) private var dismiss
    @State private var capture: Capture
    @State private var language = Language.remembered
    @FocusState private var focused: Bool
    let talk: () -> Void

    init(workspace: Workspace, talk: @escaping () -> Void) {
        _capture = State(initialValue: Capture(workspace: workspace))
        self.talk = talk
    }

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 14) {
                TextField(capture.mode == .today ? "Add to today" : "New todo", text: $capture.text, axis: .vertical)
                    .font(.system(size: 19))
                    .lineLimit(2...6)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 12)
                    .frame(minHeight: 72, alignment: .topLeading)
                    .background(Brand.sunk, in: RoundedRectangle(cornerRadius: 10))
                    .focused($focused)
                    .onChange(of: capture.text) {
                        // a line has no line breaks: Return adds it
                        if capture.text.contains("\n") {
                            capture.text = capture.text.replacingOccurrences(of: "\n", with: "")
                            add()
                        }
                    }
                    .submitLabel(.done)
                HStack(spacing: 8) {
                    Picker("Language", selection: $language) {
                        ForEach(Language.allCases) { Text($0.short).tag($0) }
                    }
                    .pickerStyle(.segmented)
                    .fixedSize()
                    .onChange(of: language) { Language.remembered = language }
                    Spacer()
                    Button(action: talkInstead) {
                        Image(systemName: "mic")
                            .font(.system(size: 19, weight: .medium))
                            .foregroundStyle(Brand.surface)
                            .frame(width: 44, height: 44)
                            .background(Brand.accent, in: Circle())
                    }
                    .disabled(capture.busy)
                    .accessibilityLabel("Talk instead of typing")
                }
                Toggle("Make it a todo", isOn: Binding(get: { capture.mode == .todo }, set: { capture.mode = $0 ? .todo : .today }))
                    .tint(Brand.accent)
                if capture.problem.isEmpty {
                    Text("\(capture.target) · due:fri in the text sets a todo's date")
                        .font(Brand.mono(11))
                        .foregroundStyle(Brand.muted)
                } else {
                    Text("◆ \(capture.problem)")
                        .font(.footnote)
                        .foregroundStyle(Brand.alert)
                }
                Spacer(minLength: 0)
            }
            .padding(16)
            .background(Brand.surface)
            .navigationTitle(capture.mode == .today ? "Add to today" : "New todo")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Add", action: add)
                        .disabled(capture.busy || capture.text.trimmingCharacters(in: .whitespaces).isEmpty)
                }
            }
        }
        .presentationDetents([.medium])
        .onAppear { focused = true }
    }

    /// talkInstead adds what is typed, then talks.
    private func talkInstead() {
        Task {
            if !capture.text.trimmingCharacters(in: .whitespaces).isEmpty {
                guard await capture.submit(alternate: false) else { return }
            }
            talk()
        }
    }

    private func add() {
        Task {
            guard await capture.submit(alternate: false) else { return }
            if let a = capture.added { ws.say(a.said) }
            dismiss()
        }
    }
}
