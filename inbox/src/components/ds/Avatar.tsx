import * as React from "react";

export type Presence = "online" | "away" | "offline";
export type AvatarSize = "xs" | "sm" | "md" | "lg" | "xl" | number;

const SIZES: Record<string, number> = { xs: 22, sm: 28, md: 36, lg: 44, xl: 56 };
const PALETTE = ["#3D63FF", "#FF7A45", "#18A957", "#7C5CFF", "#0EA5A5", "#E0599B", "#D9881A", "#2440B8"];

function initials(name = ""): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (!parts.length) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

function colorFor(name = ""): string {
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0;
  return PALETTE[h % PALETTE.length];
}

export interface AvatarProps extends Omit<React.HTMLAttributes<HTMLSpanElement>, "style"> {
  name?: string;
  src?: string | null;
  size?: AvatarSize;
  shape?: "circle" | "square";
  presence?: Presence | null;
  style?: React.CSSProperties;
}

export function Avatar({
  name = "",
  src = null,
  size = "md",
  shape = "circle",
  presence = null,
  className = "",
  style = {},
  ...rest
}: AvatarProps) {
  const px = typeof size === "number" ? size : SIZES[size] || 36;
  const cls = ["sild-avatar", shape === "square" ? "sild-avatar--square" : "", className]
    .filter(Boolean)
    .join(" ");
  const dot = Math.max(8, Math.round(px * 0.28));
  return (
    <span
      className={cls}
      style={{
        width: px,
        height: px,
        fontSize: Math.round(px * 0.38),
        background: src ? "var(--surface-sunken)" : colorFor(name),
        ...style,
      }}
      title={name || undefined}
      {...rest}
    >
      {src ? <img className="sild-avatar__img" src={src} alt={name} /> : initials(name)}
      {presence && (
        <span
          className={`sild-avatar__presence sild-avatar__presence--${presence}`}
          style={{ width: dot, height: dot }}
        />
      )}
    </span>
  );
}
