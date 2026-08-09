import { makeAutoObservable, runInAction } from "mobx";
import {
  adminApi,
  type ApiTranslationKey,
  type ApiTranslationProject,
  type ApiTranslationRelease,
  type TranslationKeyParams,
  type TranslationStateFilter,
} from "@/api/admin";
import { errorText } from "@/api/client";

const PAGE_SIZE = 50;
// Long enough that a word being typed is one request, short enough that a save
// lands without the field being left.
const SAVE_DELAY = 500;

export type SaveState = "saving" | "saved" | "failed";

type PageOf<T> = { items: T[]; next_cursor: string | null; has_more: boolean };
type FetchPage<T> = (cursor: string | null) => Promise<PageOf<T>>;

/** One cursor-paged list: page one replaces, further pages append. */
class Paged<T> {
  items: T[] = [];
  cursor: string | null = null;
  hasMore = false;
  loading = false;
  loadingMore = false;

  constructor(private idOf: (item: T) => string | number) {
    makeAutoObservable<Paged<T>, "idOf">(this, { idOf: false });
  }

  clear = () => {
    this.items = [];
    this.cursor = null;
    this.hasMore = false;
  };

  // `stale` lets the caller drop a response for a filter it has already replaced.
  load = async (fetch: FetchPage<T>, stale: () => boolean = () => false) => {
    runInAction(() => (this.loading = true));
    try {
      const page = await fetch(null);
      if (stale()) return;
      runInAction(() => {
        this.items = page.items;
        this.cursor = page.next_cursor;
        this.hasMore = page.has_more;
      });
    } finally {
      if (!stale()) runInAction(() => (this.loading = false));
    }
  };

  loadMore = async (fetch: FetchPage<T>, stale: () => boolean = () => false) => {
    if (this.loading || this.loadingMore || !this.hasMore || !this.cursor) return;
    const cursor = this.cursor;
    runInAction(() => (this.loadingMore = true));
    try {
      const page = await fetch(cursor);
      if (stale()) return;
      runInAction(() => {
        const have = new Set(this.items.map(this.idOf));
        for (const item of page.items) if (!have.has(this.idOf(item))) this.items.push(item);
        this.cursor = page.next_cursor;
        this.hasMore = page.has_more;
      });
    } catch {
      /* transient; scrolling again retries */
    } finally {
      runInAction(() => (this.loadingMore = false));
    }
  };
}

export class TranslationsStore {
  projects: ApiTranslationProject[] = [];
  projectId = "";
  locale = "";
  stateFilter: TranslationStateFilter = "";
  namespace = "";
  search = "";
  /** In flight for a project create/delete, which replaces the whole list. */
  savingProject = false;

  keyList = new Paged<ApiTranslationKey>((k) => k.key);
  releaseList = new Paged<ApiTranslationRelease>((r) => r.version);
  publishing = false;

  loaded = false;
  error: string | null = null;

  /** Text being edited, by project+locale+key — the field shows this until the PUT settles. */
  drafts = new Map<string, string>();
  /** Per-row write state, so a row can say saving/saved without a save-all bar. */
  saves = new Map<string, SaveState>();

  private keysSeq = 0;
  private searchTimer: ReturnType<typeof setTimeout> | null = null;
  private saveTimers = new Map<string, ReturnType<typeof setTimeout>>();

  constructor() {
    makeAutoObservable<TranslationsStore, "saveTimers" | "searchTimer">(this, {
      saveTimers: false,
      searchTimer: false,
      keyList: false,
      releaseList: false,
    });
  }

  get rows(): ApiTranslationKey[] {
    return this.keyList.items;
  }

  get loading(): boolean {
    return this.keyList.loading;
  }

  get loadingMore(): boolean {
    return this.keyList.loadingMore;
  }

  get releases(): ApiTranslationRelease[] {
    return this.releaseList.items;
  }

  get releasesHasMore(): boolean {
    return this.releaseList.hasMore;
  }

  get project(): ApiTranslationProject | null {
    return this.projects.find((p) => p.id === this.projectId) ?? this.projects[0] ?? null;
  }

  get enabledLocales(): string[] {
    return this.project?.locales ?? [];
  }

  get availableLocales(): string[] {
    return this.project?.available_locales ?? [];
  }

  get currentVersion(): number | null {
    return this.project?.current_version ?? null;
  }

