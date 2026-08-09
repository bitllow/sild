import { useEffect, useRef, useState } from "preact/hooks";
import type { BrandConfig, PendingAttachment, WidgetConversation, WidgetState } from "../core/types";
import type { WidgetClient } from "../core/client";
import type { I18n, Translate } from "../i18n";
import { LAUNCHER_ICON, parseTopics, type WidgetMode } from "./theme";

// Repaint on every notification from the source — new client state, or a
// published bundle replacing the strings currently on screen.
function useSubscribed(source: { subscribe(fn: () => void): () => void }): void {
  const [, setTick] = useState(0);
  useEffect(() => source.subscribe(() => setTick((t) => t + 1)), [source]);
}

function useClientState(client: WidgetClient): WidgetState {
  useSubscribed(client);
  return client.state;
}

function useTranslate(i18n: I18n): Translate {
  useSubscribed(i18n);
  return i18n.t;
}

// ── icons ──────────────────────────────────────────────────────────────────
const ChatIcon = ({ s = 27 }: { s?: number }) => (
  <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z" />
  </svg>
);
const MessageIcon = ({ s = 27 }: { s?: number }) => (
  <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
  </svg>
);
const HelpIcon = ({ s = 27 }: { s?: number }) => (
  <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <circle cx="12" cy="12" r="10" /><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3" /><line x1="12" y1="17" x2="12.01" y2="17" />
  </svg>
);
const SparkleIcon = ({ s = 27 }: { s?: number }) => (
  <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M12 3v4M12 17v4M3 12h4M17 12h4M5.6 5.6l2.8 2.8M15.6 15.6l2.8 2.8M18.4 5.6l-2.8 2.8M8.4 15.6l-2.8 2.8" />
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
const ChevronIcon = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="m9 18 6-6-6-6" />
  </svg>
);
const ClipIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48" />
  </svg>
);
const SpeakerIcon = ({ s = 20 }: { s?: number }) => (
  <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M11 4.7 6 9H2v6h4l5 4.3z" />
    <path d="M15.5 8.5a5 5 0 0 1 0 7M19 5a9 9 0 0 1 0 14" />
  </svg>
);
const SpeakerOffIcon = ({ s = 20 }: { s?: number }) => (
  <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M11 4.7 6 9H2v6h4l5 4.3z" />
    <path d="m22 9-6 6M16 9l6 6" />
  </svg>
);

// SoundToggle is the shared reply-notification control rendered in both widget
// headers (home + thread); muted swaps to the slashed-speaker glyph. Icon stays
// white — the brand-colored header already carries the emphasis.
function SoundToggle({ on, onToggle, t, size = 20 }: { on: boolean; onToggle: () => void; t: Translate; size?: number }) {
  const label = t(on ? "widget.notifications.disable" : "widget.notifications.enable");
  return (
    <button
      class="wsound"
      aria-pressed={on}
      aria-label={label}
      title={label}
      onClick={onToggle}
    >
      {on ? <SpeakerIcon s={size} /> : <SpeakerOffIcon s={size} />}
    </button>
  );
}

// LauncherGlyph renders the configured launcher icon (preset or custom upload).
function LauncherGlyph({ config, size }: { config: BrandConfig; size: number }) {
  const iconSrc = config.iconImgUrl || config.iconImg;
  if (config.launcherIcon === "custom" && iconSrc) {
    return <img src={iconSrc} alt="" style={{ width: size, height: size }} />;
  }
  switch (config.launcherIcon) {
    case "message":
      return <MessageIcon s={size} />;
    case "help":
      return <HelpIcon s={size} />;
    case "sparkle":
      return <SparkleIcon s={size} />;
    default:
      return <ChatIcon s={size} />;
  }
}

const isInlineImage = (a: { disposition: string; mimeType: string; url?: string }) =>
  a.disposition === "inline" && a.mimeType.startsWith("image/") && !!a.url;

