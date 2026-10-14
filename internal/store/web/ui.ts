import { writable } from "svelte/store";

// Shared presentation state only. Runtime operations remain behind api.ts.
export interface CatalogItem {
  address: string;
  name?: string;
  author?: string;
  author_name?: string;
  update_available?: unknown;
  icon?: { resource?: { url: string; mime?: string }; sha256: string; mime: string };
}

export const navigation = writable({ tab: "library", detail: false });
export const catalog = writable<{
  installed: Map<string, CatalogItem>; discover: CatalogItem[]; nextOffset: number | null;
}>({ installed: new Map(), discover: [], nextOffset: null });
export const runtimeStatus = writable({ text: "Connecting…", ready: false });
export const notification = writable<{ text: string; error: boolean } | null>(null);
