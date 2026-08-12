"use client";

import type * as React from "react";
import { useState } from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Badge, Button, Input, SearchIcon, Select, Switch } from "@/components/ds";
import type {
  ApiTranslationKey,
  ApiTranslationState,
  TranslationFormat,
  TranslationStateFilter,
} from "@/api/admin";
import type { TranslationsStore } from "@/store/translations";
import { cardStyle as card, fieldLabel, rowBorder } from "./styles";

const STATE_FILTERS: { value: TranslationStateFilter; label: string }[] = [
  { value: "", label: "All strings" },
  { value: "missing", label: "Untranslated" },
  { value: "custom", label: "Customised" },
  { value: "needs_review", label: "Needs review" },
];

const STATE_LABEL: Record<ApiTranslationState, string> = {
  default: "default",
  custom: "custom",
  needs_review: "needs review",
};

const STATE_VARIANT: Record<ApiTranslationState, "neutral" | "brand" | "warning"> = {
  default: "neutral",
  custom: "brand",
  needs_review: "warning",
};

// Native resource files are an import path for migration and an export path for a
// build that still feeds itself — never how Sild delivers text (ADR 0003).
const IMPORT_FORMATS: { value: TranslationFormat; label: string }[] = [
  { value: "json", label: "JSON" },
  { value: "csv", label: "CSV" },
  { value: "android", label: "strings.xml" },
  { value: "ios", label: ".strings" },
  { value: "ios-plurals", label: ".stringsdict" },
];

const EXPORT_FORMATS: { value: TranslationFormat; label: string }[] = [
  ...IMPORT_FORMATS,
  { value: "sheet", label: "Spreadsheet (key, source, translation)" },
];

