import assert from "node:assert/strict";
import { mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import { Script } from "node:vm";

const output = new URL("../dist/store-binding/", import.meta.url);
assert.deepEqual(await readdir(output), ["steamduck.js"]);
// The host installs this as an inline preamble, not as a module or network asset.
const source = (await readFile(new URL("steamduck.js", output), "utf8")).replace(/<\/script/gi, "<\\/script");
new Script(source);
assert(!/<\/script/i.test(source), "An injected preamble cannot terminate its inline script");
for (const forbidden of ["steamduckCall", "X-Steamduck-Token", "fetch("]) {
  assert(!source.includes(forbidden), "Binding must use only the NAP message transport");
}
const destination = new URL("../internal/store/built/", import.meta.url);
await mkdir(destination, { recursive: true });
await writeFile(new URL("steamduck.js", destination), source);
console.log(`Verified injectable NAP-STEAMDUCK IIFE (${Buffer.byteLength(source)} bytes)`);
