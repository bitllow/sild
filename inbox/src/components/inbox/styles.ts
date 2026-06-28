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
