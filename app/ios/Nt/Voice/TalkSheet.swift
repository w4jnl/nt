// TalkSheet is "Talk a note" (docs/design.md §9, Voice): it listens until stopped, and each pause
// adds what was said as a line to today's journal, under Log, as nt a does. The switch says what
// language is spoken; only the words are kept, never the sound. Done adds the last words, Cancel
// drops them; the lines added stay.
import SwiftUI

struct TalkSheet: View {
    @Environment(Workspace.self) private var ws
    @Environment(\.dismiss) private var dismiss
    @Environment(\.scenePhase) private var phase
    @State private var talk: Talk
    @State private var language = Language.remembered

    init(workspace: Workspace) {
        _talk = State(initialValue: Talk(workspace: workspace))
    }

    private var listener: Listener { talk.listener }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                Picker("Language", selection: $language) {
                    ForEach(Language.allCases) { Text($0.name).tag($0) }
                }
                .pickerStyle(.segmented)
                .padding(.horizontal, 16)
                .padding(.top, 4)
                .padding(.bottom, 18)
                if !talk.added.isEmpty {
                    added
                }
                ScrollView {
                    words
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(.horizontal, 20)
                        .padding(.top, 22)
                }
                .defaultScrollAnchor(.bottom, for: .sizeChanges) // the newest words stay in view
                Bars(levels: listener.levels, on: listener.state == .listening)
                    .frame(height: 48)
                clock
                    .padding(.top, 6)
                    .padding(.bottom, 14)
                button
                    .padding(.bottom, 14)
                Text("A pause adds a line to today's Log. Only the words are kept, never the sound.")
                    .font(.footnote)
                    .foregroundStyle(Brand.muted)
                    .multilineTextAlignment(.center)
                    .padding(.horizontal, 28)
                    .padding(.bottom, 10)
                Text("\(ws.journalPath(ws.today)) · Log · on this \(UIDevice.current.model), offline")
                    .font(Brand.mono(11))
                    .foregroundStyle(Brand.muted)
                    .lineLimit(1)
                    .truncationMode(.head)
                    .padding(.horizontal, 20)
                    .padding(.bottom, 16)
            }
            .background(Brand.ground)
            .navigationTitle("Talk a note")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") {
                        Task {
                            await talk.cancel()
                            dismiss()
                        }
                    }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") {
                        Task {
                            await talk.finish()
                            dismiss()
                        }
                    }
                    .fontWeight(.semibold)
                }
            }
        }
        .interactiveDismissDisabled()
        .task { await listener.start(language) }
        .onChange(of: language) {
            Language.remembered = language
            guard listener.busy else { return }
            Task {
                await listener.stop()
                await listener.start(language)
            }
        }
        .onChange(of: phase) {
            if phase == .background { Task { await listener.stop() } }
        }
    }

    /// added is the lines this talk put in today's journal, the last few.
    private var added: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Added to today")
                .font(.caption.weight(.semibold))
                .foregroundStyle(Brand.muted)
            ForEach(talk.added.suffix(4)) { line in
                HStack(alignment: .firstTextBaseline, spacing: 10) {
                    Text(line.time)
                        .font(Brand.mono(12))
                        .foregroundStyle(Brand.muted)
                        .frame(width: 44, alignment: .leading)
                    Text(line.text)
                        .font(.system(size: 15))
                        .foregroundStyle(line.failed ? Brand.alert : Brand.ink)
                        .lineLimit(2)
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 20)
    }

    /// words is what is being said: the settled words, then faintly the ones that may still
    /// change; or what it waits for, or what went wrong.
    @ViewBuilder
    private var words: some View {
        switch listener.state {
        case .preparing(let what):
            HStack(spacing: 10) {
                ProgressView()
                if !what.isEmpty { Text(what).foregroundStyle(Brand.muted) }
            }
            .font(.body)
        case .failed(let why):
            Text("◆ \(why)")
                .font(.body)
                .foregroundStyle(Brand.alert)
        default:
            if !talk.problem.isEmpty {
                Text("◆ \(talk.problem)")
                    .font(.body)
                    .foregroundStyle(Brand.alert)
            }
            Text("\(Text(listener.heard.settled))\(Text(listener.heard.settled.isEmpty || listener.heard.volatile.isEmpty ? "" : " "))\(Text(listener.heard.volatile).foregroundStyle(Brand.faint))")
                .font(.system(size: 24, weight: .medium))
                .lineSpacing(8)
                .foregroundStyle(Brand.ink)
        }
    }

    /// clock is how long it has listened.
    private var clock: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            Text(listener.started.map { Self.elapsed(context.date.timeIntervalSince($0)) } ?? " ")
                .font(Brand.mono(13))
                .foregroundStyle(Brand.muted)
        }
    }

    /// button stops listening, the last words added; stopped, it listens again.
    private var button: some View {
        Button {
            Task {
                if listener.busy { await listener.stop() } else { await listener.start(language) }
            }
        } label: {
            ZStack {
                Circle().strokeBorder(Brand.line, lineWidth: 3)
                switch listener.state {
                case .listening:
                    RoundedRectangle(cornerRadius: 6).fill(Brand.alert).frame(width: 26, height: 26)
                case .preparing:
                    ProgressView()
                default:
                    Image(systemName: "mic.fill").font(.system(size: 26)).foregroundStyle(Brand.accent)
                }
            }
            .frame(width: 72, height: 72)
        }
        .accessibilityLabel(listener.busy ? "Stop" : "Listen")
    }

    nonisolated static func elapsed(_ s: TimeInterval) -> String {
        let n = max(Int(s), 0)
        return "\(n / 60):" + String(format: "%02d", n % 60)
    }
}

