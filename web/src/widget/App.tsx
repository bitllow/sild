import { useEffect, useRef, useState } from "preact/hooks";
import type { SildClient } from "../core/client";
import type { PendingAttachment, SildConfig, WidgetState } from "../core/types";

function useClientState(client: SildClient): WidgetState {
  const [, setTick] = useState(0);
  useEffect(() => client.subscribe(() => setTick((t) => t + 1)), [client]);
  return client.state;
}

// ── icons ──────────────────────────────────────────────────────────────────
const ChatIcon = () => (
  <svg width="27" height="27" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z" />
  </svg>
);
const CloseIcon = () => (
  <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M18 6 6 18M6 6l12 12" />
  </svg>
);
const BackIcon = () => (
  <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M19 12H5M12 19l-7-7 7-7" />
  </svg>
);
const SendIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M22 2 11 13M22 2l-7 20-4-9-9-4 20-7" />
  </svg>
);
const ArrowIcon = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M5 12h14M12 5l7 7-7 7" />
  </svg>
);
const ClipIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48" />
  </svg>
);

// inlineImages = images shown in the thread; otherAttachments = files listed below.
const isInlineImage = (a: { disposition: string; mimeType: string; url?: string }) =>
  a.disposition === "inline" && a.mimeType.startsWith("image/") && !!a.url;

export function App({ client, config }: { client: SildClient; config: SildConfig }) {
  const [open, setOpen] = useState(false);
  const started = useRef(false);
  const state = useClientState(client);
  // Draft = the user clicked "New conversation" but hasn't sent yet. The
  // conversation is created server-side only on the first send, so a click never
  // leaves an empty conversation in the inbox.
  const [draft, setDraft] = useState(false);

  const toggle = () => {
    const next = !open;
    setOpen(next);
    if (next && !started.current) {
      started.current = true;
      void client.start(config.conversationId);
    }
  };

  // guest tokens are scoped to one thread → no list/back affordance (§9)
  const guestThreadOnly = !!config.conversationId;
  const inThread = !!state.activeId || draft;
  const onBack = () => (draft ? setDraft(false) : client.backToList());

  return (
    <>
      {open && (
        <div class="panel" role="dialog" aria-label="Support chat">
          {/* On mobile the panel is full-screen and the launcher is hidden, so an
              in-panel close button is the only way out (and never covers the
              composer). Hidden on desktop, where the launcher toggles closed. */}
          <button class="mobile-close" aria-label="Close chat" onClick={() => setOpen(false)}>
            <CloseIcon />
          </button>
          {inThread ? (
            <Thread
              client={client}
              state={state}
              guestThreadOnly={guestThreadOnly}
              draft={draft && !state.activeId}
              onBack={onBack}
              onCreated={() => setDraft(false)}
            />
          ) : (
            <Home client={client} state={state} onNew={() => setDraft(true)} />
          )}
        </div>
      )}
      <button class={`launcher${open ? " open" : ""}`} aria-label="Chat with us" onClick={toggle}>
        {open ? <CloseIcon /> : <ChatIcon />}
      </button>
    </>
  );
}

function Home({ client, state, onNew }: { client: SildClient; state: WidgetState; onNew: () => void }) {
  return (
    <>
      <div class="brandhead">
        <SildMark />
        <h1>Hi there.</h1>
        <p>How can we help? We typically reply in a few minutes.</p>
      </div>
      <div class="body">
        <div class="card">
          <h2>Send us a message</h2>
          <p>We'll get back to you here. No queue numbers.</p>
          <button class="btn" onClick={onNew}>
            New conversation <ArrowIcon />
          </button>
        </div>
        {state.conversations.length > 0 && <div class="eyebrow">Recent</div>}
        {state.conversations.map((c) => (
          <div class="row" key={c.id} onClick={() => void client.openConversation(c.id)}>
            <span class="av" style={{ background: "var(--brand)", color: "#fff" }}>S</span>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div class="name">Support</div>
              <div class="prev">{c.preview}</div>
            </div>
            <span class="time">{c.time}</span>
          </div>
        ))}
        {state.error && <div class="note">{state.error}</div>}
      </div>
    </>
  );
}

