import { imageURL } from "./images";
import { call } from "./api";
import { loadAccount, modalOpen, closeModal } from "./account.js";
import { loadSettings } from "./settings";
import { startController } from "./controller";

// ---- small DOM helpers ----

export const $ = (sel) => document.querySelector(sel);
let toastTimer;
export function toast(text, isError) {
  const el = $("#toast");
  el.textContent = text;
  el.classList.toggle("error", !!isError);
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (el.hidden = true), isError ? 6000 : 3000);
}

export async function busy(button, fn) {
  button.disabled = true;
  button.classList.add("busy");
  try {
    return await fn();
  } catch (err) {
    toast(err.message, true);
  } finally {
    button.disabled = false;
    button.classList.remove("busy");
  }
}

// Cached icon references are resolved by NAP-RESOURCE, not the custom API.
function loadIcon(el, item) {
  el.textContent = [...(item.name || "?").trim()][0]?.toUpperCase() || "?";
  el.style.backgroundImage = "";
  el.classList.remove("has-image");
  if (!item.icon) return Promise.resolve("");
  const reference = item.icon.resource || {url: "blossom:" + item.icon.sha256 + "." + ({"image/png":"png","image/jpeg":"jpg","image/webp":"webp"}[item.icon.mime] || "bin"), mime: item.icon.mime};
  const key = reference.url;
  el.dataset.resource = key;
  return imageURL(reference).then((url) => {
    if (!url || el.dataset.resource !== key) return "";
    el.style.backgroundImage = `url("${url}")`;
    el.classList.add("has-image");
    return url;
  });
}

function shortNpub(hex) {
  return /^[0-9a-f]{64}$/.test(hex) ? npub(hex).slice(0, 14) + "…" : "";
}

// NIP-19 npub: bech32 of the 32-byte public key.
const BECH32 = "qpzry9x8gf2tvdw0s3jn54khce6mua7l";
function npub(hex) {
  const data = [];
  let acc = 0, bits = 0;
  for (const h of hex.match(/../g)) {
    acc = ((acc << 8) | parseInt(h, 16)) & 0x1fff;
    bits += 8;
    while (bits >= 5) { bits -= 5; data.push((acc >> bits) & 31); }
  }
  if (bits) data.push((acc << (5 - bits)) & 31);
  const GEN = [0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3];
  let chk = 1;
  for (const v of [3, 3, 3, 3, 0, 14, 16, 21, 2, ...data, 0, 0, 0, 0, 0, 0]) { // "npub" expanded
    const top = chk >>> 25;
    chk = ((chk & 0x1ffffff) << 5) ^ v;
    for (let i = 0; i < 5; i++) if ((top >> i) & 1) chk ^= GEN[i];
  }
  chk ^= 1;
  for (let i = 0; i < 6; i++) data.push((chk >>> (5 * (5 - i))) & 31);
  return "npub1" + data.map((d) => BECH32[d]).join("");
}

function sats(msats) {
  return Math.floor(msats / 1000).toLocaleString();
}

function ago(ts) {
  const s = Date.now() / 1000 - ts;
  if (s < 60) return "just now";
  if (s < 3600) return Math.floor(s / 60) + "m ago";
  if (s < 86400) return Math.floor(s / 3600) + "h ago";
  if (s < 7 * 86400) return Math.floor(s / 86400) + "d ago";
  return new Date(ts * 1000).toLocaleDateString();
}

function nameOf(item) {
  return (item.name || "").trim() || "Untitled napplet";
}

function authorOf(item) {
  if (item.author_name) return item.author_name;
  return item.author ? item.author.slice(0, 8) + "…" : "";
}

// ---- state ----

const TABS = ["library", "discover", "account", "settings"];

const state = {
  tab: "library",
  installed: new Map(), // address -> item
  discover: [],
  nextOffset: null,
  query: "",
};

// ---- views ----

