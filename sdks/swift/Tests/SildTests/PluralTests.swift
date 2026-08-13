import XCTest
@testable import Sild
import SildCore

// Swift picks plural categories through the shared Kotlin core rather than keeping a
// fourth copy of the rules, and this is the proof: the same repo case table every
// other runtime answers to, run through the framework Swift actually ships.
final class PluralTests: XCTestCase {
    func testCategoryFollowsTheSharedCaseTable() {
        XCTAssertFalse(pluralCases.isEmpty, "the shared case table is empty")
        for (locale, count, category) in pluralCases {
            XCTAssertEqual(
                I18nKt.pluralCategory(locale: locale, count: count), category,
                "\(locale)/\(count)"
            )
        }
    }

    func testEveryCaseLandsInACategoryTheLocaleOffers() {
        for (locale, _, category) in pluralCases {
            XCTAssertTrue(
                I18nKt.pluralCategories(locale: locale).contains(category),
                "\(locale) does not offer \(category)"
            )
        }
    }

    func testACountRendersInTheCategoryItsLanguageUses() {
        let en = I18nKt.bundledStrings(locale: "en")
        XCTAssertEqual(en.tPlural(base: SildKeys.Plural.widgetHomeAgentsOnline, count: 1, vars: nil),
                       "1 agent online")
        XCTAssertEqual(en.tPlural(base: SildKeys.Plural.widgetHomeAgentsOnline, count: 3, vars: nil),
                       "3 agents online")
    }

    // Latvian's zero form covers 0 and the teens, which English has no key for at all.
    func testLatvianRendersItsOwnZeroForm() {
        let lv = I18nKt.bundledStrings(locale: "lv")
        let zero = lv.tPlural(base: SildKeys.Plural.widgetHomeAgentsOnline, count: 0, vars: nil)
        XCTAssertFalse(zero.isEmpty)
        XCTAssertFalse(zero.contains("agent"), "lv fell back to English: \(zero)")
        XCTAssertEqual(lv.tPlural(base: SildKeys.Plural.widgetHomeAgentsOnline, count: 11, vars: nil),
                       zero.replacingOccurrences(of: "0", with: "11"))
    }
}
