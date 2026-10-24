"use client";

import { useEffect, useRef, type CSSProperties, type ReactNode, type RefObject } from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { AlertIcon, Badge, Button, CheckIcon, ComposerBar, MessageBubble, Spinner, StatusPill, Textarea } from "@/components/ds";
import type { Message, Step } from "@sild/client";
import { clockTime } from "@/store/map";
import type { TriageConversation, TriageStore } from "@/store/triage";
import {
  TRIAGE_PILL,
  describeRequest,
  diagnosisOf,
  isFromRequester,
  isTriageActive,
  requesterOf,
  stepsOf,
  type DiagnosisPart,
  type TriageStatus,
} from "@/store/triageStatus";
import { capsLabel } from "./styles";

const ASSISTANT = "Triage assistant";
const INTRO = "Describe what's going wrong. I can look things up in your account to find the cause.";

const card: CSSProperties = {
  background: "var(--white)",
  border: "1px solid var(--border-default)",
  borderRadius: 12,
  boxShadow: "var(--shadow-sm)",
  padding: "14px 16px",
  display: "flex",
  flexDirection: "column",
  gap: 12,
};

function Event({ children }: { children: ReactNode }) {
  return (
    <div
      data-testid="triage-event"
      style={{ alignSelf: "center", maxWidth: "90%", padding: "6px 12px", borderRadius: 999, background: "var(--surface-sunken)", color: "var(--text-secondary)", fontSize: 12, fontWeight: 500, textAlign: "center" }}
    >
      {children}
    </div>
  );
}

