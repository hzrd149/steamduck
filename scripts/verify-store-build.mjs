import { createHash } from "node:crypto";
import { mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import { parse } from "parse5";
import assert from "node:assert/strict";

const destination = new URL("../internal/store/built/", import.meta.url);
await mkdir(destination, { recursive: true });
for (const [directory, name] of [["store", "napplet"], ["store-legacy", "legacy"]]) {
  const output = new URL(`../dist/${directory}/`, import.meta.url);
  const html = await readFile(new URL("index.html", output));
  const manifest = JSON.parse(await readFile(new URL(".nip5a-manifest.json", output), "utf8"));
  const hash = createHash("sha256").update(html).digest("hex");
  assert(manifest.tags.some((tag) => tag[0] === "x" && tag[1] === hash), "Manifest must hash the final HTML bytes");
  assert(manifest.tags.some((tag) => tag[0] === "R" && tag[1] === "steamduck"), "Custom domain must be required");
  assert(manifest.tags.some((tag) => tag[0] === "R" && tag[1] === "resource"), "Image resource support must be declared");
  assert(manifest.tags.some((tag) => tag[0] === "O" && tag[1] === "gamepad"), "Controller support must be declared");
  assert(manifest.tags.some((tag) => tag[0] === "O" && tag[1] === "theme"), "Theme support must be declared");
  for (const domain of ["identity", "relay", "intent"]) {
    assert(manifest.tags.some((tag) => tag[0] === "O" && tag[1] === domain), `Account capability must be declared: ${domain}`);
  }
  const files = await readdir(output);
  assert(files.every((file) => file === "index.html" || file === ".nip5a-manifest.json"), `Unexpected runtime asset: ${files}`);
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
  if (name === "napplet") {
    for (const forbidden of ["steamduckCall", "X-Steamduck-Token", "navigator.getGamepads", "fetch("]) {
      assert(!html.toString().includes(forbidden), `Legacy transport leaked into napplet: ${forbidden}`);
    }
  }
  await writeFile(new URL(`${name}.html`, destination), html);
  console.log(`Verified ${name}: one self-contained HTML artifact (${html.length} bytes), SHA-256 ${hash}`);
}
