// SettingsScreen is the phone's settings (docs/design.md §9, Settings): its name and sync, the
// repository and the phone's key, the settings every machine shares (.nt/config.toml), and the
// note's text size. The machine settings the phone has no use for (the sync timer, digests, the
// summary tool, the theme, images in the terminal) stay on the computers.
import NtCore
import SwiftUI
import UIKit

struct SettingsScreen: View {
    @Environment(Workspace.self) private var ws
    @Environment(Phone.self) private var phone
    @State private var showKey = false

    var body: some View {
        @Bindable var ws = ws
        Form {
            Section("This phone") {
                LabeledContent("Name") {
                    Text(phone.host).font(Brand.mono(14)).foregroundStyle(Brand.muted)
                }
                LabeledContent("Sync") {
                    let b = Badge.of(ws.status, syncing: ws.syncing, now: .now)
                    Text("\(Text(b.glyph).foregroundStyle(Brand.tone(b.tone))) \(b.word)\(b.detail.isEmpty ? "" : " · " + b.detail)")
                        .foregroundStyle(Brand.muted)
                        .lineLimit(2)
                }
                Button("Sync now") { Task { await ws.syncNow() } }
                    .disabled(ws.syncing)
            }
            .listRowBackground(Brand.surface)
            Section("Repository") {
                VStack(alignment: .leading, spacing: 2) {
                    Text("Address")
                    Text(phone.remote.isEmpty ? "origin" : phone.remote)
                        .font(Brand.mono(13))
                        .foregroundStyle(Brand.muted)
                        .textSelection(.enabled)
                }
                LabeledContent("Key") {
                    Text(keySummary).foregroundStyle(Brand.muted)
                }
                Button("Show and copy the key") { showKey = true }
            }
            .listRowBackground(Brand.surface)
            Section {
                let shared = ws.settings.filter { $0.file == "repo" }
                if shared.isEmpty {
                    Text(ws.settingsProblem.isEmpty ? "Reading nt config…" : ws.settingsProblem)
                        .foregroundStyle(Brand.muted)
                }
                ForEach(shared) { SettingRow(setting: $0) }
            } header: {
                Text("Every machine · .nt/config.toml")
            } footer: {
                Text("In the notes repository, shared through sync.")
            }
            .listRowBackground(Brand.surface)
            Section("The note") {
                Picker("Text size", selection: $ws.textSize) {
                    ForEach(TextSize.allCases) { Text($0.name).tag($0) }
                }
            }
            .listRowBackground(Brand.surface)
            Section {
            } footer: {
                Text("nt \(MobileVersion()) · the notes live on this phone and in your repository")
                    .font(Brand.mono(11))
                    .frame(maxWidth: .infinity)
            }
        }
        .scrollContentBackground(.hidden)
        .background(Brand.ground)
        .navigationTitle("Settings")
        .task { await ws.loadSettings() }
        .sheet(isPresented: $showKey) { KeySheet() }
    }

    private var keySummary: String {
        guard phone.key != nil else { return "none" }
        guard let made = phone.keyMade else { return "ecdsa" }
        return "ecdsa · made " + made.formatted(.dateTime.day().month(.abbreviated))
    }
}

/// KeySheet is the phone's public key, to copy into GitHub, and a way to make a new one.
struct KeySheet: View {
    @Environment(Phone.self) private var phone
    @Environment(\.dismiss) private var dismiss
    @State private var confirm = false
    @State private var copied = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text(phone.publicKey)
                        .font(Brand.mono(12))
                        .textSelection(.enabled)
                    Button(copied ? "Copied" : "Copy") {
                        UIPasteboard.general.string = phone.publicKey
                        copied = true
                    }
                } header: {
                    Text("This phone's key")
                } footer: {
                    Text("\(phone.key?.fingerprint ?? ""). On GitHub it is a deploy key of the notes repository, with write access: Settings › Deploy keys.")
                }
                Section {
                    Button("Make a new key", role: .destructive) { confirm = true }
                } footer: {
                    Text("The key in use stops working on this phone. Add the new one on GitHub, and remove the old one there.")
                }
            }
            .navigationTitle("Key")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } }
            }
            .confirmationDialog("Make a new key?", isPresented: $confirm, titleVisibility: .visible) {
                Button("Make a new key", role: .destructive) {
                    phone.makeKey()
                    copied = false
                }
            } message: {
                Text("Sync stops until the new key is added to the repository on GitHub.")
            }
        }
    }
}