// Panel is the card chrome every section on this screen shares: a titled header
// with a line of explanation, and the section's own rows beneath it.
function Panel({
  title,
  subtitle,
  right,
  testid,
  style,
  children,
}: {
  title: string;
  subtitle?: React.ReactNode;
  right?: React.ReactNode;
  testid?: string;
  style?: React.CSSProperties;
  children?: React.ReactNode;
}) {
  return (
    <div style={{ ...card, marginTop: 20, ...style }} data-testid={testid}>
      <div style={{ padding: "14px 16px", borderBottom: rowBorder }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <div style={{ flex: 1, minWidth: 0, fontSize: 15, fontWeight: 700 }}>{title}</div>
          {right}
        </div>
        {subtitle && (
          <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>{subtitle}</div>
        )}
      </div>
      {children}
    </div>
  );
}

// Locale tags are BCP-47, so the platform already knows what to call them.
const localeNames = new Intl.DisplayNames(["en"], { type: "language" });
const localeLabel = (tag: string) => `${localeNames.of(tag) ?? tag} · ${tag}`;

export const Translations = observer(function Translations() {
  const t = useStore().translations;
  const project = t.project;

  return (
    <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", background: "var(--surface-page)" }}>
      <div style={{ padding: "22px 28px 18px", flex: "none", borderBottom: "1px solid var(--border-default)" }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
          <div>
            <h1 style={{ fontSize: 22 }}>Translations</h1>
            <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>
              {project ? `${project.keys} strings` : "Loading…"} — edits are drafts until you publish.
            </div>
          </div>
          <Release />
        </div>
        <Toolbar />
      </div>

      {t.error && (
        <div style={{ padding: "10px 28px", fontSize: 13, color: "var(--danger, #B3261E)" }}>{t.error}</div>
      )}

      <div style={{ flex: 1, minWidth: 0, display: "flex", overflow: "hidden" }}>
        <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", overflow: "hidden" }}>
          <DeclareKey />
          <KeyList />
        </div>
        <aside
          style={{
            width: 320,
            flex: "none",
            overflowY: "auto",
            padding: "20px 24px",
            borderLeft: "1px solid var(--border-default)",
          }}
        >
          <Projects />
          <Languages />
          <DraftChanges />
          <Releases />
          <Transfer />
          <FromCI />
          <Delivery />
        </aside>
      </div>
    </div>
  );
});

const Release = observer(function Release() {
  const store = useStore();
  const t = store.translations;
  const version = t.currentVersion;

  return (
    <div style={{ display: "flex", alignItems: "center", gap: 12, flex: "none" }}>
      <span data-testid="translations-version" style={{ fontSize: 13, color: "var(--text-secondary)", whiteSpace: "nowrap" }}>
        {version === null ? "Not published" : `Published v${version}`}
      </span>
      {store.can("translations.publish") && (
        <Button data-testid="translations-publish" onClick={t.publish} loading={t.publishing} disabled={!t.project}>
          Publish
        </Button>
      )}
    </div>
  );
});

const Toolbar = observer(function Toolbar() {
  const t = useStore().translations;

  return (
    <div style={{ display: "flex", alignItems: "center", gap: 12, marginTop: 16, flexWrap: "wrap" }}>
      {t.projects.length > 1 && (
        <div style={{ width: 200, flex: "none" }}>
          <Select
            data-testid="translations-project"
            aria-label="Project"
            size="sm"
            value={t.project?.id ?? ""}
            options={t.projects.map((p) => ({ value: p.id, label: p.name }))}
            onChange={(e) => t.setProject(e.target.value)}
          />
        </div>
      )}
      <div style={{ width: 220, flex: "none" }}>
        <Select
          data-testid="translations-locale"
          aria-label="Locale"
          size="sm"
          value={t.locale}
          options={t.enabledLocales.map((l) => ({ value: l, label: localeLabel(l) }))}
          onChange={(e) => t.setLocale(e.target.value)}
        />
      </div>
      <div style={{ width: 180, flex: "none" }}>
        <Select
          data-testid="translations-filter"
          aria-label="State"
          size="sm"
          value={t.stateFilter}
          options={STATE_FILTERS}
          onChange={(e) => t.setStateFilter(e.target.value as TranslationStateFilter)}
        />
      </div>
      {t.namespaces.length > 1 && (
        <div style={{ width: 170, flex: "none" }}>
          <Select
            data-testid="translations-namespace"
            aria-label="Group"
            size="sm"
            value={t.namespace}
            options={[
              { value: "", label: "All groups" },
              ...t.namespaces.map((ns) => ({ value: ns, label: ns })),
            ]}
            onChange={(e) => t.setNamespace(e.target.value)}
          />
        </div>
      )}
      <div style={{ flex: 1, minWidth: 0, maxWidth: 360 }}>
        <Input
          data-testid="translations-search"
          aria-label="Search strings"
          size="sm"
          placeholder="Search keys and text"
          value={t.search}
          iconLeft={<SearchIcon size={15} stroke="var(--text-tertiary)" />}
          onChange={(e) => t.setSearch(e.target.value)}
        />
      </div>
    </div>
  );
});

const KeyList = observer(function KeyList() {
  const t = useStore().translations;

  // Scroll-loading on the same cursor the queue uses.
  const onScroll = (e: React.UIEvent<HTMLDivElement>) => {
    const el = e.currentTarget;
    if (el.scrollHeight - el.scrollTop - el.clientHeight < 240) void t.loadMoreKeys();
  };

  if (!t.loading && t.rows.length === 0) {
    return (
      <div style={{ flex: 1, minWidth: 0, padding: "48px 28px", textAlign: "center", color: "var(--text-tertiary)", fontSize: 13 }}>
        {t.loaded ? "No strings match this filter." : "Loading…"}
      </div>
    );
  }

  return (
    <div onScroll={onScroll} style={{ flex: 1, minWidth: 0, overflowY: "auto", padding: "20px 24px" }}>
      <div style={card}>
        {t.rows.map((row) => (
          <KeyRow key={row.key} row={row} t={t} />
        ))}
      </div>
      {t.loadingMore && (
        <div style={{ padding: "14px 0", textAlign: "center", color: "var(--text-tertiary)", fontSize: 12 }}>
          Loading more…
        </div>
      )}
    </div>
  );
});

const KeyRow = observer(function KeyRow({ row, t }: { row: ApiTranslationKey; t: TranslationsStore }) {
  const store = useStore();
  const canWrite = store.can("translations.write");
  const canManage = store.can("translations.manage");
  const save = t.saveStateOf(row.key);

  return (
    <div
      data-testid="translations-row"
      data-key={row.key}
      style={{ padding: "14px 18px", display: "flex", alignItems: "flex-start", gap: 16, borderBottom: rowBorder }}
    >
      <div style={{ width: 260, flex: "none", minWidth: 0 }}>
        <div style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-tertiary)", overflowWrap: "anywhere" }}>
          {row.plural_base || row.key}
        </div>
        {row.plural_category && (
          <div style={{ marginTop: 4 }}>
            <Badge data-testid="translations-plural-category" variant="neutral">
              {row.plural_category}
            </Badge>
          </div>
        )}
        <div data-testid="translations-source" style={{ fontSize: 13, marginTop: 4 }}>
          {row.source}
        </div>
        {row.placeholders && row.placeholders.length > 0 && (
          <div
            data-testid="translations-placeholders"
            style={{ fontSize: 11.5, color: "var(--text-tertiary)", marginTop: 4 }}
          >
            Fills in:{" "}
            {row.placeholders.map((name) => (
              <code key={name} style={{ marginRight: 6 }}>{`{${name}}`}</code>
            ))}
          </div>
        )}
      </div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <Input
          data-testid="translations-value"
          aria-label={`Translation for ${row.key}`}
          size="sm"
          value={t.draftValue(row)}
          disabled={!canWrite}
          onChange={(e) => t.setValue(row.key, e.target.value)}
          onBlur={() => t.flushValue(row.key)}
          onKeyDown={(e) => e.key === "Enter" && t.flushValue(row.key)}
        />
        {save && (
          <div style={{ fontSize: 12, marginTop: 4, color: save === "failed" ? "var(--danger, #B3261E)" : "var(--text-tertiary)" }}>
            {save === "saving" ? "Saving…" : save === "saved" ? "Saved" : "Not saved"}
          </div>
        )}
      </div>
      <div style={{ width: 190, flex: "none", display: "flex", alignItems: "center", justifyContent: "flex-end", gap: 8 }}>
        <Badge data-testid="translations-state" variant={STATE_VARIANT[row.state]}>
          {STATE_LABEL[row.state]}
        </Badge>
        {row.state !== "default" && canWrite && (
          <Button data-testid="translations-reset" size="sm" variant="secondary" onClick={() => void t.resetValue(row.key)}>
            Reset
          </Button>
        )}
        {t.ownProject && canManage && (
          <Button
            data-testid="translations-undeclare"
            size="sm"
            variant="ghost"
            onClick={() => void t.undeclareKey(row.key)}
          >
            Remove
          </Button>
        )}
      </div>
    </div>
  );
});

// DeclareKey adds a string to a project the tenant owns. Sild's own keys come from
// the repo, so the bar is not offered there at all.
const DeclareKey = observer(function DeclareKey() {
  const store = useStore();
  const t = store.translations;
  const [key, setKey] = useState("");
  const [source, setSource] = useState("");
  // A plural is declared by its English forms; every other language is then offered
  // its own categories (ADR 0004).
  const [plural, setPlural] = useState(false);
  const [one, setOne] = useState("");

  if (!t.ownProject || !store.can("translations.manage")) return null;

  const ready = key.trim() !== "" && source.trim() !== "" && (!plural || one.trim() !== "");

  const add = async () => {
    if (!ready) return;
    const forms = plural ? { one: one.trim(), other: source.trim() } : undefined;
    if (await t.declareKey(key, plural ? "" : source, forms)) {
      setKey("");
      setSource("");
      setOne("");
    }
  };

  return (
    <div style={{ padding: "14px 24px 0", flex: "none", display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
      <div style={{ width: 260, flex: "none" }}>
        <Input
          data-testid="translations-new-key"
          aria-label="New key"
          size="sm"
          placeholder={plural ? "cart.items" : "checkout.pay"}
          value={key}
          onChange={(e) => setKey(e.target.value)}
        />
      </div>
      {plural && (
        <div style={{ width: 200, flex: "none" }}>
          <Input
            data-testid="translations-new-source-one"
            aria-label="English source, one"
            size="sm"
            placeholder="1 item"
            value={one}
            onChange={(e) => setOne(e.target.value)}
          />
        </div>
      )}
      <div style={{ flex: 1, minWidth: 120 }}>
        <Input
          data-testid="translations-new-source"
          aria-label={plural ? "English source, other" : "English source"}
          size="sm"
          placeholder={plural ? "{count} items" : "Pay now"}
          value={source}
          onChange={(e) => setSource(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && void add()}
        />
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 6, flex: "none" }}>
        <span style={{ fontSize: 12.5, color: "var(--text-tertiary)" }}>Takes a count</span>
        <Switch
          data-testid="translations-new-plural"
          aria-label="Takes a count"
          checked={plural}
          onChange={setPlural}
        />
      </div>
      <Button data-testid="translations-declare" size="sm" onClick={() => void add()} disabled={!ready}>
        Add string
      </Button>
    </div>
  );
});

// Projects lists what the tenant translates: Sild's own strings, plus a project per
// app of theirs. Creating one is how their own copy gets managed in the same place.
const Projects = observer(function Projects() {
  const store = useStore();
  const t = store.translations;
  const [adding, setAdding] = useState(false);
  const [id, setId] = useState("");
  const [name, setName] = useState("");
  const canManage = store.can("translations.manage");

  const create = async () => {
    if (await t.createProject(id, name)) {
      setAdding(false);
      setId("");
      setName("");
    }
  };

  return (
    <div style={{ ...card, marginBottom: 20 }} data-testid="translations-projects">
      <div style={{ padding: "14px 16px", borderBottom: rowBorder }}>
        <div style={{ fontSize: 15, fontWeight: 700 }}>Projects</div>
        <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>
          Sild&apos;s own strings, and one project per app of yours.
        </div>
      </div>
      {t.projects.map((p) => (
        <div key={p.id} data-project={p.id} style={{ padding: "10px 16px", display: "flex", alignItems: "center", gap: 10, borderBottom: rowBorder }}>
          <button
            onClick={() => t.setProject(p.id)}
            style={{
              flex: 1,
              minWidth: 0,
              textAlign: "left",
              background: "none",
              border: "none",
              padding: 0,
              cursor: "pointer",
              fontSize: 13,
              fontWeight: p.id === t.project?.id ? 700 : 400,
              color: "var(--text-primary)",
            }}
          >
            {p.name}
            <span style={{ color: "var(--text-tertiary)", fontWeight: 400 }}> · {p.keys}</span>
          </button>
          {!p.platform && canManage && (
            <Button
              data-testid="translations-delete-project"
              size="sm"
              variant="secondary"
              disabled={t.savingProject}
              onClick={() => void t.deleteProject(p.id)}
            >
              Delete
            </Button>
          )}
        </div>
      ))}
      {canManage && (
        <div style={{ padding: "14px 16px" }}>
          {adding ? (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              <Input
                data-testid="translations-project-id"
                aria-label="Project id"
                size="sm"
                placeholder="shop"
                value={id}
                onChange={(e) => setId(e.target.value)}
              />
              <Input
                data-testid="translations-project-name"
                aria-label="Project name"
                size="sm"
                placeholder="Shop"
                value={name}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && void create()}
              />
              <div style={{ display: "flex", gap: 8 }}>
                <Button data-testid="translations-create-project" size="sm" loading={t.savingProject} onClick={() => void create()}>
                  Create
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setAdding(false)}>
                  Cancel
                </Button>
              </div>
            </div>
          ) : (
            <Button data-testid="translations-new-project" size="sm" variant="secondary" onClick={() => setAdding(true)}>
              New project
            </Button>
          )}
        </div>
      )}
    </div>
  );
});