function renderCards(grid, items, opts) {
  grid.replaceChildren();
  const tpl = $("#card");
  for (const item of items) {
    const card = tpl.content.firstElementChild.cloneNode(true);
    card.querySelector(".name").textContent = nameOf(item);
    card.querySelector(".sub").textContent = authorOf(item);
    loadIcon(card.querySelector(".icon"), item);
    const badge = card.querySelector(".badge");
    if (opts.library && item.update_available) {
      badge.textContent = "Update";
      badge.hidden = false;
    } else if (!opts.library && state.installed.has(item.address)) {
      badge.textContent = "Installed";
      badge.classList.add("installed");
      badge.hidden = false;
    }
    card.dataset.address = item.address;
    card.addEventListener("click", () => openDetail(item));
    grid.append(card);
  }
}

async function loadInstalledState() {
  const items = [];
  let offset = 0;
  for (;;) {
    const page = await call("installed", {offset, limit:60});
    items.push(...(page.items || []));
    if (page.next_offset == null) break;
    if (!Number.isInteger(page.next_offset) || page.next_offset <= offset) throw new Error("Could not read the installed list.");
    offset = page.next_offset;
  }
  state.installed = new Map(items.map((i) => [i.address, i]));
}

async function loadLibrary() {
  await loadInstalledState();
  const list = [...state.installed.values()].sort((a, b) => (a.name || "").localeCompare(b.name || ""));
  renderCards($("#library-grid"), list, { library: true });
  $("#library-empty").hidden = list.length > 0;
}

async function loadDiscover(append, refresh) {
  if (!append) await loadInstalledState();
  const params = { query: state.query, limit: 60 };
  if (append && state.nextOffset != null) params.offset = state.nextOffset;
  if (refresh) params.refresh = true;
  const page = await call("discover", params);
  page.items = page.items || [];
  state.discover = append ? state.discover.concat(page.items) : page.items;
  state.nextOffset = page.next_offset;
  renderCards($("#discover-grid"), state.discover, { library: false });
  $("#discover-empty").hidden = state.discover.length > 0;
  $("#more").hidden = page.next_offset == null;
  if (!append && !refresh && page.fetched_at == null) {
    // empty cache: fetch from relays once
    toast("Fetching the catalog from relays…");
    return loadDiscover(false, true);
  }
}

function showTab(name) {
  if (pageOpen()) hidePage();
  state.tab = name;
  for (const t of document.querySelectorAll(".tab")) t.setAttribute("aria-selected", String(t.dataset.tab === name));
  for (const v of TABS) $("#" + v).hidden = name !== v;
  const load = name === "library" ? loadLibrary() : name === "discover" ? loadDiscover(false, false) : name === "settings" ? loadSettings() : loadAccount();
  load.catch((err) => toast(err.message, true)).finally(() => focusFirst());
}

// ---- napplet page: a full view like Steam's game page ----

// confirm, when given, asks for a second press first. It runs before the
// button turns busy: a disabled button would swallow that press.
export function button(label, cls, fn, confirm) {
  const b = document.createElement("button");
  b.className = cls;
  b.textContent = label;
  b.addEventListener("click", async () => {
    if (confirm && !(await confirmAction(confirm))) return;
    busy(b, fn);
  });
  return b;
}

const PTABS = ["overview", "comments", "mentions", "zaps", "permissions"];

const page = {
  item: null,
  tab: "overview",
  social: null,
  scroll: 0,
  replyTo: null,
  offsets: {},
};

const pageOpen = () => !$("#napplet").hidden;

