"use client";

import type { CSSProperties } from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Button, StatusPill } from "@/components/ds";
import { DRAFT_ID, type TriageFilter } from "@/store/triage";
import { TRIAGE_PILL, type TriageStatus } from "@/store/triageStatus";
import { filterStyle } from "./styles";

const FILTERS: [TriageFilter, string][] = [
  ["all", "All"],
  ["active", "Active"],
  ["closed", "Closed"],
];

function rowStyle(on: boolean): CSSProperties {
  return {
    display: "flex",
    flexDirection: "column",
    gap: 3,
    width: "100%",
    textAlign: "left",
    padding: "12px 16px",
    border: 0,
    borderBottom: "1px solid var(--border-subtle)",
    borderLeft: `3px solid ${on ? "var(--brand)" : "transparent"}`,
    background: on ? "var(--brand-subtle)" : "var(--surface-card)",
    cursor: "pointer",
    fontFamily: "var(--font-sans)",
  };
}

const ellipsis: CSSProperties = { whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" };

function Row({
  id,
  active,
  title,
  time,
  preview,
  pill,
  owner,
  onClick,
}: {
  id: string;
  active: boolean;
  title: string;
  time: string;
  preview: string;
  pill: (typeof TRIAGE_PILL)[TriageStatus];
  owner: string;
  onClick: () => void;
}) {
  return (
    <button data-testid="triage-row" data-conversation={id} onClick={onClick} style={rowStyle(active)}>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <span style={{ flex: 1, minWidth: 0, fontSize: 14, fontWeight: 600, color: "var(--text-primary)", ...ellipsis }}>
          {title}
        </span>
        <span style={{ fontSize: 11, color: "var(--text-tertiary)", flex: "none" }}>{time}</span>
      </div>
      <div style={{ fontSize: 13, color: "var(--text-secondary)", ...ellipsis }}>{preview}</div>
      <div style={{ display: "flex", alignItems: "center", gap: 6, marginTop: 2 }}>
        <StatusPill status={pill.pill} label={pill.label} />
        <div style={{ flex: 1 }} />
        {owner && <span style={{ fontSize: 12, color: "var(--text-tertiary)", whiteSpace: "nowrap" }}>{owner}</span>}
      </div>
    </button>
  );
}

export const TriageList = observer(function TriageList() {
  const t = useStore().triage;
  const rows = t.visibleRows;
  const counts = t.counts;

  return (
    <div
      style={{
        width: 340,
        flex: "none",
        display: "flex",
        flexDirection: "column",
        minHeight: 0,
        background: "var(--surface-card)",
        borderRight: "1px solid var(--border-default)",
      }}
    >
      <div style={{ padding: "16px 16px 12px", display: "flex", flexDirection: "column", gap: 10, borderBottom: "1px solid var(--border-subtle)" }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8, minHeight: 32 }}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 700, letterSpacing: "-.01em" }}>
            {t.viewAll ? "Tech support" : "Your requests"}
          </h2>
          {t.canStart && (
            <Button size="sm" onClick={t.newRequest} data-testid="triage-new">
              New request
            </Button>
          )}
        </div>
        {t.viewAll && (
          <p style={{ margin: 0, fontSize: 12, lineHeight: 1.5, color: "var(--text-tertiary)" }}>
            Everyone&apos;s chats with the triage assistant.
          </p>
        )}
        {t.viewAll && (
          <div style={{ display: "flex", gap: 3, background: "var(--surface-sunken)", padding: 3, borderRadius: 8 }}>
            {FILTERS.map(([id, label]) => (
              <button key={id} data-testid={`triage-filter-${id}`} onClick={() => t.setFilter(id)} style={filterStyle(t.filter === id)}>
                {label}
                <span style={{ fontSize: 11, color: "var(--text-tertiary)" }}>{counts[id]}</span>
              </button>
            ))}
          </div>
        )}
      </div>

      <div
        style={{ flex: 1, overflowY: "auto", minHeight: 0 }}
        onScroll={(e) => {
          const el = e.currentTarget;
          if (el.scrollHeight - el.scrollTop - el.clientHeight < 160) void t.loadMore();
        }}
      >
        {t.drafting && (
          <Row
            id={DRAFT_ID}
            active={t.draftActive}
            title="New request"
            time="now"
            preview="Describe what's going wrong…"
            pill={TRIAGE_PILL.new}
            owner=""
            onClick={() => t.setActive(DRAFT_ID)}
          />
        )}
        {rows.map((c) => (
          <Row
            key={c.id}
            id={c.id}
            active={c.id === t.activeId}
            title={t.title(c)}
            time={t.timeOf(c)}
            preview={t.preview(c)}
            pill={TRIAGE_PILL[t.statusOf(c)]}
            owner={t.viewAll ? t.ownerName(c) : ""}
            onClick={() => t.setActive(c.id)}
          />
        ))}
        {t.loaded && rows.length === 0 && !t.drafting && (
          <div style={{ padding: "40px 24px", textAlign: "center", color: "var(--text-tertiary)", fontSize: 13, lineHeight: 1.6 }}>
            No tech support chats in this view.
          </div>
        )}
        {t.loadingMore && (
          <div style={{ padding: 12, textAlign: "center", color: "var(--text-tertiary)", fontSize: 12 }}>Loading…</div>
        )}
      </div>
    </div>
  );
});
