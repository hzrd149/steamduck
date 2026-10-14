<script lang="ts">
  import { catalog, navigation } from "../ui";
  import { busy, openDetail, refreshCatalog, searchCatalog, loadMore, syncShortcuts } from "../store.js";
  import NappletCard from "../components/NappletCard.svelte";

  let library = $derived([...$catalog.installed.values()].sort((a, b) => (a.name || "").localeCompare(b.name || "")));
</script>

<section id="library" class="view" hidden={$navigation.detail || $navigation.tab !== "library"}>
  <div class="toolbar">
    <h1>Installed</h1>
    <button id="sync" class="secondary" onclick={e => busy(e.currentTarget, syncShortcuts)}>Sync Steam shortcuts</button>
  </div>
  <div class="grid" id="library-grid">
    {#each library as item (item.address)}
      <NappletCard {item} badge={item.update_available ? "Update" : ""} onclick={() => openDetail(item)} />
    {/each}
  </div>
  <p class="empty" id="library-empty" hidden={library.length > 0}>No napplets installed yet. Find some in Discover.</p>
</section>

<section id="discover" class="view" hidden={$navigation.detail || $navigation.tab !== "discover"}>
  <div class="toolbar">
    <input id="search" type="search" placeholder="Search napplets" autocomplete="off" spellcheck="false" oninput={e => searchCatalog(e.currentTarget.value)}>
    <button id="refresh" class="secondary" onclick={e => busy(e.currentTarget, refreshCatalog)}>Refresh from relays</button>
  </div>
  <div class="grid" id="discover-grid">
    {#each $catalog.discover as item (item.address)}
      <NappletCard {item} badge={$catalog.installed.has(item.address) ? "Installed" : ""} installed onclick={() => openDetail(item)} />
    {/each}
  </div>
  <p class="empty" id="discover-empty" hidden={$catalog.discover.length > 0}>Nothing found. Try refreshing from relays.</p>
  <button id="more" class="secondary more" hidden={$catalog.nextOffset == null} onclick={e => busy(e.currentTarget, loadMore)}>Load more</button>
</section>

<style>
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(190px, 1fr)); gap: 16px; }
  #search { flex: 1; background: var(--panel); border: 2px solid var(--panel-2); border-radius: var(--radius); padding: 10px 14px; }
</style>
