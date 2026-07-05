// Turns a BrandConfig into the widget's stylesheet. This is the single source of
// truth for how brand color, theme, font, corners, and launcher geometry map to
// CSS — the same computation drives the live drop-in AND the Appearance live
// preview (preview === production), differing only by `mode` geometry.
import type { BrandConfig } from "../core/types";

export type WidgetMode = "live" | "preview";

interface Palette {
  page: string;
  card: string;
  sunken: string; // incoming bubble / sunken surface
  text: string;
  sub: string;
  tertiary: string;
  border: string;
}

// Theme palettes ported from the source design (Sild Surfaces → apVals).
const LIGHT: Palette = {
  page: "#F4F6FA",
  card: "#FFFFFF",
  sunken: "#EAEEF4",
  text: "#14181F",
  sub: "#5B6472",
  tertiary: "#8A94A4",
  border: "rgba(20,24,31,.09)",
};
const DARK: Palette = {
  page: "#0E1116",
  card: "#181C23",
  sunken: "#262C35",
  text: "#F2F5F9",
  sub: "#9AA4B2",
  tertiary: "#78828F",
  border: "rgba(255,255,255,.10)",
};

const RADII: Record<BrandConfig["radius"], { panel: number; bubble: number; btn: number; card: number; launcher: string }> = {
  sharp: { panel: 7, bubble: 8, btn: 7, card: 8, launcher: "14px" },
  default: { panel: 16, bubble: 18, btn: 10, card: 12, launcher: "18px" },
  rounded: { panel: 22, bubble: 20, btn: 13, card: 16, launcher: "26px" },
  pillowy: { panel: 26, bubble: 22, btn: 16, card: 18, launcher: "50%" },
};

const FONTS: Record<BrandConfig["font"], string> = {
  system: "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
  sild: "'Schibsted Grotesk', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
  inter: "'Inter', -apple-system, sans-serif",
  figtree: "'Figtree', -apple-system, sans-serif",
  dmsans: "'DM Sans', -apple-system, sans-serif",
};

const LAUNCHER_SIZE: Record<BrandConfig["launcherSize"], number> = { sm: 52, md: 60, lg: 68 };
export const LAUNCHER_ICON: Record<BrandConfig["launcherSize"], number> = { sm: 24, md: 27, lg: 30 };

// The Google-hosted families a config may need (system + sild's Schibsted are
// injected elsewhere / bundled). Used by index.tsx to load only what's chosen.
export const GOOGLE_FONT_HREF: Partial<Record<BrandConfig["font"], string>> = {
  sild: "https://fonts.googleapis.com/css2?family=Schibsted+Grotesk:wght@400;500;600;700;800&display=swap",
  inter: "https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&display=swap",
  figtree: "https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700;800&display=swap",
  dmsans: "https://fonts.googleapis.com/css2?family=DM+Sans:wght@400;500;600;700;800&display=swap",
};

// shade multiplies each channel toward black (pct<1) — a cheap hover/darken that
// tracks any brand hue without a color library.
function shade(hex: string, pct: number): string {
  let h = hex.replace("#", "");
  if (h.length === 3) h = h.split("").map((c) => c + c).join("");
  const n = parseInt(h, 16);
  const r = Math.round(((n >> 16) & 255) * pct);
  const g = Math.round(((n >> 8) & 255) * pct);
  const b = Math.round((n & 255) * pct);
  return `#${((1 << 24) | (r << 16) | (g << 8) | b).toString(16).slice(1)}`;
}

function paletteVars(p: Palette): string {
  return `
  --surface-page: ${p.page};
  --surface-card: ${p.card};
  --surface-sunken: ${p.sunken};
  --text-primary: ${p.text};
  --text-secondary: ${p.sub};
  --text-tertiary: ${p.tertiary};
  --border-default: ${p.border};`;
}

// parseTopics splits the newline-separated topics field into up to 4 labels.
export function parseTopics(topics: string): string[] {
  return (topics || "")
    .split("\n")
    .map((t) => t.trim())
    .filter(Boolean)
    .slice(0, 4);
}