async function openDetail(item) {
  Object.assign(page, { item, social: null, scroll: $("main").scrollTop, replyTo: null, offsets: {} });
  for (const v of TABS) $("#" + v).hidden = true;
  $("#napplet").hidden = false;
  $("main").scrollTop = 0;

  $("#d-name").textContent = nameOf(item);
  $("#d-author").textContent = item.author_name ? "by " + item.author_name : "";
  $("#d-desc").textContent = item.description || "";
  $("#d-bg").style.backgroundImage = "";
  loadIcon($("#d-icon"), item).then((url) => {
    if (url && page.item === item) $("#d-bg").style.backgroundImage = `url("${url}")`;
  });
  const tags = $("#d-tags");
  tags.replaceChildren();
  for (const t of item.archetypes || []) {
    const s = document.createElement("span");
    s.textContent = t;
    tags.append(s);
  }
  renderMeta(item);

  for (const id of ["#comment-list", "#mention-list", "#zap-list", "#d-perm-list", "#d-emoji"]) $(id).replaceChildren();
  for (const id of ["#n-comments", "#n-mentions", "#n-zaps"]) $(id).textContent = "";
  $("#comment-text").value = $("#share-text").value = $("#zap-comment").value = "";
  cancelReply();
  hideZapInvoice();
  renderSocial(null);
  renderActions();
  showPageTab("overview");
  focusFirst();
  loadSocial(item);
}

function closeDetail() {
  const address = page.item?.address;
  hidePage();
  $("#" + state.tab).hidden = false;
  $("main").scrollTop = page.scroll;
  const card = [...document.querySelectorAll("#" + state.tab + " .card")].find((c) => c.dataset.address === address);
  if (card) card.focus({ preventScroll: true });
  else focusFirst();
}

function hidePage() {
  $("#napplet").hidden = true;
  page.item = null;
}

function showPageTab(name) {
  page.tab = name;
  for (const t of document.querySelectorAll(".ptab")) t.setAttribute("aria-selected", String(t.dataset.ptab === name));
  for (const t of PTABS) $("#pt-" + t).hidden = t !== name;
  const load = LISTS[name] ? loadList(name) : name === "permissions" ? loadPermissions(page.item) : null;
  load?.catch((err) => toast(err.message, true));
}

function visiblePtabs() {
  return PTABS.filter((t) => !$(`.ptab[data-ptab="${t}"]`).hidden);
}

function renderMeta(item) {
  const meta = $("#d-meta");
  meta.replaceChildren();
  const row = (k, v) => {
    if (!v) return;
    const dt = document.createElement("dt");
    dt.textContent = k;
    const dd = document.createElement("dd");
    dd.textContent = v;
    meta.append(dt, dd);
  };
  row("Developer", item.author_name);
  if (item.version?.created_at) row("Published", new Date(item.version.created_at * 1000).toLocaleDateString());
  $("#d-unsupported").hidden = item.available !== false && !item.missing_domains?.length;
}

// The webview may refuse the page's clipboard, above all for a press from
// the controller, so the store copies it itself when that fails. Failing
// that too, the address shows so it can be typed out.
export async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {}
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.style.position = "fixed";
  ta.style.opacity = "0";
  document.body.append(ta);
  ta.select();
  let ok = false;
  try {
    ok = document.execCommand("copy");
  } catch {}
  ta.remove();
  if (ok) return true;
  try {
    await call("copy", { text });
    return true;
  } catch {
    return false;
  }
}

function renderActions() {
  const item = page.item;
  const installed = state.installed.get(item.address);
  $('.ptab[data-ptab="permissions"]').hidden = !installed;
  if (!installed && page.tab === "permissions") showPageTab("overview");
  const actions = $("#d-actions");
  actions.replaceChildren();
  if (installed) {
    actions.append(button("Play", "primary", async () => {
      await call("launch", { address: item.address });
      toast("Opening " + (item.name || "napplet") + "…");
    }));
    if (installed.update_available) {
      actions.append(button("Update", "secondary", async () => {
        await call("update", { address: item.address });
        toast("Updated");
        await loadLibrary();
        renderActions();
        focusFirst();
      }));
    }
    actions.append(button("Uninstall", "danger", async () => {
      await call("uninstall", { address: item.address });
      toast("Uninstalled");
      await loadLibrary();
      if (state.tab === "discover") renderCards($("#discover-grid"), state.discover, { library: false });
      renderActions();
      focusFirst();
    }, "Uninstall " + (item.name || "this napplet") + "? Its saved data is removed."));
  } else {
    const install = button("Install", "primary", async () => {
      await call("install", { address: item.address });
      toast("Installed " + (item.name || "napplet"));
      await loadLibrary();
      if (state.tab === "discover") renderCards($("#discover-grid"), state.discover, { library: false });
      renderActions();
      focusFirst();
    });
    install.disabled = item.available === false;
    actions.append(install);
    actions.append(button("Try without installing", "secondary", async () => {
      await call("try", { address: item.address });
      toast("Opening a trial…");
    }));
  }
}

