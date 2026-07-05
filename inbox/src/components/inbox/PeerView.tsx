"use client";

import { useEffect, useRef } from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { AvatarStack, Badge, PanelIcon } from "@/components/ds";
import { RolePill } from "./PeerBits";
import type { PeerConversation, PeerMessage } from "@/store/peer";

export const PeerView = observer(function PeerView() {
  const store = useStore();
  const peer = store.peer;
  const active = peer.active;
  const scroller = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (scroller.current) scroller.current.scrollTop = scroller.current.scrollHeight;
  }, [active?.messages.length, active?.id]);

  if (!active) {
    return (
      <div style={{ flex: 1, minWidth: 0, display: "flex", alignItems: "center", justifyContent: "center", color: "var(--text-tertiary)", fontSize: 14 }}>
        Select a peer conversation.
      </div>
    );
  }

  return (
    <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", background: "var(--surface-page)" }}>
      {/* Header — participants + reference. No claim/close/status: no assignment. */}
      <div
        data-testid="peer-header"
        style={{
          height: 64,
          flex: "none",
          padding: "0 18px",
          background: "var(--surface-card)",
          borderBottom: "1px solid var(--border-default)",
          display: "flex",
          alignItems: "center",
          gap: 12,
        }}
      >
        <AvatarStack people={active.participants.map((p) => ({ name: p.name }))} size={36} />
        <div style={{ minWidth: 0 }}>
          <div style={{ fontSize: 15, fontWeight: 700, letterSpacing: "-.01em", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
            {peer.title(active)}
          </div>
          <div style={{ fontSize: 12, color: "var(--text-tertiary)", fontFamily: "var(--font-mono)" }}>{active.reference}</div>
        </div>
        <Badge variant="neutral">{peer.kindLabel(active)}</Badge>
        <div style={{ flex: 1 }} />
        <button
          onClick={store.togglePanel}
          aria-label="Toggle details"
          style={{ width: 32, height: 32, display: "flex", alignItems: "center", justifyContent: "center", border: "1px solid var(--border-default)", background: "var(--surface-card)", borderRadius: 8, cursor: "pointer", color: "var(--text-secondary)" }}
        >
          <PanelIcon size={18} />
        </button>
      </div>

      {/* Group log */}
      <div ref={scroller} style={{ flex: 1, overflowY: "auto", padding: "20px 22px", display: "flex", flexDirection: "column", gap: 12 }}>
        {peer.loadingThread && active.messages.length === 0 && (
          <div style={{ color: "var(--text-tertiary)", fontSize: 13 }}>Loading…</div>
        )}
        {active.messages.map((m) => (
          <PeerBubble key={m.id} m={m} peer={peer} />
        ))}
      </div>

      {/* Implicit-join composer — always present; the "observing" hint shows until
          the agent has stepped in. */}
      <div style={{ flex: "none", padding: "12px 18px 16px", background: "var(--surface-card)", borderTop: "1px solid var(--border-default)" }}>
        {!active.joined && (
          <div style={{ fontSize: 12, color: "var(--text-tertiary)", padding: "0 2px 8px", lineHeight: 1.5 }}>
            You&apos;re observing. Sending a message adds you to the conversation — both parties will see you.
          </div>
        )}
        <PeerComposer active={active} />
      </div>
    </div>
  );
});

function PeerBubble({ m, peer }: { m: PeerMessage; peer: ReturnType<typeof useStore>["peer"] }) {
  if (m.joinNote) {
    return (
      <div
        data-testid="peer-message"
        style={{ alignSelf: "center", display: "inline-flex", alignItems: "center", gap: 7, maxWidth: "90%", padding: "7px 13px", borderRadius: 999, background: "var(--coral-50)", color: "var(--coral-600)", fontSize: 12, fontWeight: 600, textAlign: "center" }}
      >
        {m.author} (support) joined — {m.body}
      </div>
    );
  }
  // The signed-in operator's own messages are right-aligned in brand colour. Every
  // other message — the parties AND any other agent who stepped in — is a named,
  // left-aligned bubble, so a second operator isn't mislabelled as "You".
  const style = peer.roleStyle(m.role, m.isAgent);
  return (
    <div data-testid="peer-message" style={{ display: "flex", flexDirection: "column", gap: 3, maxWidth: "100%" }}>
      {!m.mine && (
        <div style={{ display: "flex", alignItems: "center", gap: 7, paddingLeft: 3 }}>
          <span style={{ fontSize: 13, fontWeight: 700, color: style.color }}>{m.author}</span>
          <RolePill role={m.isAgent ? "support" : m.role} isAgent={m.isAgent} peer={peer} />
          {m.time && <span style={{ fontSize: 11, color: "var(--text-tertiary)" }}>{m.time}</span>}
        </div>
      )}
      <div
        style={{
          maxWidth: "82%",
          padding: "9px 13px",
          fontSize: 14,
          lineHeight: 1.45,
          borderRadius: 16,
          alignSelf: m.mine ? "flex-end" : "flex-start",
          background: m.mine ? "var(--brand)" : "var(--surface-card)",
          color: m.mine ? "#fff" : "var(--text-primary)",
          border: m.mine ? "0" : "1px solid var(--border-default)",
          borderBottomRightRadius: m.mine ? 5 : 16,
          borderBottomLeftRadius: m.mine ? 16 : 5,
        }}
      >
        {m.body}
      </div>
      {m.mine && (
        <span style={{ alignSelf: "flex-end", fontSize: 11, color: "var(--text-tertiary)", paddingRight: 3 }}>
          You (support){m.time ? ` · ${m.time}` : ""}
        </span>
      )}
    </div>
  );
}

const PeerComposer = observer(function PeerComposer({ active }: { active: PeerConversation }) {
  const store = useStore();
  const peer = store.peer;
  const taRef = useRef<HTMLTextAreaElement>(null);
  const submit = () => void peer.send();
  return (
    <div style={{ display: "flex", alignItems: "flex-end", gap: 8, border: "1px solid var(--border-default)", borderRadius: 12, padding: 8, background: "var(--surface-card)" }}>
      <textarea
        ref={taRef}
        data-testid="peer-composer"
        rows={1}
        placeholder="Message everyone in this conversation…"
        value={peer.composer}
        onChange={(e) => {
          peer.setComposer(e.target.value);
          const el = e.currentTarget;
          el.style.height = "auto";
          el.style.height = Math.min(el.scrollHeight, 140) + "px";
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            submit();
          }
        }}
        style={{ flex: 1, resize: "none", border: 0, outline: "none", background: "transparent", font: "inherit", fontSize: 14, color: "var(--text-primary)", lineHeight: 1.4, maxHeight: 140 }}
      />
      <button
        data-testid="peer-send"
        aria-label="Send"
        onClick={submit}
        disabled={!peer.composer.trim() || peer.sending}
        style={{
          flex: "none",
          height: 34,
          padding: "0 14px",
          border: 0,
          borderRadius: 9,
          background: peer.composer.trim() ? "var(--brand)" : "var(--surface-sunken)",
          color: peer.composer.trim() ? "#fff" : "var(--text-tertiary)",
          fontFamily: "var(--font-sans)",
          fontSize: 14,
          fontWeight: 600,
          cursor: peer.composer.trim() ? "pointer" : "default",
        }}
      >
        Send
      </button>
    </div>
  );
});
