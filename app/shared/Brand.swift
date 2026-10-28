// nt's design system in code (docs/design-system): every colour the app shows, in its light and
// dark value, and the brand face. Views never name a colour or a font; they ask Brand. A test
// keeps the table equal to tokens.json, as internal/theme's does for the terminal.
import CoreText
import SwiftUI
#if os(macOS)
import AppKit
#else
import UIKit
#endif

enum Brand {
    /// The tokens the app uses: name → (light, dark), as in tokens.json.
    nonisolated static let tokens: [String: (light: UInt32, dark: UInt32)] = [
        "ground": (0xF6F8F8, 0x14181A),
        "surface": (0xFFFFFF, 0x1A2023),
        "surface-sunk": (0xEEF2F2, 0x0F1214),
        "line": (0xDCE3E4, 0x263034),
        "ink": (0x14181A, 0xF2F5F5),
        "ink-muted": (0x5A686B, 0x8A999C),
        "ink-faint": (0x98A5A7, 0x5E6D70),
        "accent": (0x0B7A7E, 0x3FD0D4),
        "accent-soft": (0xE8F6F6, 0x173437),
        "mark": (0x12999D, 0x3FD0D4),
        "ok": (0x1F7A3A, 0x5FC47A),
        "warn": (0x9A5A0E, 0xE8963C),
        "alert": (0xC0342B, 0xF0645A),
    ]

    static let ground = color("ground")
    static let surface = color("surface")
    static let sunk = color("surface-sunk")
    static let line = color("line")
    static let ink = color("ink")
    static let muted = color("ink-muted")
    static let faint = color("ink-faint")
    static let accent = color("accent")
    static let accentSoft = color("accent-soft")
    static let mark = color("mark") // the nt mark only
    static let ok = color("ok")
    static let warn = color("warn")
    static let alert = color("alert")

    static func color(_ name: String) -> Color {
        #if os(macOS)
        Color(nsColor: nsColor(name))
        #else
        Color(uiColor: uiColor(name))
        #endif
    }

    #if os(macOS)
    /// A colour that follows the window's appearance.
    static func nsColor(_ name: String) -> NSColor {
        let t = tokens[name] ?? (0xFF00FF, 0xFF00FF)
        return NSColor(name: NSColor.Name("nt." + name)) { appearance in
            let dark = appearance.bestMatch(from: [.aqua, .darkAqua]) == .darkAqua
            return NSColor(hex: dark ? t.dark : t.light)
        }
    }
    #else
    /// A colour that follows the screen's light or dark appearance.
    static func uiColor(_ name: String) -> UIColor {
        let t = tokens[name] ?? (0xFF00FF, 0xFF00FF)
        return UIColor { traits in UIColor(hex: traits.userInterfaceStyle == .dark ? t.dark : t.light) }
    }
    #endif

    /// JetBrains Mono at a fixed size, for dates, times, paths and keys; the system's monospaced
    /// face when the bundled one is missing.
    static func mono(_ size: CGFloat, bold: Bool = false) -> Font {
        _ = fontsRegistered
        let name = bold ? "JetBrainsMono-Bold" : "JetBrainsMono-Regular"
        #if os(macOS)
        let have = NSFont(name: name, size: size) != nil
        #else
        let have = UIFont(name: name, size: size) != nil
        #endif
        if have {
            return .custom(name, fixedSize: size)
        }
        return .system(size: size, weight: bold ? .bold : .regular, design: .monospaced)
    }

    private static let fontsRegistered: Bool = {
        for weight in ["400", "700"] {
            if let url = Bundle.main.url(forResource: "JetBrainsMono-" + weight, withExtension: "woff2", subdirectory: "fonts") {
                CTFontManagerRegisterFontsForURL(url as CFURL, .process, nil)
            }
        }
        return true
    }()
}

#if os(macOS)
extension NSColor {
    convenience init(hex: UInt32) {
        self.init(srgbRed: CGFloat(hex >> 16 & 0xFF) / 255, green: CGFloat(hex >> 8 & 0xFF) / 255, blue: CGFloat(hex & 0xFF) / 255, alpha: 1)
    }
}
#else
extension UIColor {
    convenience init(hex: UInt32) {
        self.init(red: CGFloat(hex >> 16 & 0xFF) / 255, green: CGFloat(hex >> 8 & 0xFF) / 255, blue: CGFloat(hex & 0xFF) / 255, alpha: 1)
    }
}
#endif