// ---- social: votes, reactions, comments, mentions, zaps ----

// Held data shows at once; a relay refresh follows and updates the page.
async function loadSocial(item) {
  $("#d-sync").textContent = "";
  try {
    renderSocial(await call("napplet.social", { address: item.address }));
  } catch {
    return;
  }
  if (page.item !== item) return;
  $("#d-sync").textContent = "Refreshing…";
  const before = page.social;
  try {
    const s = await call("napplet.social", { address: item.address, refresh: true });
    if (page.item !== item) return;
    renderSocial(s);
    $("#d-sync").textContent = "";
    const changed = { comments: s.comments.count !== before.comments.count, mentions: s.mentions.count !== before.mentions.count, zaps: s.zaps.count !== before.zaps.count };
    if (changed[page.tab]) loadList(page.tab).catch(() => {});
  } catch {
    if (page.item === item) $("#d-sync").textContent = "Couldn't refresh";
  }
}

async function reloadSocial() {
  const item = page.item;
  const s = await call("napplet.social", { address: item.address });
  if (page.item === item) renderSocial(s);
}

const REACT_EMOJI = ["❤️", "🔥", "😂", "🎮", "🦆", "🚀"];

function renderSocial(s) {
  page.social = s;
  const own = s?.own;
  for (const b of $("#d-votes").children) {
    b.querySelector(".n").textContent = s ? s.votes[b.dataset.vote] : "–";
    b.setAttribute("aria-pressed", String(own?.vote === b.dataset.vote));
  }
  if (!s) return;
  const count = (n) => (n ? String(n) : "");
  $("#n-comments").textContent = count(s.comments.count);
  $("#n-mentions").textContent = count(s.mentions.count);
  $("#n-zaps").textContent = count(s.zaps.count);
  $("#zap-total").textContent = sats(s.zaps.total_msats);
  $("#zap-count").textContent = s.zaps.count;

  // everyone's emoji, then a picker; pressing one of your own takes it back
  const mine = new Map((own?.reactions || []).map((r) => [r.content, r]));
  const chip = (content, n, url) => {
    const b = document.createElement("button");
    b.className = "chip";
    b.setAttribute("aria-pressed", String(mine.has(content)));
    b.textContent = n ? content + " " + n : content;
    b.addEventListener("click", () => busy(b, () => toggleReaction(content, url)));
    return b;
  };
  $("#d-emoji").replaceChildren(...s.emoji.map((e) => chip(e.content, e.count, e.url)));
  if (!s.emoji.length) {
    const p = document.createElement("span");
    p.className = "muted";
    p.textContent = "No reactions yet.";
    $("#d-emoji").append(p);
  }
  $("#d-react").replaceChildren(...REACT_EMOJI.filter((e) => !s.emoji.some((x) => x.content === e)).map((e) => chip(e)));
}

async function vote(dir) {
  const v = page.social?.own?.vote === dir ? "none" : dir;
  const r = await call("napplet.vote", { address: page.item.address, vote: v });
  if (r.outcome === "published" && r.published === 0) toast("Saved, but no relay took it", true);
  await reloadSocial();
}

