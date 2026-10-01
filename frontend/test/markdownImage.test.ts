import assert from "node:assert/strict";
import test from "node:test";
import { markdownUploadPath } from "../src/components/markdownImage.ts";

test("uploads served by the local API render as images", () => {
  assert.equal(markdownUploadPath("/files/3f2a9c1e-0b7d-4e55-9a51-2c6f0e1d8a42.png"), "/files/3f2a9c1e-0b7d-4e55-9a51-2c6f0e1d8a42.png");
  assert.equal(markdownUploadPath("/files/a_b-c.d.jpeg"), "/files/a_b-c.d.jpeg");
});

test("any src that leaves the machine or the /files route is not fetched", () => {
  const blocked = [
    undefined,
    "",
    "https://evil.example/p.png?d=secret",
    "http://evil.example/p.png",
    "HTTPS://evil.example/p.png",
    "//evil.example/p.png",
    "\\\\evil.example\\p.png",
    "http://127.0.0.1:8787/files/x.png",
    "data:image/png;base64,AAAA",
    "blob:wails://wails/1234",
    "files/x.png",
    "./files/x.png",
    "/files/",
    "/files/.",
    "/files/..",
    "/files/%2e%2e",
    "/files/.%2e/api/projects",
    "/files/../api/projects",
    "/files/a/b.png",
    "/files/a.png?t=x",
    "/files/a.png#x",
    " /files/a.png",
    "/files/a.png\n",
    "/api/projects"
  ];
  for (const src of blocked) {
    assert.equal(markdownUploadPath(src), null, `src ${JSON.stringify(src)}`);
  }
});
