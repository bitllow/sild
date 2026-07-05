package io.sild.ui

import androidx.compose.material3.Typography
import androidx.compose.ui.text.font.FontFamily
import io.sild.core.BrandConfig

// Font mapping. The web loads Schibsted Grotesk / Inter / Figtree / DM Sans from
// Google Fonts; to keep the SDK free of font binaries and runtime font downloads,
// this first cut maps every family to the platform sans-serif. Hosts that want
// the exact brand face can drop the font into res/font and extend familyFor().
// (Called out as a deliberate simplification — the color/radius/theme system is
// pixel-faithful; only the typeface differs.)
internal fun familyFor(cfg: BrandConfig): FontFamily = FontFamily.SansSerif

internal fun sildTypography(cfg: BrandConfig): Typography {
    val family = familyFor(cfg)
    val base = Typography()
    return base.copy(
        titleLarge = base.titleLarge.copy(fontFamily = family),
        titleMedium = base.titleMedium.copy(fontFamily = family),
        bodyLarge = base.bodyLarge.copy(fontFamily = family),
        bodyMedium = base.bodyMedium.copy(fontFamily = family),
        labelLarge = base.labelLarge.copy(fontFamily = family),
        labelSmall = base.labelSmall.copy(fontFamily = family),
    )
}
