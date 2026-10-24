"use client";

import { useEffect, useState } from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import type { ApiIdentityProvider } from "@/api/admin";
import { Badge, Button, CopyIcon, Input, Switch, Textarea } from "@/components/ds";
import { cardStyle as card, fieldLabel, rowBorder } from "./styles";

// Settings → Embedding: frame the inbox in the integrator's app, signed in with
// the token their user already has (docs/embedded-sign-in.md).
export const Embedding = observer(function Embedding() {
  const store = useStore();
  const emb = store.embedding;

  useEffect(() => {
    void emb.load();
  }, [emb]);

  if (!emb.loaded) return null;
  const p = emb.provider;

  return (
    <>
      {p && <EmbedKey provider={p} />}
      {/* Keyed by the saved version, so a save or reload resets the form to what is stored. */}
      <EmbeddingForm key={p?.updated_at ?? "off"} provider={p} />
    </>
  );
});

const EmbedKey = observer(function EmbedKey({ provider }: { provider: ApiIdentityProvider }) {
  const store = useStore();
  const snippet = `<iframe src="${window.location.origin}/embed?key=${provider.embed_key}"></iframe>`;
  return (
    <div style={{ ...card, marginBottom: 18 }}>
      <div style={{ padding: "16px 18px", borderBottom: rowBorder }}>
        <div style={{ fontSize: 15, fontWeight: 700 }}>Embed the inbox</div>
        <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>
          Put this frame on a page of your app. The page posts the frame its signed-in user&apos;s token.
        </div>
      </div>
      <div style={{ padding: "14px 18px", borderBottom: rowBorder }}>
        <div style={fieldLabel}>Embed key</div>
        <code data-testid="embed-key" style={{ display: "block", marginTop: 8, fontSize: 13, overflowWrap: "anywhere" }}>
          {provider.embed_key}
        </code>
      </div>
      <div style={{ padding: "14px 18px" }}>
        <pre
          data-testid="embed-snippet"
          style={{
            margin: 0, fontSize: 12.5, lineHeight: 1.55, overflowX: "auto",
            background: "var(--surface-page)", border: rowBorder, borderRadius: 8, padding: 12,
          }}
        >
          {snippet}
        </pre>
        <div style={{ marginTop: 12 }}>
          <Button variant="secondary" onClick={() => store.copyEmbedSnippet(snippet)}>
            <CopyIcon />
            {store.embedSnippetCopied ? "Copied" : "Copy snippet"}
          </Button>
        </div>
      </div>
    </div>
  );
});

const EmbeddingForm = observer(function EmbeddingForm({ provider }: { provider: ApiIdentityProvider | null }) {
  const store = useStore();
  const emb = store.embedding;
  const [url, setUrl] = useState(provider?.validator_url ?? "");
  const [token, setToken] = useState("");
  const [origins, setOrigins] = useState((provider?.allowed_origins ?? []).join("\n"));
  const [embeddedOnly, setEmbeddedOnly] = useState(provider?.embedded_only ?? false);
  const [confirmOff, setConfirmOff] = useState(false);

  const stored = !!provider?.has_validator_token && url.trim() === provider.validator_url;

  const save = () =>
    void emb.save({
      validator_url: url.trim(),
      ...(token ? { validator_token: token } : {}),
      allowed_origins: origins.split(/[\s,]+/).filter(Boolean),
      embedded_only: embeddedOnly,
    });

  return (
    <div style={card} data-testid="embedding-settings">
      <div style={{ padding: "16px 18px", borderBottom: rowBorder, display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
        <div>
          <div style={{ fontSize: 15, fontWeight: 700 }}>Embedded sign-in</div>
          <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>
            Your validator tells Sild whose token the frame was given. Members sign in as the team member
            linked to that identity in Settings → Team.
          </div>
        </div>
        <Badge variant={provider ? "success" : "neutral"} data-testid="embedding-status">
          {provider ? "On" : "Off"}
        </Badge>
      </div>

      <div style={{ padding: "16px 18px", borderBottom: rowBorder, display: "flex", flexDirection: "column", gap: 12 }}>
        <Input
          label="Validator URL"
          data-testid="embedding-validator-url"
          placeholder="https://app.example/sild/validate"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
        />
        <Input
          label="Validator token"
          type="password"
          autoComplete="off"
          data-testid="embedding-validator-token"
          placeholder={stored ? "Stored — leave empty to keep it" : "The API token Sild presents to your validator"}
          hint={stored ? undefined : provider?.has_validator_token ? "A new URL needs the token again." : undefined}
          value={token}
          onChange={(e) => setToken(e.target.value)}
        />
        {stored && (
          <div>
            <Badge variant="success" data-testid="embedding-token-stored">Token stored</Badge>
          </div>
        )}
        <Textarea
          label="Allowed origins"
          data-testid="embedding-origins"
          placeholder="https://app.example"
          hint="One per line. Only these pages may frame the inbox and hand it a token."
          rows={3}
          value={origins}
          onChange={(e) => setOrigins(e.target.value)}
        />
      </div>

      <div style={{ padding: "14px 18px", display: "flex", alignItems: "center", gap: 14, borderBottom: rowBorder }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontSize: 14, fontWeight: 600 }}>Embedded sign-in only</div>
          <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>
            Linked members can then sign in only through your app&apos;s page, and their other sessions end.
            Unlinked members keep their password or Google sign-in.
          </div>
        </div>
        <Switch checked={embeddedOnly} onChange={setEmbeddedOnly} data-testid="embedding-embedded-only" />
      </div>

      <div style={{ padding: "14px 18px", display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        {emb.message && (
          <span
            data-testid="embedding-message"
            style={{ fontSize: 13, color: emb.message.kind === "error" ? "var(--danger)" : "var(--success, #137333)" }}
          >
            {emb.message.text}
          </span>
        )}
        <div style={{ flex: 1 }} />
        {provider && !confirmOff && (
          <Button variant="secondary" data-testid="embedding-turn-off" onClick={() => setConfirmOff(true)} disabled={emb.busy}>
            Turn off embedded sign-in
          </Button>
        )}
        <Button data-testid="embedding-save" onClick={save} disabled={emb.busy || !url.trim()}>
          Save
        </Button>
      </div>

      {confirmOff && (
        <div
          data-testid="embedding-confirm-off"
          style={{ padding: "14px 18px", borderTop: rowBorder, display: "flex", alignItems: "center", gap: 10, background: "var(--surface-sunken)" }}
        >
          <div style={{ flex: 1, fontSize: 13, color: "var(--text-secondary)" }}>
            Every embedded session ends now, and the frame stops signing anyone in. The embed key is not kept.
          </div>
          <Button variant="secondary" size="sm" onClick={() => setConfirmOff(false)}>
            Cancel
          </Button>
          <Button variant="danger" size="sm" data-testid="embedding-confirm-off-yes" onClick={() => void emb.turnOff()} disabled={emb.busy}>
            Turn off
          </Button>
        </div>
      )}
    </div>
  );
});