export interface AppProps {
  client: WidgetClient;
  config: BrandConfig;
  i18n: I18n;
  /** Conversation to open directly (guest, single-thread mode). */
  conversationId?: string;
  /** Brand name — the header fallback shown when no logo is set. */
  name?: string;
  /** "live" floats over the host page; "preview" fills the Appearance canvas. */
  mode?: WidgetMode;
  /** In preview, which screen to show (driven by the settings Home/Conversation toggle). */
  previewView?: "home" | "chat";
  /** Imperative open request from the host (e.g. an "Open chat" card): opening the
   *  panel and, if given, navigating straight to a conversation. The seq changes
   *  on each request so the effect re-fires. */
  command?: { seq: number; conversationId?: string };
}

export function App({ client, config, i18n, conversationId, name, mode = "live", previewView = "home", command }: AppProps) {
  const preview = mode === "preview";
  const [open, setOpen] = useState(preview);
  const started = useRef(false);
  const state = useClientState(client);
  const t = useTranslate(i18n);
  const [draft, setDraft] = useState(false);
  const recorded = useRef("");

  // Only the locale the runtime settled on is worth reporting — an earlier guess
  // may have been superseded by the tenant's offering.
  const recordLocale = () => {
    if (i18n.locale === recorded.current) return;
    recorded.current = i18n.locale;
    client.recordLocale(i18n.locale);
  };

  // Published strings are fetched once the client holds a token, off the boot path.
  const start = (id?: string) => {
    started.current = true;
    void Promise.resolve(client.start(id))
      .then(() => i18n.refresh())
      .then(recordLocale);
  };

  // A tab left open for hours would never see a publish otherwise.
  useEffect(() => {
    if (preview || typeof document === "undefined") return;
    const onVisible = () => {
      if (document.visibilityState !== "visible" || !started.current) return;
      void i18n.refreshIfStale().then(recordLocale);
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, [i18n, client, preview]);

  const toggle = () => {
    if (preview) return; // in preview the panel + launcher are both always shown
    const next = !open;
    setOpen(next);
    if (next && !started.current) start(conversationId);
  };

  // Host-driven open (e.g. the "Your driver is on the way" card): open the panel
  // and navigate to the requested conversation, starting the client if needed.
  useEffect(() => {
    if (!command || preview) return;
    setOpen(true);
    if (!started.current) {
      start(command.conversationId);
    } else if (command.conversationId) {
      // The socket connected earlier, before this conversation existed, so its
      // server-side subscriptions don't cover conv:<id> yet — reconnect to
      // re-derive membership and receive live replies (§5.2), mirroring the
      // reconnect after creating a support request.
      void Promise.resolve(client.openConversation(command.conversationId)).then(() => client.reconnect());
    }
  }, [command?.seq]);

  const iconSize = LAUNCHER_ICON[config.launcherSize] || 27;
  const guestThreadOnly = !!conversationId;
  const inThread = preview ? previewView === "chat" : !!state.activeId || draft;
  const activeConv = state.conversations.find((c) => c.id === state.activeId);
  const onBack = () => (draft ? setDraft(false) : client.backToList());
  const panelOpen = preview || open;

  return (
    <>
      {panelOpen && (
        <div class="panel" role="dialog" aria-label={t("widget.thread.support")}>
          {/* One panel-level control cluster, pinned top-right and shared across
              every screen (home / support thread / peer thread) so the sound +
              close icons never shift position between views. */}
          <div class="wpanel-controls">
            <SoundToggle on={state.soundOn} onToggle={() => client.toggleSound()} t={t} size={20} />
            {!preview && (
              <button class="wpanel-close" aria-label={t("widget.launcher.close")} onClick={() => setOpen(false)}>
                <CloseIcon />
              </button>
            )}
          </div>
          {inThread ? (
            <Thread
              client={client}
              config={config}
              t={t}
              state={state}
              activeConv={activeConv}
              preview={preview}
              guestThreadOnly={guestThreadOnly}
              draft={draft && !state.activeId}
              onBack={onBack}
              onCreated={() => setDraft(false)}
            />
          ) : (
            <Home
              client={client}
              config={config}
              t={t}
              name={name}
              state={state}
              preview={preview}
              onNew={() => setDraft(true)}
            />
          )}
          {config.poweredBy && <div class="powered">Powered by Sild</div>}
        </div>
      )}
      <button class={`launcher${open && !preview ? " open" : ""}`} aria-label={t("widget.home.title")} onClick={toggle}>
        {open && !preview ? <CloseIcon /> : <LauncherGlyph config={config} size={iconSize} />}
      </button>
    </>
  );
}

// TeamHeader renders the "agents online" row shown on Home when showTeam is on.
function TeamHeader() {
  return (
    <div class="team">
      <div class="stack">
        <span class="tav" style={{ background: "#7C9CF5" }}>E</span>
        <span class="tav" style={{ background: "#E58A6B" }}>M</span>
      </div>
      <span class="online">2 agents online</span>
    </div>
  );
}

function Home({
  client,
  config,
  t,
  name,
  state,
  preview,
  onNew,
}: {
  client: WidgetClient;
  config: BrandConfig;
  t: Translate;
  name?: string;
  state: WidgetState;
  preview: boolean;
  onNew: () => void;
}) {
  const topics = parseTopics(config.topics);
  const agentName = state.agentName || t("widget.home.support");
  const agentInitial = (agentName.trim()[0] || "S").toUpperCase();
  const startTopic = () => {
    if (preview) return;
    onNew();
  };
  return (
    <>
      <div class="brandhead">
        <div class="toprow">
          <span class="brandhead-left">
            {config.logoUrl || config.logo ? (
              <img class="logo" src={config.logoUrl || config.logo} alt={name || t("widget.logoAlt")} />
            ) : (
              name && <div class="brandname">{name}</div>
            )}
          </span>
          {/* sound + close live in the shared panel-level cluster now */}
        </div>
        {config.showTeam && <TeamHeader />}
        <h1>{config.heading}</h1>
        {config.sub && <p>{config.sub}</p>}
      </div>
      <div class="body">
        <div class="card">
          <h2>{t("widget.home.cta")}</h2>
          <p>{t("widget.home.reassurance")}</p>
          <button class="btn" onClick={onNew}>
            {t("widget.home.newConversation")} <ArrowIcon />
          </button>
        </div>
        {topics.length > 0 && (
          <div class="topics">
            {topics.map((topic) => (
              <button class="topic" key={topic} onClick={startTopic}>
                {topic}
                <ChevronIcon />
              </button>
            ))}
          </div>
        )}
        {state.conversations.length > 0 && <div class="eyebrow">{t("widget.home.recent")}</div>}
        {state.conversations.map((c) => {
          const rowName = c.title || c.agentName || agentName;
          const rowInitial = (rowName.trim()[0] || "S").toUpperCase();
          return (
          <div class="row" key={c.id} onClick={() => void client.openConversation(c.id)}>
            <span class="av" style={{ background: "var(--brand)", color: "#fff" }}>{rowInitial}</span>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div class="name">{rowName}</div>
              <div class="prev">{c.preview}</div>
            </div>
            <span class="time">{c.time}</span>
          </div>
          );
        })}
        {state.error && <div class="note">{state.error}</div>}
      </div>
    </>
  );
}

function Thread({
  client,
  config,
  t,
  state,
  activeConv,
  preview,
  guestThreadOnly,
  draft,
  onBack,
  onCreated,
}: {
  client: WidgetClient;
  config: BrandConfig;
  t: Translate;
  state: WidgetState;
  activeConv?: WidgetConversation;
  preview: boolean;
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

  // Keyed on the newest id, not the count, so loading older pages does not scroll the
  // reader back down.
  const newestId = state.messages.length ? state.messages[state.messages.length - 1].id : "";
  useEffect(() => {
    if (scroller.current) scroller.current.scrollTop = scroller.current.scrollHeight;
  }, [newestId, state.loadingThread]);

  const onScroll = async () => {
    const el = scroller.current;
    if (!el || el.scrollTop > 120 || !state.olderCursor || state.loadingOlder) return;
    // Prepending grows the list above the viewport, so hold the distance from the
    // bottom — anchoring on scrollTop would jump the reader to the new oldest message.
    const fromBottom = el.scrollHeight - el.scrollTop;
    await client.loadOlder();
    el.scrollTop = el.scrollHeight - fromBottom;
  };

  const closed = !draft && state.conversations.find((c) => c.id === state.activeId)?.closed;
  const canSend = (!!text.trim() || atts.length > 0) && !closed && uploading === 0;
  // Prefer the real agent's first name (learned from incoming messages) over the
  // generic "Support"; fall back to a friendly default before any reply arrives.
  const agentName = state.agentName || (config.showTeam ? "Eva" : t("widget.home.support"));
  // A peer conversation (rider↔driver) has no agent framing: the header shows the
  // other party + "Direct chat · <ref>" instead of the support agent.
  const peer = !!activeConv?.peer;
  const headName = peer ? activeConv?.title || t("widget.home.directChat") : agentName;
  const headInitial = (headName.trim()[0] || "S").toUpperCase();

  const onFiles = (e: Event) => {
    const input = e.currentTarget as HTMLInputElement;
    const files = Array.from(input.files || []);
    input.value = "";
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
    const body = text.trim();
    const sending = atts;
    setText("");
    setAtts([]);
    if (taRef.current) taRef.current.style.height = "auto";
    if (draft) {
      void Promise.resolve(client.openSupportRequest()).then(() => {
        onCreated();
        return client.send(body, sending);
      });
    } else {
      void client.send(body, sending);
    }
  };

  return (
    <>
      <div class="threadhead">
        {!guestThreadOnly && (
          <button class="iconbtn" aria-label={t("widget.thread.back")} onClick={onBack}>
            <BackIcon />
          </button>
        )}
        {config.showTeam && !peer ? (
          <span class="av" style={{ background: "#7C9CF5" }}>{headInitial}</span>
        ) : (
          <span class="av">{headInitial}</span>
        )}
        <div>
          <div class="name">{headName}</div>
          <div class="sub">
            {peer
              ? activeConv?.reference
                ? t("widget.thread.directRef", { ref: activeConv.reference })
                : t("widget.home.directChat")
              : draft
                ? t("widget.home.start")
                : state.connection === "connected" || preview
                  ? t("widget.home.subtitle")
                  : t("widget.status.connecting")}
          </div>
        </div>
        <div style={{ flex: 1 }} />
      </div>
      <div class="body" ref={scroller} onScroll={() => void onScroll()}>
        {state.loadingThread && <div class="note">{t("widget.status.loading")}</div>}
        {state.olderCursor && !state.loadingThread && (
          <div class="note" data-sild-older>
            {state.loadingOlder ? t("widget.thread.loadingOlder") : t("widget.thread.scrollUp")}
          </div>
        )}
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
          <div class="note">{t("widget.thread.empty")}</div>
        )}
      </div>
      <div class="composer">
        {closed && <div class="banner">{t("widget.thread.closed")}</div>}
        {(atts.length > 0 || uploading > 0) && (
          <div class="pending">
            {atts.map((a, i) => (
              <span class="pchip" key={i}>
                <span class="att-name">{a.filename}</span>
                <button aria-label={t("widget.composer.remove")} onClick={() => setAtts((p) => p.filter((_, j) => j !== i))}>
                  ✕
                </button>
              </span>
            ))}
            {uploading > 0 && <span class="pchip muted">{t("widget.composer.uploading")}</span>}
          </div>
        )}
        <div class="inputwrap">
          <button class="attachbtn" aria-label={t("widget.composer.attach")} disabled={closed} onClick={() => fileRef.current?.click()}>
            <ClipIcon />
          </button>
          <input ref={fileRef} type="file" multiple style={{ display: "none" }} onChange={onFiles} />
          <textarea
            ref={taRef}
            rows={1}
            placeholder={t("widget.composer.placeholder")}
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
          <button class="send" aria-label={t("widget.composer.send")} disabled={!canSend} onClick={submit}>
            <SendIcon />
          </button>
        </div>
      </div>
    </>
  );
}
