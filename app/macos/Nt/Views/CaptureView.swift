import AppKit
import SwiftUI
import os

/// CapturePanel is the small floating field the capture shortcut opens, wherever the person is:
/// it takes the keys without bringing nt's window forward, says where the text went, and goes.
/// Esc, or a click elsewhere, closes it. It grows downwards, for the folder list.
@MainActor
final class CapturePanel: NSObject, NSWindowDelegate {
    let capture: Capture
    private var panel: KeyPanel?
    private var closing: Task<Void, Never>?
    private var top: CGFloat? // where the panel's top edge stays while it grows and shrinks
    private var keys: Any? // the folder list's keys, ahead of the field (keys(_:))
    private var activated = false // settle had to bring nt forward; hide gives the keys back
    private let log = Logger(subsystem: "nl.w4j.nt", category: "capture")

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
        Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(60))
            self?.settle(p)
        }
    }

    func hide() {
        closing?.cancel()
        panel?.orderOut(nil)
        capture.added = nil
        if activated {
            activated = false
            NSApp.deactivate() // back to the app the person was in
        }
    }

    /// settle makes sure, a moment after the panel shows, that the keys go to its field, so
    /// typing never only beeps. When they do not, it says why in the log (log show --predicate
    /// 'subsystem == "nl.w4j.nt"') and puts them there: the panel key again, bringing nt forward
    /// when it has to, and the field the first responder.
    func settle(_ p: KeyPanel) {
        guard p.isVisible, capture.added == nil else { return }
        if !p.isKeyWindow {
            let front = NSWorkspace.shared.frontmostApplication?.bundleIdentifier ?? "?"
            log.notice("the panel is not key; nt active \(NSApp.isActive), in front \(front, privacy: .public), key \(String(describing: NSApp.keyWindow), privacy: .public)")
            p.makeKeyAndOrderFront(nil)
            if !p.isKeyWindow {
                NSApp.activate()
                activated = true
                p.makeKeyAndOrderFront(nil)
            }
        }
        if !(p.firstResponder is NSTextView), let f = p.contentView.flatMap(Self.field) {
            log.notice("the field had no keys; first responder \(String(describing: p.firstResponder), privacy: .public)")
            p.makeFirstResponder(f)
        }
    }

    /// field is the panel's text field, where the keys go.
    private static func field(in v: NSView) -> NSTextField? {
        if let f = v as? NSTextField, f.isEditable { return f }
        for s in v.subviews {
            if let f = field(in: s) { return f }
        }
        return nil
    }

    /// shot draws the panel for a screenshot (-NTShot <dir>): shown far off screen and never
    /// key, so it takes nobody's keys, and a second later, once the text is in and the folder
    /// list open, its view is written to <dir>/capture.png at the screen's scale, the corners
    /// clear. assets/demo/mac.sh makes the README's.
    func shot(_ mode: Capture.Mode, to dir: String) {
        capture.began(mode)
        let p = panel ?? make()
        p.setFrameOrigin(NSPoint(x: -20000, y: -20000))
        p.orderFrontRegardless()
        Task {
            try? await Task.sleep(for: .seconds(1))
            if let t = p.firstResponder as? NSTextView { // the text as typed, not selected
                t.setSelectedRange(NSRange(location: t.string.utf16.count, length: 0))
            }
            guard let v = p.contentView, let rep = v.bitmapImageRepForCachingDisplay(in: v.bounds) else { return }
            v.cacheDisplay(in: v.bounds, to: rep)
            try? rep.representation(using: .png, properties: [:])?.write(to: URL(fileURLWithPath: dir + "/capture.png"))
        }
    }

    /// done shows what was added for a moment, then closes the panel.
    func done() {
        closing?.cancel()
        closing = Task {
            try? await Task.sleep(for: .milliseconds(1200))
            if !Task.isCancelled { hide() }
        }
    }

    /// make builds the panel; the tests make it without showing it.
    func make() -> KeyPanel {
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
        keys = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self] event in
            self?.keys(event) == true ? nil : event
        }
        return p
    }

    /// keys takes the keys of the folder list, before the field sees them: Tab opens it, and
    /// while it is open every key is the list's: typing and delete narrow it, ↑↓ choose, Return
    /// or Tab take the folder, esc goes back. The field keeps the focus all the while, so
    /// nothing hangs on moving it. It says whether it took the key.
    func keys(_ e: NSEvent) -> Bool {
        guard e.window === panel, capture.added == nil, !capture.busy,
              e.modifierFlags.isDisjoint(with: [.command, .control]) else { return false }
        guard capture.picking else {
            guard e.keyCode == 48, !e.modifierFlags.contains(.option) else { return false } // Tab
            capture.openPlaces()
            return true
        }
        switch e.keyCode {
        case 48, 36, 76: capture.choose() // Tab, Return, Enter
        case 53: capture.closePlaces() // esc
        case 125: capture.movePick(1)
        case 126: capture.movePick(-1)
        case 51, 117: capture.deleteBack() // delete, forward delete
        default:
            // letters; not arrows or function keys, which AppKit gives private-use characters
            let typed = (e.characters ?? "").unicodeScalars.filter { u in
                !CharacterSet.controlCharacters.contains(u) && !(0xF700...0xF8FF).contains(u.value)
            }
            capture.type(String(String.UnicodeScalarView(typed)))
        }
        return true
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
/// typing narrows them, ↑↓ chooses, Return or Tab takes one, esc goes back to the text (the
/// panel's keys(_:) takes those keys).
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
                        .foregroundStyle(capture.picking ? Brand.muted : Brand.ink)
                        .focused($focused)
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
        .onChange(of: capture.shown, initial: true) { focused = true }
    }

    /// places is the folder list: what was typed to narrow it, then the folders, the chosen one
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
                HStack(spacing: 1) {
                    Text(capture.query)
                        .foregroundStyle(Brand.ink)
                    Rectangle().fill(Brand.accent).frame(width: 2, height: 18)
                    if capture.query.isEmpty {
                        Text(" Folder: type to narrow")
                            .foregroundStyle(Brand.muted)
                    }
                }
                .font(.system(size: 15))
                .accessibilityElement(children: .combine)
                .accessibilityLabel(capture.query.isEmpty ? "Folder" : "Folder: \(capture.query)")
                Spacer(minLength: 0)
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
