"use strict";

// ---- transport: the webview binding, or the development HTTP server ----

const token = new URLSearchParams(location.hash.slice(1)).get("token");

async function call(method, params) {
  let reply;
  if (typeof window.steamduckCall === "function") {
    reply = await window.steamduckCall(method, params ?? null);
  } else {
    const res = await fetch("/api/" + method, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Steamduck-Token": token ?? "" },
      body: JSON.stringify(params ?? null),
    });
    reply = await res.json();
  }
  if (reply.error) {
    const err = new Error(describeError(reply.error));
    err.code = reply.error.code;
    throw err;
  }
  return reply.result;
}

function describeError(e) {
  if (e.reason === "session_unavailable") return "kwakore has no display to open windows on. Run `steamduck doctor`.";
  if (e.reason === "signer_unavailable") return "This needs a Nostr signer. Set one up in Account.";
  switch (e.code) {
    case 1003: return "kwakore is busy with this napplet. Try again in a moment.";
    case 1004: return "Unavailable: the relays or the kwakore service did not answer.";
    case 1005: return "Already up to date.";
    case 1008: return "Timed out. Check the Library to see whether it finished.";
  }
  return e.message || "Something went wrong";
}

// ---- small DOM helpers ----

const $ = (sel) => document.querySelector(sel);
let toastTimer;
function toast(text, isError) {
  const el = $("#toast");
  el.textContent = text;
  el.classList.toggle("error", !!isError);
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (el.hidden = true), isError ? 6000 : 3000);
}

async function busy(button, fn) {
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

// Icons load lazily and are cached for the session. Until one arrives, or
// when a napplet has none, the tile shows the name's first letter.
const iconCache = new Map();
function loadIcon(el, item) {
  el.textContent = [...(item.name || "?").trim()][0]?.toUpperCase() || "?";
  el.style.backgroundImage = "";
  el.classList.remove("has-image");
  if (!item.icon) return;
  let p = iconCache.get(item.address);
  if (!p) {
    p = call("icon", { address: item.address }).catch(() => "");
    iconCache.set(item.address, p);
  }
  p.then((url) => {
    // data: URLs from the daemon's verified PNGs only
    if (url && url.startsWith("data:image/png;base64,")) {
      el.style.backgroundImage = `url("${url}")`;
      el.classList.add("has-image");
    }
  });
}

function nameOf(item) {
  return (item.name || "").trim() || "Untitled napplet";
}

function authorOf(item) {
  if (item.author_name) return item.author_name;
  return item.author ? item.author.slice(0, 8) + "…" : "";
}

// ---- state ----

const TABS = ["library", "discover", "account"];

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
    card.addEventListener("click", () => openDetail(item));
    grid.append(card);
  }
}

async function loadInstalledState() {
  const items = (await call("installed")) || [];
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
  state.tab = name;
  for (const t of document.querySelectorAll(".tab")) t.setAttribute("aria-selected", String(t.dataset.tab === name));
  for (const v of TABS) $("#" + v).hidden = name !== v;
  const load = name === "library" ? loadLibrary() : name === "discover" ? loadDiscover(false, false) : loadAccount();
  load.catch((err) => toast(err.message, true)).finally(() => focusFirst());
}

// ---- detail dialog ----

let detailItem = null;
let detailReturnFocus = null;

// confirm, when given, asks for a second press first. It runs before the
// button turns busy: a disabled button would swallow that press.
function button(label, cls, fn, confirm) {
  const b = document.createElement("button");
  b.className = cls;
  b.textContent = label;
  b.addEventListener("click", async () => {
    if (confirm && !(await confirmAction(confirm))) return;
    busy(b, fn);
  });
  return b;
}

