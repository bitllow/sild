import * as React from "react";
import type { MessageAttachment } from "./MessageBubble";

const Paperclip = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="m21.44 11.05-9.19 9.19a6 6 0 01-8.49-8.49l8.57-8.57A4 4 0 1118 8.84l-8.59 8.57a2 2 0 01-2.83-2.83l8.49-8.48" />
  </svg>
);

/** Inline images (rendered in the message body). Split out so both MessageBubble
 *  and the peer view render attachments identically from one source. */
export function InlineImages({ attachments }: { attachments: MessageAttachment[] }) {
  const imgs = attachments.filter((a) => a.disposition === "inline" && a.kind === "image" && a.url);
  return (
    <>
      {imgs.map((a, i) => (
        <a key={i} href={a.url} target="_blank" rel="noopener noreferrer">
          <img className="sild-msg__att-img" src={a.url} alt={a.filename || ""} />
        </a>
      ))}
    </>
  );
}

/** Non-inline attachments as downloadable file chips (rendered below the body). */
export function AttachmentChips({ attachments }: { attachments: MessageAttachment[] }) {
  const listed = attachments.filter((a) => a.disposition !== "inline" || a.kind !== "image");
  if (listed.length === 0) return null;
  return (
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
  );
}

export function hasInlineImages(attachments: MessageAttachment[]): boolean {
  return attachments.some((a) => a.disposition === "inline" && a.kind === "image" && !!a.url);
}
