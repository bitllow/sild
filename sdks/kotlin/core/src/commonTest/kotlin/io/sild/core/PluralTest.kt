package io.sild.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

// The table is the repo's, generated from backend/internal/i18n/plural-cases.json,
// so Kotlin cannot quietly disagree with Go, TypeScript or Swift about Latvian.
class PluralTest {
    @Test
    fun categoryFollowsTheSharedCaseTable() {
        assertTrue(PLURAL_CASES.isNotEmpty(), "the shared case table is empty")
        for ((locale, count, category) in PLURAL_CASES) {
            assertEquals(category, pluralCategory(locale, count), "$locale/$count")
        }
    }

    @Test
    fun everyCaseLandsInACategoryTheLocaleOffers() {
        for ((locale, _, category) in PLURAL_CASES) {
            assertTrue(category in pluralCategories(locale), "$locale does not offer $category")
        }
    }

    @Test
    fun aLanguageWithNoTableEntryStillPluralizes() {
        assertEquals("one", pluralCategory("qq", 1))
        assertEquals("other", pluralCategory("qq", 7))
    }

    @Test
    fun aCountRendersInTheCategoryItsLanguageUses() {
        val en = bundledStrings("en")
        assertEquals("1 agent online", en.tPlural(SildKeys.Plural.widgetHomeAgentsOnline, 1))
        assertEquals("3 agents online", en.tPlural(SildKeys.Plural.widgetHomeAgentsOnline, 3))
    }

    // Latvian's zero form covers 0 and the teens, which English has no key for at all.
    @Test
    fun latvianRendersItsOwnZeroForm() {
        val lv = bundledStrings("lv")
        val zero = lv.tPlural(SildKeys.Plural.widgetHomeAgentsOnline, 0)
        assertEquals(zero.replace("0", "11"), lv.tPlural(SildKeys.Plural.widgetHomeAgentsOnline, 11))
        assertTrue(zero.isNotEmpty() && !zero.contains("agent"), "lv fell back to English: $zero")
    }

    // A category the active language has but no text was written for reads as that
    // language's other form, never as English and never as a raw key.
    @Test
    fun anUnwrittenCategoryStaysInItsLanguage() {
        val et = bundledStrings("et")
        assertEquals(
            et.tPlural(SildKeys.Plural.widgetHomeAgentsOnline, 5),
            et.t("widget.home.agentsOnline.other", mapOf("count" to 5)),
        )
    }

    @Test
    fun anUnknownKeyIsBlankUnlessDebugAsksForIt() {
        val reported = mutableListOf<String>()
        val quiet = SildI18n(source = null, explicitLocale = "en", onMissingKey = { reported += it })
        assertEquals("", quiet.t("widget.nope"))
        assertEquals(listOf("widget.nope"), reported)

        val loud = SildI18n(source = null, explicitLocale = "en", debug = true)
        assertEquals("widget.nope", loud.t("widget.nope"))
    }
}
