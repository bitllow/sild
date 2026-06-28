import * as React from "react";

export type BadgeVariant =
  | "neutral"
  | "brand"
  | "success"
  | "warning"
  | "danger"
  | "accent"
  | "solid";

export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  variant?: BadgeVariant;
  dot?: boolean;
  count?: boolean;
}

export function Badge({
  variant = "neutral",
  dot = false,
  count = false,
  className = "",
  children,
  ...rest
}: BadgeProps) {
  const cls = ["sild-badge", `sild-badge--${variant}`, count ? "sild-badge--count" : "", className]
    .filter(Boolean)
    .join(" ");
  return (
    <span className={cls} {...rest}>
      {dot && <span className="sild-badge__dot" aria-hidden="true" />}
      {children}
    </span>
  );
}
