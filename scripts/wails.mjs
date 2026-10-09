#!/usr/bin/env node
// Runs `wails` with the Go toolchain pinned to an exact version.
//
// go.mod's `toolchain` directive is only a floor: a Go newer than it is used
// as-is, and if the Wails CLI's embedded x/tools cannot read that Go's export
// data, binding generation dies with `internal error: package "math" without
// types`. Only an exact GOTOOLCHAIN caps the version, and it has to come from
// somewhere the developer does not type each time — hence this launcher behind
// the package.json `dev` / `build:app` scripts.
//
// It is Node rather than an inline `GOTOOLCHAIN=… wails` in package.json
// because that prefix is POSIX shell syntax: pnpm runs scripts through cmd.exe
// on Windows, which would treat it as a command and fail before Wails starts.
//
// The pin is read from go.mod so that the floor and the cap cannot drift apart.
//
// It also stamps wails.json's info.productVersion into the binary
// (-ldflags -X main.appVersion), the only place the running app can read its own
// version from: Wails writes that field into Info.plist and the Windows resources,
// neither of which Go reads. release.yml checks the field against the tag, so a
// release build always reports the version it was tagged with.

import { spawn } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const goModPath = join(repoRoot, "go.mod");
const wailsJSONPath = join(repoRoot, "wails.json");

const toolchain = readFileSync(goModPath, "utf8").match(
  /^toolchain\s+(go\d[0-9A-Za-z.\-]*)\s*$/m,
)?.[1];

if (!toolchain) {
  console.error(
    `${goModPath}: no \`toolchain\` directive found — cannot pin the Go version.\n` +
      "Add one (e.g. `toolchain go1.27.1`) matching a Go the Wails CLI can read.",
  );
  process.exit(1);
}

const version = JSON.parse(readFileSync(wailsJSONPath, "utf8")).info?.productVersion;
if (!/^\d+\.\d+\.\d+$/.test(version ?? "")) {
  console.error(`${wailsJSONPath}: info.productVersion must be MAJOR.MINOR.PATCH, got ${JSON.stringify(version)}.`);
  process.exit(1);
}

const args = process.argv.slice(2);
if (args[0] === "build" || args[0] === "dev") {
  const stamp = `-X main.appVersion=${version}`;
  const i = args.indexOf("-ldflags");
  if (i >= 0 && i + 1 < args.length) {
    args[i + 1] = `${stamp} ${args[i + 1]}`;
  } else {
    args.push("-ldflags", stamp);
  }
}

const child = spawn("wails", args, {
  stdio: "inherit",
  env: { ...process.env, GOTOOLCHAIN: toolchain },
});

child.on("error", (err) => {
  const hint =
    err.code === "ENOENT"
      ? "\nIs the Wails CLI installed and on PATH? See README『必要なツール』."
      : "";
  console.error(`failed to run wails: ${err.message}${hint}`);
  process.exit(1);
});

// Preserve the child's exit status so CI and shells see the real result; a
// signalled death has no exit code, so report it as a generic failure.
child.on("exit", (code, signal) => process.exit(signal ? 1 : (code ?? 1)));
