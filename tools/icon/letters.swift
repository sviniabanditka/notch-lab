// letters.swift draws the lettermark icon: heavy white letters on a deep
// indigo tile with a nebula glow along the bottom, in the manner of T3's.
// The app icon is letters-night-N_l.png, copied to tools/icon/icon.png (mygo.json icon):
//   swift tools/icon/letters.swift /tmp/icons Nl
// Usage: swift tools/icon/letters.swift <outdir> [TEXT...]; a lowercase
// letter is _x in the file name, as the macOS file system ignores case.
import AppKit

let size: CGFloat = 1024
let tile = CGRect(x: 100, y: 100, width: 824, height: 824)
let radius: CGFloat = 186
let cs = CGColorSpace(name: CGColorSpace.sRGB)!

func rgb(_ hex: UInt32, _ a: CGFloat = 1) -> CGColor {
    CGColor(red: CGFloat(hex >> 16 & 0xff) / 255, green: CGFloat(hex >> 8 & 0xff) / 255,
            blue: CGFloat(hex & 0xff) / 255, alpha: a)
}

func glow(_ ctx: CGContext, _ at: CGPoint, _ r: CGFloat, _ hex: UInt32, _ a: CGFloat) {
    let g = CGGradient(colorsSpace: cs, colors: [rgb(hex, a), rgb(hex, 0)] as CFArray, locations: [0, 1])!
    ctx.drawRadialGradient(g, startCenter: at, startRadius: 0, endCenter: at, endRadius: r, options: [])
}

// A fixed seed keeps the stars where they were between runs.
var seed: UInt64 = 0x9e3779b97f4a7c15
func rand() -> CGFloat {
    seed ^= seed << 13; seed ^= seed >> 7; seed ^= seed << 17
    return CGFloat(seed % 10_000) / 10_000
}

func lettersPath(_ text: String, height: CGFloat) -> CGPath {
    let font = NSFont.systemFont(ofSize: height, weight: .black)
    let line = CTLineCreateWithAttributedString(NSAttributedString(string: text, attributes: [.font: font, .kern: -height * 0.03]))
    let path = CGMutablePath()
    for run in CTLineGetGlyphRuns(line) as! [CTRun] {
        let n = CTRunGetGlyphCount(run)
        var gs = [CGGlyph](repeating: 0, count: n), ps = [CGPoint](repeating: .zero, count: n)
        CTRunGetGlyphs(run, CFRange(), &gs); CTRunGetPositions(run, CFRange(), &ps)
        for i in 0..<n {
            if let gp = CTFontCreatePathForGlyph(font as CTFont, gs[i], nil) {
                path.addPath(gp, transform: CGAffineTransform(translationX: ps[i].x, y: ps[i].y))
            }
        }
    }
    return path
}

// Palette is a tile: its base gradient, nebula clouds and letter colour.
struct Palette {
    let name: String
    let base: [UInt32]
    let clouds: [(CGPoint, CGFloat, UInt32, CGFloat)]
    let stars: CGFloat
    let ink: UInt32
}

let palettes = [
    Palette(name: "night", base: [0x2a2160, 0x16123a, 0x0c0a22],
            clouds: [(CGPoint(x: 700, y: 230), 380, 0x7b4dff, 0.55), (CGPoint(x: 420, y: 140), 300, 0x3a6bff, 0.45),
                     (CGPoint(x: 860, y: 380), 240, 0xc06bff, 0.35), (CGPoint(x: 560, y: 300), 160, 0xe6d8ff, 0.25)],
            stars: 1, ink: 0xffffff),
    // herdr.dev's spot colour (--spot #cba6f7, --spot-ink #17171a).
    Palette(name: "spot", base: [0xd9bdfa, 0xcba6f7, 0xa77fe8],
            clouds: [(CGPoint(x: 700, y: 230), 380, 0x8a5cf0, 0.45), (CGPoint(x: 420, y: 160), 300, 0xf3a6e0, 0.4),
                     (CGPoint(x: 860, y: 380), 240, 0x6f45d8, 0.3), (CGPoint(x: 560, y: 300), 160, 0xffffff, 0.3)],
            stars: 0.7, ink: 0x17171a),
]