async function toggleReaction(content, url) {
  const address = page.item.address;
  const own = (page.social?.own?.reactions || []).find((r) => r.content === content);
  if (own) {
    await call("napplet.delete", { address, event_id: own.id });
  } else {
    const p = { address, emoji: content };
    if (url) p.emoji_url = url;
    published(await call("napplet.react", p), "Reacted " + content);
  }
  await reloadSocial();
}

function published(r, text) {
  if (r.published === 0) toast("Saved, but no relay took it", true);
  else toast(text);
}

// Lists page through kwakore's held events, newest first.
const LISTS = {
  comments: { method: "napplet.comments", el: "#comment-list", empty: "#comment-empty", more: "#comment-more", render: renderComment },
  mentions: { method: "napplet.mentions", el: "#mention-list", empty: "#mention-empty", more: "#mention-more", render: renderMention },
  zaps: { method: "napplet.zaps", el: "#zap-list", more: "#zap-more", render: renderZap },
};

async function loadList(name, append) {
  const l = LISTS[name];
  const item = page.item;
  const params = { address: item.address, limit: 30 };
  if (append) params.offset = page.offsets[name];
  const res = await call(l.method, params);
  if (page.item !== item) return;
  const box = $(l.el);
  if (!append) box.replaceChildren();
  for (const it of res.items || []) box.append(l.render(it));
  page.offsets[name] = res.next_offset;
  $(l.more).hidden = res.next_offset == null;
  if (l.empty) $(l.empty).hidden = box.children.length > 0;
}

function post(author, name, createdAt, content) {
  const el = document.createElement("article");
  el.className = "post";
  const head = document.createElement("div");
  head.className = "post-head";
  const who = document.createElement("b");
  who.textContent = name || shortNpub(author);
  const when = document.createElement("span");
  when.className = "muted small";
  when.textContent = ago(createdAt);
  head.append(who, when);
  const body = document.createElement("p");
  body.className = "post-body";
  body.textContent = content;
  el.append(head, body);
  return el;
}

function renderComment(c) {
  const el = post(c.author, c.author_name, c.created_at, c.content + (c.truncated ? "…" : ""));
  const actions = document.createElement("div");
  actions.className = "post-actions";
  actions.append(button("Reply", "link", async () => startReply(c)));
  const replies = document.createElement("div");
  replies.className = "replies";
  if (c.reply_count) {
    const show = button(c.reply_count === 1 ? "Show 1 reply" : `Show ${c.reply_count} replies`, "link", async () => {
      const res = await call("napplet.comments", { address: page.item.address, parent_id: c.id, limit: 100 });
      replies.replaceChildren(...(res.items || []).map(renderComment));
      show.remove();
      replies.querySelector("button")?.focus();
    });
    actions.append(show);
  }
  if (c.own) {
    actions.append(button("Delete", "link bad", async () => {
      await call("napplet.delete", { address: page.item.address, event_id: c.id });
      const next = el.nextElementSibling || el.previousElementSibling;
      el.remove();
      (next?.querySelector("button") || $("#comment-text")).focus();
      toast("Comment deleted");
      await reloadSocial();
    }, "Delete your comment?"));
  }
  el.append(actions, replies);
  return el;
}

function renderMention(m) {
  const el = post(m.author, m.author_name, m.created_at, m.content + (m.truncated ? "…" : ""));
  if (m.quote) {
    const q = document.createElement("span");
    q.className = "pill";
    q.textContent = "Quote";
    el.querySelector(".post-head").append(q);
  }
  return el;
}

function renderZap(z) {
  const el = post(z.sender, z.sender_name, z.created_at, z.comment);
  const amount = document.createElement("span");
  amount.className = "pill zap";
  amount.textContent = "⚡ " + sats(z.amount_msats);
  el.querySelector(".post-head").append(amount);
  if (!z.comment) el.querySelector(".post-body").remove();
  return el;
}

