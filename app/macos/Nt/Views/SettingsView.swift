import AppKit
import SwiftUI

/// SettingsView is Settings (⌘,): nt's own settings, this Mac's and every machine's, written
/// through nt config set; and the app's, kept on this Mac: the capture shortcut and the text size.
struct SettingsView: View {
    var body: some View {
        TabView {
            Tab("nt", systemImage: "gearshape") { NtSettingsView() }
            Tab("App", systemImage: "macwindow") { AppSettingsView() }
        }
        .frame(width: 580)
    }
}

/// NtSettingsView is nt config: a section per file, each setting's key, its control by kind and
/// what it does.
struct NtSettingsView: View {
    @Environment(Workspace.self) private var ws

    var body: some View {
        Form {
            section("This Mac", file: "machine", footer: "~/.config/nt/config.toml, this machine's own")
            section("Every machine", file: "repo", footer: ".nt/config.toml in the notes repository, shared through sync")
        }
        .formStyle(.grouped)
        .frame(height: 560)
        .task { await ws.loadSettings() }
    }

    @ViewBuilder
    private func section(_ title: String, file: String, footer: String) -> some View {
        let list = ws.settings.filter { $0.file == file }
        Section {
            if list.isEmpty {
                Text(ws.settingsProblem.isEmpty ? "Reading nt config…" : ws.settingsProblem)
                    .foregroundStyle(Brand.muted)
            }
            ForEach(list) { SettingRow(setting: $0) }
        } header: {
            Text(title)
        } footer: {
            Text(footer)
                .font(Brand.mono(11))
                .foregroundStyle(Brand.muted)
        }
    }
}

/// AppSettingsView is the app's own settings, kept on this Mac: the shortcut that opens the
/// capture panel, and the note's text size.
struct AppSettingsView: View {
    @Environment(Workspace.self) private var ws
    @Environment(CaptureKey.self) private var key

    var body: some View {
        @Bindable var ws = ws
        Form {
            Section {
                LabeledContent("Add to today") { ShortcutRecorder() }
                if let what = key.taken {
                    VStack(alignment: .leading, spacing: 6) {
                        Text("◆ macOS uses \(key.shortcut.display) for \(what). Turn that off in System Settings › Keyboard › Keyboard Shortcuts › Spotlight, or record another shortcut here.")
                            .font(.system(size: 11))
                            .foregroundStyle(Brand.warn)
                            .fixedSize(horizontal: false, vertical: true)
                        Button("Open Keyboard Shortcuts…") {
                            if let url = URL(string: "x-apple.systempreferences:com.apple.Keyboard-Settings.extension") {
                                NSWorkspace.shared.open(url)
                            }
                        }
                        .controlSize(.small)
                    }
                } else if !key.registered {
                    Text("◆ Another app holds \(key.shortcut.display). Record another shortcut.")
                        .font(.system(size: 11))
                        .foregroundStyle(Brand.alert)
                }
            } header: {
                Text("Capture")
            } footer: {
                Text("Opens a field from anywhere: Return adds the line to today's journal, Shift-Return makes it a todo.")
                    .font(.system(size: 11))
                    .foregroundStyle(Brand.muted)
            }
            Section("Note") {
                Picker("Text size", selection: $ws.textSize) {
                    ForEach(TextSize.allCases) { Text($0.name).tag($0) }
                }
            }
        }
        .formStyle(.grouped)
        .frame(height: 300)
    }
}

/// ShortcutRecorder shows the capture shortcut; a click records the next key pressed with ⌘, ⌥
/// or ⌃ as the new one, esc leaves it as it was.
struct ShortcutRecorder: View {
    @Environment(CaptureKey.self) private var key
    @State private var recording = false
    @State private var monitor: Any?
    @State private var problem = ""

    var body: some View {
        HStack(spacing: 8) {
            if !problem.isEmpty {
                Text(problem)
                    .font(.system(size: 11))
                    .foregroundStyle(Brand.alert)
            }
            if key.shortcut != .standard, !recording {
                Button("Reset") { key.set(.standard) }
                    .controlSize(.small)
            }
            Button(recording ? "Type the shortcut…" : key.shortcut.display) {
                recording ? stop() : start()
            }
            .font(recording ? .system(size: 12) : Brand.mono(12))
            .frame(minWidth: 120)
        }
        .onDisappear { stop() }
    }

    private func start() {
        problem = ""
        recording = true
        key.pause()
        monitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { event in
            if event.keyCode == 53 { // esc
                stop()
                return nil
            }
            guard let s = Shortcut.pressed(keyCode: event.keyCode, flags: event.modifierFlags, characters: event.charactersIgnoringModifiers) else {
                problem = "Use ⌘, ⌥ or ⌃ with the key"
                return nil
            }
            stop()
            if !key.set(s) { problem = "Another app holds \(s.display)" }
            return nil
        }
    }

    private func stop() {
        if let monitor { NSEvent.removeMonitor(monitor) }
        monitor = nil
        if recording { key.resume() }
        recording = false
    }
}