async function openDetail(item) {
  detailItem = item;
  detailReturnFocus = document.activeElement;
  $("#d-name").textContent = nameOf(item);
  $("#d-author").textContent = item.author_name ? "by " + item.author_name : (item.author ? "by " + item.author : "");
  $("#d-desc").textContent = item.description || "";
  loadIcon($("#d-icon"), item);

  const tags = $("#d-tags");
  tags.replaceChildren();
  for (const t of item.archetypes || []) {
    const s = document.createElement("span");
    s.textContent = t;
    tags.append(s);
  }
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
  if (item.version?.created_at) row("Published", new Date(item.version.created_at * 1000).toLocaleDateString());
  row("Needs", (item.required_domains || []).join(", "));
  if (item.missing_domains?.length) row("Unsupported", item.missing_domains.join(", "));
  row("Address", item.address);

  renderActions();
  $("#d-perms").hidden = true;
  if (state.installed.has(item.address)) loadPermissions(item);
  $("#detail").hidden = false;
  focusFirst($("#detail"));
}

const PERMISSION_LABEL = {
  sign: "Sign events", encrypt: "Encrypt messages", decrypt: "Decrypt messages", publish: "Publish to relays",
  open_link: "Open links", save_file: "Save files", copy_text: "Copy text", upload: "Upload files",
  fetch: "Fetch from the web", notify: "Show notifications", media: "Play media", dispatch: "Run actions",
};

// Saved allow/deny answers for an installed napplet. Forgetting one makes
// the napplet ask again next time.
async function loadPermissions(item) {
  let res;
  try {
    res = await call("permissions.get", { address: item.address });
  } catch {
    return;
  }
  if (detailItem !== item) return;
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
      $("#d-perms").hidden = list.children.length === 0;
      if (next) next.focus();
      else focusFirst($("#detail"));
      toast("It will ask again next time");
    });
    row.append(what, decision, forget);
    list.append(row);
  }
  $("#d-perms").hidden = res.saved_rules.length === 0;
}

function renderActions() {
  const item = detailItem;
  const installed = state.installed.get(item.address);
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
      }));
    }
    actions.append(button("Uninstall", "danger", async () => {
      await call("uninstall", { address: item.address });
      toast("Uninstalled");
      await loadLibrary();
      if (state.tab === "discover") renderCards($("#discover-grid"), state.discover, { library: false });
      closeDetail();
    }, "Uninstall " + (item.name || "this napplet") + "? Its saved data is removed."));
  } else {
    const install = button("Install", "primary", async () => {
      await call("install", { address: item.address });
      toast("Installed " + (item.name || "napplet"));
      await loadLibrary();
      if (state.tab === "discover") renderCards($("#discover-grid"), state.discover, { library: false });
      renderActions();
      focusFirst($("#detail"));
    });
    install.disabled = item.available === false;
    actions.append(install);
    actions.append(button("Try without installing", "secondary", async () => {
      await call("try", { address: item.address });
      toast("Opening a trial…");
    }));
  }
}

