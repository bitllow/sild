"use client";

import { useEffect, useRef } from "react";
import type { CSSProperties } from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { Button, Input, Select, Switch, Textarea } from "@/components/ds";
import type { BrandConfig } from "@/store/types";

// The live preview mounts the REAL drop-in bundle and feeds it the draft config,
// so preview === production. The bundle defines window.Sild.preview(el, cfg, view).
interface PreviewHandle {
  update(config: BrandConfig, view: "home" | "chat"): void;
  destroy(): void;
}
interface SildGlobal {
  preview(el: HTMLElement, config: BrandConfig, view: "home" | "chat"): PreviewHandle;
}
function sild(): SildGlobal | undefined {
  return (window as unknown as { Sild?: SildGlobal }).Sild;
}
let widgetLoad: Promise<void> | null = null;
function ensureWidget(): Promise<void> {
  if (sild()?.preview) return Promise.resolve();
  if (widgetLoad) return widgetLoad;
  widgetLoad = new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = "/widget.js";
    s.async = true;
    s.onload = () => resolve();
    s.onerror = () => reject(new Error("widget bundle failed to load"));
    document.head.appendChild(s);
  });
  return widgetLoad;
}

// ── style helpers (ported byte-faithful from the source design's apVals) ─────
const card: CSSProperties = {
  background: "var(--white)",
  border: "1px solid var(--border-default)",
  borderRadius: 12,
  boxShadow: "var(--shadow-sm)",
  overflow: "hidden",
};
const cardHead: CSSProperties = { padding: "14px 18px", borderBottom: "1px solid var(--border-subtle)" };
const cardBody: CSSProperties = { padding: 18, display: "flex", flexDirection: "column", gap: 20 };
const fieldLabel: CSSProperties = { fontSize: 13, fontWeight: 600, marginBottom: 8 };
const segRow: CSSProperties = { display: "flex", gap: 4, background: "var(--surface-sunken)", padding: 4, borderRadius: 10 };

function seg(active: boolean): CSSProperties {
  return {
    flex: 1,
    border: 0,
    cursor: "pointer",
    fontFamily: "var(--font-sans)",
    fontSize: 13,
    fontWeight: 600,
    padding: "7px 6px",
    borderRadius: 7,
    whiteSpace: "nowrap",
    transition: "all .12s",
    background: active ? "#fff" : "transparent",
    color: active ? "var(--text-primary)" : "var(--text-secondary)",
    boxShadow: active ? "var(--shadow-xs)" : "none",
  };
}
function swatch(active: boolean, color: string): CSSProperties {
  return {
    width: 34,
    height: 34,
    borderRadius: 9,
    border: "1px solid rgba(0,0,0,.12)",
    background: color,
    cursor: "pointer",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    boxShadow: active ? `0 0 0 2px var(--white), 0 0 0 4px ${color}` : "none",
    transition: "box-shadow .12s",
  };
}
function iconBtn(active: boolean): CSSProperties {
  return {
    width: 46,
    height: 46,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    borderRadius: 10,
    cursor: "pointer",
    border: `1px solid ${active ? "var(--border-focus)" : "var(--border-default)"}`,
    background: active ? "var(--brand-subtle)" : "var(--white)",
    color: active ? "var(--brand)" : "var(--text-secondary)",
    boxShadow: active ? "var(--ring)" : "none",
    transition: "all .12s",
  };
}
function menuRow(active: boolean): CSSProperties {
  return {
    display: "flex",
    alignItems: "center",
    gap: 9,
    width: "100%",
    padding: "9px 10px",
    border: 0,
    background: active ? "var(--brand-subtle)" : "transparent",
    borderRadius: 7,
    cursor: "pointer",
    fontFamily: "var(--font-sans)",
    fontSize: 13,
    fontWeight: 600,
    color: active ? "var(--brand)" : "var(--text-primary)",
  };
}

const SWATCHES = ["#2563FD", "#14181F", "#1F8A5B", "#FF7A45", "#7C3AED", "#0EA5E9"];
const FONT_OPTIONS = [
  { value: "system", label: "System default" },
  { value: "sild", label: "Sild Sans (default)" },
  { value: "inter", label: "Inter" },
  { value: "figtree", label: "Figtree" },
  { value: "dmsans", label: "DM Sans" },
];

