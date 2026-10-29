import SwiftUI

/// SettingRow is one setting: a switch for true or false, a menu for a word from a list, a field
/// for the rest, which nt checks when Return is pressed or the field is left.
struct SettingRow: View {
    let setting: ConfigSetting
    @Environment(Workspace.self) private var ws
    @State private var draft = ""
    @State private var problem = ""
    @FocusState private var editing: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            LabeledContent {
                control
            } label: {
                Text(setting.key)
                    .font(Brand.mono(12))
                    .foregroundStyle(Brand.ink)
            }
            Text(setting.help)
                .font(.system(size: 11))
                .foregroundStyle(Brand.muted)
                .fixedSize(horizontal: false, vertical: true)
            if !problem.isEmpty {
                Text("◆ \(problem)")
                    .font(.system(size: 11))
                    .foregroundStyle(Brand.alert)
            }
        }
        .onAppear { draft = setting.value }
        .onChange(of: setting.value) { draft = setting.value }
    }

    @ViewBuilder
    private var control: some View {
        switch setting.kind {
        case "bool":
            Toggle(setting.key, isOn: Binding(get: { setting.value == "true" }, set: { set($0 ? "true" : "false") }))
                .labelsHidden()
                .toggleStyle(.switch)
                #if os(macOS)
                .controlSize(.small)
                #endif
        case "enum":
            Picker(setting.key, selection: Binding(get: { setting.value }, set: { set($0) })) {
                ForEach(setting.choices ?? [], id: \.self) { Text($0.isEmpty ? "none" : $0).tag($0) }
            }
            .labelsHidden()
            .fixedSize()
        default:
            TextField(setting.key, text: $draft, prompt: Text(placeholder))
                .labelsHidden()
                .multilineTextAlignment(.trailing)
                .truncationMode(.middle)
                .font(Brand.mono(12))
                .frame(maxWidth: 280)
                .focused($editing)
                .onSubmit { commit() }
                .onChange(of: editing) { if !editing { commit() } }
        }
    }

    private var placeholder: String {
        switch setting.kind {
        case "time": "HH:MM"
        case "duration": "2m"
        case "list": "a, b, c"
        default: "empty"
        }
    }

    private func commit() {
        guard draft != setting.value else { return }
        set(draft)
    }

    private func set(_ value: String) {
        Task {
            problem = await ws.setSetting(setting, to: value) ?? ""
            if !problem.isEmpty { draft = setting.value }
        }
    }
}
