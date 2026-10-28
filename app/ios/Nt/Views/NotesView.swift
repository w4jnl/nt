// The notes, newest first, with a field that adds a line to today and the sync state at the
// foot, as glyph and word; a tap there, or pulling the list, syncs.
import SwiftUI

struct NotesView: View {
    @Environment(Phone.self) private var phone
    @State private var line = ""

    var body: some View {
        NavigationStack {
            List {
                Section {
                    TextField("Add to today", text: $line)
                        .submitLabel(.done)
                        .onSubmit {
                            let l = line
                            line = ""
                            Task { await phone.add(l) }
                        }
                }
                Section {
                    ForEach(phone.notes) { note in
                        NavigationLink(value: note) {
                            VStack(alignment: .leading) {
                                Text(note.title)
                                Text(note.path).font(.caption.monospaced()).foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
            .navigationTitle("nt")
            .navigationDestination(for: Phone.Note.self) { NoteText(note: $0) }
            .refreshable { await phone.sync() }
            .safeAreaInset(edge: .bottom) { SyncBar() }
            .task {
                do {
                    try await phone.open()
                } catch {
                    phone.problem = error.localizedDescription
                }
                await phone.reload()
                await phone.sync()
            }
        }
    }
}

/// SyncBar is the sync state, as the Mac sidebar's foot shows it; a tap syncs now.
struct SyncBar: View {
    @Environment(Phone.self) private var phone

    var body: some View {
        let b = Badge.of(phone.status, syncing: phone.syncing, now: .now)
        Button {
            Task { await phone.sync() }
        } label: {
            VStack(alignment: .leading, spacing: 2) {
                Text("\(b.glyph) \(b.word)").font(.callout.monospaced())
                if let p = phone.problem ?? (b.detail.isEmpty ? nil : b.detail) {
                    Text(p).font(.caption).foregroundStyle(.secondary).lineLimit(3)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal)
            .padding(.vertical, 8)
        }
        .buttonStyle(.plain)
        .background(.bar)
    }
}

/// NoteText is a note's markdown as written; the editor comes in step 3.
struct NoteText: View {
    @Environment(Phone.self) private var phone
    let note: Phone.Note

    var body: some View {
        ScrollView {
            Text(phone.text(of: note))
                .font(.callout.monospaced())
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding()
        }
        .navigationTitle(note.title)
        .navigationBarTitleDisplayMode(.inline)
    }
}