  // ─────────────────────────── load ───────────────────────────
  load = async () => {
    if (this.loaded) return;
    try {
      const projects = await adminApi.listTranslationProjects();
      runInAction(() => {
        this.projects = projects;
        this.loaded = true;
        const first = projects[0];
        if (!first) return;
        this.projectId = first.id;
        this.locale = first.locales[0] ?? first.fallback_locale;
      });
      if (!this.project) return;
      await Promise.all([this.loadKeys(), this.loadReleases()]);
    } catch (e) {
      runInAction(() => {
        this.loaded = true;
        this.error = errorText(e);
      });
    }
  };

  get namespaces(): string[] {
    return this.project?.namespaces ?? [];
  }

  /** True for a project whose keys the tenant declares — the platform's are the repo's. */
  get ownProject(): boolean {
    return !!this.project && !this.project.platform;
  }

  completionOf(locale: string): number {
    return this.project?.completion?.[locale] ?? 0;
  }

  private get keyParams(): TranslationKeyParams {
    return {
      locale: this.locale,
      state: this.stateFilter || undefined,
      namespace: this.namespace || undefined,
      q: this.search.trim() || undefined,
      limit: PAGE_SIZE,
    };
  }

  private fetchKeys = (project: string): FetchPage<ApiTranslationKey> =>
    (cursor) => adminApi.listTranslationKeys(project, { ...this.keyParams, cursor });

  // Reload page one for the current locale + filters. Bumps keysSeq so a slow
  // response for a filter the user has already replaced is dropped.
  loadKeys = async () => {
    const project = this.project;
    if (!project || !this.locale) return;
    const seq = ++this.keysSeq;
    const stale = () => seq !== this.keysSeq;
    runInAction(() => (this.error = null));
    try {
      await this.keyList.load(this.fetchKeys(project.id), stale);
      if (stale()) return;
      runInAction(() => {
        this.drafts.clear();
        this.saves.clear();
      });
    } catch (e) {
      if (stale()) return;
      runInAction(() => {
        this.keyList.clear();
        this.error = errorText(e);
      });
    }
  };

  loadMoreKeys = async () => {
    const project = this.project;
    if (!project || !this.locale) return;
    const seq = this.keysSeq;
    await this.keyList.loadMore(this.fetchKeys(project.id), () => seq !== this.keysSeq);
  };

  // ─────────────────────────── filters ───────────────────────────
  setLocale = (locale: string) => {
    if (this.locale === locale) return;
    this.locale = locale;
    void this.loadKeys();
  };

  setStateFilter = (state: TranslationStateFilter) => {
    if (this.stateFilter === state) return;
    this.stateFilter = state;
    void this.loadKeys();
  };

  setNamespace = (namespace: string) => {
    if (this.namespace === namespace) return;
    this.namespace = namespace;
    void this.loadKeys();
  };

  setProject = (id: string) => {
    if (this.projectId === id) return;
    this.projectId = id;
    const project = this.project;
    if (!project) return;
    // Each project has its own languages and namespaces; carrying the old filters
    // over would ask for a locale this one may not offer.
    this.locale = project.locales.includes(this.locale)
      ? this.locale
      : project.locales[0] ?? project.fallback_locale;
    this.namespace = "";
    this.keyList.clear();
    this.releaseList.clear();
    void Promise.all([this.loadKeys(), this.loadReleases()]);
  };

  setSearch = (q: string) => {
    this.search = q;
    if (this.searchTimer) clearTimeout(this.searchTimer);
    this.searchTimer = setTimeout(() => void this.loadKeys(), 280);
  };

  // ─────────────────────────── editing ───────────────────────────
  draftValue = (row: ApiTranslationKey): string =>
    this.drafts.get(draftId(this.projectId, this.locale, row.key)) ?? row.value;

  saveStateOf = (key: string): SaveState | null =>
    this.saves.get(draftId(this.projectId, this.locale, key)) ?? null;

  // Typing schedules the write; there is no save-all, so leaving the field must
  // never be what commits it. The project and locale are captured here: by the
  // time the timer fires the user may be looking at another one, and the text
  // belongs to the one it was typed in.
  setValue = (key: string, value: string) => {
    const project = this.projectId;
    const locale = this.locale;
    const id = draftId(project, locale, key);
    this.drafts.set(id, value);
    const pending = this.saveTimers.get(id);
    if (pending) clearTimeout(pending);
    this.saveTimers.set(id, setTimeout(() => void this.saveValue(project, locale, key), SAVE_DELAY));
  };

