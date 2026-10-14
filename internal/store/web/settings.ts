import { call, requireStore } from "./api";
import { confirmAction } from "./store.js";

type Value = string | boolean | string[];
type Settings = Record<string, Value | unknown>;
type Definition = { field: string; title: string; help: string; type: "urls" | "toggle" | "theme" | "text"; placeholder?: string };
const definitions: Definition[] = [
  { field: "relays", title: "Discovery relays", type: "urls", placeholder: "wss://relay.example.com",
    help: "Relays Kwakore uses to discover napplets. Add relay addresses below; bare hostnames use wss://. An empty list disables these relays." },
  { field: "blossom_servers", title: "Blossom servers", type: "urls", placeholder: "https://blossom.example.com",
    help: "Servers Kwakore checks for napplet files, in priority order. Add server addresses below; bare hostnames use https://. An empty list disables these configured servers." },
  { field: "discover_on_user_relays", title: "Discover on my account’s relays", type: "toggle",
    help: "Also look for napplets on the relays associated with your signed-in Nostr account." },
  { field: "theme", title: "Napplet theme", type: "theme",
    help: "Choose light or dark for napplet windows, or follow your device’s theme." },
  { field: "desktop_entries", title: "Desktop shortcuts", type: "toggle",
    help: "Add installed napplets to your desktop application menu. Turning this off removes Kwakore’s desktop shortcuts; Steam shortcuts are managed separately." },
  { field: "sync_id", title: "Installed-list sync", type: "text", placeholder: "laptops",
    help: "Use the same sync name and Nostr account on devices that should share an installed napplet list. Leave blank to turn sync off. Up to 64 printable characters, with no spaces." },
  { field: "social_refresh", title: "Social refresh interval", type: "text", placeholder: "6h",
    help: "How often Kwakore refreshes installed napplets’ reactions, comments, mentions and zaps. Use a duration from 15m to 168h, such as 6h, or 0 to turn periodic refresh off." },
];

type Editor = {
  definition: Definition; root: HTMLElement; controls: Array<HTMLInputElement | HTMLTextAreaElement | HTMLButtonElement>;
  input?: HTMLInputElement; list?: HTMLElement; choice?: Value; baseline: Value; generation: number; timer?: ReturnType<typeof setTimeout>;
  source: HTMLElement; message: HTMLElement; retry: HTMLButtonElement; reset: HTMLButtonElement;
};
const editors = new Map<string, Editor>();
const $ = <T extends HTMLElement = HTMLElement>(id: string) => document.getElementById(id) as T;
const pendingRestore = new Set<string>();
const pendingSaves = new Set<Editor>();
let sources: Record<string, string> = {};
let methods: string[] = [];
let loading = false;
let mutating = false;
let revision = 0;
const equal = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

function element<K extends keyof HTMLElementTagNameMap>(tag: K, text = "", cls = ""): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag);
  node.textContent = text;
  node.className = cls;
  return node;
}

// Mirror the daemon's accepted schemes and reject credential-bearing URLs.
// Preserve paths and server order; remove duplicate normalized entries.
function urlList(text: string, field: string): string[] {
  const relay = field === "relays";
  const urls = text.split(/\r?\n/).map((value) => value.trim()).filter(Boolean).map((raw, index) => {
    const value = /^[a-z][a-z0-9+.-]*:\/\//i.test(raw) ? raw : `${relay ? "wss" : "https"}://${raw}`;
    let url: URL;
    try { url = new URL(value); } catch { throw new Error(`Line ${index + 1}: enter a valid ${relay ? "wss:// relay" : "http:// or https:// server"} address.`); }
    if (!url.hostname || (relay ? url.protocol !== "wss:" : !["https:", "http:"].includes(url.protocol)) ||
        url.username || url.password || value.includes("?") || value.includes("#") || /[\s\\]/.test(value)) {
      throw new Error(`Line ${index + 1}: use ${relay ? "wss://" : "http:// or https://"} with a host, without credentials, query parameters or fragments.`);
    }
    // URL canonicalizes host casing and ports. Kwakore disallows trailing
    // slashes for relays; root server URLs use the CLI's familiar bare-host form.
    let normalized = url.href;
    if (relay) normalized = normalized.replace(/\/+$/, "");
    else if (url.pathname === "/") normalized = normalized.replace(/\/$/, "");
    return normalized;
  });
  return [...new Set(urls)];
}

