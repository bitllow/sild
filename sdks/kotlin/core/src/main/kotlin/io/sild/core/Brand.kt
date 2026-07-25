package io.sild.core

import kotlinx.serialization.Serializable

// BrandConfig mirrors the web widget's BrandConfig and the Go domain.BrandConfig
// byte-for-byte on the wire (GET /v1/me/brand → { name, config }). Defaults match
// the Go DefaultBrandConfig (the authoritative served values), not the web
// pre-fetch fallback. The SDK honors the visual fields (brand color, theme,
// radius, font, heading/sub/topics, logo, showTeam, poweredBy) and ignores the
// web-only launcher geometry (launcherIcon/iconImg/launcherPos/launcherSize).
@Serializable
data class BrandConfig(
    val logo: String = "",
    val logoUrl: String? = null,
    val brand: String = "#2563FD",
    val theme: String = "light", // light | dark | auto
    val font: String = "sild", // system | sild | inter | figtree | dmsans
    val radius: String = "default", // sharp | default | rounded | pillowy
    val launcherIcon: String = "chat",
    val iconImg: String = "",
    val iconImgUrl: String? = null,
    val launcherPos: String = "right",
    val launcherSize: String = "md",
    val heading: String = "Hi there.",
    val sub: String = "How can we help? We typically reply in a few minutes.",
    val topics: String = "Track my order\nChange pickup address\nBilling question",
    val showTeam: Boolean = true,
    val poweredBy: Boolean = true,
) {
    /** Prefer the server-resolved signed URL; fall back to a legacy data:/http value. */
    val logoSrc: String? get() = logoUrl?.takeIf { it.isNotEmpty() } ?: logo.takeIf { it.isNotEmpty() }

    /** Suggested-topic labels: newline-separated, trimmed, non-empty, first 4 (parseTopics). */
    val topicList: List<String>
        get() = topics.split("\n").map { it.trim() }.filter { it.isNotEmpty() }.take(4)
}

/** GET /v1/me/brand response: { name, config }. name backs the header title fallback. */
@Serializable
data class BrandResponse(val name: String = "", val config: BrandConfig = BrandConfig())

/** One resolved surface color (0xAARRGGBB int is produced by the :ui layer). */
data class Palette(
    val page: String,
    val card: String,
    val sunken: String,
    val text: String,
    val sub: String,
    val tertiary: String,
    val border: String,
)

/** Corner radii in dp, mirroring the web RADII map. */
data class Radii(val panel: Int, val bubble: Int, val btn: Int, val card: Int)

// BrandTheme derives the concrete visual values from a BrandConfig, exactly like
// the web buildStyles(): the light/dark palette, radii, font family, and the
// brand-hover shade. Pure logic (no Android types) so it is JVM-unit-testable.
object BrandTheme {
    val LIGHT = Palette(
        page = "#F4F6FA", card = "#FFFFFF", sunken = "#EAEEF4",
        text = "#14181F", sub = "#5B6472", tertiary = "#8A94A4",
        border = "rgba(20,24,31,.09)",
    )
    val DARK = Palette(
        page = "#0E1116", card = "#181C23", sunken = "#262C35",
        text = "#F2F5F9", sub = "#9AA4B2", tertiary = "#78828F",
        border = "rgba(255,255,255,.10)",
    )

    private val RADII = mapOf(
        "sharp" to Radii(7, 8, 7, 8),
        "default" to Radii(16, 18, 10, 12),
        "rounded" to Radii(22, 20, 13, 16),
        "pillowy" to Radii(26, 22, 16, 18),
    )

    // Font family stacks per BrandConfig.font. The :ui layer maps these to bundled
    // Android font resources; the default is Schibsted Grotesk ("sild").
    val FONTS = mapOf(
        "system" to "system",
        "sild" to "Schibsted Grotesk",
        "inter" to "Inter",
        "figtree" to "Figtree",
        "dmsans" to "DM Sans",
    )

    fun radii(cfg: BrandConfig): Radii = RADII[cfg.radius] ?: RADII.getValue("default")

    fun fontFamily(cfg: BrandConfig): String = FONTS[cfg.font] ?: FONTS.getValue("sild")

    /** Dark palette when theme=dark, or theme=auto and the OS is in dark mode. */
    fun palette(cfg: BrandConfig, systemInDark: Boolean): Palette =
        when (cfg.theme) {
            "dark" -> DARK
            "auto" -> if (systemInDark) DARK else LIGHT
            else -> LIGHT
        }

    /** shade(hex, 0.85): darken each RGB channel by 15% — the web brand-hover. */
    fun shade(hex: String, pct: Double = 0.85): String {
        val (r, g, b) = rgb(hex) ?: return hex
        fun c(v: Int) = Math.round(v * pct).toInt().coerceIn(0, 255)
        return "#%02X%02X%02X".format(c(r), c(g), c(b))
    }

    /** hexToRgba(hex, a) → "rgba(r,g,b,a)" (matches the web helper). */
    fun hexToRgba(hex: String, a: Double): String {
        val (r, g, b) = rgb(hex) ?: return "rgba(0,0,0,$a)"
        return "rgba($r,$g,$b,$a)"
    }

    /** Parse a #rgb or #rrggbb hex into (r,g,b); null if malformed. */
    fun rgb(hex: String): Triple<Int, Int, Int>? {
        var h = hex.removePrefix("#")
        if (h.length == 3) h = h.map { "$it$it" }.joinToString("")
        if (h.length != 6) return null
        val v = h.toIntOrNull(16) ?: return null
        return Triple((v shr 16) and 0xFF, (v shr 8) and 0xFF, v and 0xFF)
    }
}
