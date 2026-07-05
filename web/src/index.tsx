import { render } from "preact";
import { PreviewClient, SildClient } from "./core/client";
import { App } from "./widget/App";
import { buildStyles, GOOGLE_FONT_HREF } from "./widget/theme";
import { DEFAULT_BRAND, type BrandConfig, type SildConfig } from "./core/types";

// Captured at load time, when document.currentScript is this widget's <script>.
const SELF_ORIGIN = (() => {
  try {
    const s = document.currentScript as HTMLScriptElement | null;
    return s?.src ? new URL(s.src).origin : "";
  } catch {
    return "";
  }
})();

// Load the Google-hosted family a config needs, once per href. System default
// needs nothing; the rest are pulled on demand so we ship no unused fonts.
const loadedFonts = new Set<string>();
function injectFont(font: BrandConfig["font"]) {
  if (typeof document === "undefined") return;
  const href = GOOGLE_FONT_HREF[font];
  if (!href || loadedFonts.has(href)) return;
  loadedFonts.add(href);
  const link = document.createElement("link");
  link.rel = "stylesheet";
  link.href = href;
  document.head.appendChild(link);
}

function resolveBrand(partial?: Partial<BrandConfig>): BrandConfig {
  return { ...DEFAULT_BRAND, ...(partial || {}) };
}

class SildWidgetElement extends HTMLElement {
  private client?: SildClient;
  config?: SildConfig;
  private styleEl?: HTMLStyleElement;
  private mount?: HTMLDivElement;
  private brand: BrandConfig = DEFAULT_BRAND;
  private name?: string;
  private cmd?: { seq: number; conversationId?: string };

  // openConversation is the host-facing imperative: open the panel (optionally
  // straight to a conversation), e.g. from a "Your driver is on the way" card.
  openConversation(conversationId?: string) {
    this.cmd = { seq: (this.cmd?.seq || 0) + 1, conversationId };
    this.paint();
  }

  connectedCallback() {
    const cfg = this.config!;
    const root = this.attachShadow({ mode: "open" }); // style isolation (§9)
    this.styleEl = document.createElement("style");
    root.appendChild(this.styleEl);
    this.mount = document.createElement("div");
    root.appendChild(this.mount);
    this.client = new SildClient(cfg);

    // Render immediately with defaults (+ any inline overrides) so the launcher
    // is up instantly, then refine once the active brand arrives. The fetch is
    // UNAUTHENTICATED (public, app-id-keyed) so merely rendering the launcher
    // never mints a token or creates a user — token minting stays deferred to
    // start() on first open.
    this.brand = resolveBrand(cfg.brand);
    this.paint();

    void this.client
      .fetchPublicBrand(cfg.appId)
      .then((res) => {
        this.brand = resolveBrand({ ...res.config, ...cfg.brand });
        this.name = res.name;
        this.paint();
      })
      .catch(() => {
        /* keep the default look if the brand fetch fails (e.g. no app id set) */
      });
  }

  private paint() {
    if (!this.styleEl || !this.mount || !this.client) return;
    injectFont(this.brand.font);
    this.styleEl.textContent = buildStyles(this.brand, "live");
    render(
      <App
        client={this.client}
        config={this.brand}
        conversationId={this.config?.conversationId}
        name={this.name}
        mode="live"
        command={this.cmd}
      />,
      this.mount
    );
  }

  disconnectedCallback() {
    this.client?.destroy();
  }
}

function defineElement() {
  if (!customElements.get("sild-widget")) {
    customElements.define("sild-widget", SildWidgetElement);
  }
}

/** Handle returned by Sild.preview — drives the Appearance live preview. */
export interface PreviewHandle {
  update(config: BrandConfig, view?: "home" | "chat"): void;
  destroy(): void;
}

const Sild = {
  init(config: SildConfig) {
    if (!config || typeof config.tokenProvider !== "function") {
      throw new Error("Sild.init: a tokenProvider function is required");
    }
    defineElement();
    const el = document.createElement("sild-widget") as SildWidgetElement;
    el.config = { ...config, baseUrl: config.baseUrl || SELF_ORIGIN };
    document.body.appendChild(el);
    return {
      destroy: () => el.remove(),
      // Open the widget (optionally straight to a conversation) on demand — used
      // by host entry points like the "Your driver is on the way" card.
      open: (conversationId?: string) => el.openConversation(conversationId),
    };
  },

  // preview mounts the REAL widget (in preview geometry, backed by a no-network
  // client) inside a container — the Appearance tab feeds it the draft config so
  // preview === production. Returns a handle to re-render on every edit.
  preview(container: HTMLElement, config: BrandConfig, view: "home" | "chat" = "home"): PreviewHandle {
    const host = document.createElement("div");
    host.style.cssText = "position:absolute;inset:0;";
    container.appendChild(host);
    const root = host.attachShadow({ mode: "open" });
    const styleEl = document.createElement("style");
    root.appendChild(styleEl);
    const mount = document.createElement("div");
    root.appendChild(mount);
    const client = new PreviewClient();

    const paint = (cfg: BrandConfig, v: "home" | "chat") => {
      injectFont(cfg.font);
      styleEl.textContent = buildStyles(cfg, "preview");
      render(<App client={client} config={cfg} mode="preview" previewView={v} name="Acme Rides" />, mount);
    };
    paint(config, view);

    return {
      update: (cfg, v = "home") => paint(cfg, v),
      destroy: () => {
        render(null, mount);
        host.remove();
      },
    };
  },
};

(window as unknown as { Sild: typeof Sild }).Sild = Sild;
