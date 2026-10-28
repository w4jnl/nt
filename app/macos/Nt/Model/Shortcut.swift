// The capture shortcut (docs/design.md §8, Menu bar and capture): a key and its modifiers, kept
// on this Mac in UserDefaults, never in the repository, and registered as a global hot key with
// Carbon's RegisterEventHotKey, which needs no accessibility permission. macOS keeps some keys
// for itself (⌘Space for Spotlight, ⌥⌘Space for the Finder search window); Settings says so.
import AppKit
import Carbon.HIToolbox
import Observation
import SwiftUI

struct Shortcut: Equatable, Sendable {
    var keyCode: UInt32
    var flags: UInt // the device-independent modifier flags: ⌘ ⌥ ⌃ ⇧ only
    var key: String // the key's name: Space, N, F5

    nonisolated static let standard = Shortcut(keyCode: UInt32(kVK_Space), flags: NSEvent.ModifierFlags([.command, .option]).rawValue, key: "Space")
    nonisolated static let modifierMask: UInt = NSEvent.ModifierFlags([.command, .option, .control, .shift]).rawValue

    /// display is the shortcut as macOS writes it: ⌃⌥⇧⌘ and the key.
    var display: String {
        let m = NSEvent.ModifierFlags(rawValue: flags)
        var s = ""
        if m.contains(.control) { s += "⌃" }
        if m.contains(.option) { s += "⌥" }
        if m.contains(.shift) { s += "⇧" }
        if m.contains(.command) { s += "⌘" }
        return s + key
    }

    /// carbon is the modifiers as RegisterEventHotKey takes them.
    var carbon: UInt32 {
        let m = NSEvent.ModifierFlags(rawValue: flags)
        var c: UInt32 = 0
        if m.contains(.command) { c |= UInt32(cmdKey) }
        if m.contains(.option) { c |= UInt32(optionKey) }
        if m.contains(.control) { c |= UInt32(controlKey) }
        if m.contains(.shift) { c |= UInt32(shiftKey) }
        return c
    }

    /// pressed is the shortcut a key press makes; nil without ⌘, ⌥ or ⌃, which a global
    /// shortcut needs so that it never takes a key someone is typing.
    nonisolated static func pressed(keyCode: UInt16, flags: NSEvent.ModifierFlags, characters: String?) -> Shortcut? {
        let m = flags.intersection([.command, .option, .control, .shift])
        guard !m.intersection([.command, .option, .control]).isEmpty else { return nil }
        return Shortcut(keyCode: UInt32(keyCode), flags: m.rawValue, key: name(keyCode: keyCode, characters: characters))
    }

    nonisolated static func name(keyCode: UInt16, characters: String?) -> String {
        let named: [Int: String] = [
            kVK_Space: "Space", kVK_Return: "↩", kVK_Tab: "⇥", kVK_Delete: "⌫", kVK_Escape: "⎋",
            kVK_LeftArrow: "←", kVK_RightArrow: "→", kVK_UpArrow: "↑", kVK_DownArrow: "↓",
            kVK_F1: "F1", kVK_F2: "F2", kVK_F3: "F3", kVK_F4: "F4", kVK_F5: "F5", kVK_F6: "F6",
            kVK_F7: "F7", kVK_F8: "F8", kVK_F9: "F9", kVK_F10: "F10", kVK_F11: "F11", kVK_F12: "F12",
        ]
        if let n = named[Int(keyCode)] { return n }
        return (characters ?? "").uppercased()
    }

    /// keyEquivalent is the shortcut for a menu item that shows it.
    var keyEquivalent: KeyEquivalent? {
        if keyCode == UInt32(kVK_Space) { return .space }
        guard key.count == 1, let c = key.lowercased().first else { return nil }
        return KeyEquivalent(c)
    }

    // MARK: Kept on this Mac

    static func load(_ d: UserDefaults = .standard) -> Shortcut {
        guard let code = d.object(forKey: "captureKeyCode") as? Int, let key = d.string(forKey: "captureKey") else { return .standard }
        return Shortcut(keyCode: UInt32(code), flags: UInt(d.integer(forKey: "captureFlags")), key: key)
    }