const StepsCard = observer(function StepsCard({ id, steps, t }: { id: string; steps: Step[]; t: TriageStore }) {
  const running = steps.some((s) => s.state === "running");
  const canToggle = !running && steps.some((s) => s.internal);
  const open = canToggle && t.openRequests.has(id);
  const n = steps.length;
  return (
    <div data-testid="triage-steps" style={{ background: "var(--white)", border: "1px solid var(--border-default)", borderRadius: 12, overflow: "hidden" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "9px 12px", background: "var(--surface-sunken)" }}>
        <span style={{ flex: 1, fontSize: 12, fontWeight: 600, color: "var(--text-secondary)" }}>
          {running ? "Checking your account…" : `Checked ${n} ${n === 1 ? "thing" : "things"} in your account`}
        </span>
        {canToggle && (
          <button
            data-testid="triage-requests-toggle"
            onClick={() => t.toggleRequests(id)}
            style={{ border: 0, background: "transparent", cursor: "pointer", padding: 0, fontFamily: "var(--font-sans)", fontSize: 12, fontWeight: 600, color: "var(--brand)" }}
          >
            {open ? "Hide data requests" : "Show data requests"}
          </button>
        )}
      </div>
      {steps.map((s, i) => {
        const result = s.state === "running" ? "" : s.result || (s.state === "not_available" ? "Not available" : "");
        return (
          <div key={i} style={{ display: "flex", gap: 10, padding: "9px 12px", borderTop: "1px solid var(--border-subtle)" }}>
            <span style={{ width: 16, height: 18, flex: "none", display: "flex", alignItems: "center", justifyContent: "center" }}>
              {s.state === "running" ? <Spinner /> : s.state === "done" ? <CheckIcon size={15} stroke="var(--green-600)" /> : <AlertIcon size={15} stroke="var(--amber-600)" />}
            </span>
            <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 2 }}>
              <span style={{ fontSize: 13 }}>{s.label}</span>
              {result && <span style={{ fontSize: 12, color: "var(--text-secondary)" }}>{result}</span>}
              {open && s.internal && (
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-tertiary)", wordBreak: "break-all" }}>
                  {describeRequest(s.internal)}
                </span>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
});

function DiagnosisCard({ d }: { d: DiagnosisPart }) {
  return (
    <div data-testid="triage-diagnosis" style={card}>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <Badge variant="brand">Suggestion</Badge>
        <span style={{ fontSize: 12, color: "var(--text-tertiary)" }}>From the assistant — check before acting.</span>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
        <span style={capsLabel}>Cause</span>
        <p style={{ margin: 0, fontSize: 14, lineHeight: 1.55, textWrap: "pretty" }}>{d.cause}</p>
      </div>
      {d.fix.length > 0 && (
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          <span style={capsLabel}>How to fix it</span>
          {d.fix.map((text, i) => (
            <div key={i} style={{ display: "flex", gap: 10 }}>
              <span style={{ width: 20, height: 20, flex: "none", borderRadius: "50%", background: "var(--blue-50)", color: "var(--blue-700)", fontSize: 11, fontWeight: 700, display: "flex", alignItems: "center", justifyContent: "center" }}>
                {i + 1}
              </span>
              <span style={{ fontSize: 14, lineHeight: 1.5, textWrap: "pretty" }}>{text}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function TextCard({ children }: { children: ReactNode }) {
  return (
    <div style={{ padding: "10px 14px", background: "var(--white)", border: "1px solid var(--border-default)", borderRadius: "4px 18px 18px 18px", fontSize: 14, lineHeight: 1.55, textWrap: "pretty", whiteSpace: "pre-wrap" }}>
      {children}
    </div>
  );
}

// One assistant turn: the name row only opens a run of consecutive assistant items.
function AssistantBlock({ showName, time, children }: { showName: boolean; time?: string; children: ReactNode }) {
  return (
    <div data-testid="triage-assistant" style={{ display: "flex", gap: 10, alignItems: "flex-start", maxWidth: 640 }}>
      <span style={{ width: 28, flex: "none" }}>
        {/* eslint-disable-next-line @next/next/no-img-element */}
        {showName && <img src="/assets/sild-mark-tile.png" width={28} height={28} alt="" style={{ borderRadius: 8, display: "block" }} />}
      </span>
      <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 6 }}>
        {showName && (
          <div style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12 }}>
            <span style={{ fontWeight: 700 }}>{ASSISTANT}</span>
            {time && <span style={{ color: "var(--text-tertiary)" }}>{time}</span>}
          </div>
        )}
        {children}
      </div>
    </div>
  );
}

const DescribeForm = observer(function DescribeForm({ t }: { t: TriageStore }) {
  return (
    <div style={card}>
      <Textarea
        label="Describe the issue"
        hint="What you expected, what happened instead, and any IDs, names or error messages."
        rows={5}
        value={t.composer}
        onChange={(e) => t.setComposer(e.target.value)}
        placeholder="e.g. A driver hasn't received new trips since 8:00"
      />
      <div style={{ display: "flex", justifyContent: "flex-end" }}>
        <Button size="sm" onClick={() => void t.send()} disabled={!t.composer.trim()} loading={t.sending} data-testid="triage-send">
          Send to assistant
        </Button>
      </div>
    </div>
  );
});

function Header({ title, sub, status, onClose }: { title: string; sub: string; status: TriageStatus; onClose?: () => void }) {
  const pill = TRIAGE_PILL[status];
  return (
    <div style={{ height: 64, flex: "none", display: "flex", alignItems: "center", gap: 12, padding: "0 18px", background: "var(--surface-card)", borderBottom: "1px solid var(--border-default)" }}>
      <div style={{ minWidth: 0, flex: 1 }}>
        <div style={{ fontSize: 15, fontWeight: 700, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{title}</div>
        <div style={{ fontSize: 12, color: "var(--text-tertiary)", fontFamily: "var(--font-mono)" }}>{sub}</div>
      </div>
      <StatusPill status={pill.pill} label={pill.label} data-triage-status={status} />
      {onClose && (
        <Button size="sm" variant="secondary" onClick={onClose} data-testid="triage-close">
          Close conversation
        </Button>
      )}
    </div>
  );
}

function threadItems(t: TriageStore, c: TriageConversation, own: boolean): ReactNode[] {
  const requester = requesterOf(c.members);
  const owner = t.ownerName(c);
  const out: ReactNode[] = [];
  let prevAssistant = false;
  for (const m of c.messages) {
    const isBot = m.sender_kind === "bot" && m.visibility !== "internal";
    if (m.sender_kind === "system") out.push(<Event key={m.id}>{m.body}</Event>);
    else if (isBot) out.push(<AssistantMessage key={m.id} m={m} t={t} showName={!prevAssistant} />);
    else if (isFromRequester(m, requester))
      out.push(
        <MessageBubble
          key={m.id}
          direction={own ? "out" : "in"}
          author={own ? undefined : owner}
          time={clockTime(m.created_at)}
          body={m.body}
        />
      );
    else
      out.push(
        <MessageBubble
          key={m.id}
          direction="in"
          author={m.author_name || "Support"}
          time={clockTime(m.created_at)}
          body={m.body}
          internal={m.visibility === "internal"}
        />
      );
    prevAssistant = isBot;
  }
  return out;
}

const AssistantMessage = observer(function AssistantMessage({ m, t, showName }: { m: Message; t: TriageStore; showName: boolean }) {
  const diagnosis = diagnosisOf(m);
  return (
    <AssistantBlock showName={showName} time={clockTime(m.created_at)}>
      {stepsOf(m).map((p, i) => (
        <StepsCard key={i} id={`${m.id}:${i}`} steps={p.steps} t={t} />
      ))}
      {m.body.trim() && <TextCard>{m.body}</TextCard>}
      {diagnosis && <DiagnosisCard d={diagnosis} />}
    </AssistantBlock>
  );
});

export const TriageThread = observer(function TriageThread() {
  const t = useStore().triage;
  const c = t.active;
  const scroller = useRef<HTMLDivElement>(null);
  const last = c?.messages[c.messages.length - 1];

  useEffect(() => {
    if (scroller.current) scroller.current.scrollTop = scroller.current.scrollHeight;
  }, [c?.id, last?.id, last?.updated_at, t.draftActive]);

  if (t.draftActive) {
    return (
      <Frame header={<Header title="New request" sub="" status="new" onClose={() => void t.close("closed")} />} error={t.error}>
        <AssistantBlock showName>
          <TextCard>{INTRO}</TextCard>
          <DescribeForm t={t} />
        </AssistantBlock>
      </Frame>
    );
  }

  if (!c) {
    return (
      <div style={{ flex: 1, minWidth: 0, display: "flex", alignItems: "center", justifyContent: "center", padding: 24, color: "var(--text-tertiary)", fontSize: 14, background: "var(--surface-page)" }}>
        {!t.loaded ? "Loading…" : t.canStart ? "Start a new request to talk to the triage assistant." : "Select a chat."}
      </div>
    );
  }

  const own = t.isOwn(c);
  const status = t.statusOf(c);
  const open = isTriageActive(status);
  const owner = t.ownerName(c);

  let footer: ReactNode = null;
  if (own) {
    if (status === "fixed") footer = <FootText>Marked as fixed.</FootText>;
    else if (status === "closed") footer = <FootText>Conversation closed. Start a new request if you need more help.</FootText>;
    else if (status !== "new")
      footer = (
        <>
          {status === "fix_proposed" && (
            <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
              <span style={{ fontSize: 13, color: "var(--text-secondary)", flex: 1, minWidth: 160 }}>Did this fix it?</span>
              <Button size="sm" onClick={() => void t.close("resolved")} loading={t.sending} data-testid="triage-resolve">
                Yes, it&apos;s fixed
              </Button>
            </div>
          )}
          <ComposerBar
            value={t.composer}
            onChange={(v) => t.setComposer(v)}
            onSend={() => void t.send()}
            placeholder="Ask a follow-up question…"
            disabled={t.sending}
          />
        </>
      );
  } else {
    footer = (
      <FootText>
        {open
          ? `${owner}'s chat with the assistant. Only they can reply here.`
          : status === "fixed"
            ? `Marked as fixed by ${owner}`
            : `Closed by ${owner}`}
      </FootText>
    );
  }

  return (
    <Frame
      header={
        <Header
          title={t.title(c)}
          sub={own ? c.reference : `${owner} · ${c.reference}`}
          status={status}
          onClose={own && open ? () => void t.close("closed") : undefined}
        />
      }
      footer={footer}
      error={t.error}
      scroller={scroller}
    >
      {threadItems(t, c, own)}
      {own && status === "new" && (
        <AssistantBlock showName>
          <TextCard>{INTRO}</TextCard>
          <DescribeForm t={t} />
        </AssistantBlock>
      )}
      {status === "fixed" && <Event>Marked as fixed by {owner}</Event>}
      {status === "closed" && <Event>Conversation closed</Event>}
      {(status === "waiting" || status === "diagnosing") && (
        <div data-testid="triage-busy" style={{ display: "flex", alignItems: "center", gap: 10, paddingLeft: 38, fontSize: 13, color: "var(--text-tertiary)" }}>
          <Spinner />
          {status === "waiting" ? "Waiting for the assistant" : "Looking into it"}
        </div>
      )}
    </Frame>
  );
});

function FootText({ children }: { children: ReactNode }) {
  return <span style={{ fontSize: 13, color: "var(--text-secondary)" }}>{children}</span>;
}

function Frame({
  header,
  footer,
  error,
  scroller,
  children,
}: {
  header: ReactNode;
  footer?: ReactNode;
  error: string | null;
  scroller?: RefObject<HTMLDivElement | null>;
  children: ReactNode;
}) {
  return (
    <div data-testid="triage-thread" style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", background: "var(--surface-page)" }}>
      {header}
      <div ref={scroller} style={{ flex: 1, overflowY: "auto", padding: "20px 20px 24px" }}>
        <div style={{ maxWidth: 740, margin: "0 auto", display: "flex", flexDirection: "column", gap: 14 }}>{children}</div>
      </div>
      {(footer || error) && (
        <div style={{ flex: "none", padding: "12px 20px 16px", background: "var(--surface-card)", borderTop: "1px solid var(--border-default)" }}>
          <div style={{ maxWidth: 740, margin: "0 auto", display: "flex", flexDirection: "column", gap: 10 }}>
            {error && (
              <div role="alert" style={{ fontSize: 12, color: "var(--danger-600, #c0362c)" }}>
                {error}
              </div>
            )}
            {footer}
          </div>
        </div>
      )}
    </div>
  );
}
