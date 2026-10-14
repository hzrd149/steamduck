import "./crypto";
import "./store.css";
import "./account.css";
import "./settings.css";
import { installLegacyAdapter } from "./legacy";
import { requireStore } from "./api";
import { initSettings } from "./settings";
import { initAccount } from "./account.js";
import { startStore } from "./store.js";
import { themeGet, themeOnChanged } from "@napplet/nap/theme/sdk";
import type { Theme } from "@napplet/nap/theme/types";

if (import.meta.env.MODE === "legacy") installLegacyAdapter();

function applyTheme(theme: Theme): void {
  const colors = theme.colors;
  const style = document.documentElement.style;
  style.setProperty("--bg", colors.background);
  style.setProperty("--text", colors.text);
  style.setProperty("--accent", colors.primary);
  style.setProperty("--focus", colors.primary);
}

async function start(): Promise<void> {
  try {
    await requireStore();
    initAccount();
    initSettings();
    startStore();
    if (window.napplet?.theme) {
      const subscription = themeOnChanged(applyTheme);
      window.addEventListener("pagehide", () => subscription.close(), { once: true });
      themeGet().then(applyTheme).catch(() => {});
    }
  } catch (error) {
    document.querySelector("#status")!.className = "status bad";
    document.querySelector("#status-text")!.textContent = "Store unavailable";
    const notice = document.querySelector<HTMLElement>("#runtime-unavailable")!;
    notice.hidden = false;
    notice.textContent = (error as Error).message;
  }
}

void start();
