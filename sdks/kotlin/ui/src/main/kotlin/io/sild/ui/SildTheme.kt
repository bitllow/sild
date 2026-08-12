package io.sild.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.lightColorScheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import io.sild.core.BrandConfig
import io.sild.core.BrandTheme
import io.sild.core.bundledStrings

// SildColors is the resolved palette the screens read — the brand color plus the
// light/dark surface palette, mapped from BrandConfig exactly like the web
// buildStyles(). Exposed via a CompositionLocal so every composable themes
// identically to the web widget.
data class SildColors(
    val brand: Color,
    val brandHover: Color,
    val onBrand: Color = Color.White,
    val page: Color,
    val card: Color,
    val sunken: Color,
    val text: Color,
    val sub: Color,
    val tertiary: Color,
    val border: Color,
)

val LocalSildColors = staticCompositionLocalOf<SildColors> { error("SildColors not provided") }

/** Looks up one of Sild's own strings in the active language. */
typealias SildStrings = (String, Map<String, Any>?) -> String
typealias SildPlurals = (String, Int, Map<String, Any>?) -> String

// Defaults to the bundled English so a preview or a host embedding a screen
// directly still renders words rather than keys. Built once: every label on the
// screen goes through this on each recomposition.
private val bundled by lazy { bundledStrings() }

val LocalSildStrings = staticCompositionLocalOf<SildStrings> { { key, vars -> bundled.t(key, vars) } }

val LocalSildPlurals = staticCompositionLocalOf<SildPlurals> {
    { base, count, vars -> bundled.tPlural(base, count, vars) }
}

/** The text for [key] in the language the messenger is rendering. */
@Composable
internal fun t(key: String, vars: Map<String, Any>? = null): String = LocalSildStrings.current(key, vars)

/** The text for a count, in the plural category the active language uses for it. */
@Composable
internal fun tPlural(base: String, count: Int, vars: Map<String, Any>? = null): String =
    LocalSildPlurals.current(base, count, vars)

val LocalSildRadii = staticCompositionLocalOf { BrandTheme.radii(BrandConfig()) }

/** Parse a web color string ("#rgb", "#rrggbb", or "rgba(r,g,b,a)") into a Color. */
fun parseColor(css: String): Color {
    val s = css.trim()
    if (s.startsWith("rgba(") || s.startsWith("rgb(")) {
        val nums = s.substringAfter('(').substringBefore(')').split(',').map { it.trim() }
        val r = nums.getOrNull(0)?.toIntOrNull() ?: 0
        val g = nums.getOrNull(1)?.toIntOrNull() ?: 0
        val b = nums.getOrNull(2)?.toIntOrNull() ?: 0
        val a = nums.getOrNull(3)?.toFloatOrNull() ?: 1f
        return Color(r, g, b, (a * 255).toInt())
    }
    val rgb = BrandTheme.rgb(s) ?: return Color(0xFF2563FD)
    return Color(rgb.first, rgb.second, rgb.third)
}

private fun colorsFor(cfg: BrandConfig, dark: Boolean): SildColors {
    val p = BrandTheme.palette(cfg, dark)
    return SildColors(
        brand = parseColor(cfg.brand),
        brandHover = parseColor(BrandTheme.shade(cfg.brand)),
        page = parseColor(p.page),
        card = parseColor(p.card),
        sunken = parseColor(p.sunken),
        text = parseColor(p.text),
        sub = parseColor(p.sub),
        tertiary = parseColor(p.tertiary),
        border = parseColor(p.border),
    )
}

// SildTheme applies the brand config as a Material3 theme + Sild color/radii
// locals. theme=auto follows the OS; light/dark force. Fonts map to the bundled
// family via SildFonts (default Schibsted Grotesk).
@Composable
fun SildTheme(cfg: BrandConfig, content: @Composable () -> Unit) {
    val systemDark = isSystemInDarkTheme()
    val dark = when (cfg.theme) {
        "dark" -> true
        "auto" -> systemDark
        else -> false
    }
    val c = colorsFor(cfg, dark)
    val r = BrandTheme.radii(cfg)
    val scheme = (if (dark) darkColorScheme() else lightColorScheme()).copy(
        primary = c.brand,
        onPrimary = c.onBrand,
        background = c.page,
        surface = c.card,
        onSurface = c.text,
    )
    val shapes = Shapes(
        small = RoundedCornerShape(r.btn.dp),
        medium = RoundedCornerShape(r.card.dp),
        large = RoundedCornerShape(r.panel.dp),
    )
    CompositionLocalProvider(LocalSildColors provides c, LocalSildRadii provides r) {
        MaterialTheme(colorScheme = scheme, shapes = shapes, typography = sildTypography(cfg), content = content)
    }
}
