import AppKit
import SwiftUI

/// CapturePanel is the small floating field the capture shortcut opens, wherever the person is:
/// it takes the keys without bringing nt's window forward, says where the text went, and goes.
/// Esc, or a click elsewhere, closes it.
@MainActor
final class CapturePanel: NSObject, NSWindowDelegate {
    let capture: Capture
    private var panel: KeyPanel?
    private var closing: Task<Void, Never>?

    init(capture: Capture) {
        self.capture = capture
        super.init()
        capture.panel = self
    }

    func show(_ mode: Capture.Mode) {
        closing?.cancel()
        capture.began(mode)
        let p = panel ?? make()
        let screen = NSScreen.screens.first { NSMouseInRect(NSEvent.mouseLocation, $0.frame, false) } ?? NSScreen.main
        if let f = screen?.visibleFrame {
            p.setFrameTopLeftPoint(NSPoint(x: f.midX - p.frame.width / 2, y: f.maxY - f.height * 0.22))
        }
        p.makeKeyAndOrderFront(nil)
    }

    func hide() {
        closing?.cancel()
        panel?.orderOut(nil)
        capture.added = nil
    }

    /// done shows what was added for a moment, then closes the panel.
    func done() {
        closing?.cancel()
        closing = Task {
            try? await Task.sleep(for: .milliseconds(1200))
            if !Task.isCancelled { hide() }
        }
    }

    private func make() -> KeyPanel {
        let p = KeyPanel(contentRect: NSRect(x: 0, y: 0, width: 620, height: 104), styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: true)
        p.isFloatingPanel = true
        p.level = .floating
        p.hidesOnDeactivate = false
        p.isOpaque = false
        p.backgroundColor = .clear
        p.hasShadow = true
        p.isMovableByWindowBackground = true
        p.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary, .transient]
        p.delegate = self
        p.setAccessibilityLabel("Add to today")
        let host = NSHostingController(rootView: CaptureView(capture: capture, panel: self))
        host.sizingOptions = .preferredContentSize
        p.contentViewController = host
        panel = p
        return p
    }

    func windowDidResignKey(_ notification: Notification) {
        if capture.added == nil { hide() } // a click elsewhere
    }
}

/// KeyPanel is a borderless panel that can take the keyboard.
final class KeyPanel: NSPanel {
    override var canBecomeKey: Bool { true }
}

/// CaptureView is the panel: nt's mark and the field, the file the text goes to and the keys
/// under it; once added, where it went.
struct CaptureView: View {
    @Bindable var capture: Capture
    let panel: CapturePanel
    @FocusState private var focused: Bool

    var body: some View {
        VStack(spacing: 0) {
            if let a = capture.added {
                HStack(spacing: 12) {
                    Text("✓")
                        .font(.system(size: 16))
                        .foregroundStyle(Brand.ok)
                    Text("Added to \(Text(a.place).fontWeight(.medium))\(a.detail.isEmpty ? "" : (a.section == "Log" ? " " : ", ") + a.detail)")
                        .font(.system(size: 15))
                        .foregroundStyle(Brand.ink)
                        .frame(maxWidth: .infinity, alignment: .leading)
                    Text(a.section)
                        .font(Brand.mono(11))
                        .foregroundStyle(Brand.muted)
                }
                .frame(height: 56)
                .padding(.horizontal, 20)
                .accessibilityElement(children: .combine)
                .accessibilityAddTraits(.updatesFrequently)
            } else {
                HStack(spacing: 14) {
                    Image(nsImage: MarkImage.mark(size: 24))
                        .renderingMode(.template)
                        .foregroundStyle(Brand.mark)
                        .accessibilityHidden(true)
                    TextField(capture.mode == .today ? "Add to today" : "New todo", text: $capture.text)
                        .textFieldStyle(.plain)
                        .font(.system(size: 20))
                        .foregroundStyle(Brand.ink)
                        .focused($focused)
                        .onSubmit(submit)
                        .onExitCommand { panel.hide() }
                        .disabled(capture.busy)
                }
                .frame(height: 64)
                .padding(.horizontal, 20)
                Rectangle().fill(Brand.line).frame(height: 1)
                HStack(spacing: 18) {
                    if capture.problem.isEmpty {
                        Text(capture.target)
                            .font(Brand.mono(11))
                            .foregroundStyle(Brand.muted)
                            .lineLimit(1)
                            .truncationMode(.head)
                    } else {
                        Text("◆ \(capture.problem)")
                            .font(.system(size: 12))
                            .foregroundStyle(Brand.alert)
                            .lineLimit(2)
                    }
                    Spacer(minLength: 0)
                    hint("↵", capture.mode == .today ? "today" : "todo")
                    hint("⇧↵", capture.mode == .today ? "todo" : "today")
                    hint("esc", "close")
                }
                .font(.system(size: 12))
                .padding(.horizontal, 20)
                .padding(.vertical, 10)
            }
        }
        .frame(width: 620)
        .background(Brand.surface, in: RoundedRectangle(cornerRadius: 16))
        .overlay(RoundedRectangle(cornerRadius: 16).strokeBorder(Brand.line))
        .onChange(of: capture.shown, initial: true) { focused = true }
    }

    private func hint(_ key: String, _ verb: String) -> some View {
        Text("\(Text(key).font(Brand.mono(12)).foregroundStyle(Brand.accent)) \(Text(verb).foregroundStyle(Brand.muted))")
    }

    private func submit() {
        let alternate = NSEvent.modifierFlags.contains(.shift) // Shift-Return: the other way
        Task {
            if await capture.submit(alternate: alternate) { panel.done() }
        }
    }
}
