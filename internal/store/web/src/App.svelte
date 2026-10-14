<script lang="ts">
  import { onMount, tick } from "svelte";
  import { themeGet, themeOnChanged } from "@napplet/nap/theme/sdk";
  import type { Theme } from "@napplet/nap/theme/types";
  import { requireStore } from "./api";
  import { initAccount } from "./account.js";
  import { initSettings } from "./settings";
  import { initStore, showTab, startStore } from "./store.js";
  import { navigation, notification, runtimeStatus } from "./ui";
  import Catalog from "./views/Catalog.svelte";
  import FeatureScreens from "./views/FeatureScreens.svelte";
  import logo from "./logo.png";

  let unavailable = $state("");
  let initialized = $state(false);
  function applyTheme(theme: Theme) {
    const style = document.documentElement.style;
    for (const [key, value] of Object.entries({ bg: theme.colors.background, text: theme.colors.text, accent: theme.colors.primary, focus: theme.colors.primary })) {
      style.setProperty(`--${key}`, value);
    }
  }

  onMount(() => {
    let closed = false;
    const disposers: Array<() => void> = [];
    const stop = () => {
      closed = true;
      disposers.splice(0).reverse().forEach(dispose => dispose());
    };
    window.addEventListener("pagehide", stop, { once: true });
    void (async () => {
      try {
        await requireStore();
        if (closed) return;
        await tick();
        initAccount();
        initSettings();
        disposers.push(initStore(), startStore());
        initialized = true;
        if (window.napplet?.theme) {
          const subscription = themeOnChanged(applyTheme);
          disposers.push(() => subscription.close());
          void themeGet().then(theme => { if (!closed) applyTheme(theme); }).catch(() => {});
        }
      } catch (error) {
        if (closed) return;
        unavailable = error instanceof Error ? error.message : String(error);
        runtimeStatus.set({ text: "Store unavailable", ready: false });
      }
    })();
    return () => { window.removeEventListener("pagehide", stop); stop(); };
  });
</script>

<header>
  <div class="brand"><img class="logo" src={logo} alt="" aria-hidden="true">Steamduck Store</div>
  <div class="tabs" role="tablist" aria-label="Store">
    <span class="hint" aria-hidden="true">L1</span>
    {#each ["library", "discover", "account", "settings"] as tab}
      <button class="tab" data-tab={tab} role="tab" aria-selected={$navigation.tab === tab} disabled={!initialized} onclick={() => showTab(tab)}>{tab[0].toUpperCase() + tab.slice(1)}</button>
    {/each}
    <span class="hint" aria-hidden="true">R1</span>
  </div>
  <div class="status" class:ok={$runtimeStatus.ready} class:bad={!$runtimeStatus.ready} id="status" title="kwakore service"><span class="dot"></span><span id="status-text">{$runtimeStatus.text}</span></div>
</header>

<main>
  <p id="runtime-unavailable" class="empty" role="alert" hidden={!unavailable}>{unavailable}</p>
  <Catalog />
  <FeatureScreens />
</main>
<div id="toast" class="toast" class:error={$notification?.error} role="status" hidden={!$notification}>{$notification?.text || ""}</div>
