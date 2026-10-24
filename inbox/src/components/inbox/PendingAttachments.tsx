"use client";

import { useRef } from "react";
import { observer } from "mobx-react-lite";
import type { AttachmentQueue } from "@/store/attachments";

// Files queued for the next message, with a remove button each.
export const PendingAttachments = observer(function PendingAttachments({ atts }: { atts: AttachmentQueue }) {
  if (atts.pending.length === 0 && atts.uploading === 0) return null;
  return (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 8 }}>
      {atts.pending.map((a, i) => (
        <span
          key={i}
          style={{ display: "inline-flex", alignItems: "center", gap: 6, maxWidth: 220, fontSize: 12, color: "var(--text-secondary)", background: "var(--surface-sunken)", border: "1px solid var(--border-default)", borderRadius: 8, padding: "5px 8px" }}
        >
          <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{a.filename}</span>
          <button
            onClick={() => atts.remove(i)}
            aria-label="Remove attachment"
            style={{ border: 0, background: "transparent", cursor: "pointer", color: "var(--text-tertiary)", padding: 0, display: "flex", lineHeight: 1 }}
          >
            ✕
          </button>
        </span>
      ))}
      {atts.uploading > 0 && <span style={{ fontSize: 12, color: "var(--text-tertiary)", alignSelf: "center" }}>Uploading…</span>}
    </div>
  );
});

// A hidden file input that uploads into the queue; render `input` and call `open` from an attach button.
export function useFilePicker(atts: AttachmentQueue) {
  const ref = useRef<HTMLInputElement>(null);
  const input = (
    <input
      ref={ref}
      type="file"
      multiple
      style={{ display: "none" }}
      onChange={(e) => {
        const files = Array.from(e.target.files || []);
        e.target.value = "";
        if (files.length) atts.attach(files);
      }}
    />
  );
  return { input, open: () => ref.current?.click() };
}