const Languages = observer(function Languages() {
  const store = useStore();
  const t = store.translations;
  const [adding, setAdding] = useState("");
  const project = t.project;
  if (!project) return null;
  // Which languages a tenant offers is the project's settings, not a string edit.
  const canManage = store.can("translations.manage");
  // A language the tenant added is not one Sild ships, so it is listed too.
  const offered = [...new Set([...t.availableLocales, ...project.locales])].sort();

  const add = async () => {
    if (await t.addLocale(adding)) setAdding("");
  };

  return (
    <div style={card}>
      <div style={{ padding: "14px 16px", borderBottom: rowBorder }}>
        <div style={{ fontSize: 15, fontWeight: 700 }}>Languages</div>
        <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>
          Turn on the languages your customers read. Each one is published as its own bundle.
        </div>
      </div>
      {offered.map((locale) => (
        <div
          key={locale}
          data-testid="translations-language"
          data-locale={locale}
          style={{ padding: "10px 16px", display: "flex", alignItems: "center", gap: 10, borderBottom: rowBorder }}
        >
          <span style={{ flex: 1, minWidth: 0, fontSize: 13 }}>{localeLabel(locale)}</span>
          {project.locales.includes(locale) && (
            <span
              data-testid="translations-completion"
              data-locale={locale}
              style={{ fontSize: 12, color: "var(--text-tertiary)", flex: "none" }}
            >
              {t.completionOf(locale)}%
            </span>
          )}
          <Switch
            checked={project.locales.includes(locale)}
            disabled={!canManage || locale === project.fallback_locale}
            onChange={(v) => t.toggleLocale(locale, v)}
          />
        </div>
      ))}
      {canManage && (
        <div style={{ padding: "12px 16px", display: "flex", gap: 8, borderBottom: rowBorder }}>
          <Input
            data-testid="translations-add-language"
            aria-label="Add a language"
            size="sm"
            placeholder="fi"
            value={adding}
            onChange={(e) => setAdding(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && void add()}
          />
          <Button size="sm" variant="secondary" disabled={!adding.trim()} onClick={() => void add()}>
            Add
          </Button>
        </div>
      )}
      <div style={{ padding: "14px 16px" }}>
        <div style={fieldLabel}>Fallback language</div>
        <div style={{ fontSize: 13, color: "var(--text-tertiary)", margin: "4px 0 8px" }}>
          Used when a device asks for a language you do not offer. A device&apos;s own preference always wins.
        </div>
        <Select
          aria-label="Fallback language"
          size="sm"
          disabled={!canManage}
          value={project.fallback_locale}
          options={project.locales.map((l) => ({ value: l, label: localeLabel(l) }))}
          onChange={(e) => t.setFallbackLocale(e.target.value)}
        />
      </div>
      <div style={{ padding: "14px 16px", display: "flex", alignItems: "flex-start", gap: 10, borderTop: rowBorder }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={fieldLabel}>Publish every change</div>
          <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 4 }}>
            Each edit goes live as soon as it is saved, with no separate publish step.
          </div>
        </div>
        <Switch
          data-testid="translations-auto-publish"
          aria-label="Publish every change"
          checked={project.auto_publish}
          disabled={!canManage}
          onChange={(v) => t.setAutoPublish(v)}
        />
      </div>
    </div>
  );
});

