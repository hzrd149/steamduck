import { resourceBytes } from "@napplet/nap/resource/sdk";

export interface ResourceReference { url: string; mime?: string }
const images = new Map<string, Promise<string>>();
const urls = new Set<string>();
const controllers = new Set<AbortController>();
const waiting: Array<() => void> = [];
let active = 0;
let closed = false;

export function resourceURL(reference: ResourceReference | string | null | undefined): string {
  return typeof reference === "string" ? reference : reference?.url || "";
}

// All image bytes come through the shell. Four parallel requests stay below
// RESOURCE's in-flight quota; URLs are document-owned and revoked on teardown.
export function imageURL(reference: ResourceReference | string | null | undefined): Promise<string> {
  const url = resourceURL(reference);
  if (closed || !window.napplet?.resource || !/^(https:|blossom:|nostr:)/.test(url)) return Promise.resolve("");
  const cached = images.get(url);
  if (cached) return cached;
  const task = new Promise<string>((resolve) => {
    const run = () => {
      if (closed) { resolve(""); return; }
      active++;
      const controller = new AbortController();
      controllers.add(controller);
      resourceBytes(url, {signal: controller.signal}).then((blob) => {
        if (closed || !/^image\/(png|jpeg|gif|webp|avif)$/.test(blob.type) || blob.size > 5*1024*1024) return "";
        const objectURL = URL.createObjectURL(blob);
        urls.add(objectURL);
        return objectURL;
      }).catch(() => "").then((result) => {
        if (!result) images.delete(url);
        resolve(result);
      }).finally(() => {
        controllers.delete(controller);
        active--;
        waiting.shift()?.();
      });
    };
    if (active < 4) run(); else waiting.push(run);
  });
  images.set(url, task);
  return task;
}

window.addEventListener("pagehide", () => {
  closed = true;
  for (const controller of controllers) controller.abort();
  for (const url of urls) URL.revokeObjectURL(url);
  for (const run of waiting.splice(0)) run();
  urls.clear();
  images.clear();
}, {once:true});
