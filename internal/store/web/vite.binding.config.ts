import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

export default defineConfig({
  build: {
    outDir: "dist/binding",
    emptyOutDir: true,
    target: "es2022",
    minify: "esbuild",
    rollupOptions: {
      input: fileURLToPath(new URL("../nap-steamduck/prelude.ts", import.meta.url)),
      output: { format: "iife", entryFileNames: "steamduck.js", inlineDynamicImports: true },
    },
  },
});
