"use client";

import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Banner, Button, CopyIcon, Dialog } from "@/components/ds";

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