function startReply(c) {
  page.replyTo = c;
  $("#replying-to").textContent = c.author_name || shortNpub(c.author);
  $("#replying").hidden = false;
  $("#comment-text").focus();
  $("#comment-form").scrollIntoView({ block: "nearest" });
}

function cancelReply() {
  page.replyTo = null;
  $("#replying").hidden = true;
}

async function submitComment() {
  const content = $("#comment-text").value.trim();
  if (!content) throw new Error("Write a comment first");
  const p = { address: page.item.address, content };
  if (page.replyTo) p.parent_id = page.replyTo.id;
  published(await call("napplet.comment", p), page.replyTo ? "Reply posted" : "Comment posted");
  $("#comment-text").value = "";
  cancelReply();
  await loadList("comments");
  await reloadSocial();
}

async function submitShare() {
  const p = { address: page.item.address };
  const content = $("#share-text").value.trim();
  if (content) p.content = content;
  published(await call("napplet.share", p), "Shared");
  $("#share-text").value = "";
  await reloadSocial();
}

// kwakore has no wallet: it asks the author's provider for an invoice, and
// a phone wallet pays it from the QR code.
let zapSats = 100;
function renderZapAmount() {
  for (const b of $("#zap-amounts").children) b.setAttribute("aria-pressed", String(Number(b.dataset.sats) === zapSats));
}

async function requestZap() {
  const p = { address: page.item.address, amount_msats: zapSats * 1000 };
  const comment = $("#zap-comment").value.trim();
  if (comment) p.comment = comment;
  const z = await call("napplet.zap", p);
  $("#zap-qr").src = z.qr;
  $("#zap-amount").textContent = sats(z.amount_msats);
  $("#zap-bolt11").textContent = z.invoice;
  $("#zap-invoice").hidden = false;
  $("#zap-done").focus();
  $("#zap-invoice").scrollIntoView({ block: "nearest" });
}

function hideZapInvoice() {
  if ($("#zap-invoice").hidden) return;
  $("#zap-invoice").hidden = true;
  $("#zap-qr").removeAttribute("src");
  $("#zap-comment").value = "";
  $("#zap-submit").focus();
}

// After paying, ask relays for the new receipt.
async function zapDone() {
  hideZapInvoice();
  const item = page.item;
  const s = await call("napplet.social", { address: item.address, refresh: true, categories: ["zaps"] });
  if (page.item !== item) return;
  renderSocial(s);
  await loadList("zaps");
}

const PERMISSION_LABEL = {
  sign: "Sign events", encrypt: "Encrypt messages", decrypt: "Decrypt messages", publish: "Publish to relays",
  open_link: "Open links", save_file: "Save files", copy_text: "Copy text", upload: "Upload files",
  fetch: "Fetch from the web", notify: "Show notifications", media: "Play media", dispatch: "Run actions",
};

// Saved allow/deny answers for an installed napplet. Forgetting one makes
// the napplet ask again next time.
async function loadPermissions(item) {
  const res = await call("permissions.get", { address: item.address });
  if (page.item !== item) return;
  const list = $("#d-perm-list");
  list.replaceChildren();
  for (const rule of res.saved_rules) {
    const row = document.createElement("div");
    row.className = "perm";
    const what = document.createElement("span");
    what.className = "what";
    what.textContent = (PERMISSION_LABEL[rule.permission] || rule.permission) + (rule.subject ? " · " + rule.subject : "");
    const decision = document.createElement("span");
    decision.className = rule.decision;
    decision.textContent = rule.decision === "allow" ? "Allowed" : "Denied";
    const forget = button("Forget", "secondary", async () => {
      await call("permissions.clear", { address: item.address, ...rule });
      const next = (row.nextElementSibling || row.previousElementSibling)?.querySelector("button");
      row.remove();
      $("#perm-empty").hidden = list.children.length > 0;
      if (next) next.focus();
      else $('.ptab[data-ptab="permissions"]').focus();
      toast("It will ask again next time");
    });
    row.append(what, decision, forget);
    list.append(row);
  }
  $("#perm-empty").hidden = res.saved_rules.length > 0;
}