function Thread({
  client,
  state,
  guestThreadOnly,
  draft,
  onBack,
  onCreated,
}: {
  client: SildClient;
  state: WidgetState;
  guestThreadOnly: boolean;
  draft: boolean;
  onBack: () => void;
  onCreated: () => void;
}) {
  const [text, setText] = useState("");
  const [atts, setAtts] = useState<PendingAttachment[]>([]);
  const [uploading, setUploading] = useState(0);
  const scroller = useRef<HTMLDivElement>(null);
  const taRef = useRef<HTMLTextAreaElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (scroller.current) scroller.current.scrollTop = scroller.current.scrollHeight;
  }, [state.messages.length, state.loadingThread]);

  const closed = !draft && state.conversations.find((c) => c.id === state.activeId)?.closed;
  const canSend = (!!text.trim() || atts.length > 0) && !closed && uploading === 0;

  const onFiles = (e: Event) => {
    const input = e.currentTarget as HTMLInputElement;
    const files = Array.from(input.files || []);
    input.value = ""; // allow re-selecting the same file
    for (const f of files) {
      setUploading((n) => n + 1);
      client
        .upload(f)
        .then((a) => setAtts((p) => [...p, a]))
        .catch(() => {})
        .finally(() => setUploading((n) => n - 1));
    }
  };

  const submit = () => {
    if (!canSend) return;
    const t = text.trim();
    const sending = atts;
    setText("");
    setAtts([]);
    if (taRef.current) taRef.current.style.height = "auto";
    if (draft) {
      // Create the conversation only now (on first send), then post the message.
      void client.openSupportRequest().then(() => {
        onCreated();
        return client.send(t, sending);
      });
    } else {
      void client.send(t, sending);
    }
  };

  return (
    <>
      <div class="threadhead">
        {!guestThreadOnly && (
          <button class="iconbtn" aria-label="Back" onClick={onBack}>
            <BackIcon />
          </button>
        )}
        <span class="av">S</span>
        <div>
          <div class="name">Sild support</div>
          <div class="sub">
            {draft
              ? "Type your message to start"
              : state.connection === "connected"
                ? "Replies in a few minutes"
                : "Connecting…"}
          </div>
        </div>
      </div>
      <div class="body" ref={scroller}>
        {state.loadingThread && <div class="note">Loading…</div>}
        {state.messages.map((m) => {
          const images = (m.attachments || []).filter(isInlineImage);
          const files = (m.attachments || []).filter((a) => !isInlineImage(a));
          return (
            <div class={`msg ${m.system ? "system" : m.direction}`} key={m.id}>
              {!m.system && (m.author || m.time) && (
                <div class="meta">
                  {m.author && <span class="author">{m.author}</span>}
                  {m.time && <span class="mtime">{m.time}</span>}
                </div>
              )}
              {images.map((a, i) => (
                <a class="imglink" href={a.url} target="_blank" rel="noopener noreferrer" key={`img${i}`}>
                  <img class="att-img" src={a.url} alt={a.filename} />
                </a>
              ))}
              {m.body && <div class="bubble">{m.body}</div>}
              {files.length > 0 && (
                <div class="atts">
                  {files.map((a, i) => (
                    <a class="att-chip" href={a.url} target="_blank" rel="noopener noreferrer" download={a.filename} key={`f${i}`}>
                      <ClipIcon />
                      <span class="att-name">{a.filename}</span>
                    </a>
                  ))}
                </div>
              )}
            </div>
          );
        })}
        {!state.loadingThread && state.messages.length === 0 && (
          <div class="note">Send a message to start the conversation.</div>
        )}
      </div>
      <div class="composer">
        {closed && <div class="banner">This conversation is closed.</div>}
        {(atts.length > 0 || uploading > 0) && (
          <div class="pending">
            {atts.map((a, i) => (
              <span class="pchip" key={i}>
                <span class="att-name">{a.filename}</span>
                <button aria-label="Remove" onClick={() => setAtts((p) => p.filter((_, j) => j !== i))}>
                  ✕
                </button>
              </span>
            ))}
            {uploading > 0 && <span class="pchip muted">Uploading…</span>}
          </div>
        )}
        <div class="inputwrap">
          <button class="attachbtn" aria-label="Attach a file" disabled={closed} onClick={() => fileRef.current?.click()}>
            <ClipIcon />
          </button>
          <input ref={fileRef} type="file" multiple style={{ display: "none" }} onChange={onFiles} />
          <textarea
            ref={taRef}
            rows={1}
            placeholder="Write a message…"
            value={text}
            disabled={closed}
            onInput={(e) => {
              const el = e.currentTarget;
              setText(el.value);
              el.style.height = "auto";
              el.style.height = Math.min(el.scrollHeight, 120) + "px";
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                submit();
              }
            }}
          />
          <button class="send" aria-label="Send" disabled={!canSend} onClick={submit}>
            <SendIcon />
          </button>
        </div>
        <div class="powered">Powered by Sild</div>
      </div>
    </>
  );
}

function SildMark() {
  return (
    <svg class="tile" width="34" height="34" viewBox="0 0 40 40" fill="none" aria-label="Sild">
      <rect width="40" height="40" rx="11" fill="rgba(255,255,255,.16)" />
      <g transform="translate(6.6 12.2) scale(0.202)" fill="#fff">
        <circle cx="33" cy="50" r="16" />
        <path d="M28 60 L42 60 L21 71 Z" />
        <circle cx="66" cy="49" r="13" />
        <path d="M60 58 L72 58 L66 70 Z" />
        <circle cx="99" cy="50" r="16" />
        <path d="M90 60 L104 60 L111 71 Z" />
      </g>
      <g transform="translate(6.6 12.2) scale(0.202)" fill="none" stroke="#fff" stroke-width="7" stroke-linecap="round">
        <path d="M33 33 Q66 -9 99 33" />
      </g>
    </svg>
  );
}
