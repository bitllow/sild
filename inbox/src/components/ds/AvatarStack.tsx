import * as React from "react";
import { Avatar, type AvatarSize } from "./Avatar";

export interface AvatarStackPerson {
  name: string;
  src?: string | null;
}

export interface AvatarStackProps {
  people: AvatarStackPerson[];
  size?: AvatarSize;
  /** Cap the number shown; the rest collapse into a "+N" chip. */
  max?: number;
}

const SIZES: Record<string, number> = { xs: 22, sm: 28, md: 36, lg: 44, xl: 56 };

// AvatarStack renders participants as overlapping avatars — the peer surface's
// row/header identity for a multi-party conversation.
export function AvatarStack({ people, size = "md", max = 3 }: AvatarStackProps) {
  const px = typeof size === "number" ? size : SIZES[size] || 36;
  const shown = people.slice(0, max);
  const extra = people.length - shown.length;
  const overlap = Math.round(px * 0.32);
  return (
    <span style={{ display: "inline-flex", alignItems: "center", flex: "none" }}>
      {shown.map((p, i) => (
        <span
          key={i}
          style={{
            marginLeft: i === 0 ? 0 : -overlap,
            borderRadius: "50%",
            boxShadow: "0 0 0 2px var(--surface-card)",
            display: "inline-flex",
          }}
        >
          <Avatar name={p.name} src={p.src} size={size} />
        </span>
      ))}
      {extra > 0 && (
        <span
          style={{
            marginLeft: -overlap,
            width: px,
            height: px,
            borderRadius: "50%",
            background: "var(--surface-sunken)",
            color: "var(--text-secondary)",
            fontFamily: "var(--font-sans)",
            fontSize: Math.round(px * 0.34),
            fontWeight: 700,
            display: "inline-flex",
            alignItems: "center",
            justifyContent: "center",
            boxShadow: "0 0 0 2px var(--surface-card)",
          }}
        >
          +{extra}
        </span>
      )}
    </span>
  );
}
