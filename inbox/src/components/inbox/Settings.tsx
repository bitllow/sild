"use client";

import { useState } from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { adminApi } from "@/api/admin";
import { Badge, Button, CopyIcon, Input, KeyIcon, Select, Switch, Tag, TrashIcon } from "@/components/ds";
import { cardStyle as card, fieldLabel, rowBorder, tabStyle } from "./styles";
import { Appearance } from "./Appearance";
import { TeamRoles } from "./TeamRoles";

const PlusInline = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ marginRight: 5 }}>
    <path d="M5 12h14M12 5v14" />
  </svg>
);

export const Settings = observer(function Settings() {
  const store = useStore();
  const tab = store.settingsTab;

  return (
    <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", background: "var(--surface-page)" }}>
      <div style={{ padding: "22px 28px 0", flex: "none" }}>
        <h1 style={{ fontSize: 22 }}>Settings</h1>
        <div style={{ display: "flex", gap: 24, marginTop: 18, borderBottom: "1px solid var(--border-default)" }}>
          <button onClick={() => store.setSettingsTab("installation")} style={tabStyle(tab === "installation")}>
            Installation
          </button>
          <button onClick={() => store.setSettingsTab("channels")} style={tabStyle(tab === "channels")}>
            Channels
          </button>
          <button onClick={() => store.setSettingsTab("appearance")} style={tabStyle(tab === "appearance")}>
            Appearance
          </button>
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

      {/* Appearance breaks out of the 760px column into a full-width split. */}
      {tab === "appearance" ? (
        <div style={{ flex: 1, overflowY: "auto", padding: "24px 28px" }}>
          <Appearance />
        </div>
      ) : (
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
                <Button size="sm" data-testid="new-key" onClick={() => void store.openKeyDialog()}>
                  <PlusInline />
                  New key
                </Button>
              </div>
              <BuildToken />
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
                    <div data-testid="key-reach" style={{ fontSize: 12.5, color: "var(--text-secondary)", marginTop: 4 }}>
                      Reaches {k.reach}
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
              <TeamRoles />
            </div>
          )}

          {tab === "installation" && <Installation />}

          {tab === "channels" && <Channels />}
        </div>
      </div>
      )}
    </div>
  );
});

// Installation shows the App ID and the embed snippet. The widget's first paint is
// unauthenticated and keyed by app_id, so without this a customer has nowhere to
// read the one value the embed cannot work without.
const Installation = observer(function Installation() {
  const store = useStore();
  const appId = store.appId;
  const snippet = `<script src="${store.widgetSrc}"></script>
<script>
  Sild.init({
    appId: "${appId || "<your app id>"}",
    tokenProvider: () => fetch("/your-backend/sild-token").then((r) => r.text()),
  });
</script>`;

  return (
    <>
      <div style={card}>
        <div style={{ padding: "16px 18px", borderBottom: rowBorder }}>
          <div style={{ fontWeight: 600 }}>App ID</div>
          <div style={{ color: "var(--text-secondary)", fontSize: 13, marginTop: 2 }}>
            Identifies your tenant to the messenger before a visitor has a token. Safe to
            put in page source — it grants no access on its own.
          </div>
        </div>
        <div style={{ padding: "14px 18px", display: "flex", alignItems: "center", gap: 12 }}>
          <code style={{ flex: 1, minWidth: 0, fontSize: 13, overflowWrap: "anywhere" }}>
            {appId || "—"}
          </code>
          <Button variant="secondary" onClick={() => store.copyAppId()} disabled={!appId}>
            <CopyIcon />
            {store.appIdCopied ? "Copied" : "Copy"}
          </Button>
        </div>
      </div>

      <div style={{ ...card, marginTop: 18 }}>
        <div style={{ padding: "16px 18px", borderBottom: rowBorder }}>
          <div style={{ fontWeight: 600 }}>Embed the messenger</div>
          <div style={{ color: "var(--text-secondary)", fontSize: 13, marginTop: 2 }}>
            Drop this into any page. <code>tokenProvider</code> returns a token your backend
            mints with <code>POST /v1/tokens</code>.
          </div>
        </div>
        <div style={{ padding: "14px 18px" }}>
          <pre
            style={{
              margin: 0, fontSize: 12.5, lineHeight: 1.55, overflowX: "auto",
              background: "var(--surface-page)", border: rowBorder, borderRadius: 8, padding: 12,
            }}
          >
            {snippet}
          </pre>
          <div style={{ marginTop: 12 }}>
            <Button variant="secondary" onClick={() => store.copySnippet(snippet)}>
              <CopyIcon />
              {store.snippetCopied ? "Copied" : "Copy snippet"}
            </Button>
          </div>
        </div>
      </div>
    </>
  );
});