    func save(_ d: UserDefaults = .standard) {
        d.set(Int(keyCode), forKey: "captureKeyCode")
        d.set(Int(flags), forKey: "captureFlags")
        d.set(key, forKey: "captureKey")
    }

    // MARK: Keys macOS keeps

    /// taken names what macOS uses the shortcut for, from its keyboard shortcuts
    /// (com.apple.symbolichotkeys: 64 is Spotlight, 65 the Finder search window); nil when
    /// nothing. A shortcut the person never changed is not in that file and is on.
    nonisolated static func taken(_ s: Shortcut, hotkeys: [String: Any]?) -> String? {
        let known: [(id: String, name: String, keyCode: Int, flags: Int)] = [
            ("64", "Spotlight", kVK_Space, 0x100000),
            ("65", "the Finder search window", kVK_Space, 0x180000),
        ]
        for k in known {
            var on = true, code = k.keyCode, flags = k.flags
            if let e = hotkeys?[k.id] as? [String: Any] {
                on = (e["enabled"] as? NSNumber)?.boolValue ?? true
                if let v = e["value"] as? [String: Any], let p = v["parameters"] as? [Int], p.count == 3 {
                    code = p[1]
                    flags = p[2]
                }
            }
            if on, code == Int(s.keyCode), UInt(flags) & modifierMask == s.flags & modifierMask { return k.name }
        }
        return nil
    }

    static var systemHotkeys: [String: Any]? {
        UserDefaults(suiteName: "com.apple.symbolichotkeys")?.dictionary(forKey: "AppleSymbolicHotKeys")
    }
}

/// CaptureKey is the capture shortcut as the app holds it: registered as a global hot key that
/// opens the capture panel, and changed from Settings.
@MainActor
@Observable
final class CaptureKey {
    private(set) var shortcut = Shortcut.load()
    private(set) var registered = false // macOS took it: no other app holds it
    @ObservationIgnored private let hotKey: HotKey

    init(action: @escaping @MainActor () -> Void) {
        hotKey = HotKey(action: action)
    }

    /// start registers the shortcut kept on this Mac. Under tests it does nothing.
    func start() {
        if ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] != nil { return }
        registered = hotKey.register(shortcut)
    }

    /// set makes s the shortcut, kept on this Mac; false, with the one before kept, when another
    /// app holds it.
    @discardableResult
    func set(_ s: Shortcut) -> Bool {
        guard hotKey.register(s) else {
            registered = hotKey.register(shortcut)
            return false
        }
        shortcut = s
        s.save()
        registered = true
        return true
    }

    /// pause lets the shortcut go while Settings records a new one, so that pressing it is seen.
    func pause() { hotKey.unregister() }

    func resume() { registered = hotKey.register(shortcut) }

    /// taken names what macOS itself uses the shortcut for, if anything.
    var taken: String? { Shortcut.taken(shortcut, hotkeys: Shortcut.systemHotkeys) }
}

/// HotKey is one global hot key: action runs when it is pressed, whichever app is in front.
@MainActor
final class HotKey {
    private var ref: EventHotKeyRef?
    private var handler: EventHandlerRef?
    fileprivate let action: @MainActor () -> Void

    init(action: @escaping @MainActor () -> Void) {
        self.action = action
        var spec = EventTypeSpec(eventClass: OSType(kEventClassKeyboard), eventKind: UInt32(kEventHotKeyPressed))
        InstallEventHandler(GetApplicationEventTarget(), { _, _, data in
            guard let data else { return OSStatus(eventNotHandledErr) }
            let key = Unmanaged<HotKey>.fromOpaque(data).takeUnretainedValue()
            MainActor.assumeIsolated { key.action() } // Carbon calls on the main thread
            return noErr
        }, 1, &spec, Unmanaged.passUnretained(self).toOpaque(), &handler)
    }

    /// register makes s the hot key in place of the one before; false when another app holds it.
    func register(_ s: Shortcut) -> Bool {
        unregister()
        let id = EventHotKeyID(signature: OSType(0x6E74_6361), id: 1) // "ntca"
        return RegisterEventHotKey(s.keyCode, s.carbon, id, GetApplicationEventTarget(), 0, &ref) == noErr
    }

    func unregister() {
        if let ref { UnregisterEventHotKey(ref) }
        ref = nil
    }
}
