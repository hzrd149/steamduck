import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/store",
  use: { viewport: { width: 1280, height: 800 } },
});