// ── inline icons ─────────────────────────────────────────────────────────────
const Chevron = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ flex: "none", color: "var(--text-tertiary)" }}><path d="m6 9 6 6 6-6" /></svg>
);
const Pencil = () => (
  <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 20h9" /><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z" /></svg>
);
const Check = ({ w = 16, sw = 2.4, color = "currentColor" }: { w?: number; sw?: number; color?: string }) => (
  <svg width={w} height={w} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={sw} strokeLinecap="round" strokeLinejoin="round"><path d="M20 6 9 17l-5-5" /></svg>
);
const Plus = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M5 12h14M12 5v14" /></svg>
);
const UploadGlyph = ({ s = 20 }: { s?: number }) => (
  <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><path d="M17 8l-5-5-5 5" /><path d="M12 3v12" /></svg>
);
const IChat = () => (
  <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8z" /></svg>
);
const IMsg = () => (
  <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" /></svg>
);
const IHelp = () => (
  <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10" /><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3" /><line x1="12" y1="17" x2="12.01" y2="17" /></svg>
);
const ISpark = () => (
  <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 3v4M12 17v4M3 12h4M17 12h4M5.6 5.6l2.8 2.8M15.6 15.6l2.8 2.8M18.4 5.6l-2.8 2.8M8.4 15.6l-2.8 2.8" /></svg>
);

function readAsDataURL(file: File, cb: (url: string) => void) {
  const r = new FileReader();
  r.onload = () => cb(String(r.result));
  r.readAsDataURL(file);
}

