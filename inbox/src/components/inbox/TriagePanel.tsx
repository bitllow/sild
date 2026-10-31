"use client";

import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Avatar, Tag } from "@/components/ds";
import { describeRequest, stepsOf } from "@/store/triageStatus";
import { capsLabel } from "./styles";

// Observers only: the requester sees their own chat without a details column.
export const TriagePanel = observer(function TriagePanel() {
  const t = useStore().triage;
  const c = t.active;
  if (!c || t.draftActive || t.isOwn(c)) return null;

  const name = t.ownerName(c);
  const requests = c.messages
    .flatMap((m) => stepsOf(m).flatMap((p) => p.steps))
    .filter((s) => s.state !== "running" && s.internal)
    .map((s) => describeRequest(s.internal!));

  return (
    <div
      data-testid="triage-panel"
      style={{ width: 300, flex: "none", background: "var(--surface-card)", borderLeft: "1px solid var(--border-default)", overflowY: "auto", padding: 16, display: "flex", flexDirection: "column", gap: 20 }}
    >
      <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
        <span style={capsLabel}>Requester</span>
        <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
          <Avatar name={name} size={36} />
          <div style={{ minWidth: 0, fontSize: 14, fontWeight: 600 }}>{name}</div>
        </div>
      </div>
      {requests.length > 0 && (
        <div data-testid="triage-requests" style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          <span style={capsLabel}>Data requests · {requests.length}</span>
          {requests.map((r, i) => (
            <span key={i} style={{ fontFamily: "var(--font-mono)", fontSize: 11, lineHeight: 1.45, color: "var(--text-secondary)", wordBreak: "break-all" }}>
              {r}
            </span>
          ))}
        </div>
      )}
      <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
        <Tag mono>{c.reference}</Tag>
      </div>
    </div>
  );
});