// DraftChanges is what a publish would change, read before approving it. It is
// also the whole of what a translator without the publish grant can see of their
// queue: the same list, with no button under it.
const DraftChanges = observer(function DraftChanges() {
  const store = useStore();
  const t = store.translations;
  const [open, setOpen] = useState(false);
  const diff = t.diff;
  const pending = t.pendingCount;
  const canPublish = store.can("translations.publish");

  return (
    <Panel
      title="Draft changes"
      testid="translations-drafts"
      right={
        <Badge data-testid="translations-draft-count" variant={pending ? "brand" : "neutral"}>
          {t.diffLoading && !diff ? "…" : pending}
        </Badge>
      }
      subtitle={
        pending === 0
          ? "Everything saved is live."
          : canPublish
            ? `Publishing cuts v${diff?.next_version} with these ${pending} changes.`
            : `Waiting for someone who can publish. ${pending} change${pending === 1 ? "" : "s"} queued.`
      }
    >
      {pending > 0 && (
        <>
          <div style={{ padding: "10px 16px", display: "flex", gap: 8, borderBottom: open ? rowBorder : undefined }}>
            <Button
              data-testid="translations-review"
              size="sm"
              variant="secondary"
              onClick={() => {
                setOpen(!open);
                if (!open) void t.loadDiff();
              }}
            >
              {open ? "Hide changes" : "Review changes"}
            </Button>
          </div>
          {open && (
            <div style={{ maxHeight: 260, overflowY: "auto" }}>
              {diff?.rows.map((row) => (
                <div
                  key={`${row.locale}:${row.key}`}
                  data-testid="translations-draft-row"
                  data-locale={row.locale}
                  style={{ padding: "10px 16px", borderBottom: rowBorder }}
                >
                  <div style={{ fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--text-tertiary)", overflowWrap: "anywhere" }}>
                    {row.locale} · {row.key}
                  </div>
                  {row.live && (
                    <div style={{ fontSize: 12.5, color: "var(--text-tertiary)", textDecoration: "line-through", marginTop: 3 }}>
                      {row.live}
                    </div>
                  )}
                  <div style={{ fontSize: 13, marginTop: 2 }}>{row.draft}</div>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </Panel>
  );
});

// Transfer is import and export. A file is previewed before it is applied, so what
// a tenant approves and what lands cannot differ.
const Transfer = observer(function Transfer() {
  const store = useStore();
  const t = store.translations;
  const canWrite = store.can("translations.write");
  const canManage = store.can("translations.manage");
  const report = t.importReport;

  const pick = async (file: File | undefined) => {
    if (!file) return;
    await t.previewImport(file.name, await file.text());
  };

  return (
    <Panel
      title="Import and export"
      testid="translations-transfer"
      subtitle={
        <>
          Plural forms are reassembled into <code>&lt;plurals&gt;</code> and stringsdict on the
          way out. A spreadsheet is for a translator without an account.
        </>
      }
    >

      {canWrite && (
        <div style={{ padding: "14px 16px", borderBottom: rowBorder }}>
          <div style={fieldLabel}>Import into {t.locale}</div>
          <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
            <div style={{ width: 130, flex: "none" }}>
              <Select
                data-testid="translations-import-format"
                aria-label="Import format"
                size="sm"
                value={t.importFormat}
                options={IMPORT_FORMATS}
                onChange={(e) => t.setImportFormat(e.target.value as TranslationFormat)}
              />
            </div>
            <input
              data-testid="translations-import-file"
              aria-label="Translation file"
              type="file"
              onChange={(e) => void pick(e.target.files?.[0])}
              style={{ flex: 1, minWidth: 0, fontSize: 12 }}
            />
          </div>
          {t.ownProject && canManage && (
            <div style={{ display: "flex", alignItems: "flex-start", gap: 10, marginTop: 12 }}>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 13 }}>Create new keys</div>
                <div style={{ fontSize: 12, color: "var(--text-tertiary)", marginTop: 2 }}>
                  Off by default: a key this project does not declare is reported and skipped.
                </div>
              </div>
              <Switch
                data-testid="translations-import-create-keys"
                aria-label="Create new keys"
                checked={t.importCreateKeys}
                onChange={(v) => t.setImportCreateKeys(v)}
              />
            </div>
          )}
          {t.importFile && (
            <div style={{ marginTop: 12, fontSize: 12.5, color: "var(--text-secondary)" }}>
              {t.importFile.name}
              {t.importing && " · reading…"}
            </div>
          )}
          {report && (
            <div data-testid="translations-import-report" style={{ marginTop: 10 }}>
              <div style={{ fontSize: 13 }}>
                {report.new} new · {report.changed} changed · {report.skipped} skipped
                {report.keys_created > 0 ? ` · ${report.keys_created} keys to declare` : ""}
              </div>
              <div style={{ maxHeight: 150, overflowY: "auto", marginTop: 8 }}>
                {report.rows.map((row) => (
                  <div
                    key={row.key}
                    data-testid="translations-import-row"
                    data-status={row.status}
                    style={{ display: "flex", gap: 8, padding: "4px 0", fontSize: 12 }}
                  >
                    <span style={{ flex: 1, minWidth: 0, fontFamily: "var(--font-mono)", overflowWrap: "anywhere" }}>
                      {row.key}
                    </span>
                    <span style={{ flex: "none", color: "var(--text-tertiary)" }}>
                      {row.reason || row.status}
                    </span>
                  </div>
                ))}
              </div>
              {report.dry_run && (
                <div style={{ display: "flex", gap: 8, marginTop: 10 }}>
                  <Button
                    data-testid="translations-import-apply"
                    size="sm"
                    loading={t.importing}
                    disabled={report.new + report.changed === 0}
                    onClick={() => void t.applyImport()}
                  >
                    Apply as drafts
                  </Button>
                  <Button size="sm" variant="ghost" onClick={t.clearImport}>
                    Cancel
                  </Button>
                </div>
              )}
              {!report.dry_run && (
                <div style={{ fontSize: 12.5, color: "var(--text-tertiary)", marginTop: 8 }}>
                  Applied as drafts. Publish when you are ready.
                </div>
              )}
            </div>
          )}
        </div>
      )}

      <div style={{ padding: "14px 16px" }}>
        <div style={fieldLabel}>Export {t.locale}</div>
        <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <Select
              data-testid="translations-export-format"
              aria-label="Export format"
              size="sm"
              value={t.exportFormat}
              options={EXPORT_FORMATS}
              onChange={(e) => t.setExportFormat(e.target.value as TranslationFormat)}
            />
          </div>
          <a
            data-testid="translations-export"
            className="sild-btn sild-btn--sm sild-btn--secondary"
            href={t.exportUrl ?? "#"}
            download
            style={{ flex: "none", textDecoration: "none" }}
          >
            Download
          </a>
        </div>
      </div>
    </Panel>
  );
});

// FromCI documents the two calls a build makes. There is no `sild` binary: a
// project-scoped key and curl are the whole story.
const FromCI = observer(function FromCI() {
  const store = useStore();
  const t = store.translations;
  if (!store.can("translations.manage")) return null;
  const project = t.project?.id ?? "sild";
  const snippet = `# pull the live text for one language
curl -H "Authorization: Bearer $SILD_KEY" \\
  "${location.origin}/v1/translations/projects/${project}/export?locale=${t.locale || "en"}&format=json" > ${t.locale || "en"}.json

# push what the build produced (drafts until published)
curl -X POST -H "Authorization: Bearer $SILD_KEY" \\
  --data-binary @${t.locale || "en"}.json \\
  "${location.origin}/v1/translations/projects/${project}/import?locale=${t.locale || "en"}&format=json"`;

  return (
    <Panel
      title="From CI"
      testid="translations-from-ci"
      subtitle="An API key scoped to one project pushes and pulls the same content. A build token limited to a project reaches nothing else — mint one under Settings → API keys."
    >
      <div style={{ padding: "14px 16px" }}>
        <pre
          style={{
            margin: 0,
            fontSize: 11.5,
            lineHeight: 1.6,
            overflowX: "auto",
            background: "var(--surface-page)",
            border: rowBorder,
            borderRadius: 8,
            padding: 12,
          }}
        >
          {snippet}
        </pre>
      </div>
    </Panel>
  );
});

// Delivery says how a published release reaches a device, because "published" and
// "on screen" are not the same moment and the difference is worth stating.
const Delivery = observer(function Delivery() {
  const t = useStore().translations;
  const rows: [string, string][] = [
    ["Poll", "At start, and on foreground once the held manifest is an hour old"],
    ["Web drop-in", "Applies a new bundle immediately"],
    ["Native SDKs", "Activate at the next start, unless the app calls refresh"],
    ["Offline", "The text bundled in the app, always"],
  ];
  return (
    <Panel
      title="Delivery"
      testid="translations-delivery"
      style={{ marginBottom: 20 }}
      subtitle={
        "Clients download a language only when its version moves." +
        (t.currentVersion === null ? " Nothing is published yet." : ` Serving v${t.currentVersion}.`)
      }
    >
      {rows.map(([label, value]) => (
        <div key={label} style={{ padding: "10px 16px", display: "flex", gap: 10, borderBottom: rowBorder }}>
          <div style={{ width: 96, flex: "none", fontSize: 12.5, color: "var(--text-tertiary)" }}>{label}</div>
          <div style={{ flex: 1, minWidth: 0, fontSize: 12.5 }}>{value}</div>
        </div>
      ))}
    </Panel>
  );
});

const Releases = observer(function Releases() {
  const store = useStore();
  const t = store.translations;

  return (
    <div style={{ ...card, marginTop: 20 }} data-testid="translations-releases">
      <div style={{ padding: "14px 16px", borderBottom: rowBorder }}>
        <div style={{ fontSize: 15, fontWeight: 700 }}>Releases</div>
        <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>
          Each publish is an immutable version. Rolling back cuts a new one with the old text.
        </div>
      </div>
      {t.releases.length === 0 && (
        <div style={{ padding: "16px", fontSize: 13, color: "var(--text-tertiary)" }}>Nothing published yet.</div>
      )}
      {t.releases.map((r) => (
        <div key={r.version} data-version={r.version} style={{ padding: "12px 16px", display: "flex", alignItems: "center", gap: 10, borderBottom: rowBorder }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontSize: 13, fontWeight: 600 }}>v{r.version}</div>
            <div style={{ fontSize: 12, color: "var(--text-tertiary)" }}>
              {new Date(r.published_at).toLocaleString()}
              {r.published_by ? ` · ${r.published_by}` : ""}
            </div>
          </div>
          {store.can("translations.publish") && r.version !== t.currentVersion && (
            <Button size="sm" variant="secondary" disabled={t.publishing} onClick={() => void t.rollback(r.version)}>
              Roll back
            </Button>
          )}
        </div>
      ))}
      {t.releasesHasMore && (
        <div style={{ padding: "12px 16px" }}>
          <Button size="sm" variant="ghost" onClick={() => void t.loadMoreReleases()}>
            Show older
          </Button>
        </div>
      )}
    </div>
  );
});
