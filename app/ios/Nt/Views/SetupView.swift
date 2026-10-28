// The first run: the repository's address and this device's name, the phone's key with where to
// add it on GitHub, and the clone.
import SwiftUI
import UIKit

struct SetupView: View {
    @Environment(Phone.self) private var phone

    var body: some View {
        @Bindable var phone = phone
        NavigationStack {
            Form {
                Section {
                    TextField("git@github.com:you/notes.git", text: $phone.remote)
                        .keyboardType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .font(.body.monospaced())
                } header: {
                    Text("Your notes")
                } footer: {
                    Text("The notes repository's ssh address.")
                }
                Section {
                    TextField("iphone", text: $phone.host)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .font(.body.monospaced())
                } header: {
                    Text("This device")
                } footer: {
                    Text(phone.hostOK ? "The host in commits, conflict copies and reports." : "Lowercase letters, digits and hyphens.")
                }
                Section {
                    if phone.key == nil {
                        Button("Make a key") { phone.makeKey() }
                    } else {
                        Text(phone.publicKey)
                            .font(.caption.monospaced())
                            .textSelection(.enabled)
                        Button("Copy") { UIPasteboard.general.string = phone.publicKey }
                    }
                } header: {
                    Text("This phone's key")
                } footer: {
                    if phone.key != nil {
                        Text("On GitHub: the notes repository › Settings › Deploy keys › Add deploy key. Paste the key, tick Allow write access, add it.")
                    } else {
                        Text("An ssh key made on this phone; its private half stays in the Keychain.")
                    }
                }
                Section {
                    Button {
                        Task { await phone.clone() }
                    } label: {
                        HStack {
                            Text(phone.cloning ? "Cloning" : "Clone")
                            if phone.cloning { Spacer(); ProgressView() }
                        }
                    }
                    .disabled(phone.cloning || phone.key == nil || !phone.hostOK || phone.remote.isEmpty)
                } footer: {
                    if let p = phone.problem { Text(p) }
                }
            }
            .navigationTitle("nt")
        }
    }
}