/// Bars is the loudness of the last moments, newest at the right.
private struct Bars: View {
    let levels: [Float]
    let on: Bool

    var body: some View {
        HStack(spacing: 4) {
            ForEach(levels.indices, id: \.self) { i in
                Capsule()
                    .fill(on ? Brand.accent : Brand.line)
                    .frame(width: 4, height: 6 + CGFloat(levels[i]) * 38)
            }
        }
        .animation(.easeOut(duration: 0.12), value: levels)
        .accessibilityHidden(true)
    }
}

/// Talk is a talk's lines: each line the listener gives goes to today's journal through nt a,
/// one after the other.
@MainActor
@Observable
final class Talk {
    struct Line: Identifiable {
        let id = UUID()
        let time: String
        let text: String
        var failed = false
    }

    let listener = Listener()
    private(set) var added: [Line] = []
    private(set) var problem = ""

    @ObservationIgnored private unowned let ws: Workspace
    @ObservationIgnored private let lines: AsyncStream<String>.Continuation
    @ObservationIgnored private var writing: Task<Void, Never>?

    init(workspace: Workspace) {
        ws = workspace
        let (stream, lines) = AsyncStream.makeStream(of: String.self)
        self.lines = lines
        listener.lines = true
        listener.words = { lines.yield($0) }
        writing = Task { [weak self] in
            for await line in stream { await self?.add(line) }
        }
    }

    /// finish stops listening, adds the last words, and waits until every line is in.
    func finish() async {
        await listener.stop()
        lines.finish()
        await writing?.value
    }

    /// cancel stops listening and drops the words not added yet.
    func cancel() async {
        await listener.cancel()
        lines.finish()
        await writing?.value
    }

    private func add(_ line: String) async {
        guard let nt = ws.nt else { return }
        struct Made: Decodable { let rel: String }
        do {
            _ = try await nt.json(Made.self, ["a", "--json", "--", line])
            added.append(Line(time: Capture.clock(.now), text: line))
            problem = ""
            await ws.wrote()
        } catch {
            added.append(Line(time: Capture.clock(.now), text: line, failed: true))
            problem = "Not added: \(error.localizedDescription)"
        }
    }
}
