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
  if (e.reason === "signer_unavailable") return "This needs a Nostr signer. Set one up with `kwak signer`.";
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
  const items = await call("installed");
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
  $("#library").hidden = name !== "library";
  $("#discover").hidden = name !== "discover";
  const load = name === "library" ? loadLibrary() : loadDiscover(false, false);
  load.catch((err) => toast(err.message, true)).finally(() => focusFirst());
}

// ---- detail dialog ----

let detailItem = null;
let detailReturnFocus = null;

function button(label, cls, fn) {
  const b = document.createElement("button");
  b.className = cls;
  b.textContent = label;
  b.addEventListener("click", () => busy(b, fn));
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
  $("#detail").hidden = false;
  focusFirst($("#detail"));
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
      if (!(await confirmAction("Uninstall " + (item.name || "this napplet") + "? Its saved data is removed."))) return;
      await call("uninstall", { address: item.address });
      toast("Uninstalled");
      await loadLibrary();
      if (state.tab === "discover") renderCards($("#discover-grid"), state.discover, { library: false });
      closeDetail();
    }));
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
  if (document.activeElement === $("#search")) return focusFirst();
}

function cycleTab(step) {
  if (!$("#detail").hidden) return;
  const tabs = ["library", "discover"];
  showTab(tabs[(tabs.indexOf(state.tab) + step + tabs.length) % tabs.length]);
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
showTab(new URLSearchParams(location.hash.slice(1)).get("tab") === "discover" ? "discover" : "library");
