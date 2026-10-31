import AppKit
import SwiftUI

/// CapturePanel is the small floating field the capture shortcut opens, wherever the person is:
/// it takes the keys without bringing nt's window forward, says where the text went, and goes.
/// Esc, or a click elsewhere, closes it. It grows downwards, for the folder list.
@MainActor
final class CapturePanel: NSObject, NSWindowDelegate {
    let capture: Capture
    private var panel: KeyPanel?
    private var closing: Task<Void, Never>?
    private var top: CGFloat? // where the panel's top edge stays while it grows and shrinks
    private var tab: Any? // Tab in the panel opens or takes the folder list; a field would move focus

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
            top = f.maxY - f.height * 0.22
            p.setFrameTopLeftPoint(NSPoint(x: f.midX - p.frame.width / 2, y: top ?? 0))
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
        tab = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self] event in
            guard let self, event.keyCode == 48, event.window === self.panel,
                  event.modifierFlags.isDisjoint(with: [.command, .control, .option]),
                  self.capture.added == nil, !self.capture.busy else { return event }
            self.capture.picking ? self.capture.choose() : self.capture.openPlaces()
            return nil
        }
        return p
    }

    func windowDidResignKey(_ notification: Notification) {
        if capture.added == nil { hide() } // a click elsewhere
    }

    func windowDidResize(_ notification: Notification) {
        if let p = panel, let top, p.frame.maxY != top {
            p.setFrameTopLeftPoint(NSPoint(x: p.frame.minX, y: top))
        }
    }

    func windowDidMove(_ notification: Notification) {
        top = panel?.frame.maxY // dragged by its background
    }
}

/// KeyPanel is a borderless panel that can take the keyboard.
final class KeyPanel: NSPanel {
    override var canBecomeKey: Bool { true }
}

/// CaptureView is the panel: nt's mark and the field, the file the text goes to and the keys
/// under it; once added, where it went. Tab opens the folders a todo can go to, under the field:
/// typing narrows them, ↑↓ chooses, Return or Tab takes one, esc goes back to the text.
struct CaptureView: View {
    @Bindable var capture: Capture
    let panel: CapturePanel
    @FocusState private var focus: Field?

    private enum Field { case text, place }

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
                        .focused($focus, equals: .text)
                        .onSubmit(submit)
                        .onExitCommand { panel.hide() }
                        .disabled(capture.busy)
                }
                .frame(height: 64)
                .padding(.horizontal, 20)
                Rectangle().fill(Brand.line).frame(height: 1)
                if capture.picking {
                    places
                    Rectangle().fill(Brand.line).frame(height: 1)
                }
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
                    if capture.picking {
                        hint("↑↓", "choose")
                        hint("↵", "take")
                        hint("esc", "back")
                    } else {
                        hint("↵", capture.mode == .today ? "today" : "todo")
                        hint("⇧↵", capture.mode == .today ? "todo" : "today")
                        hint("⇥", "folder")
                        hint("esc", "close")
                    }
                }
                .font(.system(size: 12))
                .padding(.horizontal, 20)
                .padding(.vertical, 10)
            }
        }
        .frame(width: 620)
        .background(Brand.surface, in: RoundedRectangle(cornerRadius: 16))
        .overlay(RoundedRectangle(cornerRadius: 16).strokeBorder(Brand.line))
        .onChange(of: capture.shown, initial: true) { focus = .text }
        .onChange(of: capture.picking) { focus = capture.picking ? .place : .text }
        .onChange(of: capture.query) { capture.pick = 0 }
    }

    /// places is the folder list: a field that narrows it, then the folders, the chosen one
    /// marked, each with its todo.md's open todos.
    private var places: some View {
        let rows = capture.places
        return VStack(spacing: 0) {
            HStack(spacing: 14) {
                Image(systemName: "folder")
                    .font(.system(size: 15))
                    .foregroundStyle(Brand.muted)
                    .frame(width: 24)
                    .accessibilityHidden(true)
                TextField("Folder", text: $capture.query)
                    .textFieldStyle(.plain)
                    .font(.system(size: 15))
                    .foregroundStyle(Brand.ink)
                    .focused($focus, equals: .place)
                    .onSubmit { capture.choose() }
                    .onKeyPress(.downArrow) { capture.movePick(1); return .handled }
                    .onKeyPress(.upArrow) { capture.movePick(-1); return .handled }
                    .onExitCommand { capture.closePlaces() }
            }
            .frame(height: 40)
            .padding(.horizontal, 20)
            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 2) {
                        if rows.isEmpty {
                            Text("No folder matches.")
                                .font(.system(size: 13))
                                .foregroundStyle(Brand.muted)
                                .padding(.horizontal, 12)
                                .padding(.vertical, 8)
                        }
                        ForEach(Array(rows.enumerated()), id: \.element) { i, place in
                            Button {
                                capture.pick = i
                                capture.choose()
                            } label: {
                                placeRow(place, selected: i == capture.pick)
                            }
                            .buttonStyle(.plain)
                            .id(place)
                        }
                    }
                    .padding(.leading, 46) // the rows' text under the field's
                    .padding(.trailing, 8)
                    .padding(.bottom, 8)
                }
                .frame(height: min(CGFloat(max(rows.count, 1)) * 34 + 8, 214))
                .onChange(of: capture.pick) {
                    if rows.indices.contains(capture.pick) { proxy.scrollTo(rows[capture.pick]) }
                }
            }
        }
    }

    private func placeRow(_ place: String, selected: Bool) -> some View {
        let open = capture.open(in: place)
        return HStack(spacing: 12) {
            Text(place.isEmpty ? "todo.md" : place)
                .font(.system(size: 14))
                .foregroundStyle(Brand.ink)
                .lineLimit(1)
                .truncationMode(.head)
            Spacer(minLength: 8)
            Text(open > 0 ? "\(open) open" : "")
                .font(Brand.mono(11))
                .foregroundStyle(Brand.muted)
        }
        .padding(.horizontal, 12)
        .frame(height: 32)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(selected ? Brand.accentSoft : .clear, in: RoundedRectangle(cornerRadius: 8))
        .overlay(alignment: .leading) {
            if selected {
                RoundedRectangle(cornerRadius: 2).fill(Brand.accent).frame(width: 3, height: 16)
            }
        }
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(selected ? .isSelected : [])
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
