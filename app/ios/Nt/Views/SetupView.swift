// The first run (docs/design.md §9): the repository's address and this device's name, the
// phone's key, made here, with Copy and where to add it on GitHub, and the clone.
import SwiftUI
import UIKit

struct SetupView: View {
    @Environment(Phone.self) private var phone
    @State private var copied = false

    var body: some View {
        @Bindable var phone = phone
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                HStack(spacing: 12) {
                    Mark(size: 40)
                    VStack(alignment: .leading, spacing: 2) {
                        Text("Clone your notes")
                            .font(.system(size: 28, weight: .bold))
                        Text("The repository nt keeps them in, on GitHub")
                            .foregroundStyle(Brand.muted)
                    }
                }
                VStack(alignment: .leading, spacing: 6) {
                    label("Repository")
                    field("git@github.com:you/notes.git", text: $phone.remote)
                        .keyboardType(.URL)
                    label("This device's name, in commits and reports")
                        .padding(.top, 6)
                    field("iphone", text: $phone.host)
                    if !phone.hostOK {
                        Text("Lowercase letters, digits and hyphens.")
                            .font(.footnote)
                            .foregroundStyle(Brand.warn)
                    }
                }
                VStack(alignment: .leading, spacing: 10) {
                    HStack {
                        Text("This phone's key").fontWeight(.semibold)
                        Spacer()
                        Button {
                            UIPasteboard.general.string = phone.publicKey
                            copied = true
                        } label: {
                            Label(copied ? "Copied" : "Copy", systemImage: "doc.on.doc")
                                .fontWeight(.semibold)
                        }
                        .disabled(phone.key == nil)
                    }
                    Text(phone.key == nil ? "Making the key…" : phone.publicKey)
                        .font(Brand.mono(12))
                        .textSelection(.enabled)
                        .padding(10)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(Brand.sunk, in: RoundedRectangle(cornerRadius: 8))
                    Text("Made on this phone; its private half never leaves it.")
                        .font(.footnote)
                        .foregroundStyle(Brand.muted)
                }
                .padding(14)
                .background(Brand.surface, in: RoundedRectangle(cornerRadius: 12))
                .overlay(RoundedRectangle(cornerRadius: 12).stroke(Brand.line))
                VStack(alignment: .leading, spacing: 8) {
                    step(1, "On GitHub, open the repository's **Settings › Deploy keys** and choose **Add deploy key**.")
                    step(2, "Paste the key, name it \(phone.host), and tick **Allow write access**.")
                    step(3, "Come back here and clone.")
                }
                Button {
                    Task { await phone.clone() }
                } label: {
                    HStack(spacing: 10) {
                        if phone.cloning { ProgressView().tint(Brand.ground) }
                        Text(phone.cloning ? "Cloning" : "Clone").fontWeight(.semibold)
                    }
                    .foregroundStyle(Brand.ground)
                    .frame(maxWidth: .infinity, minHeight: 50)
                    .background(Brand.accent, in: RoundedRectangle(cornerRadius: 12))
                }
                .disabled(phone.cloning || phone.key == nil || !phone.hostOK || phone.remote.isEmpty)
                .opacity(phone.remote.isEmpty || !phone.hostOK ? 0.5 : 1)
                if let p = phone.problem {
                    Text("◆ \(p)")
                        .font(.footnote)
                        .foregroundStyle(Brand.alert)
                        .textSelection(.enabled)
                }
                Text("nt talks to nothing but your repository, over ssh.")
                    .font(.footnote)
                    .foregroundStyle(Brand.muted)
                    .frame(maxWidth: .infinity)
            }
            .padding(20)
            .frame(maxWidth: 560)
            .frame(maxWidth: .infinity)
        }
        .foregroundStyle(Brand.ink)
        .background(Brand.ground)
        .task { if phone.key == nil { phone.makeKey() } }
    }

    private func label(_ text: String) -> some View {
        Text(text)
            .font(.footnote.weight(.semibold))
            .foregroundStyle(Brand.muted)
    }

    private func field(_ prompt: String, text: Binding<String>) -> some View {
        TextField(prompt, text: text)
            .font(Brand.mono(14))
            .textInputAutocapitalization(.never)
            .autocorrectionDisabled()
            .padding(.horizontal, 12)
            .frame(height: 44)
            .background(Brand.surface, in: RoundedRectangle(cornerRadius: 10))
            .overlay(RoundedRectangle(cornerRadius: 10).stroke(Brand.line))
    }

    private func step(_ n: Int, _ text: LocalizedStringKey) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Text("\(n).").foregroundStyle(Brand.muted)
            Text(text)
        }
        .font(.subheadline)
    }
}
