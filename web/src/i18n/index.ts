import type { TranslationBundle, TranslationManifest } from "../core/types";
import {
  DEFAULTS,
  LOCALES,
  PLURAL_DEFAULT_FAMILY,
  PLURAL_FAMILIES,
  PLURAL_LOCALES,
  SOURCE_LOCALE,
} from "./catalog.generated";
import type { PluralKey, StringKey } from "./catalog.generated";

export type { PluralKey, StringKey };

/** The reserved project Sild's own strings live in. */
export const PLATFORM_PROJECT = "sild";

export type Vars = Record<string, string | number>;
export type Translate = (key: StringKey, vars?: Vars) => string;
/** A plural is addressed by its base key and a count, never by a sibling. */
export type TranslatePlural = (base: PluralKey, count: number, vars?: Vars) => string;

/** The category every language has, and the last readable text before giving up. */
const OTHER = "other";

/** Which categories a language has — Latvian's zero, Estonian's lack of one. */
export function pluralCategories(locale: string): string[] {
  const family = PLURAL_LOCALES[normalize(locale)] || PLURAL_DEFAULT_FAMILY;
  return PLURAL_FAMILIES[family] || PLURAL_FAMILIES[PLURAL_DEFAULT_FAMILY];
}

/** The plural category for a count. Integers only, per ADR 0004. The families
 *  here are the ones backend/internal/i18n/plurals.json names. */
export function pluralCategory(locale: string, count: number): string {
  const n = Math.abs(Math.trunc(count));
  const m10 = n % 10;
  const m100 = n % 100;
  switch (PLURAL_LOCALES[normalize(locale)] || PLURAL_DEFAULT_FAMILY) {
    case "other":
      return OTHER;
    case "hindi":
      return n <= 1 ? "one" : OTHER;
    case "romance":
    case "romance_zero":
      if (n === 1) return "one";
      if (n === 0 && (PLURAL_LOCALES[normalize(locale)] || "") === "romance_zero") return "one";
      // CLDR gives Romance a "many" for round millions, which is what a compact
      // "2 million" reads as.
      if (n !== 0 && n % 1_000_000 === 0) return "many";
      return OTHER;
    case "czech":
      if (n === 1) return "one";
      return n >= 2 && n <= 4 ? "few" : OTHER;
    case "romanian":
      if (n === 1) return "one";
      return n === 0 || (m100 >= 1 && m100 <= 19) ? "few" : OTHER;
    case "lithuanian":
      if (m100 >= 11 && m100 <= 19) return OTHER;
      if (m10 === 1) return "one";
      return m10 >= 2 && m10 <= 9 ? "few" : OTHER;
    case "latvian":
      if (m10 === 0 || (m100 >= 11 && m100 <= 19)) return "zero";
      return m10 === 1 && m100 !== 11 ? "one" : OTHER;
    case "hebrew":
      if (n === 1) return "one";
      return n === 2 ? "two" : OTHER;
    case "slovenian":
      if (m100 === 1) return "one";
      if (m100 === 2) return "two";
      return m100 === 3 || m100 === 4 ? "few" : OTHER;
    case "arabic":
      if (n === 0) return "zero";
      if (n === 1) return "one";
      if (n === 2) return "two";
      if (m100 >= 3 && m100 <= 10) return "few";
      return m100 >= 11 && m100 <= 99 ? "many" : OTHER;
    case "welsh":
      if (n === 0) return "zero";
      if (n === 1) return "one";
      if (n === 2) return "two";
      if (n === 3) return "few";
      return n === 6 ? "many" : OTHER;
    case "irish":
      if (n === 1) return "one";
      if (n === 2) return "two";
      if (n >= 3 && n <= 6) return "few";
      return n >= 7 && n <= 10 ? "many" : OTHER;
    case "maltese":
      if (n === 1) return "one";
      if (n === 2) return "two";
      if (n === 0 || (m100 >= 3 && m100 <= 10)) return "few";
      return m100 >= 11 && m100 <= 19 ? "many" : OTHER;
    case "polish":
      // Polish's one is exactly one, where Russian's is any number ending in it.
      if (n === 1) return "one";
      return m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14) ? "few" : "many";
    case "slavic":
      if (m10 === 1 && m100 !== 11) return "one";
      return m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14) ? "few" : "many";
    case "serbocroatian":
      if (m10 === 1 && m100 !== 11) return "one";
      return m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14) ? "few" : OTHER;
  }
  return n === 1 ? "one" : OTHER;
}