const OTHER_CHANNELS: { name: string; desc: string }[] = [
  { name: "WhatsApp", desc: "Reply to WhatsApp messages from the inbox." },
  { name: "SMS", desc: "Turn inbound texts into conversations." },
  { name: "Slack", desc: "Handle Slack messages alongside email." },
];

// SettingRow is one labelled setting with its control on the right.
function SettingRow({
  title,
  desc,
  children,
}: {
  title: string;
  desc: string;
  children: React.ReactNode;
}) {
  return (
    <div style={{ padding: "14px 18px", display: "flex", alignItems: "center", gap: 14, borderBottom: rowBorder }}>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontSize: 14, fontWeight: 600 }}>{title}</div>
        <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>{desc}</div>
      </div>
      {children}
    </div>
  );
}

function ToggleRow(props: { title: string; desc: string; checked: boolean; onChange: (v: boolean) => void; testId?: string }) {
  return (
    <SettingRow title={props.title} desc={props.desc}>
      <Switch checked={props.checked} onChange={props.onChange} data-testid={props.testId} />
    </SettingRow>
  );
}

// The mono value field shared by the forwarding address and the push project.
const monoField: React.CSSProperties = {
  flex: 1,
  minWidth: 0,
  fontFamily: "var(--font-mono)",
  fontSize: 13,
  background: "var(--surface-sunken)",
  border: "1px solid var(--border-subtle)",
  borderRadius: 8,
  padding: "9px 12px",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

const Channels = observer(function Channels() {
  const store = useStore();
  const ch = store.emailChannel;

  return (
    <>
      <div style={card}>
        <div style={{ padding: "16px 18px", borderBottom: rowBorder, display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
          <div>
            <div style={{ fontSize: 15, fontWeight: 700 }}>Email</div>
            <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>
              Set up forwarding from your support mailbox to the address below. Every email that arrives
              becomes a conversation in this inbox.
            </div>
          </div>
          <span
            style={{
              flex: "none",
              fontSize: 12,
              fontWeight: 600,
              padding: "4px 10px",
              borderRadius: 999,
              whiteSpace: "nowrap",
              background: ch?.verified ? "var(--success-subtle, #E6F4EA)" : "var(--warning-subtle, #FBF3E0)",
              color: ch?.verified ? "var(--success, #137333)" : "var(--warning, #8A6100)",
            }}
          >
            {ch?.verified ? "Verified" : "Awaiting first email"}
          </span>
        </div>

        <div style={{ padding: "16px 18px", borderBottom: rowBorder }}>
          <div style={fieldLabel}>Forwarding address</div>
          <div style={{ display: "flex", alignItems: "center", gap: 10, marginTop: 8 }}>
            <code style={{ ...monoField, color: "var(--text-primary)" }}>{ch?.forwardingAddress || "…"}</code>
            <Button size="sm" variant="secondary" onClick={store.copyForwardingAddress} disabled={!ch}>
              <CopyIcon size={15} />
              <span style={{ marginLeft: 5 }}>{store.channelCopied ? "Copied" : "Copy"}</span>
            </Button>
          </div>
        </div>

        <ToggleRow
          title="Auto-reply to new emails"
          desc="Send an acknowledgement with your expected response time."
          checked={!!ch?.autoReply}
          onChange={store.toggleAutoReply}
        />
        <ToggleRow
          title="Mark autoresponder emails as spam"
          desc="Keep out-of-office and no-reply bounces out of the queue."
          checked={!!ch?.spamFilter}
          onChange={store.toggleSpamFilter}
        />
      </div>

      <Push />

      <div style={{ ...card, marginTop: 20 }}>
        <div style={{ padding: "16px 18px", borderBottom: rowBorder }}>
          <div style={{ fontSize: 15, fontWeight: 700 }}>Other channels</div>
          <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>
            Connect more channels to handle every conversation in one place.
          </div>
        </div>
        {OTHER_CHANNELS.map((c) => (
          <div key={c.name} style={{ padding: "14px 18px", display: "flex", alignItems: "center", gap: 14, borderBottom: rowBorder }}>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontSize: 14, fontWeight: 600 }}>{c.name}</div>
              <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>{c.desc}</div>
            </div>
            <Button size="sm" variant="secondary" disabled>
              Connect
            </Button>
          </div>
        ))}
      </div>
    </>
  );
});

const SENDER_SOURCE_OPTIONS = [
  { value: "brand", label: "your brand name" },
  { value: "agent", label: "the agent who replied" },
];

// Push (§5.5). Notifications go through the tenant's own push project, so the
// credential is theirs to supply — Sild cannot address an app it does not own.
const Push = observer(function Push() {
  const store = useStore();
  const ch = store.pushChannel;
  // The credential is the owner's capability, so an admin never sees the panel.
  if (!store.can("push_config.manage") || !ch) return null;
  const configured = !!ch.project_id;

  const onFile = (file: File | undefined) => {
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => void store.uploadPushCredential(String(reader.result));
    reader.readAsText(file);
  };

  return (
    <div style={{ ...card, marginTop: 20 }} data-testid="push-settings">
      <div style={{ padding: "16px 18px", borderBottom: rowBorder, display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
        <div>
          <div style={{ fontSize: 15, fontWeight: 700 }}>Push notifications</div>
          <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>
            Notify people in your app when they are not looking at it. Upload the service-account key
            from your own Firebase project — the same project your app was built against.
          </div>
        </div>
        <Badge variant={ch.verified ? "success" : "warning"} data-testid="push-status">
          {ch.verified ? "Delivering" : configured ? "Awaiting first delivery" : "Not set up"}
        </Badge>
      </div>

      <div style={{ padding: "16px 18px", borderBottom: rowBorder }}>
        <div style={fieldLabel}>Service account</div>
        {configured ? (
          <div style={{ display: "flex", alignItems: "center", gap: 10, marginTop: 8 }}>
            <code data-testid="push-project" style={monoField}>
              {ch.project_id} · {ch.client_email}
            </code>
            <Button size="sm" variant="secondary" onClick={store.removePushCredential} disabled={store.pushBusy}>
              <TrashIcon size={15} />
              <span style={{ marginLeft: 5 }}>Remove</span>
            </Button>
          </div>
        ) : (
          <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 8 }}>
            No credential yet. Nothing is notified until one is uploaded.
          </div>
        )}
        <label
          style={{
            display: "inline-flex",
            alignItems: "center",
            marginTop: 10,
            padding: "6px 12px",
            fontSize: 13,
            fontWeight: 600,
            borderRadius: 8,
            cursor: store.pushBusy ? "default" : "pointer",
            opacity: store.pushBusy ? 0.6 : 1,
            border: "1px solid var(--border-default)",
            background: "var(--white)",
          }}
        >
          <input
            type="file"
            accept="application/json,.json"
            data-testid="push-credential-file"
            style={{ display: "none" }}
            disabled={store.pushBusy}
            onChange={(e) => onFile(e.target.files?.[0])}
          />
          {configured ? "Replace key file" : "Upload key file"}
        </label>
      </div>

      <ToggleRow
        title="Show who sent it"
        desc="Put the sender's name on the notification instead of just “New message”."
        checked={ch.include_sender}
        onChange={store.togglePushIncludeSender}
        testId="push-include-sender"
      />
      <ToggleRow
        title="Show the message"
        desc="Include the message text. Leave off to keep conversations off lock screens."
        checked={ch.include_body}
        onChange={store.togglePushIncludeBody}
        testId="push-include-body"
      />

      <SettingRow
        title="Name support replies as"
        desc="Whose name appears when an agent replies. Messages between your users always name the sender."
      >
        <Select
          value={ch.sender_source}
          options={SENDER_SOURCE_OPTIONS}
          onChange={(e) => store.setPushSenderSource(e.target.value as "brand" | "agent")}
          disabled={!ch.include_sender}
        />
      </SettingRow>

      <TestPushRow />

      {store.pushMessage && (
        <div
          data-testid="push-message"
          style={{
            padding: "12px 18px",
            fontSize: 13,
            color: store.pushMessage.kind === "error" ? "var(--danger, #B3261E)" : "var(--success, #137333)",
          }}
        >
          {store.pushMessage.text}
        </div>
      )}
    </div>
  );
});

// TestPushRow sends to a token pasted from a debug build. Deliberately not a
// picker over registered devices: every registered device belongs to a real
// end user, so a picker rings a customer's phone.
const TestPushRow = observer(function TestPushRow() {
  const store = useStore();
  const [token, setToken] = useState("");
  return (
    <div style={{ padding: "14px 18px", borderBottom: rowBorder }}>
      <div style={{ fontSize: 14, fontWeight: 600 }}>Send a test notification</div>
      <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>
        Paste the device token your app logs on startup to check the whole path.
      </div>
      <div style={{ display: "flex", gap: 10, marginTop: 10 }}>
        <Input
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder="Device token"
          data-testid="push-test-token"
          style={{ flex: 1 }}
        />
        <Button
          size="sm"
          variant="secondary"
          data-testid="push-test-send"
          disabled={!token || store.pushBusy}
          onClick={() => void store.sendTestPush(token)}
        >
          Send
        </Button>
      </div>
    </div>
  );
});

// BuildToken mints a key held to one translation project — what CI pushes and
// pulls with. Publishing is a separate grant: a build that can rewrite text is not
// automatically one that can put it live.
const BuildToken = observer(function BuildToken() {
  const store = useStore();
  const [open, setOpen] = useState(false);
  const [projects, setProjects] = useState<{ id: string; name: string }[]>([]);
  const [project, setProject] = useState("");
  const [publish, setPublish] = useState(false);

  const start = async () => {
    setOpen(true);
    try {
      setProjects(await adminApi.listTranslationProjects());
    } catch {
      /* the platform project is in every tenant, so the fallback below still works */
    }
  };

  const create = async () => {
    await store.openKeyDialog({ projects: [project || projects[0]?.id || "sild"], publish });
    setOpen(false);
    setPublish(false);
  };

  if (!open) {
    return (
      <div style={{ padding: "12px 18px", borderBottom: rowBorder }}>
        <Button size="sm" variant="secondary" data-testid="new-build-token" onClick={() => void start()}>
          New build token
        </Button>
        <span style={{ fontSize: 12.5, color: "var(--text-tertiary)", marginLeft: 10 }}>
          Scoped to one translation project — for CI.
        </span>
      </div>
    );
  }

  return (
    <div style={{ padding: "14px 18px", borderBottom: rowBorder, display: "flex", flexDirection: "column", gap: 10 }}>
      <div style={fieldLabel}>Project</div>
      <Select
        data-testid="build-token-project"
        aria-label="Project"
        size="sm"
        value={project || projects[0]?.id || ""}
        options={projects.map((p) => ({ value: p.id, label: p.name }))}
        onChange={(e) => setProject(e.target.value)}
      />
      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <div style={{ flex: 1, minWidth: 0, fontSize: 13 }}>
          May publish a release
          <div style={{ fontSize: 12, color: "var(--text-tertiary)", marginTop: 2 }}>
            Off means the build pushes drafts and a person publishes them.
          </div>
        </div>
        <Switch
          data-testid="build-token-publish"
          aria-label="May publish a release"
          checked={publish}
          onChange={setPublish}
        />
      </div>
      <div style={{ display: "flex", gap: 8 }}>
        <Button size="sm" data-testid="build-token-create" onClick={() => void create()}>
          Create
        </Button>
        <Button size="sm" variant="ghost" onClick={() => setOpen(false)}>
          Cancel
        </Button>
      </div>
    </div>
  );
});