// A second press on the same button confirms; anything else cancels.
export function confirmAction(text) {
  toast(text + " Press again to confirm.");
  const target = document.activeElement;
  return new Promise((resolve) => {
    const done = (ok) => {
      document.removeEventListener("click", onClick, true);
      document.removeEventListener("keydown", onKey, true);
      resolve(ok);
    };
    const onClick = (e) => { e.stopPropagation(); e.preventDefault(); done(e.target === target); };
    const onKey = (e) => {
      if (e.key === "Enter" || e.key === " ") { e.preventDefault(); e.stopPropagation(); done(true); }
      else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); done(false); }
    };
    setTimeout(() => {
      document.addEventListener("click", onClick, true);
      document.addEventListener("keydown", onKey, true);
    });
  });
}

// ---- controller and keyboard navigation ----

// An open account modal, or the napplet page, keeps focus inside it, away
// from the header tabs.
function scope() {
  if (modalOpen()) return $("#account-modal");
  return pageOpen() ? $("#napplet") : document.body;
}

function focusables(root) {
  return [...root.querySelectorAll("button, input, textarea")].filter((el) => !el.disabled && el.offsetParent !== null);
}

export function focusFirst(root) {
  const r = root || scope();
  const list = focusables(r);
  const first = list.find((el) => el.classList.contains("card")) ||
    list.find((el) => $("#d-actions").contains(el)) ||
    list.find((el) => !el.classList.contains("tab") && el.id !== "d-back") || list[0];
  first?.focus();
}

// Move focus to the nearest element in a direction.
function move(dx, dy) {
  const list = focusables(scope());
  const cur = document.activeElement;
  if (!list.includes(cur)) return focusFirst();
  const a = cur.getBoundingClientRect();
  const ax = a.left + a.width / 2, ay = a.top + a.height / 2;
  let best = null, bestScore = Infinity;
  for (const el of list) {
    if (el === cur) continue;
    const b = el.getBoundingClientRect();
    const bx = b.left + b.width / 2, by = b.top + b.height / 2;
    const ddx = bx - ax, ddy = by - ay;
    const along = dx ? ddx * dx : ddy * dy;
    if (along <= 1) continue;
    const across = dx ? Math.abs(ddy) : Math.abs(ddx);
    const score = along + across * 3;
    if (score < bestScore) { bestScore = score; best = el; }
  }
  if (best) {
    best.focus();
    best.scrollIntoView({ block: "nearest" });
  }
}

function back() {
  if (modalOpen()) return closeModal();
  if (pageOpen()) {
    const el = document.activeElement;
    if (!$("#zap-invoice").hidden) return hideZapInvoice();
    if (page.replyTo) return cancelReply();
    if (el && ["INPUT", "TEXTAREA"].includes(el.tagName)) return $(`.ptab[data-ptab="${page.tab}"]`).focus();
    return closeDetail();
  }
  if (document.activeElement === $("#search")) return focusFirst();
}

function cycleTab(step) {
  if (modalOpen()) return;
  if (pageOpen()) {
    const tabs = visiblePtabs();
    const next = tabs[(tabs.indexOf(page.tab) + step + tabs.length) % tabs.length];
    showPageTab(next);
    $(`.ptab[data-ptab="${next}"]`).focus();
    return;
  }
  showTab(TABS[(TABS.indexOf(state.tab) + step + TABS.length) % TABS.length]);
}