function read(editor: Editor, validate = false): Value {
  const { definition, input } = editor;
  if (!input || definition.type === "urls") return editor.choice!;
  const text = input.value.trim();
  if (validate && definition.field === "sync_id" && !/^[!-~]{0,64}$/.test(text)) {
    throw new Error("Use up to 64 printable characters without spaces, or leave blank to turn sync off.");
  }
  if (validate && definition.field === "social_refresh") {
    const shape = /^(0|([0-9]+(\.[0-9]+)?(ns|us|ms|s|m|h))+)$/.test(text);
    const units: Record<string, number> = { ns: 1e-9, us: 1e-6, ms: 1e-3, s: 1, m: 60, h: 3600 };
    let seconds = 0;
    for (const part of text.matchAll(/([0-9]+(?:\.[0-9]+)?)(ns|us|ms|s|m|h)/g)) seconds += Number(part[1]) * units[part[2]];
    if (!shape || text.length > 32 || !Number.isFinite(seconds) || seconds !== 0 && (seconds < 900 || seconds > 604800)) {
      throw new Error("Use 0, or a duration between 15m and 168h, such as 6h.");
    }
  }
  return text;
}

function dirty(editor: Editor): boolean {
  try { return !equal(read(editor), editor.baseline); } catch { return true; }
}

function updateActions(): void {
  for (const editor of editors.values()) {
    const changed = dirty(editor);
    editor.retry.disabled = mutating || loading || !changed;
    editor.reset.hidden = !methods.includes("settings.clear");
    editor.reset.disabled = mutating || loading || sources[editor.definition.field] === "default" || sources[editor.definition.field] === "config";
    editor.source.textContent = sources[editor.definition.field] === "override" ? "Saved on this device" :
      sources[editor.definition.field] === "config" ? "From your configuration" : sources[editor.definition.field] === "default" ? "Built-in default" : "Current value";
  }
  $("settings-refresh").setAttribute("aria-busy", String(loading));
  ($("settings-refresh") as HTMLButtonElement).disabled = loading || mutating;
  ($("settings-reload") as HTMLButtonElement).disabled = loading || mutating;
  $("settings-reload").hidden = !methods.includes("settings.reload");
}

function setValue(editor: Editor, value: Value): void {
  editor.baseline = Array.isArray(value) ? [...value] : value;
  if (editor.definition.type === "urls") {
    const changed = !equal(editor.choice, value);
    editor.choice = [...value as string[]];
    if (changed) renderURLs(editor);
  } else if (editor.input) editor.input.value = String(value);
  else {
    editor.choice = value;
    for (const control of editor.controls) control.setAttribute("aria-pressed", String(control.dataset.value === String(value)));
  }
}

function scheduleSave(editor: Editor, delay = 0): void {
  clearTimeout(editor.timer);
  pendingSaves.delete(editor);
  editor.timer = setTimeout(() => {
    if (dirty(editor)) pendingSaves.add(editor);
    void flushSaves();
  }, delay);
}

async function flushSaves(): Promise<void> {
  if (loading || mutating) return;
  const editor = pendingSaves.values().next().value;
  if (!editor) return;
  pendingSaves.delete(editor);
  if (dirty(editor)) await mutate(editor, false);
  void flushSaves();
}

function renderURLs(editor: Editor): void {
  const values = editor.choice as string[];
  editor.list!.replaceChildren();
  if (!values.length) editor.list!.append(element("p", "No addresses added", "muted small"));
  for (const url of values) {
    const row = element("li", "", "setting-url");
    row.append(element("span", url));
    const remove = element("button", "Remove", "secondary");
    remove.setAttribute("aria-label", `Remove ${url}`);
    remove.addEventListener("click", () => {
      const index = (editor.choice as string[]).indexOf(url);
      editor.choice = (editor.choice as string[]).filter((value) => value !== url);
      renderURLs(editor);
      const buttons = editor.list!.querySelectorAll<HTMLButtonElement>("button");
      (buttons[Math.min(index, buttons.length - 1)] || editor.input)?.focus();
      changed(editor);
    });
    row.append(remove);
    editor.list!.append(row);
  }
}

function changed(editor: Editor): void {
  editor.generation++;
  pendingRestore.delete(editor.definition.field);
  editor.message.textContent = dirty(editor) ? "Waiting to save…" : "";
  editor.message.classList.remove("bad");
  editor.retry.hidden = true;
  editor.input?.removeAttribute("aria-invalid");
  updateActions();
  scheduleSave(editor, editor.definition.type === "text" ? 500 : 0);
}