  /** Commit a scheduled edit now — on blur or Enter. */
  flushValue = (key: string) => {
    const project = this.projectId;
    const locale = this.locale;
    const id = draftId(project, locale, key);
    const pending = this.saveTimers.get(id);
    if (!pending) return;
    clearTimeout(pending);
    this.saveTimers.delete(id);
    void this.saveValue(project, locale, key);
  };

  private saveValue = async (projectId: string, locale: string, key: string) => {
    const project = this.projects.find((p) => p.id === projectId);
    const id = draftId(projectId, locale, key);
    const value = this.drafts.get(id);
    if (!project || !locale || value === undefined) return;
    this.saveTimers.delete(id);
    runInAction(() => this.saves.set(id, "saving"));
    try {
      await adminApi.setTranslationValue(project.id, key, locale, value);
      runInAction(() => {
        // Typing carried on while this was in flight; that write owns the outcome.
        if (this.drafts.get(id) !== value) return;
        this.drafts.delete(id);
        this.saves.set(id, "saved");
        if (locale !== this.locale || projectId !== this.projectId) return;
        const row = this.rows.find((r) => r.key === key);
        if (row) {
          row.value = value;
          row.state = "custom";
        }
      });
    } catch (e) {
      runInAction(() => {
        this.saves.set(id, "failed");
        this.error = errorText(e);
      });
    }
  };

  resetValue = async (key: string) => {
    const project = this.project;
    const locale = this.locale;
    if (!project || !locale) return;
    const id = draftId(project.id, locale, key);
    const pending = this.saveTimers.get(id);
    if (pending) {
      clearTimeout(pending);
      this.saveTimers.delete(id);
    }
    runInAction(() => {
      this.drafts.delete(id);
      this.saves.set(id, "saving");
    });
    try {
      await adminApi.resetTranslationValue(project.id, key, locale);
    } catch (e) {
      runInAction(() => {
        this.saves.set(id, "failed");
        this.error = errorText(e);
      });
      return;
    }
    runInAction(() => this.saves.set(id, "saved"));
    await this.refreshRow(project.id, locale, key);
  };

  // Only the server knows the shipped default, so re-read the one row rather
  // than the page — replacing the page would drop everything scrolled in.
  private refreshRow = async (project: string, locale: string, key: string) => {
    try {
      const page = await adminApi.listTranslationKeys(project, { locale, q: key, limit: PAGE_SIZE });
      const fresh = page.items.find((r) => r.key === key);
      runInAction(() => {
        if (!fresh || locale !== this.locale || project !== this.projectId) return;
        const row = this.rows.find((r) => r.key === key);
        if (row) Object.assign(row, fresh);
      });
    } catch {
      /* transient; the next filter change re-reads the page */
    }
  };

  // ─────────────────────────── projects ───────────────────────────
  createProject = async (id: string, name: string) => {
    if (this.savingProject) return false;
    runInAction(() => {
      this.savingProject = true;
      this.error = null;
    });
    try {
      const created = await adminApi.createTranslationProject(id.trim(), name.trim());
      runInAction(() => this.projects.push(created));
      this.setProject(created.id);
      return true;
    } catch (e) {
      runInAction(() => (this.error = errorText(e)));
      return false;
    } finally {
      runInAction(() => (this.savingProject = false));
    }
  };

  deleteProject = async (id: string) => {
    if (this.savingProject) return;
    runInAction(() => {
      this.savingProject = true;
      this.error = null;
    });
    try {
      await adminApi.deleteTranslationProject(id);
      runInAction(() => (this.projects = this.projects.filter((p) => p.id !== id)));
      // The platform project is in every tenant, so there is always one left.
      this.setProject(this.projects[0]?.id ?? "");
    } catch (e) {
      runInAction(() => (this.error = errorText(e)));
    } finally {
      runInAction(() => (this.savingProject = false));
    }
  };

  // ─────────────────────────── declared keys ───────────────────────────
  declareKey = async (key: string, source: string) => {
    const project = this.project;
    if (!project) return false;
    runInAction(() => (this.error = null));
    try {
      await adminApi.declareTranslationKey(project.id, key.trim(), source.trim());
    } catch (e) {
      runInAction(() => (this.error = errorText(e)));
      return false;
    }
    // The key set moved, so the completion figures and the page both have to.
    await Promise.all([this.reloadProject(), this.loadKeys()]);
    return true;
  };

  undeclareKey = async (key: string) => {
    const project = this.project;
    if (!project) return;
    runInAction(() => (this.error = null));
    try {
      await adminApi.undeclareTranslationKey(project.id, key);
    } catch (e) {
      runInAction(() => (this.error = errorText(e)));
      return;
    }
    await Promise.all([this.reloadProject(), this.loadKeys()]);
  };

