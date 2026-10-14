"use strict";

// ---- account: the player's social account ----
//
// Signed out, the tab offers the ways to sign in, each in its own modal.
// Signed in, it shows the account's public profile and a sign out button.

async function loadAccount() {
  const [signer, settings] = await Promise.all([call("signer.status"), call("settings.get")]);
  renderTheme(settings.theme);
  await renderAccount(signer);
}

// Which of the three account views is showing, and the key it is for.
let accountKey = null;

async function renderAccount(s) {
  const show = (id) => {
    for (const v of ["acct-loading", "acct-profile", "acct-offline", "acct-signin"]) $("#" + v).hidden = v !== id;
  };
  if (s.mode === "none") {
    accountKey = null;
    show("acct-signin");
    return;
  }
  if (s.connection_state !== "connected" || !s.public_key) {
    accountKey = null;
    show("acct-offline");
    return;
  }
  // a profile already on screen stays while it refreshes
  if (accountKey !== s.public_key) {
    renderProfile({ public_key: s.public_key, npub: s.npub, found: false, loading: true });
    show("acct-profile");
  }
  accountKey = s.public_key;
  let p;
  try {
    p = await call("profile.get");
  } catch (err) {
    toast(err.message, true);
    return;
  }
  if (p && p.public_key === accountKey) renderProfile(p);
}

function shortId(npub) {
  return npub ? npub.slice(0, 12) + "…" + npub.slice(-6) : "";
}

function setImage(el, url) {
  const ok = url && /^data:image\/(png|jpeg|gif|webp);base64,/.test(url);
  el.style.backgroundImage = ok ? `url("${url}")` : "";
  el.classList.toggle("has-image", !!ok);
  return ok;
}

