#!/usr/bin/env node
// Regenerates the plural table and its case table from the platform's own CLDR
// data (Intl.PluralRules), so neither is a hand-maintained guess.
//
//     node scripts/gen-plurals.mjs
//
// Families are DISCOVERED, not invented: two languages that answer every probe
// identically are one family. The rule per family is still written by hand in each
// runtime — that is the part a table cannot carry — and plural-cases.json holds
// those rules to CLDR.
import { readFile, writeFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const TABLE = join(ROOT, "backend/internal/i18n/plurals.json");
const CASES = join(ROOT, "backend/internal/i18n/plural-cases.json");

// Every integer a rule can turn on: the small ones, the teens and twenties every
// Slavic rule keys off, and the powers of ten Romance "many" needs.
const probes = [];
for (let n = 0; n <= 200; n++) probes.push(n);
for (const n of [201, 221, 1000, 1001, 10_000, 100_000, 1e6, 1_500_000, 2e6, 1e7, 1e8, 1e9]) probes.push(n);

// The cases the four runtimes are held to. Wide enough to separate every family,
// short enough to read in a diff.
const caseCounts = [0, 1, 2, 3, 4, 5, 6, 7, 10, 11, 12, 14, 15, 19, 20, 21, 22, 25, 100, 101, 102, 111, 121, 1e6, 2e6];

async function main() {
  const current = JSON.parse(await readFile(TABLE, "utf8"));
  const locales = Object.keys(current.locales).sort();

  // A locale's behaviour is what it answers to every probe; identical answers are
  // one family.
  const signature = new Map();
  for (const locale of locales) {
    const rules = new Intl.PluralRules(locale);
    signature.set(locale, probes.map((n) => rules.select(n)).join(","));
  }

  const bySignature = new Map();
  for (const locale of locales) {
    const sig = signature.get(locale);
    if (!bySignature.has(sig)) bySignature.set(sig, []);
    bySignature.get(sig).push(locale);
  }

  // Keep the names already in the table where a family still holds the same
  // languages — the rule switches are written against them.
  const families = {};
  const assigned = {};
  const taken = new Set();
  // Biggest group first, so the family a name meant today keeps it and the smaller
  // one that had been folded into it wrongly gets its own.
  for (const [sig, members] of [...bySignature].sort((a, b) => b[1].length - a[1].length)) {
    const named = members.map((l) => current.locales[l]).filter(Boolean);
    const inherited = named.find((n) => members.every((l) => current.locales[l] === n) && !taken.has(n));
    const name = NAMES[members.join(" ")] || inherited;
    if (!name) {
      // Every family is a `case` in three hand-written rule switches. A new one
      // would fall through to one/other in all of them and only surface as a failing
      // case table later, so it stops here instead.
      throw new Error(
        `no rule is written for the family holding ${members.join(" ")} — ` +
          `add a case to plural.go, index.ts and I18n.kt, then name it in NAMES`
      );
    }
    taken.add(name);
    // Every language has "other": it is the last readable text a lookup falls to,
    // even where no integer selects it.
    const cats = new Set(sig.split(","));
    cats.add("other");
    families[name] = [...cats].sort(byCldrOrder);
    for (const l of members) assigned[l] = name;
  }

  const table = {
    _comment: current._comment,
    default: current.default,
    families: sortedByKey(families),
    locales: sortedByKey(assigned),
  };
  await writeFile(TABLE, format(table), "utf8");

  const cases = [];
  for (const locale of locales) {
    const rules = new Intl.PluralRules(locale);
    for (const count of caseCounts) cases.push({ locale, count, category: rules.select(count) });
  }
  const caseDoc = JSON.parse(await readFile(CASES, "utf8"));
  await writeFile(CASES, JSON.stringify({ _comment: caseDoc._comment, cases }, null, 2) + "\n", "utf8");

  console.log(`${Object.keys(families).length} families over ${locales.length} languages, ${cases.length} cases`);
  for (const [name, cats] of Object.entries(table.families)) {
    const members = locales.filter((l) => assigned[l] === name);
    console.log(`  ${name.padEnd(14)} [${cats.join(", ")}]  ${members.join(" ")}`);
  }
}

// A family is named for what it is, where discovery would otherwise name it after
// whichever member sorted first.
const NAMES = {
  "bs hr sr": "serbocroatian",
  "ca es it": "romance",
  "fr pt": "romance_zero",
  hi: "hindi",
};

const ORDER = ["zero", "one", "two", "few", "many", "other"];
const byCldrOrder = (a, b) => ORDER.indexOf(a) - ORDER.indexOf(b);

function sortedByKey(obj) {
  return Object.fromEntries(Object.keys(obj).sort().map((k) => [k, obj[k]]));
}

// One family or locale per line: a diff should read as which language moved.
function format(table) {
  const lines = ["{", `  "_comment": ${JSON.stringify(table._comment)},`, `  "default": ${JSON.stringify(table.default)},`, '  "families": {'];
  const fams = Object.entries(table.families);
  fams.forEach(([name, cats], i) => {
    lines.push(`    ${JSON.stringify(name)}: [${cats.map((c) => JSON.stringify(c)).join(", ")}]${i < fams.length - 1 ? "," : ""}`);
  });
  lines.push("  },", '  "locales": {');
  const locs = Object.entries(table.locales);
  locs.forEach(([locale, family], i) => {
    lines.push(`    ${JSON.stringify(locale)}: ${JSON.stringify(family)}${i < locs.length - 1 ? "," : ""}`);
  });
  lines.push("  }", "}");
  return lines.join("\n") + "\n";
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
