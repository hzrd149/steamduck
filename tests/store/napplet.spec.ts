import { expect, test, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";

const napplet = readFileSync(new URL("../../internal/store/built/napplet.html", import.meta.url), "utf8");
const legacy = readFileSync(new URL("../../internal/store/built/legacy.html", import.meta.url), "utf8");
const methods = ["status", "installed", "discover", "icon", "launch", "sync", "signer.status", "settings.get",
  "profile.get", "napplet.social", "settings.set", "settings.inspect", "settings.clear", "settings.reload", "signer.switch"];
const item = {
  address: "35129:" + "a".repeat(64) + ":demo",
  name: "Demo game", format: "napplet", available: true, compatible: true,
  version: { event_id: "b".repeat(64), created_at: 1, artifact_hash: "c".repeat(64) },
  update_available: null,
};

async function openStore(page: Page, options: { missing?: boolean; version?: number; legacy?: boolean; identity?: boolean } = {}) {
  const errors: string[] = [];
  const requests: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.route("**/*", (route) => { requests.push(route.request().url()); return route.abort(); });
  const prelude = `
    window.testKey = "a".repeat(64);
    window.testProfile = {displayName: "Duck player", name: "duck", about: "Deck life", picture: "https://example.com/avatar.png"};
    window.rawProfile = {display_name: "Duck player", name: "duck", about: "Deck life", lud06: "lnurl1keep", custom: {preserve: true}};
    window.resourceURLs = [];
    window.testSettings = {relays: ["wss://relay.example"], blossom_servers: ["https://blossom.example"],
      discover_on_user_relays: true, theme: "dark", desktop_entries: true, gnome_search: false,
      sync_id: "", social_refresh: "6h", signer: {mode: "none"}};
    window.testSources = Object.fromEntries(Object.keys(window.testSettings).map(field => [field, "default"]));
    window.testConfigured = {...window.testSettings, relays: ["wss://configured.example"]};
    window.testCalls = [];
    window.testState = { available: true, focused: true, pads: [] };
    window.testErrors = {};
    window.testInfoCalls = 0;
    navigator.getGamepads = () => { ${options.legacy ? "return [];" : 'throw new Error("Native gamepad API polled");'} };
    const execute = async (method, params) => {
      window.testCalls.push({method, params});
      if (window.testErrors[method]) throw window.testErrors[method];
      switch (method) {
        case "status": return { health: { ready: true, version: "test" } };
        case "installed": return [${JSON.stringify(item)}];
        case "discover": return {items: [], next_offset: null, fetched_at: "2026-10-13", complete: true};
        case "icon": return "";
        case "launch": return {outcome: "opened", address: params.address};
        case "sync": return {deferred: true, shortcut_count: 1};
        case "signer.status": return {mode: "none", public_key: "", connection_state: "disconnected", npub: ""};
        case "settings.get": return window.testSettings;
        case "settings.inspect": return {settings: window.testSettings, sources: window.testSources};
        case "settings.set":
          window.testSettings[params.field] = params.value;
          window.testSources[params.field] = "override";
          if (window.failReadAfterSave) window.testErrors["settings.inspect"] = {message: "Read failed"};
          return null;
        case "settings.clear":
          window.testSettings[params.field] = window.testConfigured[params.field];
          window.testSources[params.field] = "config";
          return null;
        case "settings.reload": return {settings: window.testSettings};
        case "napplet.social": return null;
      }
      return null;
    };
    ${options.legacy ? `window.steamduckCall = async (method, params) => ({ result: await execute(method, params) });` :
      options.missing ? "" : `window.napplet = {
        steamduck: {
          async info() { window.testInfoCalls++; return {version: ${options.version ?? 1}, methods: ${JSON.stringify(methods)}, kwakore_protocol_version: 1}; },
          call: execute,
        },
        ${options.identity ? `identity: {
          async getPublicKey() { return window.testKey; },
          onChanged(handler) { window.changeIdentity = handler; return {close() { window.identityClosed = true; }}; },
          async getProfile() {
            if (window.profileDeferred) return new Promise(resolve => { window.resolveProfile = resolve; });
            return window.testProfile;
          },
          async getFollows() { if (window.failFollows) throw new Error("Permission denied"); return ["c".repeat(64)]; },
          async getRelays() { return {"wss://relay.example": {read: true, write: false}}; },
          async getBadges() { return [{id: "badge", name: "Early duck", awardedBy: "d".repeat(64)}]; },
          async getZaps() { return [{eventId: "zap", sender: "e".repeat(64), amount: 21000, content: "Nice games!"}]; },
          async getMutes() { return ["f".repeat(64)]; },
          async getBlocked() { return []; },
          async getList(type) { window.lastList = type; return ["nostr:note1example"]; },
        },
        relay: {
          async query(filters) {
            window.testCalls.push({method: "relay.query", params: filters});
            if (window.queryDenied) throw new Error("Relay access denied");
            return [{event: {kind: 0, pubkey: window.testKey, created_at: 100,
              id: "a".repeat(64), tags: [["custom", "tag"]], content: JSON.stringify(window.rawProfile)}}];
          },
          async publish(template) {
            window.published = template;
            if (window.publishDenied) throw new Error("Signing denied");
            return {...template, pubkey: window.testKey, id: "b".repeat(64)};
          },
        },
        resource: {
          async bytes(url) {
            window.resourceURLs.push(url);
            return new Blob([new Uint8Array([137, 80, 78, 71])], {type: "image/png"});
          },
        },
        intent: {
          async open(uri, options) { window.openedProfile = {uri, options}; return {ok: true}; },
        },` : ""}
        gamepad: {
          get available() { return window.testState.available; },
          get focused() { return window.testState.focused; },
          getGamepads() { return window.testState.pads; },
        },
        theme: {
          async get() { return {colors: {background: "#112233", text: "#eeeeee", primary: "#ffcc33"}}; },
          onChanged(handler) { window.testTheme = handler; return {close() {}}; },
        },
      };`}
  `;
  const document = `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; connect-src 'none'"><script>${prelude}</script>${options.legacy ? legacy : napplet}`;
  await page.setContent('<iframe title="Store" sandbox="allow-scripts" style="border:0;width:1280px;height:800px"></iframe>');
  await page.locator("iframe").evaluate((frame, html) => { (frame as HTMLIFrameElement).srcdoc = html; }, document);
  const frame = page.frameLocator("iframe");
  await expect(frame.locator(".brand")).toHaveText("Steamduck Store");
  return { frame, errors, requests };
}

test("the production HTML works in an opaque sandbox using injected NAP APIs", async ({ page }) => {
  const { frame, errors, requests } = await openStore(page);
  await expect(frame.locator("#library-grid .card")).toHaveCount(1);
  await expect(frame.locator("#status-text")).toHaveText("kwakore test");
  await expect(frame.locator("html")).toHaveCSS("--bg", "#112233");
  await frame.locator('.tab[data-tab="account"]').click();
  await expect(frame.locator("#acct-signin")).toBeVisible();
  await frame.locator('.tab[data-tab="settings"]').click();
  await frame.locator('[data-field="theme"] [data-value="light"]').click();
  await frame.locator('[data-field="theme"]').getByRole("button", {name: "Save", exact: true}).click();
  await expect(frame.locator('[data-field="theme"] .setting-message')).toHaveText("Saved");
  const calls = await frame.locator("body").evaluate(() => (window as any).testCalls);
  expect(calls).toContainEqual({ method: "settings.set", params: { field: "theme", value: "light" } });
  expect(await frame.locator("body").evaluate(() => (window as any).testInfoCalls)).toBe(1);
  expect(requests).toEqual([]);
  expect(errors).toEqual([]);
});

test("a runtime without the private API gets an explicit unavailable view", async ({ page }) => {
  const { frame, errors, requests } = await openStore(page, { missing: true });
  await expect(frame.locator("#runtime-unavailable")).toHaveText("Open the store in a supported Steamduck runtime.");
  await expect(frame.locator("#status-text")).toHaveText("Store unavailable");
  expect(requests).toEqual([]);
  expect(errors).toEqual([]);
});

test("an incompatible bridge version cannot dispatch store operations", async ({ page }) => {
  const { frame } = await openStore(page, { version: 2 });
  await expect(frame.locator("#runtime-unavailable")).toHaveText("This Steamduck runtime needs an update.");
  expect(await frame.locator("body").evaluate(() => (window as any).testCalls)).toEqual([]);
});

test("NAP gamepad actions stop in the background and require release after focus returns", async ({ page }) => {
  const { frame, errors } = await openStore(page);
  await expect(frame.locator("#library-grid .card")).toHaveCount(1);
  await frame.locator("#library-grid .card").click();
  const play = frame.getByRole("button", { name: "Play", exact: true });
  await play.focus();
  // Four animation frames let the polling loop observe each supplied snapshot.
  async function snapshot(focused: boolean, held: boolean) {
    await frame.locator("body").evaluate(async (_, value) => {
      (window as any).testState = {available: true, focused: value.focused, pads: [null, {
        axes: [0, 0], buttons: [{pressed: value.held}],
      }]};
      for (let i = 0; i < 4; i++) await new Promise(requestAnimationFrame);
    }, { focused, held });
  }
  const launches = () => frame.locator("body").evaluate(() => (window as any).testCalls.filter((call: any) => call.method === "launch").length);
  await snapshot(false, true);
  await snapshot(true, true);
  expect(await launches()).toBe(0);
  await snapshot(true, false);
  await snapshot(true, true);
  expect(await launches()).toBe(1);
  await snapshot(false, true);
  await snapshot(true, true);
  expect(await launches()).toBe(1);
  expect(errors).toEqual([]);
});

test("public backend errors retain their store-facing explanation", async ({ page }) => {
  const { frame } = await openStore(page);
  await expect(frame.locator("#library-grid .card")).toHaveCount(1);
  await frame.locator("body").evaluate(() => {
    (window as any).testErrors.sync = {code: 1004, message: "Unavailable", data: {reason: "signer_unavailable"}};
  });
  await frame.locator("#sync").click();
  await expect(frame.locator("#toast")).toHaveText("This needs you to be signed in. Sign in on the Account tab.");
});

test("the separate compatibility build still uses the existing native backend", async ({ page }) => {
  const { frame } = await openStore(page, { legacy: true });
  await expect(frame.locator("#library-grid .card")).toHaveCount(1);
  await frame.locator("#sync").click();
  await expect(frame.locator("#toast")).toContainText("Steam is running");
});

test("the compatibility browser transport preserves the loopback token check", async ({ page }) => {
  const requests: Array<{ method: string; token: string | undefined }> = [];
  await page.route("http://store.test/**", async (route) => {
    const request = route.request();
    if (request.method() === "GET") {
      return route.fulfill({ contentType: "text/html", body: legacy,
        headers: { "Content-Security-Policy": "default-src 'self' 'unsafe-inline' data:" } });
    }
    const method = new URL(request.url()).pathname.slice("/api/".length);
    requests.push({ method, token: request.headers()["x-steamduck-token"] });
    const result = method === "installed" ? [item] : method === "icon" ? "" :
      method === "status" ? { health: { ready: true, version: "loopback" } } : null;
    return route.fulfill({ json: { result } });
  });
  await page.goto("http://store.test/#token=development-token");
  await expect(page.locator("#library-grid .card")).toHaveCount(1);
  await expect(page.locator("#status-text")).toHaveText("kwakore loopback");
  expect(requests.map((request) => request.method)).toEqual(expect.arrayContaining(["status", "installed"]));
  expect(requests.every((request) => request.token === "development-token")).toBe(true);
});


test("identity profile and social data use host domains without network requests", async ({ page }) => {
  const {frame, errors, requests} = await openStore(page, {identity: true});
  await frame.locator('.tab[data-tab="account"]').click();
  await expect(frame.locator("#p-name")).toHaveText("Duck player");
  await expect(frame.locator("#p-about")).toHaveText("Deck life");
  await expect(frame.locator("#p-avatar")).toHaveClass(/has-image/);
  await expect(frame.locator("#social-status")).toHaveText("1 people followed");
  await frame.locator("#social-content button").click();
  expect(await frame.locator("body").evaluate(() => (window as any).openedProfile)).toEqual({
    uri: "napplet:profile/open", options: {payload: {pubkey: "c".repeat(64)}},
  });
  for (const [section, text] of [["relays", "1 relays"], ["badges", "1 badges"], ["zaps", "21 sats"], ["mutes", "1 muted"], ["blocked", "0 blocked"], ["lists", "1 public entries"]]) {
    await frame.locator(`[data-section="${section}"]`).click();
    await expect(frame.locator("#social-status")).toContainText(text);
  }
  await frame.getByRole("button", {name: "Interests", exact: true}).click();
  await expect.poll(() => frame.locator("body").evaluate(() => (window as any).lastList)).toBe("interests");
  expect(await frame.locator("body").evaluate(() => (window as any).resourceURLs)).toContain("https://example.com/avatar.png");
  expect(await frame.locator("body").evaluate(() => (window as any).testCalls.some((c: any) => c.method === "profile.get"))).toBe(false);
  expect(errors).toEqual([]);
  expect(requests).toEqual([]);
});

test("profile publishing merges latest metadata and preserves unknown fields and tags", async ({ page }) => {
  const {frame, errors} = await openStore(page, {identity: true});
  await frame.locator('.tab[data-tab="account"]').click();
  await frame.getByRole("button", {name: "Edit profile", exact: true}).click();
  await expect(frame.locator("#edit-display_name")).toHaveValue("Duck player");
  await frame.locator("#edit-display_name").fill("New duck");
  await frame.locator("#edit-about").fill("");
  await frame.locator("body").evaluate(() => { (window as any).rawProfile.website = "https://new.example"; });
  await frame.getByRole("button", {name: "Publish profile", exact: true}).click();
  await expect(frame.locator("#account-modal")).toBeHidden();
  await expect(frame.locator("#p-name")).toHaveText("New duck");
  const published = await frame.locator("body").evaluate(() => (window as any).published);
  expect(published.kind).toBe(0);
  expect(published.tags).toEqual([["custom", "tag"]]);
  expect(JSON.parse(published.content)).toEqual({display_name: "New duck", name: "duck", lud06: "lnurl1keep", custom: {preserve: true}, website: "https://new.example"});
  expect(errors).toEqual([]);
});

test("a denied publication keeps the editor and edits available for retry", async ({ page }) => {
  const {frame} = await openStore(page, {identity: true});
  await frame.locator('.tab[data-tab="account"]').click();
  await frame.getByRole("button", {name: "Edit profile", exact: true}).click();
  await frame.locator("#edit-name").fill("retry-duck");
  await frame.locator("body").evaluate(() => { (window as any).publishDenied = true; });
  await frame.getByRole("button", {name: "Publish profile", exact: true}).click();
  await expect(frame.locator(".m-error")).toHaveText("Signing denied");
  await expect(frame.locator("#edit-name")).toHaveValue("retry-duck");
  await expect(frame.locator("#edit-name")).toBeEnabled();
  await frame.locator("body").evaluate(() => { (window as any).publishDenied = false; });
  await frame.getByRole("button", {name: "Publish profile", exact: true}).click();
  await expect(frame.locator("#account-modal")).toBeHidden();
});

test("identity changes clear old data and discard a late profile response", async ({ page }) => {
  const {frame, errors} = await openStore(page, {identity: true});
  await frame.locator('.tab[data-tab="account"]').click();
  await expect(frame.locator("#p-name")).toHaveText("Duck player");
  await frame.locator("body").evaluate(() => { (window as any).profileDeferred = true; });
  await frame.locator("#profile-refresh").click();
  await expect.poll(() => frame.locator("body").evaluate(() => typeof (window as any).resolveProfile)).toBe("function");
  await frame.locator("body").evaluate(() => {
    const w = window as any;
    w.testKey = "";
    w.changeIdentity("");
    w.resolveProfile({displayName: "Stale profile"});
  });
  await expect(frame.locator("#acct-signin")).toBeVisible();
  await expect(frame.locator("#acct-profile")).toBeHidden();
  await expect(frame.locator("#account-social")).toBeHidden();
  await frame.locator("body").evaluate(() => {
    const w = window as any;
    w.profileDeferred = false;
    w.testKey = "b".repeat(64);
    w.testProfile = {displayName: "Second duck"};
    w.changeIdentity(w.testKey);
  });
  await expect(frame.locator("#p-name")).toHaveText("Second duck");
  expect(errors).toEqual([]);
});

test("denied social data has a retry and does not hide the profile", async ({ page }) => {
  const {frame} = await openStore(page, {identity: true});
  await frame.locator('.tab[data-tab="account"]').click();
  await frame.locator("body").evaluate(() => { (window as any).failFollows = true; });
  await frame.locator('[data-section="follows"]').click();
  await expect(frame.locator("#social-status")).toContainText("Could not load");
  await expect(frame.locator("#p-name")).toHaveText("Duck player");
  await frame.locator("body").evaluate(() => { (window as any).failFollows = false; });
  await frame.getByRole("button", {name: "Retry", exact: true}).click();
  await expect(frame.locator("#social-status")).toContainText("1 people followed");
});


test("switching accounts closes the profile editor before an update can be signed", async ({ page }) => {
  const {frame} = await openStore(page, {identity: true});
  await frame.locator('.tab[data-tab="account"]').click();
  await frame.getByRole("button", {name: "Edit profile", exact: true}).click();
  await frame.locator("#edit-name").fill("never-publish");
  await frame.locator("body").evaluate(() => {
    const w = window as any;
    w.testKey = "b".repeat(64);
    w.testProfile = {displayName: "Another duck"};
    w.changeIdentity(w.testKey);
  });
  await expect(frame.locator("#account-modal")).toBeHidden();
  await expect(frame.locator("#p-name")).toHaveText("Another duck");
  expect(await frame.locator("body").evaluate(() => (window as any).published)).toBeUndefined();
});

test("a first profile can be created without a resource or intent domain", async ({ page }) => {
  const {frame, errors, requests} = await openStore(page, {identity: true});
  await frame.locator("body").evaluate(() => {
    const w = window as any;
    delete w.napplet.resource;
    delete w.napplet.intent;
    w.testProfile = null;
    w.napplet.relay.query = async () => [];
  });
  await frame.locator('.tab[data-tab="account"]').click();
  await expect(frame.locator("#p-note")).toBeVisible();
  await expect(frame.locator("#profile-open")).toBeHidden();
  await frame.getByRole("button", {name: "Edit profile", exact: true}).click();
  await frame.locator("#edit-display_name").fill("First duck");
  await frame.getByRole("button", {name: "Publish profile", exact: true}).click();
  await expect(frame.locator("#p-name")).toHaveText("First duck");
  expect(JSON.parse(await frame.locator("body").evaluate(() => (window as any).published.content))).toEqual({display_name: "First duck"});
  expect(errors).toEqual([]);
  expect(requests).toEqual([]);
});

test("profile edits remove the legacy displayName alias when clearing the display name", async ({ page }) => {
  const {frame} = await openStore(page, {identity: true});
  await frame.locator('.tab[data-tab="account"]').click();
  await frame.locator("body").evaluate(() => {
    const w = window as any;
    delete w.rawProfile.display_name;
    w.rawProfile.displayName = "Old alias";
  });
  await frame.getByRole("button", {name: "Edit profile", exact: true}).click();
  await frame.locator("#edit-display_name").fill("");
  await frame.getByRole("button", {name: "Publish profile", exact: true}).click();
  await expect(frame.locator("#account-modal")).toBeHidden();
  const metadata = JSON.parse(await frame.locator("body").evaluate(() => (window as any).published.content));
  expect(metadata).not.toHaveProperty("displayName");
  expect(metadata).not.toHaveProperty("display_name");
});

test("settings exposes every editable daemon field and saves normalized server lists", async ({ page }) => {
  const {frame, errors, requests} = await openStore(page);
  await frame.locator('.tab[data-tab="settings"]').click();
  await expect(frame.locator(".setting-card")).toHaveCount(8);
  const relays = frame.locator('[data-field="relays"]');
  await frame.locator("#setting-relays").fill("RELAY.New.Example/\nrelay.new.example\nwss://other.example:443/feed/");
  await relays.getByRole("button", {name: "Save", exact: true}).click();
  await expect(relays.locator(".setting-message")).toHaveText("Saved");
  await expect(frame.locator("#setting-relays")).toHaveValue("wss://relay.new.example\nwss://other.example/feed");
  const blossom = frame.locator('[data-field="blossom_servers"]');
  await frame.locator("#setting-blossom_servers").fill("blossom.new.example\nhttp://localhost:3000/files");
  await blossom.getByRole("button", {name: "Save", exact: true}).click();
  await expect(blossom.locator(".setting-message")).toHaveText("Saved");
  const calls = await frame.locator("body").evaluate(() => (window as any).testCalls);
  expect(calls).toContainEqual({method: "settings.set", params: {field: "relays", value: ["wss://relay.new.example", "wss://other.example/feed"]}});
  expect(calls).toContainEqual({method: "settings.set", params: {field: "blossom_servers", value: ["https://blossom.new.example", "http://localhost:3000/files"]}});
  expect(errors).toEqual([]);
  expect(requests).toEqual([]);
});

test("invalid settings are explained inline without reaching the daemon", async ({ page }) => {
  const {frame} = await openStore(page);
  await frame.locator('.tab[data-tab="settings"]').click();
  for (const [field, value] of [["relays", "https://relay.example"], ["relays", "wss://user:secret@relay.example"],
    ["blossom_servers", "https://blossom.example?token=secret"], ["sync_id", "has spaces"], ["social_refresh", "2m"], ["social_refresh", "999h"]]) {
    const card = frame.locator(`[data-field="${field}"]`);
    await frame.locator(`#setting-${field}`).fill(value);
    await card.getByRole("button", {name: "Save", exact: true}).click();
    await expect(card.locator(".setting-message")).toHaveClass(/bad/);
    await expect(frame.locator(`#setting-${field}`)).toHaveAttribute("aria-invalid", "true");
  }
  expect(await frame.locator("body").evaluate(() => (window as any).testCalls.filter((c: any) => c.method === "settings.set"))).toEqual([]);
});

test("draft settings survive tab changes and a failed save can be retried", async ({ page }) => {
  const {frame} = await openStore(page);
  await frame.locator('.tab[data-tab="settings"]').click();
  const card = frame.locator('[data-field="relays"]');
  await frame.locator("#setting-relays").fill("wss://draft.example");
  await frame.locator('.tab[data-tab="account"]').click();
  await frame.locator('.tab[data-tab="settings"]').click();
  await expect(frame.locator("#setting-relays")).toHaveValue("wss://draft.example");
  await frame.locator("body").evaluate(() => { (window as any).testErrors["settings.set"] = {message: "Could not persist settings"}; });
  await card.getByRole("button", {name: "Save", exact: true}).click();
  await expect(card.locator(".setting-message")).toHaveText("Could not persist settings");
  await expect(frame.locator("#setting-relays")).toHaveValue("wss://draft.example");
  await expect(card.getByRole("button", {name: "Save", exact: true})).toBeEnabled();
  await frame.locator("body").evaluate(() => { delete (window as any).testErrors["settings.set"]; });
  await card.getByRole("button", {name: "Save", exact: true}).click();
  await expect(card.locator(".setting-message")).toHaveText("Saved");
});

test("reset clears an override and displays the configured value", async ({ page }) => {
  const {frame} = await openStore(page);
  await frame.locator('.tab[data-tab="settings"]').click();
  const card = frame.locator('[data-field="relays"]');
  await frame.locator("#setting-relays").fill("wss://override.example");
  await card.getByRole("button", {name: "Save", exact: true}).click();
  await expect(card.locator(".setting-source")).toHaveText("Saved on this device");
  const reset = card.getByRole("button", {name: "Reset to configured value", exact: true});
  await reset.click();
  await expect(frame.locator("#toast")).toContainText("Press again to confirm");
  await reset.click();
  await expect(card.locator(".setting-message")).toHaveText("Configured value restored");
  await expect(frame.locator("#setting-relays")).toHaveValue("wss://configured.example");
  await expect(card.locator(".setting-source")).toHaveText("From your configuration");
  expect(await frame.locator("body").evaluate(() => (window as any).testCalls)).toContainEqual({method: "settings.clear", params: {field: "relays"}});
});

test("empty lists, disabled toggles and disabled sync are real settings values", async ({ page }) => {
  const {frame} = await openStore(page, {legacy: true});
  await frame.locator('.tab[data-tab="settings"]').click();
  const relays = frame.locator('[data-field="relays"]');
  await frame.locator("#setting-relays").fill("");
  await relays.getByRole("button", {name: "Save", exact: true}).click();
  await expect(relays.locator(".setting-message")).toHaveText("Saved");
  for (const field of ["discover_on_user_relays", "desktop_entries"]) {
    const card = frame.locator(`[data-field="${field}"]`);
    await card.getByRole("button", {name: "Off", exact: true}).click();
    await card.getByRole("button", {name: "Save", exact: true}).click();
    await expect(card.locator(".setting-message")).toHaveText("Saved");
  }
  const refresh = frame.locator('[data-field="social_refresh"]');
  await frame.locator("#setting-social_refresh").fill("0");
  await refresh.getByRole("button", {name: "Save", exact: true}).click();
  await expect(refresh.locator(".setting-message")).toHaveText("Saved");
  const calls = await frame.locator("body").evaluate(() => (window as any).testCalls);
  expect(calls).toContainEqual({method: "settings.set", params: {field: "relays", value: []}});
  expect(calls).toContainEqual({method: "settings.set", params: {field: "desktop_entries", value: false}});
  expect(calls).toContainEqual({method: "settings.set", params: {field: "social_refresh", value: "0"}});
});

test("older daemons expose supported settings and retain basic save controls", async ({ page }) => {
  const {frame} = await openStore(page);
  await frame.locator("body").evaluate(() => {
    const w = window as any;
    w.testErrors["settings.inspect"] = {code: -32601, message: "Method not found"};
    w.testSettings = {theme: "dark", relays: ["wss://older.example"]};
  });
  await frame.locator('.tab[data-tab="settings"]').click();
  await expect(frame.locator(".setting-card")).toHaveCount(2);
  await expect(frame.locator("#settings-reload")).toBeHidden();
  await expect(frame.getByRole("button", {name: "Reset to configured value"}).first()).toBeHidden();
  await frame.locator("#setting-relays").fill("wss://new.example");
  const card = frame.locator('[data-field="relays"]');
  await card.getByRole("button", {name: "Save", exact: true}).click();
  await expect(card.locator(".setting-message")).toHaveText("Saved");
});

test("a successful write followed by a failed read is reported as saved", async ({ page }) => {
  const {frame} = await openStore(page);
  await frame.locator('.tab[data-tab="settings"]').click();
  await frame.locator("#setting-relays").fill("wss://saved.example");
  await frame.locator("body").evaluate(() => { (window as any).failReadAfterSave = true; });
  const card = frame.locator('[data-field="relays"]');
  await card.getByRole("button", {name: "Save", exact: true}).click();
  await expect(card.locator(".setting-message")).toContainText("Change saved");
  await expect(card.getByRole("button", {name: "Save", exact: true})).toBeDisabled();
  expect(await frame.locator("body").evaluate(() => (window as any).testSettings.relays)).toEqual(["wss://saved.example"]);
});

test("saving one field and reloading configuration preserve other unsaved edits", async ({ page }) => {
  const {frame} = await openStore(page);
  await frame.locator('.tab[data-tab="settings"]').click();
  await frame.locator("#setting-sync_id").fill("my-decks");
  await frame.locator("#setting-blossom_servers").fill("https://draft.example");
  const sync = frame.locator('[data-field="sync_id"]');
  await sync.getByRole("button", {name: "Save", exact: true}).click();
  await expect(sync.locator(".setting-message")).toHaveText("Saved");
  await expect(frame.locator("#setting-blossom_servers")).toHaveValue("https://draft.example");
  await frame.locator("#settings-reload").click();
  await expect(frame.locator("#toast")).toContainText("Press again to confirm");
  await frame.locator("#settings-reload").click();
  await expect.poll(() => frame.locator("body").evaluate(() => (window as any).testCalls.filter((c: any) => c.method === "settings.reload").length)).toBe(1);
  await expect(frame.locator("#settings-refresh")).toBeEnabled();
  await expect(frame.locator("#setting-blossom_servers")).toHaveValue("https://draft.example");
  await frame.locator("#setting-sync_id").fill("");
  await sync.getByRole("button", {name: "Save", exact: true}).click();
  await expect(sync.locator(".setting-message")).toHaveText("Saved");
  expect(await frame.locator("body").evaluate(() => (window as any).testCalls)).toContainEqual({method: "settings.set", params: {field: "sync_id", value: ""}});
});

test("settings loading can recover after the daemon is unavailable", async ({ page }) => {
  const {frame} = await openStore(page);
  await frame.locator("body").evaluate(() => { (window as any).testErrors["settings.inspect"] = {message: "Daemon unavailable"}; });
  await frame.locator('.tab[data-tab="settings"]').click();
  await expect(frame.locator("#settings-status")).toContainText("Daemon unavailable");
  await expect(frame.locator(".setting-card")).toHaveCount(0);
  await frame.locator("body").evaluate(() => { delete (window as any).testErrors["settings.inspect"]; });
  await frame.locator("#settings-refresh").click();
  await expect(frame.locator(".setting-card")).toHaveCount(8);
});
