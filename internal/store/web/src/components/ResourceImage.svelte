<script lang="ts">
  import { imageURL, type ResourceReference } from "../images";

  let { name = "", icon }: {
    name?: string;
    icon?: { resource?: ResourceReference; sha256: string; mime: string };
  } = $props();
  let url = $state("");

  $effect(() => {
    let current = true;
    url = "";
    if (icon) {
      const extension = ({ "image/png": "png", "image/jpeg": "jpg", "image/webp": "webp" } as Record<string, string>)[icon.mime] || "bin";
      void imageURL(icon.resource || { url: `blossom:${icon.sha256}.${extension}`, mime: icon.mime })
        .then(value => { if (current) url = value; });
    }
    return () => { current = false; };
  });
</script>

<span class="icon" class:has-image={!!url} style:background-image={url ? `url("${url}")` : undefined} aria-hidden="true">
  {[...(name.trim() || "?")][0].toUpperCase()}
</span>

<style>
  .icon {
    width: 96px; height: 96px; border-radius: 18px; flex: none;
    display: grid; place-items: center; font-size: 44px; font-weight: 700;
    color: var(--muted); background: var(--panel-2) center / cover no-repeat;
  }
  .has-image { color: transparent; }
</style>
