"use client";

import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Avatar, Badge, CloseIcon, StatusPill, Tag } from "@/components/ds";

// First word of a contact's name for the "Earlier from …" heading.
function contactFirstName(name?: string): string {
  return (name || "this contact").trim().split(/\s+/)[0];
}

const sectionLabel: React.CSSProperties = {
  fontSize: 11,
  fontWeight: 600,
  letterSpacing: ".04em",
  textTransform: "uppercase",
  color: "var(--text-tertiary)",
  marginBottom: 8,
};

export const MemberPanel = observer(function MemberPanel() {
  const store = useStore();
  const active = store.active;
  if (!active) return null;

  return (
    <div
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
          style={{
            width: 30,
            height: 30,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            border: 0,
            background: "transparent",
            borderRadius: 6,
            cursor: "pointer",
            color: "var(--text-secondary)",
          }}
        >
          <CloseIcon size={18} />
        </button>
      </div>

      <div style={{ padding: 16, overflowY: "auto", display: "flex", flexDirection: "column", gap: 18 }}>
        {/* Assignment */}
        <div>
          <div style={sectionLabel}>Assignment</div>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <StatusPill status={active.status} />
            <span style={{ fontSize: 13, color: "var(--text-secondary)" }}>{store.assignLabel}</span>
          </div>
          <div style={{ marginTop: 8, display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
            <Tag mono>{active.reference}</Tag>
            <Tag mono>{store.activeChannelTag}</Tag>
          </div>
        </div>

        {/* Contact history — the active contact's earlier threads */}
        {store.contactEarlier.length > 0 && (
          <div>
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 8 }}>
              <span style={{ ...sectionLabel, marginBottom: 0 }}>
                Earlier from {contactFirstName(store.activeContact?.name)} ({store.contactEarlier.length})
              </span>
              <button
                data-testid="contact-view-all"
                onClick={store.viewAllFromActive}
                style={{ border: 0, background: "transparent", cursor: "pointer", fontFamily: "var(--font-sans)", fontSize: 12, fontWeight: 600, color: "var(--brand)", padding: 0 }}
              >
                View all
              </button>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              {store.contactEarlier.map((h) => (
                <button
                  key={h.id}
                  data-testid="contact-history-row"
                  onClick={() => store.setActive(h.id)}
                  style={{ display: "flex", alignItems: "center", gap: 10, width: "100%", textAlign: "left", padding: "9px 10px", border: "1px solid var(--border-default)", borderRadius: 10, background: "var(--white)", cursor: "pointer" }}
                >
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: 13, color: "var(--text-primary)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
                      {h.preview || h.subject || "Conversation"}
                    </div>
                    <div style={{ fontSize: 11, color: "var(--text-tertiary)", fontFamily: "var(--font-mono)", marginTop: 2 }}>
                      {h.reference} · {h.time}
                    </div>
                  </div>
                  <StatusPill status={h.status} />
                </button>
              ))}
            </div>
          </div>
        )}

        {/* Members */}
        <div>
          <div style={sectionLabel}>Members ({active.members.length})</div>
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            {active.members.map((mem, i) => (
              <div key={i} style={{ display: "flex", gap: 10 }}>
                <Avatar name={mem.name} size={36} />
                <div style={{ minWidth: 0, flex: 1 }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                    <span style={{ fontSize: 14, fontWeight: 600 }}>{mem.name}</span>
                    <Badge variant="neutral">{mem.role}</Badge>
                  </div>
                  <div style={{ marginTop: 6, display: "flex", flexDirection: "column", gap: 4 }}>
                    {Object.entries(mem.meta).map(([k, v]) => (
                      <div key={k} style={{ display: "flex", gap: 6, fontSize: 12 }}>
                        <span style={{ fontFamily: "var(--font-mono)", color: "var(--text-tertiary)", whiteSpace: "nowrap" }}>
                          {k}
                        </span>
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
