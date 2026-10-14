<script lang="ts">
  import ResourceImage from "./ResourceImage.svelte";
  import type { CatalogItem } from "../ui";

  let { item, badge = "", installed = false, onclick }: {
    item: CatalogItem; badge?: string; installed?: boolean; onclick: () => void;
  } = $props();
</script>

<button class="card" data-address={item.address} {onclick}>
  <ResourceImage name={item.name} icon={item.icon} />
  <span class="name">{item.name?.trim() || "Untitled napplet"}</span>
  <span class="sub muted">{item.author_name || (item.author ? item.author.slice(0, 8) + "…" : "")}</span>
  {#if badge}<span class="badge" class:installed>{badge}</span>{/if}
</button>

<style>
  .card {
    position: relative; display: flex; flex-direction: column; align-items: center; gap: 6px;
    background: var(--panel); border: 0; border-radius: var(--radius); padding: 18px 12px 14px;
    text-align: center; min-height: 210px; transition: transform .08s ease, background .08s ease;
  }
  .card:hover, .card:focus-visible { background: var(--panel-2); transform: scale(1.03); }
  .name { font-weight: 700; max-width: 100%; overflow-wrap: anywhere; overflow: hidden; display: -webkit-box; line-clamp: 2; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
  .sub { font-size: 14px; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .badge { position: absolute; top: 10px; right: 10px; background: var(--accent); color: var(--accent-ink); border-radius: 999px; padding: 2px 10px; font-size: 12px; font-weight: 700; }
  .installed { background: var(--panel-2); color: var(--ok); }
</style>