/** The client calls the i18n runtime needs — implemented by SildClient. */
export interface TranslationSource {
  /** Null when the held validator still matches (304). */
  fetchTranslationManifest(
    project: string,
    etag?: string
  ): Promise<{ manifest: TranslationManifest; etag: string } | null>;
  fetchTranslationBundle(project: string, locale: string, version: number): Promise<TranslationBundle>;
}

/** How long a held manifest is trusted before a foreground re-poll. */
const MANIFEST_TTL_MS = 60 * 60 * 1000;

// Sild ships flat languages, so "en-GB" and "en" are one locale.
export function normalize(tag: string): string {
  const t = (tag || "").trim();
  const i = t.search(/[-_]/);
  return (i > 0 ? t.slice(0, i) : t).toLowerCase();
}

/** The caller's most preferred locale that is actually offered, or "" if none. */
export function negotiate(prefs: string[], offered: string[]): string {
  const have = new Set(offered.map(normalize));
  for (const p of prefs) {
    const n = normalize(p);
    if (have.has(n)) return n;
  }
  return "";
}

function interpolate(text: string, vars?: Vars): string {
  if (!vars) return text;
  return text.replace(/\{(\w+)\}/g, (m, name: string) => (name in vars ? String(vars[name]) : m));
}

function devicePrefs(): string[] {
  if (typeof navigator === "undefined") return [];
  return navigator.languages?.length ? [...navigator.languages] : [navigator.language || ""];
}

interface Saved {
  version: number;
  strings: Record<string, string>;
}

/** The last manifest seen, with the validator that revalidates it. */
interface Held {
  etag: string;
  at: number;
  fallback: string;
  locales: Record<string, number>;
}

function storageKey(appId: string, project: string, locale: string): string {
  return `sild_i18n_${appId}_${project}_${locale}`;
}

// The manifest covers every locale, so it is held once per project, not per locale.
const MANIFEST_SLOT = "@manifest";

// Without a tenant there is no key that can only be this tenant's, and two sites
// on one origin would read each other's wording — so nothing is cached at all.
function readSlot<T>(appId: string, project: string, slot: string): T | null {
  if (!appId) return null;
  try {
    const raw = window.localStorage.getItem(storageKey(appId, project, slot));
    return raw ? (JSON.parse(raw) as T) : null;
  } catch {
    return null;
  }
}

function writeSlot(appId: string, project: string, slot: string, value: unknown): void {
  if (!appId) return;
  try {
    window.localStorage.setItem(storageKey(appId, project, slot), JSON.stringify(value));
  } catch {
    /* storage unavailable — the next start falls back to the bundled defaults */
  }
}

function readSaved(appId: string, project: string, locale: string): Saved | null {
  const saved = readSlot<Saved>(appId, project, locale);
  return saved && saved.strings ? saved : null;
}

function readHeld(appId: string, project: string): Held | null {
  const held = readSlot<Held>(appId, project, MANIFEST_SLOT);
  return held && held.etag && held.locales ? held : null;
}

export interface I18nOptions {
  /** Explicit locale from the host's init config; wins over the device's. */
  locale?: string;
  project?: string;
  /** Tenant the cached bundle belongs to; omit it and nothing is persisted. */
  appId?: string;
  /** Omit for surfaces with no backend (the Appearance preview): bundled text only. */
  source?: TranslationSource;
  /** Called when a key resolves to nothing at all — a key no locale declares. */
  onMissingKey?: (key: string) => void;
  /** Show the key itself for an unresolved lookup. Off ships a blank instead, so
   *  a customer never reads a dotted key. */
  debug?: boolean;
}

/** The widget's active locale and strings. Renders from bundled defaults (or a
 *  previously fetched bundle) at once; published overrides arrive via refresh(). */
export class I18n {
  locale: string;
  private project: string;
  private appId: string;
  private source?: TranslationSource;
  private strings: Record<string, string>;
  private version = 0;
  // True when the host named the locale: an instruction, not a guess, so the
  // tenant's offering never overrides it.
  private explicit: boolean;
  private held: Held | null;
  private listeners = new Set<() => void>();
  private onMissingKey?: (key: string) => void;
  private debug: boolean;

