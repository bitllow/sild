import * as React from "react";
import { AttachmentChips, InlineImages, hasInlineImages } from "./MessageAttachments";

export interface MessageAttachment {
  disposition?: "inline" | "attachment";
  kind?: string;
  url?: string;
  filename?: string;
}

export interface MessageBubbleProps extends React.HTMLAttributes<HTMLDivElement> {
  direction?: "in" | "out";
  author?: string;
  time?: string;
  body?: React.ReactNode;
  channel?: "app" | "email";
  internal?: boolean;
  system?: boolean;
  attachments?: MessageAttachment[];
  readReceipt?: string;
  [key: `data-${string}`]: string | undefined;
}

const Mail = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <rect x="2" y="4" width="20" height="16" rx="2" />
    <path d="m22 7-10 5L2 7" />
  </svg>
);

const LockGlyph = () => (
  <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
    <rect x="3" y="11" width="18" height="11" rx="2" />
    <path d="M7 11V7a5 5 0 0110 0v4" />
  </svg>
);

export function MessageBubble({
  direction = "in",
  author,
  time,
  body,
  channel,
  internal = false,
  system = false,
  attachments = [],
  readReceipt,
  className = "",
  ...rest
}: MessageBubbleProps) {
  const kind = system ? "system" : direction;
  const cls = ["sild-msg", `sild-msg--${kind}`, internal ? "sild-msg--internal" : "", className]
    .filter(Boolean)
    .join(" ");

  if (system) {
    return (
      <div className={cls} data-testid="message" data-kind="system" {...rest}>
        <div className="sild-msg__bubble">{body}</div>
      </div>
    );
  }

  return (
    <div
      className={cls}
      data-testid="message"
      data-kind={kind}
      data-internal={internal ? "true" : "false"}
      {...rest}
    >
      {(author || time || internal || channel === "email") && (
        <div className="sild-msg__meta">
          {author && <span className="sild-msg__author">{author}</span>}
          {internal && (
            <span className="sild-msg__intlabel">
              <LockGlyph /> Internal note
            </span>
          )}
          {channel === "email" && (
            <span className="sild-msg__chan">
              <Mail /> Email
            </span>
          )}
          {time && <span className="sild-msg__time">{time}</span>}
        </div>
      )}
      {(body || hasInlineImages(attachments)) && (
        <div className="sild-msg__bubble">
          {body}
          <InlineImages attachments={attachments} />
        </div>
      )}
      <AttachmentChips attachments={attachments} />
      {readReceipt && <div className="sild-msg__read">{readReceipt}</div>}
    </div>
  );
}
