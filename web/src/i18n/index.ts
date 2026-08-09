import type { TranslationBundle, TranslationManifest } from "../core/types";
import { DEFAULTS, LOCALES, SOURCE_LOCALE } from "./catalog.generated";

/** The reserved project Sild's own strings live in. */
export const PLATFORM_PROJECT = "sild";

export type Vars = Record<string, string | number>;
export type Translate = (key: string, vars?: Vars) => string;

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

  constructor(opts: I18nOptions = {}) {
    this.project = opts.project || PLATFORM_PROJECT;
    this.appId = opts.appId || "";
    this.source = opts.source;
    const named = opts.locale ? normalize(opts.locale) : "";
    this.explicit = !!named && LOCALES.includes(named);
    this.locale = (this.explicit ? named : negotiate(devicePrefs(), LOCALES)) || SOURCE_LOCALE;
    this.strings = {};
    this.held = readHeld(this.appId, this.project);
    this.load();
  }

  t: Translate = (key, vars) => {
    const text =
      this.strings[key] ?? DEFAULTS[this.locale]?.[key] ?? DEFAULTS[SOURCE_LOCALE][key] ?? key;
    return interpolate(text, vars);
  };

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
    const offered = Object.keys(manifest.locales || {}).filter((l) => LOCALES.includes(l));
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