func render(_ text: String, _ p: Palette, to dir: String) {
    seed = 0x9e3779b97f4a7c15
    let ctx = CGContext(data: nil, width: Int(size), height: Int(size), bitsPerComponent: 8, bytesPerRow: 0,
                        space: cs, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
    let shape = CGPath(roundedRect: tile, cornerWidth: radius, cornerHeight: radius, transform: nil)
    ctx.saveGState()
    ctx.setShadow(offset: CGSize(width: 0, height: -12), blur: 28, color: rgb(0, 0.45))
    ctx.addPath(shape); ctx.setFillColor(rgb(0x14112e)); ctx.fillPath()
    ctx.restoreGState()
    ctx.saveGState(); ctx.addPath(shape); ctx.clip()
    let base = CGGradient(colorsSpace: cs, colors: p.base.map { rgb($0) } as CFArray,
                          locations: [0, 0.55, 1])!
    ctx.drawLinearGradient(base, start: CGPoint(x: 300, y: tile.maxY), end: CGPoint(x: 700, y: tile.minY), options: [.drawsBeforeStartLocation, .drawsAfterEndLocation])
    for (at, r, hex, a) in p.clouds { glow(ctx, at, r, hex, a) }
    for _ in 0..<70 {
        let star = CGPoint(x: tile.minX + rand() * tile.width, y: tile.minY + rand() * tile.height)
        let r = 1 + rand() * 2.6
        ctx.setFillColor(rgb(0xffffff, (0.25 + rand() * 0.6) * p.stars))
        ctx.fillEllipse(in: CGRect(x: star.x - r, y: star.y - r, width: r * 2, height: r * 2))
    }
    var letters = lettersPath(text, height: 400)
    let b = letters.boundingBoxOfPath
    let scale = min(560 / b.width, 340 / b.height)
    var t = CGAffineTransform(translationX: 512 - b.midX * scale, y: 540 - b.midY * scale).scaledBy(x: scale, y: scale)
    letters = letters.copy(using: &t)!
    ctx.saveGState()
    ctx.setShadow(offset: CGSize(width: 0, height: -8), blur: 24, color: rgb(0x05030f, p.ink == 0xffffff ? 0.6 : 0.25))
    ctx.addPath(letters); ctx.setFillColor(rgb(p.ink)); ctx.fillPath()
    ctx.restoreGState()
    let sheen = CGGradient(colorsSpace: cs, colors: [rgb(0xffffff, 0.12), rgb(0xffffff, 0)] as CFArray, locations: [0, 1])!
    ctx.drawLinearGradient(sheen, start: CGPoint(x: 512, y: tile.maxY), end: CGPoint(x: 512, y: tile.midY), options: [])
    ctx.restoreGState()
    ctx.addPath(shape); ctx.setStrokeColor(rgb(0xffffff, 0.12)); ctx.setLineWidth(3); ctx.strokePath()
    let url = URL(fileURLWithPath: dir).appendingPathComponent("letters-\(p.name)-" + text.map { $0.isUppercase ? "\($0)" : "_\($0)" }.joined() + ".png")
    let dest = CGImageDestinationCreateWithURL(url as CFURL, "public.png" as CFString, 1, nil)!
    CGImageDestinationAddImage(dest, ctx.makeImage()!, nil)
    CGImageDestinationFinalize(dest)
}

let args = CommandLine.arguments
let out = args.count > 1 ? args[1] : "."
try FileManager.default.createDirectory(atPath: out, withIntermediateDirectories: true)
for text in args.count > 2 ? Array(args[2...]) : ["Nl"] {
    for p in palettes { render(text, p, to: out) }
}
