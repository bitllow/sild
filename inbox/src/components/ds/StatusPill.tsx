import * as React from "react";

export type Status = "open" | "queued" | "assigned" | "closed";

const LABELS: Record<Status, string> = {
  open: "Open",
  queued: "Queued",
  assigned: "Assigned",
  closed: "Closed",
};

export interface StatusPillProps extends React.HTMLAttributes<HTMLSpanElement> {
  status?: Status;
  label?: string;
}

export function StatusPill({ status = "open", label, className = "", ...rest }: StatusPillProps) {
  return (
    <span className={["sild-status", `sild-status--${status}`, className].filter(Boolean).join(" ")} {...rest}>
      <span className="sild-status__dot" aria-hidden="true" />
      {label || LABELS[status] || status}
    </span>
  );
}
