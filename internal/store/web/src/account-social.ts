import { identityGetFollows, identityGetRelays, identityGetBadges, identityGetZaps,
  identityGetMutes, identityGetBlocked, identityGetList } from "@napplet/nap/identity/sdk";
import { openUserProfile, profileImage } from "./identity";
import { busy, copyText, toast } from "./store.js";

const $ = <T extends HTMLElement = HTMLElement>(id: string) => document.getElementById(id) as T;
let key = "";
let section = "follows";
let generation = 0;

function node(tag: string, text: string, cls?: string): HTMLElement {
  const element = document.createElement(tag);
  element.textContent = text;
  if (cls) element.className = cls;
  return element;
}

function person(pubkey: string, label?: string): HTMLElement {
  const row = node("div", "", "social-row");
  const name = node("span", label || pubkey, label ? "" : "mono");
  row.append(name);
  if (window.napplet?.intent) {
    const open = node("button", "Open profile", "secondary") as HTMLButtonElement;
    open.addEventListener("click", () => busy(open, () => openUserProfile(pubkey)));
    row.append(open);
  } else {
    const copy = node("button", "Copy ID", "secondary") as HTMLButtonElement;
    copy.addEventListener("click", () => busy(copy, async () => {
      toast(await copyText(pubkey) ? "Account ID copied" : pubkey);
    }));
    row.append(copy);
  }
  return row;
}

export function setSocialIdentity(pubkey: string): void {
  generation++;
  key = pubkey;
  $("social-content").replaceChildren();
  $("social-status").textContent = "";
  $("account-social").hidden = !key || !window.napplet?.identity;
  if (key && window.napplet?.identity) void loadSection();
}

async function loadSection(listType = "bookmarks"): Promise<void> {
  const current = ++generation;
  const content = $("social-content");
  content.replaceChildren();
  $("social-status").textContent = "Loading…";
  for (const button of Array.from($("social-tabs").querySelectorAll("button"))) {
    button.setAttribute("aria-pressed", String(button.dataset.section === section));
  }
  try {
    const rows: HTMLElement[] = [];
    let summary = "";
    switch (section) {
      case "follows": case "mutes": case "blocked": {
        const read = section === "follows" ? identityGetFollows : section === "mutes" ? identityGetMutes : identityGetBlocked;
        const people = [...new Set(await read())];
        rows.push(...people.map((pubkey) => person(pubkey)));
        summary = `${people.length} ${section === "follows" ? "people followed" : section === "mutes" ? "muted accounts" : "blocked accounts"}`;
        break;
      }
      case "relays": {
        const relays = await identityGetRelays();
        for (const [url, permissions] of Object.entries(relays)) {
          const row = node("div", "", "social-row");
          row.append(node("span", url, "mono"), node("span", [permissions.read && "Read", permissions.write && "Write"].filter(Boolean).join(" · ") || "Disabled", "muted small"));
          rows.push(row);
        }
        summary = `${rows.length} relays in your public relay list`;
        break;
      }
      case "badges": {
        const badges = await identityGetBadges();
        for (const badge of badges) {
          const row = node("div", "", "social-badge");
          row.append(node("h3", badge.name || badge.id), node("p", badge.description || ""), person(badge.awardedBy, "Awarded by " + badge.awardedBy));
          const url = badge.thumbs?.[0] || badge.image;
          if (url) void profileImage(url).then((image) => {
            if (current !== generation || !image) return;
            const img = document.createElement("img");
            img.src = image; img.alt = badge.name || "Badge";
            row.prepend(img);
          }).catch(() => {});
          rows.push(row);
        }
        summary = `${badges.length} badges awarded`;
        break;
      }
      case "zaps": {
        const zaps = await identityGetZaps();
        const unique = [...new Map(zaps.map((zap) => [zap.eventId, zap])).values()];
        let millisats = 0;
        for (const zap of unique) {
          const amount = Number.isSafeInteger(zap.amount) && zap.amount > 0 ? zap.amount : 0;
          millisats += amount;
          const row = node("div", "", "social-zap");
          row.append(person(zap.sender, `${(amount / 1000).toLocaleString()} sats · ${zap.sender}`));
          if (zap.content) row.append(node("p", zap.content));
          rows.push(row);
        }
        summary = `${(millisats / 1000).toLocaleString()} sats from ${unique.length} received zaps`;
        break;
      }
      case "lists": {
        const picker = node("div", "", "segmented list-types");
        for (const [type, label] of [["bookmarks", "Bookmarks"], ["interests", "Interests"], ["pins", "Pinned notes"], ["blossom", "Media servers"], ["dm-relays", "DM relays"]]) {
          const button = node("button", label) as HTMLButtonElement;
          button.setAttribute("aria-pressed", String(type === listType));
          button.addEventListener("click", () => { void loadSection(type); });
          picker.append(button);
        }
        rows.push(picker);
        const entries = await identityGetList(listType);
        rows.push(...entries.map((entry) => node("p", entry, "social-entry mono")));
        summary = `${entries.length} public entries · Private list content stays in your signer`;
        break;
      }
    }
    if (current !== generation || !key) return;
    content.replaceChildren(...rows);
    $("social-status").textContent = summary;
    if (!rows.length) content.append(node("p", "Nothing here yet.", "muted"));
  } catch (error) {
    if (current !== generation) return;
    $("social-status").textContent = "Could not load this account information. Try again.";
    const retry = node("button", "Retry", "secondary") as HTMLButtonElement;
    retry.addEventListener("click", () => { void loadSection(listType); });
    content.replaceChildren(retry);
  }
}

export function initSocial(): void {
  for (const button of Array.from($("social-tabs").querySelectorAll("button"))) {
    button.addEventListener("click", () => {
      section = button.dataset.section!;
      void loadSection();
    });
  }
  window.addEventListener("pagehide", () => { generation++; key = ""; }, { once: true });
}