function renderProfile(p) {
  const name = p.display_name || p.name || (p.loading ? "" : "Your account");
  $("#p-name").textContent = name;
  $("#p-name").classList.toggle("skeleton", !!p.loading);

  const handle = p.nip05 ? p.nip05.replace(/^_@/, "") : p.name && p.name !== name ? "@" + p.name : shortId(p.npub);
  $("#p-handle").textContent = handle;

  const avatar = $("#p-avatar");
  if (!setImage(avatar, p.picture)) avatar.textContent = [...(name || "?").trim()][0]?.toUpperCase() || "?";
  else avatar.textContent = "";
  setImage($("#p-banner"), p.banner);

  $("#p-about").textContent = p.about || "";
  $("#p-about").hidden = !p.about;

  const facts = $("#p-facts");
  facts.replaceChildren();
  const fact = (k, v, cls) => {
    if (!v) return;
    const dt = document.createElement("dt");
    dt.textContent = k;
    const dd = document.createElement("dd");
    dd.textContent = v;
    if (cls) dd.className = cls;
    facts.append(dt, dd);
  };
  fact("Website", p.website?.replace(/^https?:\/\//, "").replace(/\/$/, ""));
  fact("Tips", p.lud16);
  fact("Account ID", p.npub, "mono");

  const note = $("#p-note");
  note.hidden = p.loading || p.found;
  note.textContent = "This account has no public profile yet. Add a name and picture in your signer or any app that uses this account, and it shows up here.";
}

async function signOut() {
  const s = await call("signer.switch", { mode: "none" });
  toast("Signed out");
  await renderAccount(s);
  focusFirst();
}

// ---- modal ----

let modalClose = null;
let modalReturnFocus = null;

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  return e;
}

// openModal shows one sign-in flow. onClose runs when it closes for any
// reason, so a flow can cancel what it started.
function openModal(title, nodes, onClose) {
  if (modalClose) closeModal();
  modalReturnFocus = document.activeElement;
  modalClose = onClose || (() => {});
  $("#m-title").textContent = title;
  $("#m-body").replaceChildren(...nodes);
  $("#account-modal").hidden = false;
  focusFirst($("#account-modal"));
}

function closeModal() {
  if (!modalClose) return;
  const onClose = modalClose;
  modalClose = null;
  $("#account-modal").hidden = true;
  $("#m-body").replaceChildren();
  onClose();
  if (modalReturnFocus && document.contains(modalReturnFocus) && modalReturnFocus.offsetParent) modalReturnFocus.focus();
  else focusFirst();
}

function modalOpen() {
  return !$("#account-modal").hidden;
}

// signedIn closes the flow and shows the new account.
async function signedIn(s) {
  closeModal();
  toast("Signed in");
  await renderAccount(s);
  focusFirst();
}

function errorLine() {
  const p = el("p", "m-error");
  p.hidden = true;
  p.setAttribute("role", "alert");
  return p;
}

function showError(line, text) {
  line.textContent = text;
  line.hidden = !text;
}

// ---- sign-in flows ----

const FLOWS = {
  // Scan a pairing code with a signer app on the phone, then wait for it.
  phone() {
    let active = true;
    const qr = el("img", "qr");
    qr.alt = "Sign-in code";
    const status = el("p", "muted small", "Getting a code…");
    const error = errorLine();
    const retry = el("button", "secondary", "Show a new code");
    retry.hidden = true;
    const steps = el("ol", "steps");
    for (const t of [
      "Open your signer app, such as Amber, Primal or nsec.app.",
      "Choose to connect an app, then scan this code.",
      "Approve the request in the app.",
    ]) steps.append(el("li", null, t));
    const side = el("div", "m-side");
    const actions = el("div", "actions");
    actions.append(retry, button("Cancel", "secondary", async () => closeModal()));
    side.append(steps, status, error, actions);
    const row = el("div", "m-row");
    row.append(qr, side);

    async function offer() {
      retry.hidden = true;
      showError(error, "");
      status.textContent = "Getting a code…";
      qr.removeAttribute("src");
      let o;
      try {
        o = await call("signer.pair.start");
      } catch (err) {
        showError(error, err.message);
        retry.hidden = false;
        return;
      }
      if (!active) return call("signer.pair.cancel").catch(() => {});
      qr.src = o.qr;
      status.textContent = "Waiting for your phone… The code works for two minutes.";
      try {
        const s = await call("signer.pair.wait");
        if (active) await signedIn(s);
      } catch (err) {
        if (!active) return;
        qr.removeAttribute("src");
        status.textContent = "";
        showError(error, err.code === 1008 ? "The code expired." : err.message);
        retry.hidden = false;
        retry.focus();
      }
    }
    retry.addEventListener("click", () => busy(retry, offer));

    openModal("Sign in with your phone", [row], () => {
      active = false;
      call("signer.pair.cancel").catch(() => {});
    });
    offer();
  },

  link() {
    secretFlow({
      title: "Use a login link",
      mode: "bunker",
      label: "Login link",
      placeholder: "bunker://…",
      help: "Copy the bunker link from your signer app (nsec.app, Amber and others have one) and paste it here. Press Steam + X for the keyboard.",
      valid: (v) => v.startsWith("bunker://"),
      invalid: "A login link starts with bunker://",
      failed: "Your signer didn't accept that link. Check it is current and approve the request in the app.",
      waiting: "Waiting for your signer to approve…",
    });
  },

  key() {
    secretFlow({
      title: "Enter a private key",
      mode: "nsec",
      label: "Private key",
      placeholder: "nsec1…",
      help: "Your key is saved on this device, readable only by you. Signing in with your phone keeps the key off this device and is safer.",
      valid: (v) => /^nsec1[02-9ac-hj-np-z]{58}$/.test(v),
      invalid: "A private key starts with nsec1 and is 63 characters long.",
      failed: "That key didn't work.",
      waiting: "Signing in…",
      password: true,
    });
  },

  // A signer service on this machine.
  system() {
    const error = errorLine();
    const connect = button("Connect", "primary", async () => {
      showError(error, "");
      const s = await call("signer.switch", { mode: "system" });
      if (s.connection_state === "connected") return signedIn(s);
      await call("signer.switch", { mode: "none" }).catch(() => {});
      showError(error, "No signer app is answering on this device. Start it and try again.");
    });
    const actions = el("div", "actions");
    actions.append(connect, button("Cancel", "secondary", async () => closeModal()));
    openModal("Use this device's signer", [
      el("p", "muted", "If a signer app is already running on this device, Steamduck can use it. Your key stays in that app, and it asks you before anything is signed."),
      error,
      actions,
    ]);
  },
};

function secretFlow(f) {
  const form = el("form", "m-form");
  const label = el("label", null, f.label);
  label.htmlFor = "m-secret";
  const input = el("input");
  input.id = "m-secret";
  input.type = f.password ? "password" : "text";
  input.placeholder = f.placeholder;
  input.autocomplete = "off";
  input.spellcheck = false;
  const help = el("p", "muted small", f.help);
  const error = errorLine();
  const submit = el("button", "primary", "Sign in");
  submit.type = "submit";
  const cancel = el("button", "secondary", "Cancel");
  cancel.type = "button";
  cancel.addEventListener("click", closeModal);
  const actions = el("div", "actions");
  actions.append(submit, cancel);
  form.append(label, input, help, error, actions);

  form.addEventListener("submit", (e) => {
    e.preventDefault();
    busy(submit, async () => {
      const secret = input.value.trim();
      if (!f.valid(secret)) return showError(error, f.invalid);
      showError(error, "");
      help.textContent = f.waiting;
      let s;
      try {
        s = await call("signer.switch", { mode: f.mode, secret });
      } catch (err) {
        help.textContent = f.help;
        return showError(error, err.code === 1004 ? f.failed : err.message);
      }
      input.value = "";
      if (s.connection_state === "connected") return signedIn(s);
      closeModal();
      toast("Saved, but your signer isn't answering yet.");
      await renderAccount(s);
    });
  });
  openModal(f.title, [form], () => (input.value = ""));
  input.focus();
}

// ---- settings ----

function renderTheme(theme) {
  for (const b of $("#theme").children) b.setAttribute("aria-pressed", String(b.dataset.value === theme));
}

// ---- wiring ----

for (const b of document.querySelectorAll("#acct-signin .option")) {
  b.addEventListener("click", () => FLOWS[b.dataset.flow]());
}
const SIGN_OUT_CONFIRM = "Sign out? Open games close, and a saved key or login link is forgotten.";
$("#signout").addEventListener("click", async (e) => {
  const b = e.currentTarget;
  if (await confirmAction(SIGN_OUT_CONFIRM)) busy(b, signOut);
});
$("#signout-offline").addEventListener("click", async (e) => {
  const b = e.currentTarget;
  if (await confirmAction(SIGN_OUT_CONFIRM)) busy(b, signOut);
});
$("#account-modal").addEventListener("click", (e) => {
  if (e.target === e.currentTarget) closeModal();
});
for (const b of $("#theme").children) {
  b.addEventListener("click", () => busy(b, async () => {
    await call("settings.set", { field: "theme", value: b.dataset.value });
    renderTheme(b.dataset.value);
  }));
}
