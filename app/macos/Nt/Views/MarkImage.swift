import AppKit

/// MarkImage is nt's mark drawn as a template image, from the 24-unit geometry of
/// assets/brand/nt-mark.svg: the frame's two brackets, two lines of text and the cursor. The
/// alert form (nt-mark-alert.svg) is a solid tile with the lines and the cursor cut out, for the
/// menu bar while sync needs you (docs/design-system, Favicon, menu bar and app icon).
enum MarkImage {
    static func mark(alert: Bool = false, size: CGFloat = 18) -> NSImage {
        let image = NSImage(size: NSSize(width: size, height: size), flipped: true) { rect in
            let k = rect.width / 24
            func p(_ x: CGFloat, _ y: CGFloat) -> NSPoint { NSPoint(x: x * k, y: y * k) }
            NSColor.black.set()
            let lines = NSBezierPath()
            lines.move(to: p(7.6, 7.8))
            lines.line(to: p(16.4, 7.8))
            lines.move(to: p(7.6, 11.8))
            lines.line(to: p(11.2, 11.8))
            lines.lineCapStyle = .round
            if alert {
                NSBezierPath(roundedRect: NSRect(x: 2.6 * k, y: 2.2 * k, width: 18.8 * k, height: 19.6 * k), xRadius: 5.2 * k, yRadius: 5.2 * k).fill()
                NSGraphicsContext.current?.compositingOperation = .destinationOut
                lines.lineWidth = 3.1 * k
                lines.stroke()
                NSBezierPath(roundedRect: NSRect(x: 9.6 * k, y: 16.4 * k, width: 4.8 * k, height: 2.3 * k), xRadius: 1.15 * k, yRadius: 1.15 * k).fill()
                return true
            }
            let frame = NSBezierPath()
            frame.move(to: p(8, 3.2))
            frame.appendArc(from: p(3.8, 3.2), to: p(3.8, 20.8), radius: 2.2 * k)
            frame.appendArc(from: p(3.8, 20.8), to: p(8, 20.8), radius: 2.2 * k)
            frame.line(to: p(8, 20.8))
            frame.move(to: p(16, 3.2))
            frame.appendArc(from: p(20.2, 3.2), to: p(20.2, 20.8), radius: 2.2 * k)
            frame.appendArc(from: p(20.2, 20.8), to: p(16, 20.8), radius: 2.2 * k)
            frame.line(to: p(16, 20.8))
            frame.lineWidth = 1.6 * k
            frame.lineCapStyle = .round
            frame.lineJoinStyle = .round
            frame.stroke()
            lines.lineWidth = 2.9 * k
            lines.stroke()
            NSBezierPath(roundedRect: NSRect(x: 9.6 * k, y: 16.2 * k, width: 4.8 * k, height: 2.2 * k), xRadius: 1.1 * k, yRadius: 1.1 * k).fill()
            return true
        }
        image.isTemplate = true
        image.accessibilityDescription = alert ? "nt, sync needs you" : "nt"
        return image
    }
}