// A second press on the same button confirms; anything else cancels.
function confirmAction(text) {
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

function closeDetail() {
  $("#detail").hidden = true;
  detailItem = null;
  if (detailReturnFocus && document.contains(detailReturnFocus)) detailReturnFocus.focus();
  else focusFirst();
}

// ---- account: the Nostr signer and settings ----

const MODE_LABEL = { none: "Not signed in", nsec: "Private key", bunker: "Remote signer", system: "System signer" };

async function loadAccount() {
  const [signer, settings] = await Promise.all([call("signer.status"), call("settings.get")]);
  renderSigner(signer);
  renderTheme(settings.theme);
}

function renderSigner(s) {
  const connected = s.connection_state === "connected";
  $("#signer-dot").className = "dot " + (s.mode === "none" ? "" : connected ? "ok" : "bad");
  $("#signer-mode").textContent = MODE_LABEL[s.mode] + (s.mode !== "none" && !connected ? " (not connected)" : "");
  $("#signer-npub").textContent = s.npub || (s.mode === "none" ? "Napplets that need a key will not be able to sign in." : "");

  const actions = $("#signer-actions");
  actions.replaceChildren();
  const signedIn = s.mode !== "none";
  actions.append(
    button("Connect a signer app", signedIn ? "secondary" : "primary", startPair),
    button("Paste a bunker link", "secondary", () => openSecretForm("bunker")),
    button("Use a private key", "secondary", () => openSecretForm("nsec")),
    button("Use the system signer", "secondary", switchSystem),
  );
  if (signedIn) {
    actions.append(button("Sign out", "danger", async () => {
      renderSigner(await call("signer.switch", { mode: "none" }));
      toast("Signed out");
      focusFirst();
    }, "Sign out? Open napplets close, and a saved private key or bunker link is forgotten."));
  }
}

// nostrconnect: show a QR code for the signer app, then wait for its answer.
let pairing = false;
async function startPair() {
  const offer = await call("signer.pair.start");
  pairing = true;
  closeSecretForm();
  $("#pair-qr").src = offer.qr;
  $("#pair-relay").textContent = offer.relay;
  $("#pair").hidden = false;
  $("#pair-cancel").focus();
  $("#pair").scrollIntoView({ block: "nearest" });
  try {
    const s = await call("signer.pair.wait");
    if (!pairing) return;
    renderSigner(s);
    toast("Connected to your signer");
  } catch (err) {
    if (pairing) toast(err.code === 1008 ? "The pairing offer expired. Try again." : err.message, true);
  } finally {
    if (pairing) hidePair();
  }
}

function hidePair() {
  pairing = false;
  $("#pair").hidden = true;
  $("#pair-qr").removeAttribute("src");
  focusFirst();
}

function cancelPair() {
  hidePair();
  call("signer.pair.cancel").catch(() => {});
}

const SECRET_FORMS = {
  bunker: {
    label: "Bunker link",
    placeholder: "bunker://…",
    help: "From your remote signer (nsec.app, Amber, nsecBunker). Press Steam + X for the keyboard.",
    valid: (v) => v.startsWith("bunker://"),
    invalid: "A bunker link starts with bunker://",
  },
  nsec: {
    label: "Private key",
    placeholder: "nsec1…",
    help: "Your key is stored by kwakore in your home directory, readable only by you. A signer app is safer.",
    valid: (v) => v.startsWith("nsec1"),
    invalid: "A private key starts with nsec1",
  },
};
let secretMode = null;

function openSecretForm(mode) {
  if (pairing) cancelPair();
  const f = SECRET_FORMS[mode];
  secretMode = mode;
  $("#secret-label").textContent = f.label;
  $("#secret-help").textContent = f.help;
  const input = $("#secret-input");
  input.type = mode === "nsec" ? "password" : "text";
  input.placeholder = f.placeholder;
  input.value = "";
  $("#secret-form").hidden = false;
  input.focus();
}

function closeSecretForm() {
  if ($("#secret-form").hidden) return;
  secretMode = null;
  $("#secret-input").value = "";
  $("#secret-form").hidden = true;
  focusFirst();
}

async function submitSecret() {
  const f = SECRET_FORMS[secretMode];
  const secret = $("#secret-input").value.trim();
  if (!f.valid(secret)) throw new Error(f.invalid);
  let s;
  try {
    s = await call("signer.switch", { mode: secretMode, secret });
  } catch (err) {
    if (err.code === 1004) throw new Error("Could not connect with that " + f.label.toLowerCase() + ".");
    throw err;
  }
  closeSecretForm();
  renderSigner(s);
  toast(s.connection_state === "connected" ? "Signed in" : "Saved, but not connected yet");
}

async function switchSystem() {
  const s = await call("signer.switch", { mode: "system" });
  renderSigner(s);
  toast(s.connection_state === "connected" ? "Using the system signer" : "No system signer is answering; kwakore keeps trying.");
}

function renderTheme(theme) {
  for (const b of $("#theme").children) b.setAttribute("aria-pressed", String(b.dataset.value === theme));
}

// ---- controller and keyboard navigation ----

function scope() {
  return $("#detail").hidden ? document.body : $("#detail");
}

function focusables(root) {
  return [...root.querySelectorAll("button, input")].filter(
    (el) => !el.disabled && el.offsetParent !== null && !(root === document.body && $("#detail").contains(el)),
  );
}

function focusFirst(root) {
  const r = root || scope();
  const list = focusables(r);
  const first = list.find((el) => el.classList.contains("card")) || list.find((el) => !el.classList.contains("tab")) || list[0];
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
  if (!$("#detail").hidden) return closeDetail();
  if (!$("#pair").hidden) return cancelPair();
  if (!$("#secret-form").hidden) return closeSecretForm();
  if (document.activeElement === $("#search")) return focusFirst();
}

function cycleTab(step) {
  if (!$("#detail").hidden) return;
  showTab(TABS[(TABS.indexOf(state.tab) + step + TABS.length) % TABS.length]);
}

document.addEventListener("keydown", (e) => {
  const typing = e.target.tagName === "INPUT";
  switch (e.key) {
    case "ArrowLeft": if (typing) return; move(-1, 0); break;
    case "ArrowRight": if (typing) return; move(1, 0); break;
    case "ArrowUp": move(0, -1); break;
    case "ArrowDown": move(0, 1); break;
    case "Escape": case "Backspace": if (typing && e.key === "Backspace") return; back(); break;
    case "PageUp": cycleTab(-1); break;
    case "PageDown": cycleTab(1); break;
    default: return;
  }
  e.preventDefault();
});

// Standard-mapping gamepad: d-pad or left stick moves, A activates, B goes
// back, L1/R1 switch tabs. Steam Input presents the Deck's controls as one.
const pressed = new Set();
let repeatAt = 0;
function pollPads(t) {
  const pads = navigator.getGamepads ? [...navigator.getGamepads()].filter(Boolean) : [];
  const now = new Set();
  for (const p of pads) {
    const b = (i) => p.buttons[i]?.pressed;
    const ax = p.axes[0] || 0, ay = p.axes[1] || 0;
    if (b(14) || ax < -0.6) now.add("left");
    if (b(15) || ax > 0.6) now.add("right");
    if (b(12) || ay < -0.6) now.add("up");
    if (b(13) || ay > 0.6) now.add("down");
    if (b(0)) now.add("a");
    if (b(1)) now.add("b");
    if (b(4)) now.add("l1");
    if (b(5)) now.add("r1");
  }
  for (const k of now) {
    const fresh = !pressed.has(k);
    const repeat = ["left", "right", "up", "down"].includes(k) && t > repeatAt;
    if (!fresh && !repeat) continue;
    if (["left", "right", "up", "down"].includes(k)) repeatAt = t + (fresh ? 380 : 120);
    switch (k) {
      case "left": move(-1, 0); break;
      case "right": move(1, 0); break;
      case "up": move(0, -1); break;
      case "down": move(0, 1); break;
      case "a": if (fresh) document.activeElement?.click(); break;
      case "b": if (fresh) back(); break;
      case "l1": if (fresh) cycleTab(-1); break;
      case "r1": if (fresh) cycleTab(1); break;
    }
  }
  pressed.clear();
  for (const k of now) pressed.add(k);
  requestAnimationFrame(pollPads);
}
requestAnimationFrame(pollPads);

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
$("#sync").addEventListener("click", (e) => busy(e.currentTarget, async () => {
  const r = await call("sync");
  toast(r?.deferred
    ? "Steam is running: shortcuts will be written when it exits (or use the Decky plugin)."
    : "Steam shortcuts updated (" + (r?.shortcut_count ?? 0) + ")");
}));

$("#pair-cancel").addEventListener("click", cancelPair);
$("#secret-cancel").addEventListener("click", closeSecretForm);
$("#secret-form").addEventListener("submit", (e) => {
  e.preventDefault();
  busy($("#secret-submit"), submitSecret);
});
for (const b of $("#theme").children) {
  b.addEventListener("click", () => busy(b, async () => {
    await call("settings.set", { field: "theme", value: b.dataset.value });
    renderTheme(b.dataset.value);
  }));
}

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

refreshStatus();
setInterval(refreshStatus, 15000);
const startTab = new URLSearchParams(location.hash.slice(1)).get("tab");
showTab(TABS.includes(startTab) ? startTab : "library");
