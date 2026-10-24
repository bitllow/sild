"use client";

import { useEffect, useRef } from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Avatar, Badge, Banner, Button, ComposerBar, MessageBubble, PanelIcon, StatusPill } from "@/components/ds";
import { PendingAttachments, useFilePicker } from "./PendingAttachments";
import { panelStyle } from "./styles";

export const ConversationView = observer(function ConversationView() {
  const store = useStore();
  const active = store.active;
  const picker = useFilePicker(store.atts);
  const transcriptRef = useRef<HTMLDivElement>(null);
  const newestId = active?.messages.length ? active.messages[active.messages.length - 1].id : "";

  // Keyed on the newest id, not the count, so loading older pages does not scroll the
  // reader back down.
  useEffect(() => {
    if (transcriptRef.current) transcriptRef.current.scrollTop = transcriptRef.current.scrollHeight;
  }, [newestId, active?.id]);

  const onTranscriptScroll = async () => {
    const el = transcriptRef.current;
    if (!el || el.scrollTop > 240 || !active?.olderCursor || store.loadingOlder) return;
    // Prepending grows the list above the viewport, so hold the distance from the
    // bottom — anchoring on scrollTop would jump the reader to the new oldest message.
    const fromBottom = el.scrollHeight - el.scrollTop;
    await store.loadOlderMessages();
    el.scrollTop = el.scrollHeight - fromBottom;
  };

  if (!active) {
    return (
      <div
        style={{
          flex: 1,
          minWidth: 0,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          background: "var(--surface-page)",
          color: "var(--text-tertiary)",
          fontSize: 14,
          textAlign: "center",
          padding: 24,
        }}
      >
        {store.loadingConvs
          ? "Loading conversations…"
          : "No conversations yet. New support requests appear here the moment they're assigned."}
      </div>
    );
  }

  const isEmail = active.channel === "email";
  const isQueued = active.status === "queued";
  const isClosed = active.status === "closed";
  const notClosed = !isClosed;

  return (
    <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", background: "var(--surface-page)" }}>
      {/* Header */}
      <div
        data-testid="conversation-header"
        style={{
          height: 64,
          flex: "none",
          padding: "0 18px",
          display: "flex",
          alignItems: "center",
          gap: 12,
          background: "var(--surface-card)",
          borderBottom: "1px solid var(--border-default)",
        }}
      >
        <Avatar name={active.name} presence={active.presence} size={36} />
        <div style={{ minWidth: 0 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <span style={{ fontSize: 15, fontWeight: 700, letterSpacing: "-.01em" }}>{active.name}</span>
            {isEmail && <Badge variant="brand">Email</Badge>}
          </div>
          {store.typingConvId === active.id ? (
            <div style={{ fontSize: 12, color: "var(--brand)", fontWeight: 600 }}>typing…</div>
          ) : isEmail && active.subject ? (
            <div
              title={active.subject}
              style={{ fontSize: 12, color: "var(--text-tertiary)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", maxWidth: 360 }}
            >
              {active.subject}
            </div>
          ) : (
            <div style={{ fontSize: 12, color: "var(--text-tertiary)", fontFamily: "var(--font-mono)" }}>
              {active.reference}
            </div>
          )}
        </div>
        <div style={{ flex: 1 }} />
        <StatusPill status={active.status} />
        {isQueued && (
          <Button size="sm" onClick={store.claim} disabled={store.actionBusy}>
            Claim
          </Button>
        )}
        {notClosed && (
          <Button size="sm" variant="secondary" onClick={store.closeConv} disabled={store.actionBusy}>
            Close conversation
          </Button>
        )}
        <button onClick={store.togglePanel} aria-label="Toggle details" style={panelStyle(store.panelOpen)}>
          <PanelIcon size={20} />
        </button>
      </div>

      {/* Transcript */}
      <div
        ref={transcriptRef}
        data-testid="thread-transcript"
        onScroll={() => void onTranscriptScroll()}
        style={{ flex: 1, overflowY: "auto", padding: "20px 22px", display: "flex", flexDirection: "column", gap: 14 }}
      >
        {active.olderCursor && (
          <div
            data-testid="thread-older"
            style={{ alignSelf: "center", fontSize: 12, color: "var(--text-tertiary)" }}
          >
            {store.loadingOlder ? "Loading earlier messages…" : "Scroll up for earlier messages"}
          </div>
        )}
        {active.messages.map((m) => (
          <MessageBubble
            key={m.id}
            direction={m.dir}
            author={m.author}
            time={m.time}
            body={m.body}
            channel={m.channel}
            internal={m.internal}
            system={m.system}
            attachments={m.attachments}
            readReceipt={m.read}
          />
        ))}
      </div>

      {/* Composer / closed banner */}
      <div style={{ flex: "none", padding: "12px 18px 16px", background: "var(--surface-card)", borderTop: "1px solid var(--border-default)" }}>
        {isClosed ? (
          <Banner variant="info">
            This conversation is closed. Closed is terminal — open a new support request to continue.
          </Banner>
        ) : (
          <>
            <PendingAttachments atts={store.atts} />
            {picker.input}
            <ComposerBar
              value={store.composer}
              onChange={(v) => store.setComposer(v)}
              onSend={store.sendMessage}
              onAttach={picker.open}
              canSendEmpty={store.atts.pending.length > 0}
              disabled={store.atts.uploading > 0}
              showInternalToggle
              internal={store.internal}
              onToggleInternal={(v) => store.setInternal(v)}
            />
          </>
        )}
      </div>
    </div>
  );
});
