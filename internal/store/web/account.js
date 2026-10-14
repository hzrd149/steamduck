import { call } from "./api";
import { $, toast, busy, button, confirmAction, focusFirst, copyText } from "./store.js";

import { identityGetPublicKey, identityOnChanged } from "@napplet/nap/identity/sdk";
import { hasIdentity, canEditProfile, readProfile, profileImage, openUserProfile,
  latestProfile, profileMetadata, publishProfile } from "./identity";
import { initSocial, setSocialIdentity } from "./account-social";

// ---- account: the player's social account ----
//
// Signed out, the tab offers the ways to sign in, each in its own modal.
// Signed in, it shows the account's public profile and a sign out button.

let accountKey = null;
let accountRevision = 0;
let imageRevision = 0;
let currentProfile = null;

export async function loadAccount() {
  const revision = ++accountRevision;
  const [signer, pubkey] = await Promise.all([
    call("signer.status"), hasIdentity() ? identityGetPublicKey() : null,
  ]);
  if (revision !== accountRevision) return;
  await renderAccount(hasIdentity() ? { ...signer, public_key: pubkey,
    connection_state: pubkey ? "connected" : "disconnected", mode: pubkey ? signer.mode === "none" ? "system" : signer.mode : signer.mode } : signer);
}

async function renderAccount(s) {
  const revision = ++accountRevision;
  const show = (id) => {
    for (const v of ["acct-loading", "acct-profile", "acct-offline", "acct-signin"]) $("#" + v).hidden = v !== id;
  };
  if (s.mode === "none" || s.connection_state !== "connected" || !s.public_key) {
    accountKey = null;
    currentProfile = null;
    imageRevision++;
    setSocialIdentity("");
    show(s.mode === "none" ? "acct-signin" : "acct-offline");
    return;
  }
  if (accountKey !== s.public_key) {
    accountKey = s.public_key;
    renderProfile({ public_key: s.public_key, npub: s.npub || s.public_key, found: false, loading: true });
  }
  show("acct-profile");
  setSocialIdentity(accountKey);
  $("#profile-edit").hidden = !canEditProfile();
  $("#profile-open").hidden = !window.napplet?.intent;
  try {
    const p = hasIdentity() ? await readProfile(s.public_key) : await call("profile.get");
    if (revision === accountRevision && p?.public_key === accountKey) renderProfile(p);
  } catch (err) {
    if (revision !== accountRevision) return;
    $("#p-name").classList.remove("skeleton");
    $("#p-name").textContent = "Your account";
    $("#p-note").hidden = false;
    $("#p-note").textContent = "Could not load your profile. Use Refresh profile to try again.";
    toast(err.message, true);
  }
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
  currentProfile = p;
  const revision = ++imageRevision;
  const name = p.display_name || p.name || (p.loading ? "" : "Your account");
  $("#p-name").textContent = name;
  $("#p-name").classList.toggle("skeleton", !!p.loading);

  const handle = p.nip05 ? p.nip05.replace(/^_@/, "") : p.name && p.name !== name ? "@" + p.name : shortId(p.npub);
  $("#p-handle").textContent = handle;

  const avatar = $("#p-avatar");
  if (!setImage(avatar, p.picture)) avatar.textContent = [...(name || "?").trim()][0]?.toUpperCase() || "?";
  else avatar.textContent = "";
  setImage($("#p-banner"), p.banner);
  if (hasIdentity() && !p.loading) {
    for (const [element, url] of [[avatar, p.picture], [$("#p-banner"), p.banner]]) {
      if (!url) continue;
      profileImage(url).then((image) => {
        if (revision !== imageRevision || !image) return;
        setImage(element, image);
        if (element === avatar) element.textContent = "";
      }).catch(() => {});
    }
  }

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
  fact("Account ID", p.npub || p.public_key, "mono");

  const note = $("#p-note");
  note.hidden = p.loading || p.found;
  note.textContent = "This account has no public profile yet. Add a name and picture with Edit profile or another Nostr app.";
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

export function closeModal() {
  if (!modalClose) return;
  const onClose = modalClose;
  modalClose = null;
  $("#account-modal").hidden = true;
  $("#m-body").replaceChildren();
  onClose();
  if (modalReturnFocus && document.contains(modalReturnFocus) && modalReturnFocus.offsetParent) modalReturnFocus.focus();
  else focusFirst();
}

export function modalOpen() {
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

  const signIn = (e) => {
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
  };
  submit.addEventListener("click", signIn);
  form.addEventListener("submit", signIn);
  openModal(f.title, [form], () => (input.value = ""));
  input.focus();
}

// ---- public profile editor ----

async function editProfile() {
  const pubkey = accountKey;
  if (!pubkey || !canEditProfile()) return;
  let active = true;
  const status = el("p", "muted", "Loading your latest profile…");
  openModal("Edit public profile", [status, button("Cancel", "secondary", async () => closeModal())], () => { active = false; });
  let baseline, metadata;
  try {
    baseline = await latestProfile(pubkey);
    metadata = profileMetadata(baseline);
    if (!baseline && currentProfile?.found) throw new Error("Could not load your existing profile for editing. Refresh and try again.");
  } catch (err) {
    if (active) status.textContent = err.message;
    return;
  }
  if (!active || accountKey !== pubkey) return;
  const form = el("form", "m-form profile-editor");
  form.append(el("p", "muted small", "These details are public across Nostr. Kwakore asks your signer to approve the update."));
  const inputs = new Map();
  const original = new Map();
  for (const [field, labelText, type] of [
    ["display_name", "Display name", "text"], ["name", "Username", "text"],
    ["about", "About", "textarea"], ["picture", "Picture URL", "url"],
    ["banner", "Banner URL", "url"], ["website", "Website", "url"],
    ["nip05", "Nostr address (NIP-05)", "text"], ["lud16", "Lightning address", "text"],
  ]) {
    const label = el("label", null, labelText);
    const input = el(type === "textarea" ? "textarea" : "input");
    input.id = "edit-" + field; label.htmlFor = input.id;
    if (type !== "textarea") input.type = type;
    else input.rows = 3;
    const value = field === "display_name" ? metadata.display_name ?? metadata.displayName : metadata[field];
    input.value = typeof value === "string" ? value : "";
    original.set(field, input.value);
    input.autocomplete = "off";
    inputs.set(field, input);
    form.append(label, input);
  }
  const error = errorLine();
  const save = el("button", "primary", "Publish profile");
  save.type = "submit";
  const cancel = el("button", "secondary", "Cancel");
  cancel.type = "button";
  cancel.addEventListener("click", closeModal);
  const actions = el("div", "actions"); actions.append(save, cancel);
  form.append(error, actions);
  $("#m-body").replaceChildren(form);
  focusFirst($("#account-modal"));
  const publish = (event) => {
    event.preventDefault();
    busy(save, async () => {
      showError(error, "");
      const changes = {};
      for (const [field, input] of inputs) {
        const value = input.value.trim();
        if (value !== original.get(field)) changes[field] = value;
      }
      if (!Object.keys(changes).length) { closeModal(); return; }
      for (const input of inputs.values()) input.disabled = true;
      try {
        if (!active || accountKey !== pubkey) return;
        const profile = await publishProfile(pubkey, changes, baseline);
        if (!active || accountKey !== pubkey) return;
        accountRevision++;
        renderProfile(profile);
        closeModal();
        toast("Profile published");
      } catch (err) {
        if (active) showError(error, err.message || "Could not publish your profile. Try again.");
      } finally {
        for (const input of inputs.values()) input.disabled = false;
      }
    });
  };
  // A scripts-only sandbox blocks native form submission. Handle activation
  // directly while keeping submit semantics for Enter and accessible forms.
  save.addEventListener("click", publish);
  form.addEventListener("submit", publish);
}

// ---- wiring ----

export function initAccount() {
  initSocial();
  if (hasIdentity()) {
    const subscription = identityOnChanged((pubkey) => {
      accountRevision++;
      closeModal();
      // Clear the old account immediately, before any async reads complete.
      accountKey = null;
      currentProfile = null;
      imageRevision++;
      setSocialIdentity("");
      renderAccount({ mode: pubkey ? "system" : "none", public_key: pubkey,
        connection_state: pubkey ? "connected" : "disconnected" }).catch((err) => toast(err.message, true));
    });
    window.addEventListener("pagehide", () => {
      subscription.close(); accountRevision++; imageRevision++; closeModal();
    }, { once: true });
    loadAccount().catch((err) => toast(err.message, true));
  }
  $("#profile-edit").addEventListener("click", (e) => busy(e.currentTarget, editProfile));
  $("#profile-refresh").addEventListener("click", (e) => busy(e.currentTarget, loadAccount));
  $("#profile-open").addEventListener("click", (e) => busy(e.currentTarget, () => openUserProfile(accountKey)));
  $("#profile-copy").addEventListener("click", (e) => busy(e.currentTarget, async () => {
    const id = currentProfile?.npub || accountKey;
    if (id) toast(await copyText(id) ? "Account ID copied" : id);
  }));
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

}
