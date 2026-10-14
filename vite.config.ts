import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import { nip5aManifest } from "@napplet/vite-plugin";
import { svelte } from "@sveltejs/vite-plugin-svelte";

export default defineConfig(({ mode }) => ({
  root: fileURLToPath(new URL("./internal/store/web", import.meta.url)),
  base: "./",
  server: {
    host: "127.0.0.1",
    proxy: mode === "legacy" ? { "/api": "http://127.0.0.1:8080" } : undefined,
  },
  plugins: [svelte(), nip5aManifest({
    nappletType: "steamduck-store",
    title: "Steamduck Store",
    description: "Discover, install and play napplets on Steam Deck with your Nostr account.",
    requires: ["steamduck", "resource"],
    optional: ["gamepad", "theme", "identity", "relay", "intent"],
    artifactMode: "single-file",
    icon: {
      data: readFileSync(new URL("./internal/store/web/logo.png", import.meta.url)),
      mimeType: "image/png",
    },
  })],
  build: {
    outDir: fileURLToPath(new URL(mode === "legacy" ? "./dist/store-legacy" : "./dist/store", import.meta.url)),
    emptyOutDir: true,
    assetsInlineLimit: Number.MAX_SAFE_INTEGER,
    target: "es2022",
    cssCodeSplit: false,
    modulePreload: false,
  },
}));