function makeEditor(definition: Definition, value: Value): Editor {
  const root = element("section", "", "setting-card");
  root.dataset.field = definition.field;
  const title = element("h2", definition.title);
  title.id = `setting-title-${definition.field}`;
  root.setAttribute("aria-labelledby", title.id);
  const help = element("p", definition.help, "muted small");
  help.id = `setting-help-${definition.field}`;
  const controls: Editor["controls"] = [];
  let input: Editor["input"];
  let list: HTMLElement | undefined;
  let form: HTMLFormElement | undefined;
  let addButton: HTMLButtonElement | undefined;
  root.append(title, help);
  if (definition.type === "urls" || definition.type === "text") {
    input = element("input");
    input.id = `setting-${definition.field}`;
    input.setAttribute("aria-label", definition.type === "urls" ? `Add ${definition.field === "relays" ? "relay" : "Blossom server"}` : definition.title);
    input.setAttribute("aria-describedby", help.id);
    input.placeholder = definition.placeholder || "";
    input.autocomplete = "off";
    input.spellcheck = false;
    controls.push(input);
    if (definition.type === "urls") {
      list = element("ul", "", "setting-urls");
      list.setAttribute("aria-labelledby", title.id);
      form = element("form", "", "setting-add");
      const add = element("button", "Add", "primary");
      add.type = "button";
      addButton = add;
      controls.push(add);
      form.append(input, add);
      root.append(list, form);
    } else root.append(input);
  } else {
    const choices = element("div", "", "segmented setting-choices");
    choices.setAttribute("aria-labelledby", title.id);
    for (const [choice, label] of definition.type === "theme" ? [["system", "System"], ["light", "Light"], ["dark", "Dark"]] : [["true", "On"], ["false", "Off"]]) {
      const button = element("button", label);
      button.dataset.value = choice;
      choices.append(button);
      controls.push(button);
    }
    root.append(choices);
  }
  const source = element("p", "", "muted small setting-source");
  const message = element("p", "", "setting-message small");
  message.id = `setting-message-${definition.field}`;
  message.setAttribute("role", "status");
  input?.setAttribute("aria-errormessage", message.id);
  const actions = element("div", "", "actions setting-actions");
  const retry = element("button", "Retry saving", "primary");
  retry.hidden = true;
  const reset = element("button", "Reset to configured value", "secondary");
  actions.append(retry, reset);
  root.append(source, message, actions);
  const editor: Editor = { definition, root, controls, input, list, baseline: value, generation: 0, source, message, retry, reset };
  setValue(editor, value);
  if (form) {
    const addAddress = () => {
      try {
        const text = input!.value.trim();
        if (!text || /[\r\n]/.test(text)) throw new Error("Enter one address to add.");
        const [url] = urlList(text, definition.field);
        if ((editor.choice as string[]).includes(url)) throw new Error("This address is already added.");
        editor.choice = [...editor.choice as string[], url];
        input!.value = "";
        renderURLs(editor);
        changed(editor);
        input!.focus();
      } catch (error) {
        message.textContent = (error as Error).message;
        message.classList.add("bad");
        input!.setAttribute("aria-invalid", "true");
      }
    };
    // Opaque napplet sandboxes do not allow browser form submission. Handle
    // both controller clicks and Enter locally without navigating the document.
    addButton!.addEventListener("click", addAddress);
    form.addEventListener("submit", (event) => { event.preventDefault(); addAddress(); });
    input!.addEventListener("keydown", (event) => {
      if (event.key === "Enter") { event.preventDefault(); addAddress(); }
    });
    input!.addEventListener("input", () => {
      input!.removeAttribute("aria-invalid");
      if (!dirty(editor)) { message.textContent = ""; message.classList.remove("bad"); }
    });
  } else input?.addEventListener("input", () => changed(editor));
  if (!input) for (const control of controls) control.addEventListener("click", () => {
    editor.choice = definition.type === "toggle" ? control.dataset.value === "true" : control.dataset.value;
    for (const button of controls) button.setAttribute("aria-pressed", String(button === control));
    changed(editor);
  });
  retry.addEventListener("click", () => scheduleSave(editor));
  reset.addEventListener("click", () => { void mutate(editor, true); });
  return editor;
}

async function snapshot(): Promise<{ settings: Settings; sources: Record<string, string> }> {
  if (methods.includes("settings.inspect")) {
    try {
      const result = await call("settings.inspect");
      if (!result?.settings || typeof result.settings !== "object") throw new Error("Kwakore returned invalid settings.");
      return { settings: result.settings, sources: result.sources || {} };
    } catch (error) {
      if ((error as {code?: number}).code !== -32601) throw error;
      // The legacy adapter can run against an older daemon.
      methods = methods.filter((method) => !["settings.inspect", "settings.clear", "settings.reload"].includes(method));
    }
  }
  const settings = await call("settings.get");
  if (!settings || typeof settings !== "object") throw new Error("Kwakore returned invalid settings.");
  return {settings, sources: {}};
}

