// Mark is nt's mark, from the 24-unit geometry of assets/brand/nt-mark.svg: the frame's two
// brackets, two lines of text and the cursor, in the mark's colour.
import SwiftUI

struct Mark: View {
    var size: CGFloat = 22

    var body: some View {
        Canvas { ctx, s in
            let k = s.width / 24
            func p(_ x: CGFloat, _ y: CGFloat) -> CGPoint { CGPoint(x: x * k, y: y * k) }
            var brackets = Path()
            for (outer, inner) in [(3.8, 8.0), (20.2, 16.0)] {
                let r = 2.2, dir: CGFloat = outer < 12 ? 1 : -1
                brackets.move(to: p(inner, 3.2))
                brackets.addLine(to: p(outer + dir * r, 3.2))
                brackets.addQuadCurve(to: p(outer, 3.2 + r), control: p(outer, 3.2))
                brackets.addLine(to: p(outer, 18.6))
                brackets.addQuadCurve(to: p(outer + dir * r, 20.8), control: p(outer, 20.8))
                brackets.addLine(to: p(inner, 20.8))
            }
            ctx.stroke(brackets, with: .color(Brand.mark), style: StrokeStyle(lineWidth: 1.6 * k, lineCap: .round, lineJoin: .round))
            var lines = Path()
            lines.move(to: p(7.6, 7.8))
            lines.addLine(to: p(16.4, 7.8))
            lines.move(to: p(7.6, 11.8))
            lines.addLine(to: p(11.2, 11.8))
            ctx.stroke(lines, with: .color(Brand.mark), style: StrokeStyle(lineWidth: 2.9 * k, lineCap: .round))
            ctx.fill(Path(roundedRect: CGRect(x: 9.6 * k, y: 16.2 * k, width: 4.8 * k, height: 2.2 * k), cornerRadius: 1.1 * k), with: .color(Brand.mark))
        }
        .frame(width: size, height: size)
        .accessibilityHidden(true)
    }
}
