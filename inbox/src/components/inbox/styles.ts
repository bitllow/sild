import type { CSSProperties } from "react";

/** Style helpers ported from the source surface's render logic (segStyle,
 *  navStyle, filterStyle, tabStyle, panelStyle) — kept byte-faithful. */

export function navStyle(active: boolean): CSSProperties {
  return {
    width: 44,
    height: 44,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    borderRadius: 8,
    border: 0,
    cursor: "pointer",
    background: active ? "var(--brand-subtle)" : "transparent",
    color: active ? "var(--brand)" : "var(--slate-400)",
  };
}

export function filterStyle(active: boolean): CSSProperties {
  return {
    flex: 1,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    gap: 6,
    border: 0,
    cursor: "pointer",
    fontFamily: "var(--font-sans)",
    fontSize: 13,
    fontWeight: 600,
    padding: "6px 0",
    borderRadius: 6,
    transition: "all .12s",
    background: active ? "#fff" : "transparent",
    color: active ? "var(--text-primary)" : "var(--text-secondary)",
    boxShadow: active ? "var(--shadow-xs)" : "none",
  };
}

// Neutral count pill shown inline in the You/Unassigned scope tabs.
export function tabCountStyle(): CSSProperties {
  return {
    minWidth: 18,
    height: 18,
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    padding: "0 5px",
    borderRadius: 9,
    background: "rgba(0,0,0,.06)",
    fontSize: 11,
    fontWeight: 700,
  };
}

// "Show closed · N" / "Hide closed" pill next to the open count.
export function closedToggleStyle(active: boolean): CSSProperties {
  return {
    display: "inline-flex",
    alignItems: "center",
    gap: 5,
    height: 22,
    padding: "0 10px",
    borderRadius: 999,
    border: `1px solid ${active ? "var(--border-focus)" : "var(--border-default)"}`,
    background: active ? "var(--brand-subtle)" : "var(--white)",
    color: active ? "var(--brand)" : "var(--text-secondary)",
    fontFamily: "var(--font-sans)",
    fontSize: 11,
    fontWeight: 600,
    cursor: "pointer",
    whiteSpace: "nowrap",
  };
}

// 32px square sound toggle in the inbox header; muted turns coral.
export function soundBtnStyle(soundOn: boolean): CSSProperties {
  return {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    width: 32,
    height: 32,
    flex: "none",
    borderRadius: 8,
    border: `1px solid ${soundOn ? "var(--border-default)" : "var(--coral-500)"}`,
    background: "var(--white)",
    color: soundOn ? "var(--text-secondary)" : "var(--coral-500)",
    cursor: "pointer",
  };
}

/** The bordered white panel every settings-shaped surface is built from. */
export const cardStyle: CSSProperties = {
  background: "var(--white)",
  border: "1px solid var(--border-default)",
  borderRadius: 12,
  boxShadow: "var(--shadow-sm)",
  overflow: "hidden",
};

export const rowBorder = "1px solid var(--border-subtle)";

/** The uppercase label above a value field. */
export const fieldLabel: CSSProperties = {
  fontSize: 12,
  fontWeight: 600,
  color: "var(--text-tertiary)",
  textTransform: "uppercase",
  letterSpacing: ".04em",
};

export function tabStyle(active: boolean): CSSProperties {
  return {
    border: 0,
    background: "transparent",
    cursor: "pointer",
    fontFamily: "var(--font-sans)",
    fontSize: 14,
    fontWeight: 600,
    padding: "0 0 12px",
    borderBottom: "2px solid",
    borderBottomColor: active ? "var(--brand)" : "transparent",
    color: active ? "var(--brand)" : "var(--text-secondary)",
  };
}

export function panelStyle(active: boolean): CSSProperties {
  return {
    width: 36,
    height: 36,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    borderRadius: 8,
    cursor: "pointer",
    color: active ? "var(--brand)" : "var(--text-secondary)",
    border: active ? "1px solid var(--border-default)" : "1px solid transparent",
    background: active ? "var(--white)" : "transparent",
  };
}
