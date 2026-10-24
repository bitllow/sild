"use client";

import type { CSSProperties } from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import type { ApiKey } from "@/store/types";
import { Banner, Button, CopyIcon, Dialog, KeyIcon } from "@/components/ds";
import { rowBorder } from "./styles";

export const KeyDialog = observer(function KeyDialog() {
  const store = useStore();
  return (
    <Dialog title="API key created" subtitle="This is the only time the full key is shown." onClose={store.closeKeyDialog}>
      <Banner variant="warning">Copy it now — it won&apos;t be shown again.</Banner>
      <div
        style={{
          marginTop: 14,
          display: "flex",
          alignItems: "center",
          gap: 10,
          background: "var(--surface-sunken)",
          border: "1px solid var(--border-default)",
          borderRadius: 8,
          padding: "10px 12px",
        }}
      >
        <span style={{ flex: 1, minWidth: 0, fontFamily: "var(--font-mono)", fontSize: 13, color: "var(--text-primary)", wordBreak: "break-all" }}>
          {store.revealedKey}
        </span>
        <button
          onClick={store.copyKey}
          aria-label="Copy key"
          style={{
            flex: "none",
            display: "flex",
            alignItems: "center",
            gap: 6,
            border: "1px solid var(--border-default)",
            background: "var(--white)",
            borderRadius: 6,
            padding: "6px 10px",
            cursor: "pointer",
            fontFamily: "var(--font-sans)",
            fontSize: 13,
            fontWeight: 600,
            color: "var(--text-primary)",
          }}
        >
          <CopyIcon size={15} />
          Copy
        </button>
      </div>
      <div style={{ marginTop: 18, display: "flex", justifyContent: "flex-end" }}>
        <Button onClick={store.closeKeyDialog}>Done</Button>
      </div>
    </Dialog>
  );
});

const row: CSSProperties = { padding: "14px 18px", display: "flex", alignItems: "center", gap: 14, borderBottom: rowBorder };
const compactRow: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 10,
  padding: "7px 10px",
  border: "1px solid var(--border-subtle)",
  borderRadius: 8,
  background: "var(--surface-page)",
};
const tile: CSSProperties = {
  width: 34,
  height: 34,
  flex: "none",
  borderRadius: 8,
  background: "var(--brand-subtle)",
  color: "var(--brand)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
};
const masked: CSSProperties = {
  fontFamily: "var(--font-mono)",
  fontSize: 12,
  color: "var(--text-tertiary)",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

/** One issued key with its revoke action; compact under a service account's roles. */
export const KeyRow = observer(function KeyRow({ k, compact = false }: { k: ApiKey; compact?: boolean }) {
  const store = useStore();
  const label = <span style={{ fontSize: compact ? 13 : 14, fontWeight: 600, whiteSpace: compact ? "nowrap" : undefined }}>{k.label}</span>;
  return (
    <div data-testid={compact ? "service-key" : undefined} style={compact ? compactRow : row}>
      {compact ? (
        <>
          <KeyIcon size={15} />
          {label}
          <span style={{ ...masked, flex: 1, minWidth: 0 }}>{k.masked}</span>
        </>
      ) : (
        <>
          <div style={tile}>
            <KeyIcon size={18} />
          </div>
          <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 2 }}>
            {label}
            <span style={masked}>{k.masked}</span>
          </div>
        </>
      )}
      <span style={{ fontSize: 12, color: "var(--text-tertiary)", whiteSpace: "nowrap" }}>{k.created}</span>
      <Button size="sm" variant="danger" data-testid={compact ? "revoke-service-key" : undefined} onClick={() => store.revokeKey(k.id)}>
        Revoke
      </Button>
    </div>
  );
});
