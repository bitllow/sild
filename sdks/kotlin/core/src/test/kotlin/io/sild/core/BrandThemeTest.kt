package io.sild.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

// Pure-logic tests for the brand/theme derivation — no backend, always run.
class BrandThemeTest {
    @Test fun shadeDarkens15Percent() {
        // #2563FD → each channel ×0.85, rounded (the web brand-hover).
        assertEquals("#1F54D7", BrandTheme.shade("#2563FD"))
    }

    @Test fun hexToRgba() {
        assertEquals("rgba(37,99,253,0.32)", BrandTheme.hexToRgba("#2563FD", 0.32))
    }

    @Test fun expandsThreeDigitHex() {
        assertEquals(Triple(0xFF, 0x00, 0x00), BrandTheme.rgb("#f00"))
    }

    @Test fun radiiPerPreset() {
        assertEquals(Radii(16, 18, 10, 12), BrandTheme.radii(BrandConfig(radius = "default")))
        assertEquals(Radii(7, 8, 7, 8), BrandTheme.radii(BrandConfig(radius = "sharp")))
    }

    @Test fun autoThemeFollowsSystem() {
        val cfg = BrandConfig(theme = "auto")
        assertEquals(BrandTheme.DARK, BrandTheme.palette(cfg, systemInDark = true))
        assertEquals(BrandTheme.LIGHT, BrandTheme.palette(cfg, systemInDark = false))
        // Forced themes ignore the system.
        assertEquals(BrandTheme.DARK, BrandTheme.palette(BrandConfig(theme = "dark"), systemInDark = false))
    }

    @Test fun topicListParsesTrimsAndCapsAtFour() {
        val cfg = BrandConfig(topics = " a \n\nb\nc\nd\ne")
        assertEquals(listOf("a", "b", "c", "d"), cfg.topicList)
    }

    @Test fun defaultsMatchServerSeed() {
        val d = BrandConfig()
        assertEquals("#2563FD", d.brand)
        assertEquals("sild", d.font)
        assertTrue(d.showTeam) // Go default is true (differs from web pre-fetch fallback)
        assertEquals("Hi there.", d.heading)
    }
}
