import SwiftUI

/// MenuClock is the time the menu bar reads: the minute, for the age in the sync line, and the
/// 1150 ms beat of the alert mark. It ticks on its own, and a view redraws only when the value it
/// read changes. A TimelineView must not drive a menu bar item: the item's host renders it, the
/// timeline asks for its next update, the host renders again at once, and so on until the stack
/// overflows (crash reports 2026-10-31 and 2026-11-02, with .periodic and .everyMinute alike).
@MainActor @Observable
final class MenuClock {
    private(set) var minute = Date()
    private(set) var on = true

    init() {
        Task { [weak self] in
            while let self {
                try? await Task.sleep(for: .seconds(1.15))
                self.on.toggle()
                let m = Date(timeIntervalSinceReferenceDate: (Date().timeIntervalSinceReferenceDate / 60).rounded(.down) * 60)
                if m != self.minute { self.minute = m }
            }
        }
    }
}

/// MenuBarLabel is nt's mark in the menu bar: the template mark, and while sync needs you (a
/// conflict, an error; offline is not one) the solid alert mark, alternating with it every
/// 1150 ms until the app is in front.
struct MenuBarLabel: View {
    @Environment(Workspace.self) private var ws
    @Environment(MenuClock.self) private var clock

    var body: some View {
        Image(nsImage: MarkImage.mark(alert: ws.needsYou && (ws.active || clock.on)))
    }
}

/// MenuBarMenu is the menu under the mark: the sync state, adding a line or a todo, the notes
/// opened last, Sync now, the window, Settings and Quit.
struct MenuBarMenu: View {
    @Environment(Workspace.self) private var ws
    @Environment(Capture.self) private var capture
    @Environment(CaptureKey.self) private var key
    @Environment(MenuClock.self) private var clock
    @Environment(\.openWindow) private var openWindow
    @Environment(\.openSettings) private var openSettings

    var body: some View {
        let b = Badge.of(ws.status, syncing: ws.syncing, now: clock.minute)
        Text(b.detail.isEmpty ? "\(b.glyph) \(b.word)" : "\(b.glyph) \(b.word) · \(b.detail)")
        Divider()
        add
        Button("New todo…") { capture.open(.todo) }
        let recent = ws.recentNotes(3)
        if !recent.isEmpty {
            Divider()
            Section("Recent") {
                ForEach(recent, id: \.self) { path in
                    Button {
                        bringForward()
                        ws.show(path)
                    } label: {
                        Text(ws.title(path))
                        Text(folder(path))
                    }
                }
            }
        }
        Divider()
        Button("Sync now") { Task { await ws.syncNow() } }
            .disabled(ws.syncing)
        Button("Open nt") { bringForward() }
        Button("Settings…") {
            NSApp.activate()
            openSettings()
        }
        .keyboardShortcut(",")
        Divider()
        Button("Quit nt") { NSApp.terminate(nil) }
            .keyboardShortcut("q")
    }

    /// add is Add to today, with the capture shortcut beside it when a menu can show it.
    @ViewBuilder
    private var add: some View {
        let s = key.shortcut
        if let k = s.keyEquivalent {
            Button("Add to today…") { capture.open(.today) }
                .keyboardShortcut(k, modifiers: EventModifiers(s))
        } else {
            Button("Add to today…") { capture.open(.today) }
        }
    }

    /// folder is the top folder a note is in, as the menu names it beside the title.
    private func folder(_ path: String) -> String {
        let dir = (path as NSString).deletingLastPathComponent
        return dir.isEmpty ? "/" : String(dir.split(separator: "/").first ?? "")
    }

    private func bringForward() {
        NSApp.activate()
        openWindow(id: "main")
    }
}

extension EventModifiers {
    init(_ s: Shortcut) {
        let m = NSEvent.ModifierFlags(rawValue: s.flags)
        self = []
        if m.contains(.command) { insert(.command) }
        if m.contains(.option) { insert(.option) }
        if m.contains(.control) { insert(.control) }
        if m.contains(.shift) { insert(.shift) }
    }
}