export const Appearance = observer(function Appearance() {
  const store = useStore();
  const brand = store.activeBrand;

  const canvasRef = useRef<HTMLDivElement>(null);
  const handleRef = useRef<PreviewHandle | null>(null);

  const config = brand?.config;
  const cfgJson = config ? JSON.stringify(config) : "";
  const view = store.brandPreview;

  // Mount the real widget into the canvas, then re-feed it the draft config on
  // every edit. Destroyed on unmount.
  useEffect(() => {
    let alive = true;
    if (!config) return;
    ensureWidget()
      .then(() => {
        if (!alive || !canvasRef.current) return;
        const s = sild();
        if (!s) return;
        if (handleRef.current) handleRef.current.update(config, view);
        else handleRef.current = s.preview(canvasRef.current, config, view);
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cfgJson, view]);

  useEffect(
    () => () => {
      handleRef.current?.destroy();
      handleRef.current = null;
    },
    []
  );

  if (!brand || !config) {
    return <div style={{ padding: 40, color: "var(--text-tertiary)", fontSize: 14 }}>Loading appearance…</div>;
  }

  const patch = (p: Partial<BrandConfig>) => store.patchBrand(p);

  // Upload logo/icon to the bucket: show the picked file instantly (local data
  // URL) while the object key uploads in the background. On failure, drop the
  // preview so we don't imply a saved asset that isn't there.
  const uploadLogo = (f: File) => {
    readAsDataURL(f, (url) => patch({ logoUrl: url }));
    store.uploadBrandAsset(f).then((key) => patch({ logo: key })).catch(() => patch({ logo: "", logoUrl: "" }));
  };
  const uploadIcon = (f: File) => {
    readAsDataURL(f, (url) => patch({ iconImgUrl: url, launcherIcon: "custom" }));
    store.uploadBrandAsset(f).then((key) => patch({ iconImg: key })).catch(() => patch({ iconImg: "", iconImgUrl: "" }));
  };
  const logoSrc = config.logoUrl || config.logo;
  const iconSrc = config.iconImgUrl || config.iconImg;

  return (
    <div style={{ maxWidth: 1140, position: "relative" }}>
      {/* Profile bar */}
      <div style={{ display: "flex", alignItems: "flex-end", gap: 16, marginBottom: 20 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontSize: 11, fontWeight: 600, letterSpacing: ".04em", textTransform: "uppercase", color: "var(--text-tertiary)", marginBottom: 7 }}>
            Brand profile
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 10, position: "relative" }}>
            {!store.brandEditingName ? (
              <>
                <button
                  onClick={store.toggleBrandMenu}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                    width: 288,
                    height: 40,
                    padding: "0 12px",
                    border: `1px solid ${store.brandMenuOpen ? "var(--border-focus)" : "var(--border-default)"}`,
                    borderRadius: 8,
                    background: "var(--white)",
                    boxShadow: store.brandMenuOpen ? "var(--ring)" : "var(--shadow-xs)",
                    cursor: "pointer",
                    fontFamily: "var(--font-sans)",
                    fontSize: 14,
                    fontWeight: 600,
                    color: "var(--text-primary)",
                  }}
                >
                  <span style={{ width: 22, height: 22, borderRadius: 6, flex: "none", background: config.brand }} />
                  <span style={{ flex: 1, textAlign: "left", minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    {brand.name}
                  </span>
                  <Chevron />
                </button>
                <button
                  onClick={store.startBrandRename}
                  aria-label="Rename profile"
                  style={{ width: 40, height: 40, flex: "none", display: "flex", alignItems: "center", justifyContent: "center", border: "1px solid var(--border-default)", background: "var(--white)", borderRadius: 8, cursor: "pointer", color: "var(--text-secondary)", boxShadow: "var(--shadow-xs)" }}
                >
                  <Pencil />
                </button>
              </>
            ) : (
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <input
                  value={brand.name}
                  autoFocus
                  placeholder="Profile name"
                  onChange={(e) => store.renameBrand(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === "Escape") {
                      e.preventDefault();
                      store.stopBrandRename();
                    }
                  }}
                  onBlur={store.stopBrandRename}
                  style={{ width: 288, height: 40, padding: "0 12px", border: "1px solid var(--border-focus)", borderRadius: 8, background: "var(--white)", boxShadow: "var(--ring)", fontFamily: "var(--font-sans)", fontSize: 14, fontWeight: 600, color: "var(--text-primary)", outline: "none" }}
                />
                <button
                  onMouseDown={store.stopBrandRename}
                  aria-label="Done"
                  style={{ width: 40, height: 40, flex: "none", display: "flex", alignItems: "center", justifyContent: "center", border: 0, background: "var(--brand)", borderRadius: 8, cursor: "pointer", color: "#fff" }}
                >
                  <Check color="#fff" />
                </button>
              </div>
            )}

            {store.brandMenuOpen && (
              <div style={{ position: "absolute", top: 46, left: 0, width: 260, background: "var(--white)", border: "1px solid var(--border-default)", borderRadius: 10, boxShadow: "var(--shadow-lg)", padding: 6, zIndex: 30 }}>
                {store.brands.map((p) => (
                  <button key={p.id} onClick={() => store.selectBrand(p.id)} style={menuRow(p.id === store.activeBrandId)}>
                    <span style={{ width: 18, height: 18, borderRadius: 5, flex: "none", background: p.config.brand }} />
                    <span style={{ flex: 1, textAlign: "left" }}>{p.name}</span>
                    {p.id === store.activeBrandId && <Check w={16} sw={2.4} color="var(--brand)" />}
                  </button>
                ))}
                <div style={{ height: 1, background: "var(--border-subtle)", margin: "6px 4px" }} />
                <button onClick={store.newBrand} style={{ display: "flex", alignItems: "center", gap: 9, width: "100%", padding: "9px 10px", border: 0, background: "transparent", borderRadius: 7, cursor: "pointer", fontFamily: "var(--font-sans)", fontSize: 13, fontWeight: 600, color: "var(--brand)" }}>
                  <Plus />
                  New profile
                </button>
              </div>
            )}

            <span style={{ fontSize: 13, color: "var(--text-tertiary)" }}>applies to your web and in-app (SDK) messengers</span>
          </div>
        </div>

        <div style={{ display: "flex", alignItems: "center", gap: 14, flex: "none" }}>
          {store.brandDirty && (
            <span style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 13, color: "var(--text-tertiary)" }}>
              <span style={{ width: 7, height: 7, borderRadius: "50%", background: "var(--coral-500, #FF7A45)" }} />
              Unsaved changes
            </span>
          )}
          <Button variant="secondary" onClick={store.resetBrands} disabled={!store.brandDirty}>
            Reset
          </Button>
          <Button onClick={store.saveBrands} disabled={!store.brandDirty} loading={store.brandSaving}>
            Save changes
          </Button>
        </div>
      </div>

      {store.brandMenuOpen && <div onClick={store.closeBrandMenu} style={{ position: "fixed", inset: 0, zIndex: 20 }} />}

      {/* Split: controls + preview */}
      <div style={{ display: "flex", gap: 28, alignItems: "flex-start" }}>
        {/* ===== CONTROLS ===== */}
        <div style={{ flex: 1, minWidth: 0, maxWidth: 560, display: "flex", flexDirection: "column", gap: 18 }}>
          {/* Brand */}
          <div style={card}>
            <div style={cardHead}>
              <div style={{ fontSize: 15, fontWeight: 700 }}>Brand</div>
              <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>Your logo and color. Everything else is generated from these.</div>
            </div>
            <div style={cardBody}>
              <div>
                <div style={fieldLabel}>Logo</div>
                <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
                  <label style={{ position: "relative", width: 124, height: 66, flex: "none", border: "1.5px dashed var(--border-default)", borderRadius: 10, background: "var(--surface-sunken)", display: "flex", alignItems: "center", justifyContent: "center", cursor: "pointer", overflow: "hidden" }}>
                    {logoSrc ? (
                      <img src={logoSrc} alt="Logo" style={{ maxWidth: "88%", maxHeight: "80%", objectFit: "contain" }} />
                    ) : (
                      <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 4, color: "var(--text-tertiary)" }}>
                        <UploadGlyph />
                        <span style={{ fontSize: 11, fontWeight: 600 }}>Upload</span>
                      </div>
                    )}
                    <input
                      type="file"
                      accept="image/png,image/svg+xml,image/jpeg"
                      onChange={(e) => {
                        const f = e.target.files?.[0];
                        if (f) uploadLogo(f);
                        e.target.value = "";
                      }}
                      style={{ position: "absolute", width: 1, height: 1, opacity: 0, left: 0, top: 0 }}
                    />
                  </label>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: 13, color: "var(--text-secondary)", lineHeight: 1.5 }}>PNG or SVG with a transparent background works best. Sits in the chat header.</div>
                    {logoSrc && (
                      <button onClick={() => patch({ logo: "", logoUrl: "" })} style={{ marginTop: 6, border: 0, background: "transparent", padding: 0, cursor: "pointer", fontFamily: "var(--font-sans)", fontSize: 13, fontWeight: 600, color: "var(--danger, #DC2626)" }}>
                        Remove logo
                      </button>
                    )}
                  </div>
                </div>
              </div>
              <div>
                <div style={fieldLabel}>Brand color</div>
                <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
                  {SWATCHES.map((col) => (
                    <button key={col} data-testid="brand-swatch" data-color={col} onClick={() => patch({ brand: col })} aria-label="Pick color" style={swatch(config.brand === col, col)}>
                      {config.brand === col && <Check w={16} sw={3} color="#fff" />}
                    </button>
                  ))}
                  <label style={{ position: "relative", display: "flex", alignItems: "center", gap: 8, height: 34, padding: "0 12px", border: "1px solid var(--border-default)", borderRadius: 8, cursor: "pointer", background: "var(--white)" }}>
                    <span style={{ width: 16, height: 16, borderRadius: 4, background: config.brand, border: "1px solid rgba(0,0,0,.1)" }} />
                    <span style={{ fontFamily: "var(--font-mono)", fontSize: 13, color: "var(--text-secondary)" }}>{config.brand}</span>
                    <input type="color" value={config.brand} onInput={(e) => patch({ brand: (e.target as HTMLInputElement).value })} style={{ position: "absolute", width: 1, height: 1, opacity: 0, left: 0, top: 0 }} />
                  </label>
                </div>
              </div>
            </div>
          </div>

          {/* Look & feel */}
          <div style={card}>
            <div style={cardHead}>
              <div style={{ fontSize: 15, fontWeight: 700 }}>Look &amp; feel</div>
            </div>
            <div style={cardBody}>
              <div>
                <div style={fieldLabel}>Theme</div>
                <div style={segRow}>
                  {(["light", "dark", "auto"] as const).map((v) => (
                    <button key={v} onClick={() => patch({ theme: v })} style={seg(config.theme === v)}>
                      {v === "light" ? "Light" : v === "dark" ? "Dark" : "Auto"}
                    </button>
                  ))}
                </div>
                {config.theme === "auto" && (
                  <div style={{ fontSize: 12, color: "var(--text-tertiary)", marginTop: 7 }}>{"Follows each visitor's system setting. Preview shows the light variant."}</div>
                )}
              </div>
              <div>
                <div style={fieldLabel}>Font</div>
                <Select options={FONT_OPTIONS} value={config.font} onChange={(e) => patch({ font: e.target.value as BrandConfig["font"] })} />
              </div>
              <div>
                <div style={fieldLabel}>Corners</div>
                <div style={segRow}>
                  {(["sharp", "default", "rounded", "pillowy"] as const).map((v) => (
                    <button key={v} onClick={() => patch({ radius: v })} style={seg(config.radius === v)}>
                      {v.charAt(0).toUpperCase() + v.slice(1)}
                    </button>
                  ))}
                </div>
              </div>
            </div>
          </div>

          {/* Launcher */}
          <div style={card}>
            <div style={cardHead}>
              <div style={{ fontSize: 15, fontWeight: 700 }}>Launcher</div>
              <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2 }}>The floating button that opens the chat.</div>
            </div>
            <div style={cardBody}>
              <div>
                <div style={fieldLabel}>Icon</div>
                <div style={{ display: "flex", gap: 10 }}>
                  <button onClick={() => patch({ launcherIcon: "chat" })} aria-label="Speech bubble" style={iconBtn(config.launcherIcon === "chat")}><IChat /></button>
                  <button onClick={() => patch({ launcherIcon: "message" })} aria-label="Message" style={iconBtn(config.launcherIcon === "message")}><IMsg /></button>
                  <button onClick={() => patch({ launcherIcon: "help" })} aria-label="Question" style={iconBtn(config.launcherIcon === "help")}><IHelp /></button>
                  <button onClick={() => patch({ launcherIcon: "sparkle" })} aria-label="Sparkle" style={iconBtn(config.launcherIcon === "sparkle")}><ISpark /></button>
                  <div style={{ width: 1, alignSelf: "stretch", background: "var(--border-subtle)", margin: "0 2px" }} />
                  <label aria-label="Upload custom icon" style={{ ...iconBtn(config.launcherIcon === "custom"), position: "relative", overflow: "hidden", cursor: "pointer" }}>
                    {iconSrc ? (
                      <img src={iconSrc} alt="" style={{ width: 24, height: 24, objectFit: "contain" }} />
                    ) : (
                      <UploadGlyph s={19} />
                    )}
                    <input
                      type="file"
                      accept="image/png,image/svg+xml"
                      onChange={(e) => {
                        const f = e.target.files?.[0];
                        if (f) uploadIcon(f);
                        e.target.value = "";
                      }}
                      style={{ position: "absolute", width: 1, height: 1, opacity: 0, left: 0, top: 0 }}
                    />
                  </label>
                </div>
                <div style={{ fontSize: 12, color: "var(--text-tertiary)", marginTop: 8 }}>Upload your own (SVG or PNG). A white, transparent icon reads best on the colored button.</div>
              </div>
              <div style={{ display: "flex", gap: 18 }}>
                <div style={{ flex: 1 }}>
                  <div style={fieldLabel}>Position</div>
                  <div style={segRow}>
                    {(["left", "right"] as const).map((v) => (
                      <button key={v} onClick={() => patch({ launcherPos: v })} style={seg(config.launcherPos === v)}>
                        {v === "left" ? "Left" : "Right"}
                      </button>
                    ))}
                  </div>
                </div>
                <div style={{ flex: 1 }}>
                  <div style={fieldLabel}>Size</div>
                  <div style={segRow}>
                    {(["sm", "md", "lg"] as const).map((v) => (
                      <button key={v} onClick={() => patch({ launcherSize: v })} style={seg(config.launcherSize === v)}>
                        {v === "sm" ? "Small" : v === "md" ? "Medium" : "Large"}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          </div>

          {/* Welcome screen */}
          <div style={card}>
            <div style={cardHead}>
              <div style={{ fontSize: 15, fontWeight: 700 }}>Welcome screen</div>
            </div>
            <div style={{ ...cardBody, gap: 18 }}>
              <div>
                <div style={fieldLabel}>Heading</div>
                <Input value={config.heading} placeholder="Hi there." onChange={(e) => patch({ heading: e.target.value })} />
              </div>
              <div>
                <div style={fieldLabel}>Subtext</div>
                <Textarea value={config.sub} rows={2} placeholder="How can we help?" onChange={(e) => patch({ sub: e.target.value })} />
              </div>
              <div>
                <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 4 }}>Suggested topics</div>
                <div style={{ fontSize: 12, color: "var(--text-tertiary)", marginBottom: 8 }}>One per line. Shown as tappable buttons. Leave blank to hide.</div>
                <Textarea value={config.topics} rows={3} placeholder={"Track my order\nBilling question"} onChange={(e) => patch({ topics: e.target.value })} />
              </div>
              <div style={{ display: "flex", alignItems: "flex-start", gap: 12, paddingTop: 4 }}>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 14, fontWeight: 600 }}>Show team in header</div>
                  <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2, lineHeight: 1.5 }}>{'Display agent avatars and an "online" status so it feels human.'}</div>
                </div>
                <Switch checked={config.showTeam} onChange={(v) => patch({ showTeam: v })} />
              </div>
            </div>
          </div>

          {/* Footer */}
          <div style={card}>
            <div style={{ padding: "16px 18px", display: "flex", alignItems: "flex-start", gap: 12 }}>
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 14, fontWeight: 600 }}>{'Show "Powered by Sild"'}</div>
                <div style={{ fontSize: 13, color: "var(--text-tertiary)", marginTop: 2, lineHeight: 1.5 }}>A small credit at the bottom of the chat. Can be turned off on paid plans.</div>
              </div>
              <Switch checked={config.poweredBy} onChange={(v) => patch({ poweredBy: v })} />
            </div>
          </div>
        </div>

        {/* ===== LIVE PREVIEW ===== */}
        <div style={{ width: 430, flex: "none", position: "sticky", top: 0 }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 12 }}>
            <div style={{ fontSize: 11, fontWeight: 600, letterSpacing: ".04em", textTransform: "uppercase", color: "var(--text-tertiary)" }}>Live preview</div>
            <div style={{ display: "flex", gap: 3, background: "var(--surface-sunken)", padding: 3, borderRadius: 9 }}>
              {(["home", "chat"] as const).map((v) => (
                <button key={v} onClick={() => store.setBrandPreview(v)} style={seg(store.brandPreview === v)}>
                  {v === "home" ? "Home" : "Conversation"}
                </button>
              ))}
            </div>
          </div>

          <div style={{ position: "relative", height: 560, borderRadius: 14, border: "1px solid var(--border-default)", overflow: "hidden", background: "linear-gradient(180deg,#EEF1F6,#E4E8EF)" }}>
            {/* faux site chrome */}
            <div style={{ height: 34, background: "#fff", borderBottom: "1px solid rgba(20,24,31,.07)", display: "flex", alignItems: "center", gap: 6, padding: "0 12px" }}>
              <span style={{ width: 9, height: 9, borderRadius: "50%", background: "#E2655A" }} />
              <span style={{ width: 9, height: 9, borderRadius: "50%", background: "#E8B14C" }} />
              <span style={{ width: 9, height: 9, borderRadius: "50%", background: "#5BB463" }} />
              <div style={{ marginLeft: 8, height: 14, flex: 1, maxWidth: 200, borderRadius: 7, background: "var(--surface-sunken)" }} />
            </div>
            <div style={{ padding: "22px 22px 0", opacity: 0.55 }}>
              <div style={{ width: "46%", height: 13, borderRadius: 6, background: "#fff", marginBottom: 12 }} />
              <div style={{ width: "72%", height: 9, borderRadius: 5, background: "#fff", marginBottom: 7 }} />
              <div style={{ width: "64%", height: 9, borderRadius: 5, background: "#fff", marginBottom: 7 }} />
              <div style={{ width: "68%", height: 9, borderRadius: 5, background: "#fff" }} />
            </div>
            {/* the real widget mounts here (absolute, fills the canvas) */}
            <div ref={canvasRef} data-testid="appearance-preview" style={{ position: "absolute", inset: 0 }} />
          </div>
          <div style={{ fontSize: 12, color: "var(--text-tertiary)", marginTop: 10, textAlign: "center" }}>This is exactly what visitors see on your site.</div>
        </div>
      </div>
    </div>
  );
});
