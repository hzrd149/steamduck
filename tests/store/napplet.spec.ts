import { expect, test, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";

const napplet = readFileSync(new URL("../../internal/store/built/napplet.html", import.meta.url), "utf8");
const legacy = readFileSync(new URL("../../internal/store/built/legacy.html", import.meta.url), "utf8");
const methods = ["status", "installed", "discover", "icon", "launch", "sync", "signer.status", "settings.get",
  "profile.get", "napplet.social", "settings.set", "signer.switch"];
const item = {
  address: "35129:" + "a".repeat(64) + ":demo",
  name: "Demo game", format: "napplet", available: true, compatible: true,
  version: { event_id: "b".repeat(64), created_at: 1, artifact_hash: "c".repeat(64) },
  update_available: null,
};

async function openStore(page: Page, options: { missing?: boolean; version?: number; legacy?: boolean } = {}) {
  const errors: string[] = [];
  const requests: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.route("**/*", (route) => { requests.push(route.request().url()); return route.abort(); });
  const prelude = `
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
        case "settings.get": return {theme: "dark"};
        case "settings.set": return null;
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
  await frame.locator('#theme [data-value="light"]').click();
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
