"use client";

import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Avatar, Badge, CloseIcon, Tag } from "@/components/ds";
import { RolePill } from "./PeerBits";

const sectionLabel: React.CSSProperties = {
  fontSize: 11,
  fontWeight: 600,
  letterSpacing: ".04em",
  textTransform: "uppercase",
  color: "var(--text-tertiary)",
  marginBottom: 8,
};

export const PeerPanel = observer(function PeerPanel() {
  const store = useStore();
  const peer = store.peer;
  const active = peer.active;
  if (!active) return null;

  return (
    <div
      data-testid="peer-panel"
      style={{
        width: 320,
        flex: "none",
        borderLeft: "1px solid var(--border-default)",
        background: "var(--surface-card)",
        display: "flex",
        flexDirection: "column",
      }}
    >
      <div
        style={{
          height: 64,
          flex: "none",
          padding: "0 16px",
          borderBottom: "1px solid var(--border-default)",
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
        }}
      >
        <span style={{ fontSize: 13, fontWeight: 700, letterSpacing: "-.01em" }}>Details</span>
        <button
          onClick={store.togglePanel}
          aria-label="Close panel"
          style={{ width: 30, height: 30, display: "flex", alignItems: "center", justifyContent: "center", border: 0, background: "transparent", borderRadius: 6, cursor: "pointer", color: "var(--text-secondary)" }}
        >
          <CloseIcon size={18} />
        </button>
      </div>

      <div style={{ padding: 16, overflowY: "auto", display: "flex", flexDirection: "column", gap: 18 }}>
        {/* Conversation — reference + role-pair badge. No assignment applies. */}
        <div>
          <div style={sectionLabel}>Conversation</div>
          <div style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
            <Tag mono>{active.reference}</Tag>
            <Badge variant="neutral">{peer.kindLabel(active)}</Badge>
          </div>
          <div style={{ marginTop: 8, fontSize: 12, color: "var(--text-tertiary)", lineHeight: 1.5 }}>
            No assignment — you're here to observe, or step in if needed.
          </div>
        </div>

        {/* Participants — avatar, presence, role pill, full metadata. */}
        <div>
          <div style={sectionLabel}>Participants ({active.participants.length})</div>
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            {active.participants.map((p, i) => (
              <div key={i} style={{ display: "flex", gap: 10 }}>
                <Avatar name={p.name} presence={p.presence} size={36} />
                <div style={{ minWidth: 0, flex: 1 }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                    <span style={{ fontSize: 14, fontWeight: 600 }}>{p.name}</span>
                    <RolePill role={p.role} isAgent={p.isAgent} peer={peer} />
                  </div>
                  <div style={{ marginTop: 6, display: "flex", flexDirection: "column", gap: 4 }}>
                    {Object.entries(p.meta).map(([k, v]) => (
                      <div key={k} style={{ display: "flex", gap: 6, fontSize: 12 }}>
                        <span style={{ fontFamily: "var(--font-mono)", color: "var(--text-tertiary)", whiteSpace: "nowrap" }}>{k}</span>
                        <span style={{ color: "var(--text-secondary)", wordBreak: "break-all" }}>{v}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
});
