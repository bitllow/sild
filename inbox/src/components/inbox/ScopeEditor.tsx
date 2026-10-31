"use client";

import { useEffect } from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Switch } from "@/components/ds";
import type { RoleDefinition, RoleDimension, RoleScope } from "@/store/types";

export const SCOPE_ALL = "all";

type OptionLabel = (source: string | undefined, value: string) => string;

/** The chip's caption: what this assignment reaches, in the role's own words. */
export function scopeLabel(def: RoleDefinition | undefined, scope: RoleScope, label: OptionLabel): string {
  const dims = def?.dimensions ?? [];
  if (dims.length === 0) return "tenant-wide";
  const parts = dims.map((d) => dimensionLabel(def, d, scope, label)).filter(Boolean);
  return parts.length ? parts.join(" · ") : `no ${dims[0].label.toLowerCase()}`;
}

function dimensionLabel(def: RoleDefinition | undefined, d: RoleDimension, scope: RoleScope, label: OptionLabel): string {
  if (d.kind === "toggle") return scope[d.key] ? d.label.toLowerCase() : "";
  const set = (scope[d.key] as string[] | undefined) ?? [];
  if (set.includes(SCOPE_ALL)) return `all ${d.label.toLowerCase()}`;
  // An agent with no queues works the support queue, not nothing.
  if (set.length === 0 && def?.role === "agent" && d.key === "queues") return label(d.source, "support");
  if (set.length === 0) return `no ${d.label.toLowerCase()}`;
  return set.map((v) => label(d.source, v)).join(", ");
}

// A selectable pill: role picker in the invite, option chips in a scope.
export const pill = (selected: boolean) => ({
  border: `1px solid ${selected ? "var(--brand)" : "var(--border-default)"}`,
  background: selected ? "var(--brand-subtle)" : "var(--white)",
  color: selected ? "var(--brand)" : "var(--text-secondary)",
  borderRadius: 999,
  padding: "5px 11px",
  cursor: "pointer",
  fontFamily: "var(--font-sans)",
  fontSize: 13,
  fontWeight: 600,
});

export const chip = (dashed: boolean) => ({
  display: "flex",
  alignItems: "center",
  gap: 8,
  maxWidth: "100%",
  border: `1px ${dashed ? "dashed" : "solid"} var(--border-default)`,
  borderRadius: 999,
  background: dashed ? "transparent" : "var(--white)",
  padding: "5px 12px",
  cursor: "pointer",
  fontFamily: "var(--font-sans)",
  fontSize: 13,
  color: dashed ? "var(--text-tertiary)" : "var(--text-primary)",
});

// ScopeEditor renders the dimensions the role declares — a switch for a toggle,
// chips for a set. The caller decides where a change goes.
export const ScopeEditor = observer(function ScopeEditor({
  def,
  scope,
  onChange,
}: {
  def: RoleDefinition | undefined;
  scope: RoleScope;
  onChange: (next: RoleScope) => void;
}) {
  const store = useStore();
  // Only a translation dimension needs the tenant's projects and languages, so
  // the fetch waits until one is about to render. Idempotent.
  useEffect(() => {
    if ((def?.dimensions ?? []).some((d) => d.source?.startsWith("translation_"))) void store.translations.load();
  }, [store, def]);

  const toggleMember = (d: RoleDimension, value: string) => {
    const set = (scope[d.key] as string[] | undefined) ?? [];
    const next = set.includes(value) ? set.filter((v) => v !== value) : [...set, value];
    onChange({ ...scope, [d.key]: value === SCOPE_ALL && !set.includes(SCOPE_ALL) ? [SCOPE_ALL] : next });
  };

  return (
    <>
      {(def?.dimensions ?? []).map((d) => (
        <div key={d.key} style={{ marginBottom: 18 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div style={{ minWidth: 0 }}>
              <div style={{ fontSize: 13, fontWeight: 600 }}>{d.label}</div>
              <div style={{ fontSize: 12.5, color: "var(--text-tertiary)", marginTop: 2, lineHeight: 1.5 }}>{d.help}</div>
            </div>
            <div style={{ flex: 1 }} />
            {d.kind === "toggle" && (
              <Switch checked={!!scope[d.key]} onChange={(v) => onChange({ ...scope, [d.key]: v })} />
            )}
          </div>
          {d.kind === "set" && (
            <div style={{ display: "flex", gap: 6, marginTop: 9, flexWrap: "wrap" }}>
              {[SCOPE_ALL, ...store.scopeOptions(d.source)].map((option) => {
                const set = (scope[d.key] as string[] | undefined) ?? [];
                return (
                  <button
                    key={option}
                    type="button"
                    data-testid={`scope-${d.key}-${option}`}
                    onClick={() => toggleMember(d, option)}
                    style={pill(set.includes(option))}
                  >
                    {option === SCOPE_ALL ? option : store.scopeOptionLabel(d.source, option)}
                  </button>
                );
              })}
            </div>
          )}
        </div>
      ))}
      {def && (def.dimensions ?? []).length === 0 && (
        <p style={{ fontSize: 13, color: "var(--text-tertiary)", lineHeight: 1.55 }}>
          This role is tenant-wide. It defines no scope of its own.
        </p>
      )}
    </>
  );
});