/** buildStyles renders the full shadow-root stylesheet for a config + mode. */
export function buildStyles(cfg: BrandConfig, mode: WidgetMode): string {
  const brand = cfg.brand;
  const brandHover = shade(brand, 0.85);
  const rad = RADII[cfg.radius] || RADII.default;
  const font = FONTS[cfg.font] || FONTS.sild;
  const lSize = LAUNCHER_SIZE[cfg.launcherSize] || 60;

  // Auto follows the visitor's OS in the live widget; the preview shows light.
  const base = cfg.theme === "dark" ? DARK : LIGHT;
  const autoDark = mode === "live" && cfg.theme === "auto";

  const preview = mode === "preview";
  const edge = preview ? 18 : 24;
  const side = cfg.launcherPos === "left" ? "left" : "right";
  const panelBottom = edge + lSize + (preview ? 6 : 16);
  const panelW = preview ? 300 : 380;
  const panelH = preview ? 430 : 600;
  const origin = side === "left" ? "bottom left" : "bottom right";

  // Type scale: the compact preview mirrors the design's mini-widget; the live
  // widget is a touch larger.
  const headPad = preview ? "18px 18px 20px" : "22px 22px 26px";
  const fsH1 = preview ? 21 : 22;
  const fsSub = preview ? 13 : 15;
  const fsBody = preview ? 13 : 14;

  // In preview the host fills the faux-site canvas (absolute); live floats fixed.
  const hostPos = preview
    ? `position: absolute; inset: 0; overflow: hidden;`
    : `position: fixed; ${side}: ${edge}px; bottom: ${edge}px; z-index: 2147483000;`;
  const posMode = preview ? "absolute" : "fixed";

  const launcherShadow = preview
    ? "0 10px 26px rgba(20,24,31,.3)"
    : `0 8px 24px ${hexToRgba(brand, 0.36)}, 0 2px 6px rgba(20,24,31,.16)`;

  return `
:host {
  --brand: ${brand};
  --brand-hover: ${brandHover};
  --r-panel: ${rad.panel}px;
  --r-bubble: ${rad.bubble}px;
  --r-btn: ${rad.btn}px;
  --r-card: ${rad.card}px;
  --r-launcher: ${rad.launcher};
  --r-tail: 6px;
  --font-sans: ${font};
  --shadow-sm: 0 1px 3px rgba(20,24,31,.08), 0 1px 2px rgba(20,24,31,.05);
  --shadow-widget: 0 16px 48px rgba(20,24,31,.22), 0 4px 12px rgba(20,24,31,.10);
  --shadow-launcher: ${launcherShadow};
  ${paletteVars(base)}
  ${hostPos}
  font-family: var(--font-sans);
}
${autoDark ? `@media (prefers-color-scheme: dark) { :host { ${paletteVars(DARK)} } }` : ""}
*, *::before, *::after { box-sizing: border-box; }

.launcher {
  position: ${posMode}; ${side}: ${edge}px; bottom: ${edge}px;
  width: ${lSize}px; height: ${lSize}px; border-radius: var(--r-launcher); border: 0;
  background: var(--brand); color: #fff; cursor: pointer;
  box-shadow: var(--shadow-launcher);
  display: flex; align-items: center; justify-content: center;
  transition: transform .2s cubic-bezier(.34,1.56,.64,1);
  padding: 0;
}
.launcher:hover { transform: scale(1.05); }
.launcher.open { transform: scale(.92); }
.launcher img { object-fit: contain; }

.panel {
  position: ${posMode}; ${side}: ${edge}px; bottom: ${panelBottom}px;
  width: ${panelW}px; height: ${panelH}px;
  ${preview ? "" : "max-height: calc(100vh - 124px); max-width: calc(100vw - 48px);"}
  background: var(--surface-card); border-radius: var(--r-panel);
  box-shadow: var(--shadow-widget); overflow: hidden;
  display: flex; flex-direction: column;
  border: 1px solid var(--border-default);
  transform-origin: ${origin};
  animation: sild-in .22s cubic-bezier(.16,1,.3,1);
}
@keyframes sild-in { from { opacity: 0; transform: translateY(10px) scale(.97); } to { opacity: 1; transform: none; } }

.brandhead { background: var(--brand); color: #fff; padding: ${headPad}; flex: none; }
.brandhead .toprow { display: flex; align-items: center; justify-content: space-between; min-height: 22px; }
.brandhead .brandhead-left { display: flex; align-items: center; min-width: 0; }
.brandhead .logo { height: 28px; max-width: 150px; object-fit: contain; display: block; }
.brandhead .brandname { font-size: 15px; font-weight: 800; letter-spacing: -.01em; }
.brandhead h1 { margin: 14px 0 0; font-size: ${fsH1}px; font-weight: 800; letter-spacing: -.02em; line-height: 1.2; }
.brandhead p { margin: 4px 0 0; font-size: ${fsSub}px; color: rgba(255,255,255,.85); line-height: 1.45; }
.team { display: flex; align-items: center; gap: 8px; margin-top: 14px; }
.team .stack { display: flex; }
.team .tav { width: 26px; height: 26px; border-radius: 50%; border: 2px solid var(--brand); display: flex; align-items: center; justify-content: center; font-size: 11px; font-weight: 700; color: #fff; }
.team .tav + .tav { margin-left: -8px; }
.team .online { font-size: 12px; color: rgba(255,255,255,.85); }

.threadhead { background: var(--brand); color: #fff; padding: 12px 14px; display: flex; align-items: center; gap: 10px; flex: none; }
.threadhead .name { font-size: 15px; font-weight: 700; letter-spacing: -.01em; }
.threadhead .sub { font-size: 12px; color: rgba(255,255,255,.8); }
.iconbtn { border: 0; background: transparent; color: #fff; cursor: pointer; display: flex; padding: 4px; border-radius: 8px; }
.iconbtn:hover { background: rgba(255,255,255,.15); }
.wsound { border: 0; background: transparent; color: #fff; cursor: pointer; display: flex; padding: 4px; border-radius: 8px; flex: none; }
.wsound:hover { background: rgba(255,255,255,.15); }
.av { width: 34px; height: 34px; border-radius: 50%; background: rgba(255,255,255,.18); display: flex; align-items: center; justify-content: center; font-weight: 700; font-size: 13px; flex: none; }

.body { flex: 1; min-height: 0; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; gap: 12px; background: var(--surface-page); }

.card { background: var(--surface-card); border: 1px solid var(--border-default); border-radius: var(--r-card); box-shadow: var(--shadow-sm); padding: 16px; }
.card h2 { margin: 0; font-size: 14px; font-weight: 700; color: var(--text-primary); }
.card p { margin: 3px 0 13px; font-size: 13px; color: var(--text-secondary); }

.btn { width: 100%; height: 40px; border: 0; border-radius: var(--r-btn); background: var(--brand); color: #fff; font-family: inherit; font-size: 14px; font-weight: 600; cursor: pointer; display: inline-flex; align-items: center; justify-content: center; gap: 6px; }
.btn:hover { background: var(--brand-hover); }

.topics { display: flex; flex-direction: column; gap: 7px; }
.topic { width: 100%; display: flex; align-items: center; justify-content: space-between; gap: 8px; background: var(--surface-card); border: 1px solid var(--border-default); border-radius: var(--r-btn); box-shadow: var(--shadow-sm); padding: 10px 12px; font-family: inherit; font-size: 13px; font-weight: 600; color: var(--text-primary); cursor: pointer; text-align: left; }
.topic:hover { background: var(--surface-sunken); }
.topic svg { color: var(--text-tertiary); flex: none; }

.eyebrow { font-size: 11px; font-weight: 600; letter-spacing: .04em; text-transform: uppercase; color: var(--text-tertiary); padding: 4px 4px 0; }

.row { display: flex; gap: 11px; align-items: flex-start; padding: 12px 14px; cursor: pointer; background: var(--surface-card); border: 1px solid var(--border-default); border-radius: var(--r-card); box-shadow: var(--shadow-sm); }
.row:hover { background: var(--surface-sunken); }
.row .name { font-size: 14px; font-weight: 600; color: var(--text-primary); }
.row .prev { font-size: 13px; color: var(--text-secondary); margin-top: 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.row .time { font-size: 11px; color: var(--text-tertiary); flex: none; }

.msg { display: flex; flex-direction: column; max-width: 80%; }
.msg.in { align-self: flex-start; align-items: flex-start; }
.msg.out { align-self: flex-end; align-items: flex-end; }
.msg.system { align-self: center; align-items: center; max-width: 90%; }
.msg .meta { display: flex; gap: 7px; align-items: center; margin-bottom: 4px; padding: 0 4px; }
.msg .author { font-size: 12px; font-weight: 600; color: var(--text-secondary); }
.msg .mtime { font-size: 11px; color: var(--text-tertiary); }
.bubble { font-size: ${fsBody}px; line-height: 1.5; padding: 9px 13px; border-radius: var(--r-bubble); white-space: pre-wrap; word-break: break-word; }
.msg.in .bubble { background: var(--surface-sunken); color: var(--text-primary); border-bottom-left-radius: var(--r-tail); }
.msg.out .bubble { background: var(--brand); color: #fff; border-bottom-right-radius: var(--r-tail); }
.msg.system .bubble { background: transparent; color: var(--text-tertiary); font-size: 12px; padding: 4px 8px; }

.imglink { display: block; max-width: 100%; }
.att-img { max-width: 220px; max-height: 240px; width: auto; height: auto; border-radius: var(--r-card); display: block; }
.atts { display: flex; flex-direction: column; gap: 6px; margin-top: 4px; max-width: 100%; }
.att-chip { display: inline-flex; align-items: center; gap: 7px; max-width: 240px; text-decoration: none; font-size: 13px; padding: 8px 11px; border-radius: var(--r-btn); border: 1px solid var(--border-default); background: var(--surface-card); color: var(--text-primary); }
.att-chip:hover { background: var(--surface-sunken); }
.att-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.composer { padding: 10px 12px 12px; background: var(--surface-card); border-top: 1px solid var(--border-default); flex: none; }
.inputwrap { display: flex; align-items: flex-end; gap: 8px; background: var(--surface-card); border: 1px solid var(--border-default); border-radius: var(--r-card); padding: 6px 8px; transition: border-color .14s, box-shadow .14s; }
.inputwrap:focus-within { border-color: var(--brand); box-shadow: 0 0 0 3px ${hexToRgba(brand, 0.32)}; }
.inputwrap textarea { flex: 1; border: 0; outline: none; resize: none; background: transparent; font-family: inherit; font-size: ${fsBody}px; line-height: 1.5; color: var(--text-primary); max-height: 120px; padding: 6px 2px; }
.attachbtn { width: 34px; height: 34px; flex: none; border: 0; border-radius: var(--r-btn); background: transparent; color: var(--text-tertiary); cursor: pointer; display: flex; align-items: center; justify-content: center; }
.attachbtn:hover:not(:disabled) { color: var(--brand); background: var(--surface-sunken); }
.attachbtn:disabled { opacity: .4; cursor: not-allowed; }
.pending { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 8px; }
.pchip { display: inline-flex; align-items: center; gap: 6px; max-width: 200px; font-size: 12px; background: var(--surface-sunken); border: 1px solid var(--border-default); border-radius: var(--r-btn); padding: 5px 8px; color: var(--text-secondary); }
.pchip.muted { color: var(--text-tertiary); }
.pchip button { border: 0; background: transparent; cursor: pointer; color: var(--text-tertiary); padding: 0; font-size: 13px; line-height: 1; display: flex; }
.pchip .att-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.send { width: 34px; height: 34px; flex: none; border: 0; border-radius: var(--r-btn); background: var(--brand); color: #fff; cursor: pointer; display: flex; align-items: center; justify-content: center; }
.send:hover { background: var(--brand-hover); }
.send:disabled { opacity: .4; cursor: not-allowed; }
.powered { text-align: center; font-size: 11px; color: var(--text-tertiary); padding: 8px 0 ${preview ? "10px" : "0"}; background: var(--surface-card); }

.note { font-size: 12px; color: var(--text-tertiary); text-align: center; padding: 8px 16px; }
.banner { margin: 0 0 4px; background: var(--surface-sunken); color: var(--text-secondary); font-size: 12px; border-radius: var(--r-btn); padding: 8px 10px; text-align: center; }

.mobile-close {
  display: none;
  position: absolute; top: 10px; ${side === "left" ? "left" : "right"}: 10px; z-index: 1;
  width: 36px; height: 36px; border: 0; border-radius: 10px;
  background: rgba(255,255,255,.16); color: #fff; cursor: pointer;
  align-items: center; justify-content: center;
}
.mobile-close:hover { background: rgba(255,255,255,.26); }
${preview ? "" : `
@media (max-width: 480px) {
  .launcher { ${side}: 16px; bottom: 16px; }
  .launcher.open { display: none; }
  .panel {
    left: 0; right: 0; top: 0; bottom: 0;
    width: auto; height: auto; max-width: none; max-height: none;
    border-radius: 0;
    transform-origin: bottom center;
  }
  .mobile-close { display: flex; }
}`}
`;
}

function hexToRgba(hex: string, a: number): string {
  let h = hex.replace("#", "");
  if (h.length === 3) h = h.split("").map((c) => c + c).join("");
  const n = parseInt(h, 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${a})`;
}