  constructor(opts: I18nOptions = {}) {
    this.project = opts.project || PLATFORM_PROJECT;
    this.appId = opts.appId || "";
    this.source = opts.source;
    this.onMissingKey = opts.onMissingKey;
    this.debug = !!opts.debug;
    const named = opts.locale ? normalize(opts.locale) : "";
    // Honoured even when Sild ships no bundled text for it: the tenant may have
    // added the language, and its published bundle is what renders.
    this.explicit = !!named;
    this.locale = (this.explicit ? named : negotiate(devicePrefs(), LOCALES)) || SOURCE_LOCALE;
    this.strings = {};
    this.held = readHeld(this.appId, this.project);
    this.load();
  }

  t: Translate = (key, vars) => interpolate(this.lookup(key), vars);

  /** The text for a count, from the category this language uses for it. */
  tPlural: TranslatePlural = (base, count, vars) => {
    const category = pluralCategory(this.locale, count);
    const text = this.lookup(`${base}.${category}`, `${base}.${OTHER}`);
    return interpolate(text, { count, ...vars });
  };

  // lookup walks the published bundle, then this language's bundled text, then the
  // source language's. A key that resolves nowhere is a programming error: it is
  // reported, and only a debug build puts it on screen.
  private lookup(key: string, alt?: string): string {
    const text = this.text(key) ?? (alt !== undefined ? this.text(alt) : undefined);
    if (text !== undefined) return text;
    this.onMissingKey?.(key);
    return this.debug ? key : "";
  }

  private text(key: string): string | undefined {
    return this.strings[key] ?? DEFAULTS[this.locale]?.[key] ?? DEFAULTS[SOURCE_LOCALE][key];
  }

  subscribe(fn: () => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  /** Fetch the current published bundle for the active locale and apply it. Never
   *  throws: a failed or offline fetch leaves the text already on screen. */
  async refresh(): Promise<void> {
    const source = this.source;
    if (!source) return;
    try {
      const res = await source.fetchTranslationManifest(this.project, this.held?.etag);
      const held = res ? { etag: res.etag, manifest: res.manifest } : this.revalidated();
      if (!held) return;
      this.hold(held.etag, held.manifest);
      this.adopt(held.manifest);
      const version = held.manifest.locales?.[this.locale];
      if (!version || version === this.version) return;
      const bundle = await source.fetchTranslationBundle(this.project, this.locale, version);
      this.apply(version, bundle.strings);
    } catch {
      /* keep the bundled/persisted text */
    }
  }

  /** refresh() only once the held manifest has aged out — the foreground poll. */
  async refreshIfStale(): Promise<void> {
    if (this.held && Date.now() - this.held.at < MANIFEST_TTL_MS) return;
    await this.refresh();
  }

  private revalidated(): { etag: string; manifest: TranslationManifest } | null {
    const h = this.held;
    if (!h) return null;
    return {
      etag: h.etag,
      manifest: { project: this.project, fallback_locale: h.fallback, locales: h.locales },
    };
  }

  private hold(etag: string, manifest: TranslationManifest): void {
    const held: Held = {
      etag,
      at: Date.now(),
      fallback: manifest.fallback_locale || "",
      locales: manifest.locales || {},
    };
    this.held = held;
    writeSlot(this.appId, this.project, MANIFEST_SLOT, held);
  }

  private load(): void {
    const saved = readSaved(this.appId, this.project, this.locale);
    this.version = saved?.version || 0;
    this.strings = saved?.strings || { ...DEFAULTS[this.locale] };
  }

  private apply(version: number, strings: Record<string, string>): void {
    this.version = version;
    this.strings = strings;
    writeSlot(this.appId, this.project, this.locale, { version, strings });
    this.emit();
  }

  // Which locales the tenant offers is unknowable until the manifest arrives, so a
  // guess negotiated against the repo catalog is re-negotiated against the offering.
  private adopt(manifest: TranslationManifest): void {
    // Whatever the tenant publishes is on offer, including a language Sild ships
    // no bundled text for — its bundle carries every key.
    const offered = Object.keys(manifest.locales || {});
    if (this.explicit || !offered.length || offered.includes(this.locale)) return;
    const locale = negotiate(devicePrefs(), offered) || normalize(manifest.fallback_locale || "");
    if (!offered.includes(locale)) return;
    this.locale = locale;
    this.load();
    this.emit();
  }

  private emit(): void {
    for (const l of this.listeners) l();
  }
}
