import { createHash } from "node:crypto";
import { mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import { parse } from "parse5";
import assert from "node:assert/strict";

const output = new URL("../dist/", import.meta.url);
const destination = new URL("../../built/", import.meta.url);
const html = await readFile(new URL("index.html", output));
const manifest = JSON.parse(await readFile(new URL(".nip5a-manifest.json", output), "utf8"));
const hash = createHash("sha256").update(html).digest("hex");
assert(manifest.tags.some(tag => tag[0] === "x" && tag[1] === hash), "Manifest must hash the final HTML bytes");
for (const domain of ["steamduck", "resource"]) {
  assert(manifest.tags.some(tag => tag[0] === "R" && tag[1] === domain), `Required domain must be declared: ${domain}`);
}
for (const domain of ["gamepad", "theme", "identity", "relay", "intent"]) {
  assert(manifest.tags.some(tag => tag[0] === "O" && tag[1] === domain), `Optional domain must be declared: ${domain}`);
}
const files = await readdir(output);
assert(files.every(file => ["index.html", ".nip5a-manifest.json", "binding"].includes(file)), `Unexpected runtime asset: ${files}`);
function check(node) {
  for (const attr of node.attrs || []) {
    if (["src", "srcset"].includes(attr.name) || node.tagName === "link" && attr.name === "href") {
      assert(attr.value.startsWith("data:"), `External runtime asset: ${attr.value}`);
    }
  }
  for (const child of node.childNodes || []) check(child);
  if (node.content) check(node.content);
}
check(parse(html.toString()));
for (const forbidden of ["steamduckCall", "X-Steamduck-Token", "navigator.getGamepads", "fetch(", "legacy-screens"]) {
  assert(!html.toString().includes(forbidden), `Legacy transport or template leaked into napplet: ${forbidden}`);
}
await mkdir(destination, { recursive: true });
await writeFile(new URL("napplet.html", destination), html);
console.log(`Verified napplet: self-contained HTML (${html.length} bytes), SHA-256 ${hash}`);
