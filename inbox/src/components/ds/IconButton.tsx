import * as React from "react";

export interface IconButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  size?: "sm" | "md" | "lg";
  variant?: "ghost" | "solid" | "bordered";
  "aria-label": string;
}

export function IconButton({
  size = "md",
  variant = "ghost",
  disabled = false,
  className = "",
  children,
  ...rest
}: IconButtonProps) {
  const cls = [
    "sild-iconbtn",
    `sild-iconbtn--${size}`,
    variant !== "ghost" ? `sild-iconbtn--${variant}` : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <button type="button" className={cls} disabled={disabled} {...rest}>
      {children}
    </button>
  );
}
