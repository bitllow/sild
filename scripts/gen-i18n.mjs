#!/usr/bin/env node
// Generates the clients' bundled string defaults, plural tables and typed key
// constants from the canonical locale files.
//
//     node scripts/gen-i18n.mjs
//
// Output is deterministic (sorted locales, sorted keys) so re-running it on an
// unchanged input rewrites the same bytes.
import { readdir, readFile, mkdir, writeFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const I18N_DIR = join(ROOT, "backend/internal/i18n");
const SRC_DIR = join(I18N_DIR, "locales");
const PLURALS = join(I18N_DIR, "plurals.json");
const PLURAL_CASES = join(I18N_DIR, "plural-cases.json");
const OUT = join(ROOT, "web/src/i18n/catalog.generated.ts");
const KOTLIN_OUT = join(ROOT, "sdks/kotlin/core/src/commonMain/kotlin/io/sild/core/I18nCatalog.generated.kt");
const SWIFT_OUT = join(ROOT, "sdks/swift/Sources/Sild/SildKeys.generated.swift");
const CASES_TS = join(ROOT, "web/src/i18n/plural-cases.generated.ts");
const CASES_KT = join(ROOT, "sdks/kotlin/core/src/commonTest/kotlin/io/sild/core/PluralCases.generated.kt");
const CASES_SWIFT = join(ROOT, "sdks/swift/Tests/SildTests/PluralCases.generated.swift");
const SOURCE_LOCALE = "en";
const COMMAND = "make i18n";

const q = (s) => JSON.stringify(s);
// Kotlin reads `$` in a string literal as a template start, so it has to be escaped.
const k = (s) => JSON.stringify(s).replaceAll("$", "\\$");
const banner = (comment) =>
  `${comment} Generated from backend/internal/i18n by scripts/gen-i18n.mjs — run \`${COMMAND}\`. Do not edit.`;

// A key's identifier: "widget.home.cta" → widgetHomeCta. Renaming a key removes the
// member, so a stale lookup is a build error rather than a blank label.
//
// The same rule as i18n.Ident in Go, which names a TENANT's exported keys — the two
// must agree, and a contract test holds them to each other.
function ident(key) {
  const parts = key.split(/[.\-_]/).filter(Boolean);
  const out = parts
    .map((p, i) => (i === 0 ? p : p[0].toUpperCase() + p.slice(1)))
    .join("");
  if (!out) return "key";
  if (/^[0-9]/.test(out) || RESERVED.has(out)) return "key" + out[0].toUpperCase() + out.slice(1);
  return out;
}

// What a constant must not be called in Kotlin, Swift or TypeScript.
const RESERVED = new Set(
  ("as break case catch class const continue default do else enum extension false for fun guard if " +
    "import in init interface internal is let new null nil object private protocol public return self " +
    "static struct super switch this throw true try typealias typeof val var void when where while")
    .split(" ")
);

// Which keys are plural: a base carrying more than one category sibling, which a
// key merely ending in a category word cannot be.
function pluralBases(keys, categories) {
  const cats = new Map();
  for (const key of keys) {
    const i = key.lastIndexOf(".");
    if (i <= 0) continue;
    const cat = key.slice(i + 1);
    if (!categories.has(cat)) continue;
    const base = key.slice(0, i);
    if (!cats.has(base)) cats.set(base, new Set());
    cats.get(base).add(cat);
  }
  return [...cats.entries()].filter(([, s]) => s.size > 1).map(([base]) => base).sort();
}

async function main() {
  const files = (await readdir(SRC_DIR)).filter((f) => f.endsWith(".json")).sort();
  const byLocale = new Map();
  for (const file of files) {
    const locale = file.slice(0, -".json".length);
    byLocale.set(locale, JSON.parse(await readFile(join(SRC_DIR, file), "utf8")));
  }
  if (!byLocale.has(SOURCE_LOCALE)) throw new Error(`${SRC_DIR}/${SOURCE_LOCALE}.json is missing`);

  const locales = [...byLocale.keys()].sort();
  const plurals = JSON.parse(await readFile(PLURALS, "utf8"));
  const families = Object.entries(plurals.families).sort(([a], [b]) => a.localeCompare(b));
  const pluralLocales = Object.entries(plurals.locales).sort(([a], [b]) => a.localeCompare(b));
  const everyCategory = new Set(families.flatMap(([, cats]) => cats));
  const sourceKeys = Object.keys(byLocale.get(SOURCE_LOCALE)).sort();
  const bases = pluralBases(sourceKeys, everyCategory);
  // A plural is addressed by its base and a count, never by a sibling key.
  const plainKeys = sourceKeys.filter((key) => {
    const i = key.lastIndexOf(".");
    return !(i > 0 && bases.includes(key.slice(0, i)));
  });

  await writeTypeScript({ locales, byLocale, families, pluralLocales, plurals, plainKeys, bases });
  await writeKotlin({ locales, byLocale, families, pluralLocales, plurals, plainKeys, bases });
  await writeSwift({ plainKeys, bases });
  await writeCases();
}

async function write(path, lines) {
  await mkdir(dirname(path), { recursive: true });
  await writeFile(path, lines.join("\n"), "utf8");
  console.log(`wrote ${path}`);
}

async function writeTypeScript({ locales, byLocale, families, pluralLocales, plurals, plainKeys, bases }) {
  const lines = [
    banner("//"),
    "",
    `export const SOURCE_LOCALE = ${q(SOURCE_LOCALE)};`,
    "",
    `export const LOCALES: string[] = [${locales.map(q).join(", ")}];`,
    "",
    "/** Every key a surface may look up: a typo or a renamed key is a type error. */",
    "export type StringKey =",
    ...plainKeys.map((key) => `  | ${q(key)}`),
    ";",
    "",
    "/** Keys addressed by a count, whose siblings carry the categories. */",
    "export type PluralKey =",
    ...(bases.length ? bases.map((base) => `  | ${q(base)}`) : ["  never"]),
    bases.length ? ";" : "",
    "",
    "export const DEFAULTS: Record<string, Record<string, string>> = {",
  ];
  for (const locale of locales) {
    const strings = byLocale.get(locale);
    lines.push(`  ${q(locale)}: {`);
    for (const key of Object.keys(strings).sort()) lines.push(`    ${q(key)}: ${q(strings[key])},`);
    lines.push("  },");
  }
  lines.push(
    "};",
    "",
    `export const PLURAL_DEFAULT_FAMILY = ${q(plurals.default)};`,
    "",
    "export const PLURAL_FAMILIES: Record<string, string[]> = {",
    ...families.map(([name, cats]) => `  ${q(name)}: [${cats.map(q).join(", ")}],`),
    "};",
    "",
    "export const PLURAL_LOCALES: Record<string, string> = {",
    ...pluralLocales.map(([locale, family]) => `  ${q(locale)}: ${q(family)},`),
    "};",
    ""
  );
  await write(OUT, lines);
}

async function writeKotlin({ locales, byLocale, families, pluralLocales, plurals, plainKeys, bases }) {
  const kt = [
    banner("//"),
    "package io.sild.core",
    "",
    `internal const val I18N_SOURCE_LOCALE: String = ${k(SOURCE_LOCALE)}`,
    "",
    `internal val I18N_LOCALES: List<String> = listOf(${locales.map(k).join(", ")})`,
    "",
    "internal val I18N_DEFAULTS: Map<String, Map<String, String>> = mapOf(",
  ];
  for (const locale of locales) {
    const strings = byLocale.get(locale);
    kt.push(`    ${k(locale)} to mapOf(`);
    for (const key of Object.keys(strings).sort()) kt.push(`        ${k(key)} to ${k(strings[key])},`);
    kt.push("    ),");
  }
  kt.push(
    ")",
    "",
    `internal const val I18N_PLURAL_DEFAULT_FAMILY: String = ${k(plurals.default)}`,
    "",
    "internal val I18N_PLURAL_FAMILIES: Map<String, List<String>> = mapOf(",
    ...families.map(([name, cats]) => `    ${k(name)} to listOf(${cats.map(k).join(", ")}),`),
    ")",
    "",
    "internal val I18N_PLURAL_LOCALES: Map<String, String> = mapOf(",
    ...pluralLocales.map(([locale, family]) => `    ${k(locale)} to ${k(family)},`),
    ")",
    "",
    "/** Every key Sild renders. A renamed key removes its member, so a stale lookup",
    " *  fails the build instead of reaching a customer as a raw key. */",
    "object SildKeys {",
    ...plainKeys.map((key) => `    const val ${ident(key)}: String = ${k(key)}`),
    "",
    "    /** Keys addressed by a count — see [SildI18n.tPlural]. */",
    "    object Plural {",
    ...bases.map((base) => `        const val ${ident(base)}: String = ${k(base)}`),
    "    }",
    "}",
    ""
  );
  await write(KOTLIN_OUT, kt);
}

async function writeSwift({ plainKeys, bases }) {
  const sw = [
    banner("//"),
    "",
    "/// Every key Sild renders. A renamed key removes its member, so a stale lookup",
    "/// fails the build instead of reaching a customer as a raw key.",
    "public enum SildKeys {",
    ...plainKeys.map((key) => `    public static let ${ident(key)} = ${q(key)}`),
    "",
    "    /// Keys addressed by a count.",
    "    public enum Plural {",
    ...bases.map((base) => `        public static let ${ident(base)} = ${q(base)}`),
    "    }",
    "}",
    "",
  ];
  await write(SWIFT_OUT, sw);
}

// The shared plural table, fanned out to the client test suites so Go,
// TypeScript, Kotlin and Swift all answer to the same cases.
async function writeCases() {
  const { cases } = JSON.parse(await readFile(PLURAL_CASES, "utf8"));
  const rows = cases.map((c) => [c.locale, c.count, c.category]);
  await write(CASES_TS, [
    banner("//"),
    "",
    "export const PLURAL_CASES: [string, number, string][] = [",
    ...rows.map(([l, n, cat]) => `  [${q(l)}, ${n}, ${q(cat)}],`),
    "];",
    "",
  ]);
  await write(CASES_KT, [
    banner("//"),
    "package io.sild.core",
    "",
    "internal val PLURAL_CASES: List<Triple<String, Int, String>> = listOf(",
    ...rows.map(([l, n, cat]) => `    Triple(${k(l)}, ${n}, ${k(cat)}),`),
    ")",
    "",
  ]);
  await write(CASES_SWIFT, [
    banner("//"),
    "",
    "let pluralCases: [(locale: String, count: Int32, category: String)] = [",
    ...rows.map(([l, n, cat]) => `    (${q(l)}, ${n}, ${q(cat)}),`),
    "]",
    "",
  ]);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
