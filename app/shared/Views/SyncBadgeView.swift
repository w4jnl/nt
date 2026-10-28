import SwiftUI

/// SyncBadgeView is the sync state as glyph and word; a click syncs now.
struct SyncBadgeView: View {
    @Environment(Workspace.self) private var ws
    @State private var hover = false
    private static let spinner = ["◐", "◓", "◑", "◒"]

    var body: some View {
        TimelineView(.periodic(from: .now, by: ws.syncing ? 0.15 : 30)) { ctx in
            let b = Badge.of(ws.status, syncing: ws.syncing, now: ctx.date)
            let frame = Int(ctx.date.timeIntervalSinceReferenceDate / 0.15) % Self.spinner.count
            Button {
                Task { await ws.syncNow() }
            } label: {
                HStack(spacing: 6) {
                    Text(ws.syncing ? Self.spinner[frame] : b.glyph)
                        .font(.system(size: 10))
                        .foregroundStyle(Brand.tone(b.tone))
                    Text(b.word)
                        .font(.system(size: 12))
                        .foregroundStyle(Brand.muted)
                }
                .padding(.horizontal, 6)
                .frame(height: 24)
                .background(hover ? Brand.sunk : .clear, in: RoundedRectangle(cornerRadius: 6))
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .onHover { hover = $0 }
            .help(b.detail.isEmpty ? "Sync now  ⌥⌘S" : b.detail + ". Click to sync now  ⌥⌘S")
            .accessibilityLabel("Sync state: \(b.word). Sync now")
        }
    }
}

extension Brand {
    static func tone(_ t: Badge.Tone) -> Color {
        switch t {
        case .ok: ok
        case .accent: accent
        case .muted: muted
        case .warn: warn
        case .alert: alert
        case .faint: faint
        }
    }
}
