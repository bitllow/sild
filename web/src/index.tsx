import { render } from "preact";
import { SildClient } from "./core/client";
import { App } from "./widget/App";
import { css } from "./widget/styles";
import type { SildConfig } from "./core/types";

// Captured at load time, when document.currentScript is this widget's <script>.
const SELF_ORIGIN = (() => {
  try {
    const s = document.currentScript as HTMLScriptElement | null;
    return s?.src ? new URL(s.src).origin : "";
  } catch {
    return "";
  }
})();

let fontInjected = false;
function injectFont() {
  if (fontInjected || typeof document === "undefined") return;
  fontInjected = true;
  const link = document.createElement("link");
  link.rel = "stylesheet";
  link.href =
    "https://fonts.googleapis.com/css2?family=Schibsted+Grotesk:wght@400;500;600;700;800&display=swap";
  document.head.appendChild(link);
}

class SildWidgetElement extends HTMLElement {
  private client?: SildClient;
  config?: SildConfig;

  connectedCallback() {
    const cfg = this.config!;
    const root = this.attachShadow({ mode: "open" }); // style isolation (§9)
    const style = document.createElement("style");
    style.textContent = css;
    root.appendChild(style);
    const mount = document.createElement("div");
    root.appendChild(mount);
    this.client = new SildClient(cfg);
    render(<App client={this.client} config={cfg} />, mount);
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

const Sild = {
  init(config: SildConfig) {
    if (!config || typeof config.tokenProvider !== "function") {
      throw new Error("Sild.init: a tokenProvider function is required");
    }
    injectFont();
    defineElement();
    const el = document.createElement("sild-widget") as SildWidgetElement;
    el.config = { ...config, baseUrl: config.baseUrl || SELF_ORIGIN };
    document.body.appendChild(el);
    return { destroy: () => el.remove() };
  },
};

(window as unknown as { Sild: typeof Sild }).Sild = Sild;
