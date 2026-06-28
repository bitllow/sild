"use client";

import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Avatar, Button, KeyIcon, Select, Switch, Tag, TrashIcon } from "@/components/ds";
import type { PlatformRole } from "@/store/types";
import { tabStyle } from "./styles";

const ROLE_OPTIONS = [
  { value: "owner", label: "owner" },
  { value: "admin", label: "admin" },
  { value: "agent", label: "agent" },
];

const PlusInline = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ marginRight: 5 }}>
    <path d="M5 12h14M12 5v14" />
  </svg>
);

const card: React.CSSProperties = {
  background: "var(--white)",
  border: "1px solid var(--border-default)",
  borderRadius: 12,
  boxShadow: "var(--shadow-sm)",
  overflow: "hidden",
};
const rowBorder = "1px solid var(--border-subtle)";

export const Settings = observer(function Settings() {
  const store = useStore();
  const tab = store.settingsTab;

  return (
    <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", background: "var(--surface-page)" }}>
      <div style={{ padding: "22px 28px 0", flex: "none" }}>
        <h1 style={{ fontSize: 22 }}>Settings</h1>
        <div style={{ display: "flex", gap: 24, marginTop: 18, borderBottom: "1px solid var(--border-default)" }}>
          <button onClick={() => store.setSettingsTab("keys")} style={tabStyle(tab === "keys")}>
            API keys
          </button>
          <button onClick={() => store.setSettingsTab("webhooks")} style={tabStyle(tab === "webhooks")}>
            Webhooks
          </button>
          <button onClick={() => store.setSettingsTab("team")} style={tabStyle(tab === "team")}>
            Team
          </button>
        </div>
      </div>

      <div style={{ flex: 1, overflowY: "auto", padding: "24px 28px" }}>
        <div style={{ maxWidth: 760 }}>
          {tab === "keys" && (
            <div style={card}>
              <div style={{ padding: "16px 18px", borderBottom: rowBorder, display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                <div>
                  <div style={{ fontSize: 15, fontWeight: 700 }}>API keys</div>
                  <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>
                    Server-side only. Shown once on creation — store it safely.
                  </div>
                </div>
                <Button size="sm" onClick={store.openKeyDialog}>
                  <PlusInline />
                  New key
                </Button>
              </div>
              {store.keys.map((k) => (
                <div key={k.id} style={{ padding: "14px 18px", display: "flex", alignItems: "center", gap: 14, borderBottom: rowBorder }}>
                  <div style={{ width: 34, height: 34, flex: "none", borderRadius: 8, background: "var(--brand-subtle)", color: "var(--brand)", display: "flex", alignItems: "center", justifyContent: "center" }}>
                    <KeyIcon size={18} />
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: 14, fontWeight: 600 }}>{k.label}</div>
                    <div style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-tertiary)", marginTop: 2 }}>
                      {k.masked}
                    </div>
                  </div>
                  <span style={{ fontSize: 12, color: "var(--text-tertiary)", whiteSpace: "nowrap" }}>{k.created}</span>
                  <Button size="sm" variant="danger" onClick={() => store.revokeKey(k.id)}>
                    Revoke
                  </Button>
                </div>
              ))}
            </div>
          )}

          {tab === "webhooks" && (
            <div style={card}>
              <div style={{ padding: "16px 18px", borderBottom: rowBorder }}>
                <div style={{ fontSize: 15, fontWeight: 700 }}>Webhook endpoints</div>
                <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>
                  Signed POST per event, with retry and a delivery log.
                </div>
              </div>
              {store.webhooks.map((w) => (
                <div key={w.id} style={{ padding: "14px 18px", display: "flex", alignItems: "center", gap: 14, borderBottom: rowBorder }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontFamily: "var(--font-mono)", fontSize: 13, color: "var(--text-primary)", wordBreak: "break-all" }}>
                      {w.url}
                    </div>
                    <div style={{ marginTop: 7, display: "flex", gap: 6, flexWrap: "wrap" }}>
                      {w.events.map((ev) => (
                        <Tag key={ev} mono>
                          {ev}
                        </Tag>
                      ))}
                    </div>
                  </div>
                  <Switch checked={w.active} onChange={(v) => store.toggleWebhook(w.id, v)} />
                  <button
                    onClick={() => store.deleteWebhook(w.id)}
                    aria-label="Delete webhook"
                    style={{ width: 32, height: 32, flex: "none", display: "flex", alignItems: "center", justifyContent: "center", border: 0, background: "transparent", borderRadius: 6, cursor: "pointer", color: "var(--text-tertiary)" }}
                  >
                    <TrashIcon size={18} />
                  </button>
                </div>
              ))}
            </div>
          )}

          {tab === "team" && (
            <div style={card}>
              <div style={{ padding: "16px 18px", borderBottom: rowBorder }}>
                <div style={{ fontSize: 15, fontWeight: 700 }}>Team</div>
                <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>
                  Platform roles guard the API and the inbox. owner and admin can manage keys and webhooks.
                </div>
              </div>
              {store.team.map((t) => (
                <div key={t.id} style={{ padding: "14px 18px", display: "flex", alignItems: "center", gap: 12, borderBottom: rowBorder }}>
                  <Avatar name={t.name} size={36} />
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: 14, fontWeight: 600 }}>{t.name}</div>
                    <div style={{ fontSize: 12, color: "var(--text-tertiary)" }}>{t.email}</div>
                  </div>
                  <div style={{ width: 130, flex: "none" }}>
                    <Select
                      options={ROLE_OPTIONS}
                      value={t.role}
                      onChange={(e) => store.setRole(t.id, e.target.value as PlatformRole)}
                      size="sm"
                    />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
});
