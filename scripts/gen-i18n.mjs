#!/usr/bin/env node
// Generates the widget's bundled string defaults from the canonical locale files.
//
//     node scripts/gen-i18n.mjs
//
// Output is deterministic (sorted locales, sorted keys) so re-running it on an
// unchanged input rewrites the same bytes.
import { readdir, readFile, mkdir, writeFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const SRC_DIR = join(ROOT, "backend/internal/i18n/locales");
const OUT = join(ROOT, "web/src/i18n/catalog.generated.ts");
const KOTLIN_OUT = join(ROOT, "sdks/kotlin/core/src/commonMain/kotlin/io/sild/core/I18nCatalog.generated.kt");
const SOURCE_LOCALE = "en";
const COMMAND = "make i18n";

const q = (s) => JSON.stringify(s);
// Kotlin reads `$` in a string literal as a template start, so it has to be escaped.
const k = (s) => JSON.stringify(s).replaceAll("$", "\\$");

async function main() {
  const files = (await readdir(SRC_DIR)).filter((f) => f.endsWith(".json")).sort();
  const byLocale = new Map();
  for (const file of files) {
    const locale = file.slice(0, -".json".length);
    byLocale.set(locale, JSON.parse(await readFile(join(SRC_DIR, file), "utf8")));
  }
  if (!byLocale.has(SOURCE_LOCALE)) throw new Error(`${SRC_DIR}/${SOURCE_LOCALE}.json is missing`);

  const locales = [...byLocale.keys()].sort();
  const lines = [
    `// Generated from backend/internal/i18n/locales by scripts/gen-i18n.mjs — run \`${COMMAND}\`. Do not edit.`,
    "",
    `export const SOURCE_LOCALE = ${q(SOURCE_LOCALE)};`,
    "",
    `export const LOCALES: string[] = [${locales.map(q).join(", ")}];`,
    "",
    "export const DEFAULTS: Record<string, Record<string, string>> = {",
  ];
  for (const locale of locales) {
    const strings = byLocale.get(locale);
    lines.push(`  ${q(locale)}: {`);
    for (const key of Object.keys(strings).sort()) lines.push(`    ${q(key)}: ${q(strings[key])},`);
    lines.push("  },");
  }
  lines.push("};", "");

  await mkdir(dirname(OUT), { recursive: true });
  await writeFile(OUT, lines.join("\n"), "utf8");
  console.log(`wrote ${OUT} (${locales.length} locales)`);

  const kt = [
    `// Generated from backend/internal/i18n/locales by scripts/gen-i18n.mjs — run \`${COMMAND}\`. Do not edit.`,
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
  kt.push(")", "");

  await mkdir(dirname(KOTLIN_OUT), { recursive: true });
  await writeFile(KOTLIN_OUT, kt.join("\n"), "utf8");
  console.log(`wrote ${KOTLIN_OUT} (${locales.length} locales)`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
