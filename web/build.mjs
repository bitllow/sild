import * as esbuild from "esbuild";
import { copyFile, stat } from "node:fs/promises";

const watch = process.argv.includes("--watch");

// Where the Go backend embeds the bundle from (go:embed needs it inside the
// module tree). `npm run build` copies here so the next `go build` embeds it.
const EMBED_DIR = "../backend/internal/webasset";

async function syncEmbed() {
  await copyFile("dist/widget.js", `${EMBED_DIR}/widget.js`);
  await copyFile("public/demo.html", `${EMBED_DIR}/demo.html`);
}

/** @type {import('esbuild').BuildOptions} */
const opts = {
  entryPoints: ["src/index.tsx"],
  bundle: true,
  format: "iife", // self-contained: defines window.Sild, no module loader needed
  outfile: "dist/widget.js",
  minify: !watch,
  sourcemap: watch,
  target: ["es2019"],
  jsx: "automatic",
  jsxImportSource: "preact",
  legalComments: "none",
  define: { "process.env.NODE_ENV": '"production"' },
};

async function run() {
  if (watch) {
    const ctx = await esbuild.context(opts);
    await ctx.watch();
    await copyFile("public/demo.html", "dist/demo.html").catch(() => {});
    await syncEmbed().catch(() => {});
    console.log("watching… widget → dist/widget.js (+ embedded into backend)");
  } else {
    await esbuild.build(opts);
    await copyFile("public/demo.html", "dist/demo.html");
    await syncEmbed();
    const { size } = await stat("dist/widget.js");
    console.log(`built dist/widget.js (${(size / 1024).toFixed(1)} KB) → embedded into backend/internal/webasset`);
  }
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