function applySnapshot(settings: Settings, forceField?: string): void {
  for (const definition of definitions) {
    const value = settings[definition.field];
    const valid = definition.type === "urls" ? Array.isArray(value) && value.every((item) => typeof item === "string") :
      definition.type === "toggle" ? typeof value === "boolean" : typeof value === "string";
    if (!valid) continue; // Older daemons may omit settings they don't support.
    const editor = editors.get(definition.field);
    if (!editor) {
      const created = makeEditor(definition, value as Value);
      editors.set(definition.field, created);
      $("settings-fields").append(created.root);
    } else if (!dirty(editor) || forceField === definition.field || pendingRestore.has(definition.field)) {
      const restored = pendingRestore.has(definition.field);
      setValue(editor, value as Value);
      pendingRestore.delete(definition.field);
      if (restored || editor.message.classList.contains("bad")) {
        editor.message.textContent = restored ? "Configured value restored" : "Current value refreshed";
        editor.message.classList.remove("bad");
        editor.input?.removeAttribute("aria-invalid");
      }
    }
  }
  updateActions();
}

export async function loadSettings(): Promise<void> {
  if (mutating) return;
  const current = ++revision;
  loading = true;
  $("settings-status").textContent = "Loading settings…";
  updateActions();
  try {
    methods = (await requireStore()).methods;
    const result = await snapshot();
    if (current !== revision) return;
    sources = result.sources;
    applySnapshot(result.settings);
    $("settings-status").textContent = editors.size ? "Changes save automatically. Reset restores your configuration or the built-in default." : "This runtime exposes no editable settings.";
  } catch (error) {
    if (current === revision) $("settings-status").textContent = `${(error as Error).message} Use Refresh settings to try again.`;
  } finally {
    if (current === revision) { loading = false; updateActions(); void flushSaves(); }
  }
}

async function mutate(editor: Editor, reset: boolean): Promise<void> {
  if (loading || mutating) return;
  let value: Value;
  try { value = read(editor, !reset); } catch (error) {
    if (!reset) {
      editor.message.textContent = (error as Error).message;
      editor.message.classList.add("bad");
      editor.input?.setAttribute("aria-invalid", "true");
      return;
    }
    value = editor.baseline;
  }
  if (reset && !await confirmAction(`Reset ${editor.definition.title.toLowerCase()} to your configuration or the built-in default? Unsaved edits for this setting will be discarded.`)) return;
  if (loading || mutating) return;
  clearTimeout(editor.timer);
  pendingSaves.delete(editor);
  const generation = editor.generation;
  mutating = true;
  revision++;
  updateActions();
  editor.retry.hidden = true;
  editor.message.classList.remove("bad");
  editor.message.textContent = reset ? "Resetting…" : "Saving…";
  let committed = false;
  try {
    await call(reset ? "settings.clear" : "settings.set", reset ? {field: editor.definition.field} : {field: editor.definition.field, value});
    committed = true;
    if (reset && generation === editor.generation) pendingRestore.add(editor.definition.field);
    if (!reset) { editor.baseline = Array.isArray(value) ? [...value] : value; sources[editor.definition.field] = "override"; }
    const result = await snapshot();
    sources = result.sources;
    applySnapshot(result.settings, reset && generation === editor.generation ? editor.definition.field : undefined);
    editor.input?.removeAttribute("aria-invalid");
    if (generation === editor.generation) editor.message.textContent = reset ? "Configured value restored" : "Saved";
  } catch (error) {
    editor.retry.hidden = committed;
    editor.message.classList.add("bad");
    editor.message.textContent = committed ? "Change saved, but the current value could not be read. Refresh settings to verify it." : (error as Error).message;
  } finally {
    mutating = false;
    updateActions();
    void flushSaves();
  }
}

export function initSettings(): void {
  $("settings-refresh").addEventListener("click", () => { void loadSettings(); });
  $("settings-reload").addEventListener("click", async () => {
    if (loading || mutating) return;
    if (Array.from(editors.values()).some(dirty) && !await confirmAction("Reload the configuration? Your unsaved edits will remain available.")) return;
    mutating = true;
    revision++;
    updateActions();
    try {
      await call("settings.reload");
      $("settings-status").textContent = "Configuration reloaded.";
    } catch (error) {
      $("settings-status").textContent = (error as Error).message;
      mutating = false; updateActions(); void flushSaves(); return;
    }
    mutating = false;
    await loadSettings();
  });
  window.addEventListener("pagehide", () => {
    revision++;
    for (const editor of editors.values()) clearTimeout(editor.timer);
    pendingSaves.clear();
  }, { once: true });
}
