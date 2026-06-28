import * as React from "react";
import { Avatar, Presence } from "./Avatar";

export interface ConversationRowProps extends React.HTMLAttributes<HTMLDivElement> {
  name: string;
  preview?: string;
  time?: string;
  unread?: number;
  active?: boolean;
  channel?: "app" | "email";
  reference?: string;
  presence?: Presence | null;
  src?: string;
  status?: React.ReactNode;
}

const Mail = () => (
  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <rect x="2" y="4" width="20" height="16" rx="2" />
    <path d="m22 7-10 5L2 7" />
  </svg>
);

export function ConversationRow({
  name,
  preview,
  time,
  unread = 0,
  active = false,
  channel = "app",
  reference,
  presence = null,
  src,
  status,
  onClick,
  className = "",
  ...rest
}: ConversationRowProps) {
  const isUnread = unread > 0;
  return (
    <div
      className={["sild-convrow", active ? "sild-convrow--active" : "", className].filter(Boolean).join(" ")}
      onClick={onClick}
      role="button"
      tabIndex={0}
      {...rest}
    >
      <Avatar name={name} src={src} presence={presence} size="md" />
      <div className="sild-convrow__main">
        <div className="sild-convrow__top">
          <span className={["sild-convrow__name", isUnread ? "sild-convrow__name--unread" : ""].filter(Boolean).join(" ")}>
            {name}
          </span>
          {time && <span className="sild-convrow__time">{time}</span>}
        </div>
        <div className={["sild-convrow__preview", isUnread ? "sild-convrow__preview--unread" : ""].filter(Boolean).join(" ")}>
          {preview}
        </div>
        {(reference || channel === "email" || status) && (
          <div className="sild-convrow__sub">
            {channel === "email" && (
              <span className="sild-convrow__chan">
                <Mail />
              </span>
            )}
            {reference && <span className="sild-convrow__ref">{reference}</span>}
            {status}
          </div>
        )}
      </div>
      <div className="sild-convrow__right">
        {isUnread && <span className="sild-convrow__count">{unread > 99 ? "99+" : unread}</span>}
      </div>
    </div>
  );
}
