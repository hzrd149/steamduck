import { expect, test, type FrameLocator, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";

const prelude = readFileSync(new URL("../../built/steamduck.js", import.meta.url), "utf8");
const extension = readFileSync(new URL("./fixtures/kwakore-extension.js", import.meta.url), "utf8");
const app = readFileSync(new URL("../../built/napplet.html", import.meta.url), "utf8");
const metadata = { version: 1, methods: ["status", "installed", "icon", "sync"], kwakore_protocol_version: 1 };

async function setup(page: Page, withApp = false) {
  await page.setContent('<iframe id="store" sandbox="allow-scripts" style="width:1280px;height:800px"></iframe>');
  await page.evaluate(({ withApp, metadata }) => {
    (window as any).requests = [];
    (window as any).ports = [];
    window.addEventListener("message", (event) => {
      const frame = document.querySelector<HTMLIFrameElement>("#store")!;
      if (event.source !== frame.contentWindow || event.data?.type !== "__kwakore.document" || !event.ports[0]) return;
      const port = event.ports[0];
      (window as any).ports.push(port);
      (window as any).activePort = port;
      port.onmessage = (event) => {
        if (!event.data?.type?.startsWith("steamduck.")) return;
        (window as any).requests.push(event.data);
        if (!withApp) return;
        const message = event.data;
        let result: unknown = null;
        if (message.type === "steamduck.info") result = metadata;
        else if (message.method === "status") result = { health: { ready: true, version: "real-binding" } };
        else if (message.method === "installed") result = {items:[{
          address: "35129:" + "a".repeat(64) + ":demo", name: "Bound game",
          available: true, compatible: true, format: "napplet", update_available: null,
          version: {created_at: 1},
        }], total:1, next_offset:null};
        else if (message.method === "icon") result = "";
        else if (message.method === "sync") result = {deferred: true, shortcut_count: 1};
        port.postMessage({type: message.type + ".result", id: message.id, result});
      };
    });
  }, {withApp, metadata});
  const document = '<meta http-equiv="Content-Security-Policy" content="default-src \'none\'; script-src \'unsafe-inline\'; style-src \'unsafe-inline\'; img-src data: blob:">' +
    '<script>window.results = {}; window.napplet = {gamepad: {available:true, focused:true, getGamepads(){return []}}};</script>' +
    `<script>(function(){${extension}
parent.postMessage({type:"__kwakore.document"},"*",[extensionChannel.port2]);
${prelude}})()</script>` + (withApp ? app : "<body>Binding test</body>");
  await page.locator("#store").evaluate((frame, html) => { (frame as HTMLIFrameElement).srcdoc = html; }, document);
  const frame = page.frameLocator("#store");
  await expect.poll(() => frame.locator("body").evaluate(() => !!(window as any).napplet?.steamduck)).toBe(true);
  return { frame, document };
}

async function call(frame: FrameLocator, key: string, operation: "info" | "call" | "kwakore", method?: string, params?: unknown) {
  await frame.locator("body").evaluate((_, value) => {
    const api = (window as any).napplet.steamduck;
    const promise = value.operation === "info" ? api.info() : value.operation === "kwakore" ?
      api.kwakore.call(value.method, value.params) : api.call(value.method, value.params);
    promise.then((result: unknown) => { (window as any).results[value.key] = {ok: true, result}; }, (error: any) => {
      (window as any).results[value.key] = {ok: false, error: {
        code: error.code, message: error.message, reason: error.reason, data: error.data,
      }};
    });
  }, { key, operation, method, params });
}

const requests = (page: Page): Promise<any[]> => page.evaluate(() => (window as any).requests);
const results = (frame: FrameLocator) => frame.locator("body").evaluate(() => (window as any).results);
async function reply(page: Page, envelope: object, port?: number) {
  await page.evaluate(({message, port}) => {
    const channel = port === undefined ? (window as any).activePort : (window as any).ports[port];
    channel.postMessage(message);
  }, {message: envelope, port});
}

test("all operations use unique IDs and correctly settle out-of-order results", async ({ page }) => {
  const {frame} = await setup(page);
  await call(frame, "info", "info");
  await call(frame, "store", "call", "installed");
  await call(frame, "direct", "kwakore", "service.status", {probe: true});
  await expect.poll(async () => (await requests(page)).length).toBe(3);
  const sent = await requests(page);
  expect(sent.map((message) => message.type)).toEqual(["steamduck.info", "steamduck.request", "steamduck.kwakore.request"]);
  expect(new Set(sent.map((message) => message.id)).size).toBe(3);
  expect(sent.every((message) => typeof message.id === "string" && message.id.length <= 128)).toBe(true);
  expect(sent[0]).not.toHaveProperty("params");
  expect(sent[1].params).toBeNull();
  expect(sent[2].params).toEqual({probe: true});
  await reply(page, {type: sent[2].type + ".result", id: sent[2].id, result: {protocol_version: 1}});
  await reply(page, {type: sent[0].type + ".result", id: sent[0].id, result: metadata});
  await reply(page, {type: sent[1].type + ".result", id: sent[1].id, result: null});
  await expect.poll(() => results(frame)).toMatchObject({
    info: {ok: true, result: metadata}, store: {ok: true, result: null}, direct: {ok: true, result: {protocol_version: 1}},
  });
  expect(await frame.locator("body").evaluate(() => {
    const api = (window as any).napplet.steamduck;
    return {gamepad: !!(window as any).napplet.gamepad, frozen: Object.isFrozen(api),
      installer: typeof (window as any).installSteamduckBinding};
  })).toEqual({gamepad: true, frozen: true, installer: "undefined"});
});

test("self-originated, unknown and wrong-type responses cannot resolve a call", async ({ page }) => {
  const {frame} = await setup(page);
  await call(frame, "sync", "call", "sync");
  await expect.poll(async () => (await requests(page)).length).toBe(1);
  const message = (await requests(page))[0];
  const response = {type: message.type + ".result", id: message.id, result: "spoof"};
  await frame.locator("body").evaluate((_, response) => { window.postMessage(response, "*"); }, response);
  await reply(page, {...response, id: "unknown"});
  await reply(page, {...response, type: "steamduck.info.result"});
  await frame.locator("body").evaluate(async () => {
    for (let i = 0; i < 3; i++) await new Promise(requestAnimationFrame);
  });
  expect(await results(frame)).toEqual({});
  await reply(page, {...response, result: "accepted"});
  await expect.poll(() => results(frame)).toEqual({sync: {ok: true, result: "accepted"}});
  await reply(page, {...response, result: "duplicate"});
  expect(await results(frame)).toEqual({sync: {ok: true, result: "accepted"}});
});

test("structured errors retain their codes, reasons and data; malformed results fail safely", async ({ page }) => {
  const {frame} = await setup(page);
  await call(frame, "error", "call", "napplet.comment");
  await call(frame, "bad", "call", "sync");
  await call(frame, "metadata", "info");
  await expect.poll(async () => (await requests(page)).length).toBe(3);
  const sent = await requests(page);
  await reply(page, {type: sent[0].type + ".result", id: sent[0].id,
    error: {code: 1004, message: "Unavailable", data: {reason: "signer_unavailable"}}});
  await reply(page, {type: sent[1].type + ".result", id: sent[1].id, error: {code: "malformed", message: "Unavailable"}});
  await reply(page, {type: sent[2].type + ".result", id: sent[2].id, result: {version: 1}});
  await expect.poll(() => results(frame)).toMatchObject({
    error: {ok: false, error: {code: 1004, message: "Unavailable", reason: "signer_unavailable", data: {reason: "signer_unavailable"}}},
    bad: {ok: false, error: {code: -32603, message: "Internal error"}},
    metadata: {ok: false, error: {code: -32603, message: "Internal error"}},
  });
});

test("invalid JSON parameters and takeover operations are rejected before posting", async ({ page }) => {
  const {frame} = await setup(page);
  await frame.locator("body").evaluate(async () => {
    const api = (window as any).napplet.steamduck;
    const cyclic: any = {}; cyclic.self = cyclic;
    const calls = [
      () => api.call("", null), () => api.call("test", []), () => api.call("test", cyclic),
      () => api.call("test", {secret: () => "do not reflect"}), () => api.call("test", {value: undefined}),
      () => api.call("test", {value: Infinity}), () => api.call("test", {value: 1n}),
      () => api.kwakore.call("events.subscribe"), () => api.kwakore.call("napplet.launch", {attach: true}),
      () => api.kwakore.call("runtime.builtin.open", {}),
    ];
    (window as any).validation = await Promise.all(calls.map((run) => run().catch((error: any) => ({code: error.code, message: error.message}))));
  });
  expect(await frame.locator("body").evaluate(() => (window as any).validation)).toEqual([
    ...Array.from({length: 7}, () => ({code: -32602, message: "Invalid params"})),
    ...Array.from({length: 3}, () => ({code: 1100, message: "Unsupported operation"})),
  ]);
  expect(await requests(page)).toEqual([]);
});

test("long request budgets are respected without mutation replay", async ({ page }) => {
  await page.clock.install();
  const {frame} = await setup(page);
  await call(frame, "info", "info");
  await call(frame, "pair", "call", "signer.pair.wait");
  await call(frame, "install", "call", "install", {address: "example"});
  await expect.poll(async () => (await requests(page)).length).toBe(3);
  await page.clock.runFor(35_001);
  await expect.poll(() => results(frame)).toMatchObject({info: {ok: false, error: {code: 1008, message: "Timeout"}}});
  expect(await results(frame)).not.toHaveProperty("pair");
  await page.clock.runFor(105_000);
  await expect.poll(() => results(frame)).toMatchObject({pair: {ok: false, error: {code: 1008}}});
  expect(await results(frame)).not.toHaveProperty("install");
  await page.clock.runFor(45_000);
  await expect.poll(() => results(frame)).toMatchObject({install: {ok: false, error: {code: 1008}}});
  const sent = await requests(page);
  expect(sent).toHaveLength(3);
  await reply(page, {type: sent[2].type + ".result", id: sent[2].id, result: "late success"});
  expect(await results(frame)).toMatchObject({install: {ok: false, error: {code: 1008}}});
});

test("closing rejects pending and future calls; a new document ignores old responses", async ({ page }) => {
  const {frame, document} = await setup(page);
  await call(frame, "pending", "call", "sync");
  await expect.poll(async () => (await requests(page)).length).toBe(1);
  const old = (await requests(page))[0];
  await frame.locator("body").evaluate(() => window.dispatchEvent(new Event("pagehide")));
  await call(frame, "future", "info");
  await expect.poll(() => results(frame)).toMatchObject({pending: {ok: false, error: {code: 1007}}, future: {ok: false, error: {code: 1007}}});
  expect(await requests(page)).toHaveLength(1);
  await page.locator("#store").evaluate((frame, html) => { (frame as HTMLIFrameElement).srcdoc = html; }, document);
  await expect.poll(() => results(frame)).toEqual({});
  await call(frame, "new", "call", "sync");
  await expect.poll(async () => (await requests(page)).length).toBe(2);
  const fresh = (await requests(page))[1];
  // IDs may repeat in a fresh realm; private document ports isolate replies.
  await reply(page, {type: old.type + ".result", id: old.id, result: "stale"}, 0);
  await reply(page, {type: fresh.type + ".result", id: fresh.id, result: "fresh"});
  await expect.poll(() => results(frame)).toEqual({new: {ok: true, result: "fresh"}});
});

test("pending work is bounded and excess calls fail without dispatch", async ({ page }) => {
  const {frame} = await setup(page);
  await frame.locator("body").evaluate(() => {
    for (let i = 0; i < 65; i++) (window as any).napplet.steamduck.call("sync").catch((error: any) => {
      (window as any).results[i] = {code: error.code, message: error.message};
    });
  });
  await expect.poll(async () => (await requests(page)).length).toBe(64);
  await expect.poll(() => results(frame)).toEqual({64: {code: 1003, message: "Busy"}});
});

test("the production store runs through the actual injected prelude and NAP envelopes", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const {frame} = await setup(page, true);
  await expect(frame.locator("#library-grid .card .name")).toHaveText("Bound game");
  await expect(frame.locator("#status-text")).toHaveText("kwakore real-binding");
  await frame.locator("#sync").click();
  await expect(frame.locator("#toast")).toContainText("Steam is running");
  expect((await requests(page)).some((message) => message.type === "steamduck.request" && message.method === "sync")).toBe(true);
  expect(errors).toEqual([]);
});
