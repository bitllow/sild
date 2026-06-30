import * as React from "react";

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
}

const Paperclip = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="m21.44 11.05-9.19 9.19a6 6 0 01-8.49-8.49l8.57-8.57A4 4 0 1118 8.84l-8.59 8.57a2 2 0 01-2.83-2.83l8.49-8.48" />
  </svg>
);

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
      <div className={cls} {...rest}>
        <div className="sild-msg__bubble">{body}</div>
      </div>
    );
  }

  const inlineImgs = attachments.filter((a) => a.disposition === "inline" && a.kind === "image");
  const listed = attachments.filter((a) => a.disposition !== "inline" || a.kind !== "image");

  return (
    <div className={cls} {...rest}>
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
      {(body || inlineImgs.length > 0) && (
        <div className="sild-msg__bubble">
          {body}
          {inlineImgs.map((a, i) =>
            a.url ? (
              <a key={i} href={a.url} target="_blank" rel="noopener noreferrer">
                <img className="sild-msg__att-img" src={a.url} alt={a.filename || ""} />
              </a>
            ) : null
          )}
        </div>
      )}
      {listed.length > 0 && (
        <div className="sild-msg__atts">
          {listed.map((a, i) =>
            a.url ? (
              <a key={i} className="sild-msg__att" href={a.url} target="_blank" rel="noopener noreferrer" download={a.filename || undefined}>
                <Paperclip /> <span className="sild-msg__att-name">{a.filename || "attachment"}</span>
              </a>
            ) : (
              <span key={i} className="sild-msg__att">
                <Paperclip /> <span className="sild-msg__att-name">{a.filename || "attachment"}</span>
              </span>
            )
          )}
        </div>
      )}
      {readReceipt && <div className="sild-msg__read">{readReceipt}</div>}
    </div>
  );
}
