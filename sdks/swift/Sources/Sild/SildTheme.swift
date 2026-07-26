import SwiftUI
import UIKit
import SildCore

/// The resolved palette the screens read — the brand color plus the light/dark surface
/// palette, mapped from `BrandConfig` by the shared `BrandTheme`, so iOS, Android and
/// the web widget derive identical colors from one server config.
public struct SildColors {
    public let brand: Color
    public let brandHover: Color
    public let onBrand: Color = .white
    public let page: Color
    public let card: Color
    public let sunken: Color
    public let text: Color
    public let sub: Color
    public let tertiary: Color
    public let border: Color

    init(config: BrandConfig, dark: Bool) {
        let p = BrandTheme.shared.palette(cfg: config, systemInDark: dark)
        brand = SildColors.parse(config.brand)
        brandHover = SildColors.parse(BrandTheme.shared.shade(hex: config.brand, pct: 0.85))
        page = SildColors.parse(p.page)
        card = SildColors.parse(p.card)
        sunken = SildColors.parse(p.sunken)
        text = SildColors.parse(p.text)
        sub = SildColors.parse(p.sub)
        tertiary = SildColors.parse(p.tertiary)
        border = SildColors.parse(p.border)
    }

    /// Parse a web color string ("#rgb", "#rrggbb", or "rgba(r,g,b,a)").
    static func parse(_ css: String) -> Color {
        let s = css.trimmingCharacters(in: .whitespaces)
        if s.hasPrefix("rgba(") || s.hasPrefix("rgb(") {
            let inner = s.drop(while: { $0 != "(" }).dropFirst().prefix(while: { $0 != ")" })
            let parts = inner.split(separator: ",").map { $0.trimmingCharacters(in: .whitespaces) }
            let r = Double(parts.count > 0 ? parts[0] : "0") ?? 0
            let g = Double(parts.count > 1 ? parts[1] : "0") ?? 0
            let b = Double(parts.count > 2 ? parts[2] : "0") ?? 0
            let a = Double(parts.count > 3 ? parts[3] : "1") ?? 1
            return Color(.sRGB, red: r / 255, green: g / 255, blue: b / 255, opacity: a)
        }
        guard let rgb = BrandTheme.shared.rgb(hex: s),
              let r = rgb.first as? NSNumber,
              let g = rgb.second as? NSNumber,
              let b = rgb.third as? NSNumber
        else { return Color(.sRGB, red: 0.145, green: 0.388, blue: 0.992, opacity: 1) }
        return Color(.sRGB, red: r.doubleValue / 255, green: g.doubleValue / 255, blue: b.doubleValue / 255, opacity: 1)
    }
}

/// Corner radii for the brand's radius preset, from the shared `BrandTheme.radii`.
public struct SildRadii {
    public let panel: CGFloat
    public let card: CGFloat
    public let btn: CGFloat
    public let bubble: CGFloat

    init(config: BrandConfig) {
        let r = BrandTheme.shared.radii(cfg: config)
        panel = CGFloat(r.panel)
        card = CGFloat(r.card)
        btn = CGFloat(r.btn)
        bubble = CGFloat(r.bubble)
    }
}

/// The theme resolved for the current brand config and color scheme.
public struct SildStyle {
    public let colors: SildColors
    public let radii: SildRadii
    public let fontFamily: String

    /// The tenant's face at [size], falling back to the system font when the host has
    /// not bundled it — the SDK ships no font binaries, matching the Android `:ui`.
    public func font(_ size: CGFloat, _ weight: Font.Weight = .regular) -> Font {
        guard fontFamily != "system" else { return .system(size: size, weight: weight) }
        guard UIFont(name: fontFamily, size: size) != nil else {
            return .system(size: size, weight: weight)
        }
        return .custom(fontFamily, size: size).weight(weight)
    }

    init(config: BrandConfig, systemDark: Bool) {
        // theme=auto follows the OS; light/dark force, matching SildTheme.kt.
        let dark: Bool
        switch config.theme {
        case "dark": dark = true
        case "auto": dark = systemDark
        default: dark = false
        }
        colors = SildColors(config: config, dark: dark)
        radii = SildRadii(config: config)
        fontFamily = BrandTheme.shared.fontFamily(cfg: config)
    }
}

/// A brand-filled control that darkens while pressed — the web widget's brand-hover.
public struct SildBrandButtonStyle: ButtonStyle {
    let colors: SildColors
    let radius: CGFloat

    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .background(configuration.isPressed ? colors.brandHover : colors.brand)
            .clipShape(RoundedRectangle(cornerRadius: radius))
    }
}

private struct SildStyleKey: EnvironmentKey {
    static let defaultValue = SildStyle(config: InteropKt.defaultBrandConfig(), systemDark: false)
}

public extension EnvironmentValues {
    var sildStyle: SildStyle {
        get { self[SildStyleKey.self] }
        set { self[SildStyleKey.self] = newValue }
    }
}