  // Only the server counts completion and namespaces, so a key or value write
  // re-reads the project rather than guessing at the new figures.
  private reloadProject = async () => {
    try {
      const projects = await adminApi.listTranslationProjects();
      runInAction(() => (this.projects = projects));
    } catch {
      /* transient; the next filter change re-reads it */
    }
  };

  // ─────────────────────────── project settings ───────────────────────────
  setFallbackLocale = (locale: string) => this.saveSettings({ fallback_locale: locale });

  setAutoPublish = (on: boolean) => this.saveSettings({ auto_publish: on });

  /** Offer a language Sild does not ship. Any BCP-47 tag is storable, so the set
   *  grows without a release. */
  addLocale = async (tag: string) => {
    const project = this.project;
    const locale = tag.trim().toLowerCase().split(/[-_]/)[0];
    if (!project || !/^[a-z]{2,3}$/.test(locale)) {
      runInAction(() => (this.error = "A language is a two- or three-letter code, like fi."));
      return false;
    }
    if (project.locales.includes(locale)) return true;
    await this.saveSettings({ locales: [...project.locales, locale] });
    return !this.error;
  };

  toggleLocale = (locale: string, on: boolean) => {
    const project = this.project;
    if (!project) return;
    // A tenant cannot fall back to a language they have turned off.
    if (!on && locale === project.fallback_locale) return;
    void this.saveSettings({
      locales: on ? [...project.locales, locale] : project.locales.filter((l) => l !== locale),
    });
  };

  rename = (name: string) => this.saveSettings({ name });

  private saveSettings = async (
    change: Partial<{ name: string; fallback_locale: string; auto_publish: boolean; locales: string[] }>
  ) => {
    const project = this.project;
    if (!project) return;
    const prev = { ...project };
    runInAction(() => Object.assign(project, change));
    try {
      await adminApi.saveTranslationProject(project.id, {
        name: project.name,
        fallback_locale: project.fallback_locale,
        auto_publish: project.auto_publish,
        locales: project.locales,
      });
      // The selected locale may have just been turned off.
      if (!project.locales.includes(this.locale)) this.setLocale(project.locales[0] ?? project.fallback_locale);
      // Turning a language on adds a column to the completion figures.
      await this.reloadProject();
    } catch (e) {
      runInAction(() => {
        Object.assign(project, prev);
        this.error = errorText(e);
      });
    }
  };

  // ─────────────────────────── releases ───────────────────────────
  loadReleases = async () => {
    const project = this.project;
    if (!project) return;
    try {
      await this.releaseList.load((cursor) => adminApi.listTranslationReleases(project.id, cursor));
    } catch {
      /* the next publish reloads the list */
    }
  };

  loadMoreReleases = async () => {
    const project = this.project;
    if (!project) return;
    await this.releaseList.loadMore((cursor) => adminApi.listTranslationReleases(project.id, cursor));
  };

  publish = () => this.runRelease((id) => adminApi.publishTranslationRelease(id));

  rollback = (version: number) => this.runRelease((id) => adminApi.rollbackTranslationRelease(id, version));

  // Publish and rollback both cut a version, so both refresh the same three things.
  private runRelease = async (fn: (projectId: string) => Promise<{ version: number }>) => {
    const project = this.project;
    if (!project || this.publishing) return;
    runInAction(() => {
      this.publishing = true;
      this.error = null;
    });
    try {
      const released = await fn(project.id);
      runInAction(() => (project.current_version = released.version));
      await Promise.all([this.loadReleases(), this.loadKeys(), this.reloadProject()]);
    } catch (e) {
      runInAction(() => (this.error = errorText(e)));
    } finally {
      runInAction(() => (this.publishing = false));
    }
  };

  reset = () => {
    for (const t of this.saveTimers.values()) clearTimeout(t);
    this.saveTimers.clear();
    if (this.searchTimer) clearTimeout(this.searchTimer);
    this.projects = [];
    this.projectId = "";
    this.namespace = "";
    this.keyList.clear();
    this.releaseList.clear();
    this.drafts.clear();
    this.saves.clear();
    this.loaded = false;
    this.error = null;
  };
}

// Neither a project id nor a locale contains a newline, so this triple cannot
// collide with another.
function draftId(project: string, locale: string, key: string): string {
  return `${project}\n${locale}\n${key}`;
}