document.addEventListener("keydown", (e) => {
  const typing = ["INPUT", "TEXTAREA"].includes(e.target.tagName);
  const multiline = e.target.tagName === "TEXTAREA";
  switch (e.key) {
    case "ArrowLeft": if (typing) return; move(-1, 0); break;
    case "ArrowRight": if (typing) return; move(1, 0); break;
    case "ArrowUp": if (multiline) return; move(0, -1); break;
    case "ArrowDown": if (multiline) return; move(0, 1); break;
    case "Escape": case "Backspace": if (typing && e.key === "Backspace") return; back(); break;
    case "PageUp": cycleTab(-1); break;
    case "PageDown": cycleTab(1); break;
    default: return;
  }
  e.preventDefault();
});

// ---- wiring ----

for (const t of document.querySelectorAll(".tab")) t.addEventListener("click", () => showTab(t.dataset.tab));

let searchTimer;
$("#search").addEventListener("input", (e) => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    state.query = e.target.value.trim();
    loadDiscover(false, false).catch((err) => toast(err.message, true));
  }, 250);
});
$("#refresh").addEventListener("click", (e) => busy(e.currentTarget, async () => {
  await loadDiscover(false, true);
  toast("Catalog refreshed");
}));
$("#more").addEventListener("click", (e) => busy(e.currentTarget, () => loadDiscover(true, false)));
$("#settings-account").addEventListener("click", () => showTab("account"));
$("#sync").addEventListener("click", (e) => busy(e.currentTarget, async () => {
  const r = await call("sync");
  toast(r?.deferred
    ? "Steam is running: shortcuts will be written when it exits (or use the Decky plugin)."
    : "Steam shortcuts updated (" + (r?.shortcut_count ?? 0) + ")");
}));

$("#d-back").addEventListener("click", closeDetail);
$("#d-copy").addEventListener("click", async () => {
  const address = page.item.address;
  if (await copyText(address)) toast("Address copied");
  else toast("Couldn't copy. The address is " + address, true);
});
for (const t of document.querySelectorAll(".ptab")) t.addEventListener("click", () => showPageTab(t.dataset.ptab));
for (const b of $("#d-votes").children) b.addEventListener("click", () => busy(b, () => vote(b.dataset.vote)));
for (const name of Object.keys(LISTS)) {
  $(LISTS[name].more).addEventListener("click", (e) => busy(e.currentTarget, () => loadList(name, true)));
}
$("#comment-form").addEventListener("submit", (e) => {
  e.preventDefault();
  busy($("#comment-submit"), submitComment);
});
$("#reply-cancel").addEventListener("click", () => { cancelReply(); $("#comment-text").focus(); });
$("#share-form").addEventListener("submit", (e) => {
  e.preventDefault();
  busy($("#share-submit"), submitShare);
});
for (const b of $("#zap-amounts").children) {
  b.addEventListener("click", () => { zapSats = Number(b.dataset.sats); renderZapAmount(); });
}
renderZapAmount();
$("#zap-submit").addEventListener("click", (e) => busy(e.currentTarget, requestZap));
$("#zap-done").addEventListener("click", (e) => busy(e.currentTarget, zapDone));

async function refreshStatus() {
  const el = $("#status");
  try {
    const s = await call("status");
    el.className = "status " + (s.health.ready ? "ok" : "bad");
    $("#status-text").textContent = "kwakore " + s.health.version;
  } catch (err) {
    el.className = "status bad";
    $("#status-text").textContent = "kwakore unavailable";
  }
}

export function startStore() {
  refreshStatus();
  const statusTimer = setInterval(refreshStatus, 15000);
  const stopController = startController({
    left: () => move(-1, 0), right: () => move(1, 0),
    up: () => move(0, -1), down: () => move(0, 1),
    a: () => document.activeElement?.click(), b: back,
    l1: () => cycleTab(-1), r1: () => cycleTab(1),
  });
  window.addEventListener("pagehide", () => {
    clearInterval(statusTimer);
    stopController();
  }, { once: true });
  const startTab = new URLSearchParams(location.hash.slice(1)).get("tab");
  showTab(TABS.includes(startTab) ? startTab : "library");
}
