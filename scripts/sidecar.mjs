#!/usr/bin/env node
// Fetches the pinned llama.cpp release and stages its llama-server where the app
// looks for it (internal/embed/sidecar_*.go):
//
//   (default)  build/sidecar/<GOOS>-<GOARCH>/   devServerBinaryPath, for `pnpm dev`
//   --app      macOS: build/bin/<name>.app/Contents/Resources/
//              Windows: build/bin/ (beside the exe)
//                                               defaultServerBinaryPath, for the
//                                               output of `pnpm build:app`
//   --arch     arm64 | amd64; defaults to the CPU Node runs on
//
// This file is the one place the sidecar's llama.cpp release is pinned: CI and
// scripts/build-mac-signed.sh call it instead of downloading on their own, so a
// version bump is RELEASE plus the sha256 values below.
//
// It is Node rather than shell for the same reason as scripts/wails.mjs: it has
// to run from PowerShell on Windows without WSL. Extraction uses the OS's own
// bsdtar, which reads both the macOS .tar.gz and the Windows .zip, so it needs no
// npm dependency.

import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  copyFileSync,
  createReadStream,
  createWriteStream,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  readlinkSync,
  renameSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { basename, dirname, join } from "node:path";
import { Readable } from "node:stream";
import { pipeline } from "node:stream/promises";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";

// Must contain llama.cpp's ModernBERT graph (>= b9437), which ruri-v3 needs.
const RELEASE = "b11126";

// Keyed by Go's GOOS-GOARCH, the same naming as build/sidecar/<GOOS>-<GOARCH>/.
const ASSETS = {
  "darwin-arm64": {
    file: `llama-${RELEASE}-bin-macos-arm64.tar.gz`,
    sha256: "5adfb8e114b5b875a319029d6980e414242ce693e0f51b6d2169f325f5506476",
  },
  "darwin-amd64": {
    file: `llama-${RELEASE}-bin-macos-x64.tar.gz`,
    sha256: "6032d4d94ef80bcb0ea8d9b10f9912381ace983c5887f075fbe565ca739f8713",
  },
  "windows-amd64": {
    file: `llama-${RELEASE}-bin-win-cpu-x64.zip`,
    sha256: "88b6648aa8a96c751a5279cff96ad79cb6070bc3ef2b4f77fc10ea4c29909d1c",
  },
  "windows-arm64": {
    file: `llama-${RELEASE}-bin-win-cpu-arm64.zip`,
    sha256: "6ca055d307664d95c30a0c68aa2a23f78d7c242b09f1980325fca695e71e8c95",
  },
};

const GOOS = { darwin: "darwin", win32: "windows" };
const GOARCH = { arm64: "arm64", x64: "amd64" };

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const sidecarRoot = join(repoRoot, "build", "sidecar");
const downloadDir = join(sidecarRoot, ".downloads");
const binDir = join(repoRoot, "build", "bin");
// Records which archive a directory was staged from, so a re-run with the same
// pin is a no-op.
const markerName = ".llama-server-release";

function fail(message) {
  console.error(`sidecar: ${message}`);
  process.exit(1);
}

function parseOptions() {
  const { values } = parseArgs({
    options: {
      app: { type: "boolean", default: false },
      arch: { type: "string" },
      help: { type: "boolean", short: "h", default: false },
    },
  });
  if (values.help) {
    console.log("usage: pnpm sidecar [--app] [--arch arm64|amd64]");
    process.exit(0);
  }
  const goos = GOOS[process.platform];
  if (!goos) {
    fail(`unsupported OS ${process.platform}; the sidecar is bundled for macOS and Windows only`);
  }
  const goarch = values.arch ?? GOARCH[process.arch];
  const target = `${goos}-${goarch}`;
  if (!ASSETS[target]) {
    fail(`no llama.cpp ${RELEASE} asset pinned for ${target} (known: ${Object.keys(ASSETS).join(", ")})`);
  }
  return { goos, target, app: values.app };
}

function resolveDestination(goos, target, app) {
  if (!app) {
    return join(sidecarRoot, target);
  }
  if (!existsSync(binDir)) {
    fail(`${binDir} does not exist; run \`pnpm build:app\` first`);
  }
  if (goos === "windows") {
    return binDir;
  }
  const bundles = readdirSync(binDir).filter((name) => name.endsWith(".app"));
  if (bundles.length !== 1) {
    fail(`expected exactly one .app in ${binDir}, found ${bundles.length}; run \`pnpm build:app\` first`);
  }
  return join(binDir, bundles[0], "Contents", "Resources");
}

async function sha256Of(path) {
  const hash = createHash("sha256");
  await pipeline(createReadStream(path), hash);
  return hash.digest("hex");
}

// Returns a local copy of the archive whose sha256 matches the pin, reusing an
// earlier download so that re-staging into a freshly built app stays offline.
async function fetchArchive({ file, sha256 }) {
  const archive = join(downloadDir, file);
  if (existsSync(archive) && (await sha256Of(archive)) === sha256) {
    return archive;
  }
  mkdirSync(downloadDir, { recursive: true });
  const url = `https://github.com/ggml-org/llama.cpp/releases/download/${RELEASE}/${file}`;
  console.log(`sidecar: downloading ${url}`);
  const response = await fetch(url);
  if (!response.ok) {
    fail(`download failed: HTTP ${response.status} for ${url}`);
  }
  const partial = `${archive}.part`;
  await pipeline(Readable.fromWeb(response.body), createWriteStream(partial));
  const actual = await sha256Of(partial);
  if (actual !== sha256) {
    rmSync(partial, { force: true });
    fail(
      `sha256 mismatch for ${file}\n  expected ${sha256}\n  actual   ${actual}\n` +
        "The archive was discarded. If the pin was changed on purpose, update ASSETS in scripts/sidecar.mjs.",
    );
  }
  renameSync(partial, archive);
  return archive;
}

function extract(archive, into) {
  // Windows' own tar.exe is bsdtar and reads .zip. Git for Windows puts a GNU tar
  // on PATH in some shells, which cannot, so the system copy is named explicitly.
  const tar =
    process.platform === "win32"
      ? join(process.env.SystemRoot ?? "C:\\Windows", "System32", "tar.exe")
      : "tar";
  const result = spawnSync(tar, ["-xf", archive, "-C", into], { stdio: "inherit" });
  if (result.error || result.status !== 0) {
    fail(`extracting ${archive} with ${tar} failed${result.error ? `: ${result.error.message}` : ""}`);
  }
}

function findServerDir(root, serverName) {
  const hit = readdirSync(root, { recursive: true }).find((entry) => basename(entry) === serverName);
  if (!hit) {
    fail(`${serverName} not found in the extracted archive`);
  }
  return dirname(join(root, hit));
}

// Only llama-server and the shared libraries it loads are staged. The release
// ships ~25 other tools, and on macOS any unsigned extra executable fails
// notarization. Its LICENSE is left out too, because in build/bin it would
// overwrite the app's own; THIRD_PARTY_NOTICES.md carries llama.cpp's license.
function isStaged(name, serverName) {
  return name === serverName || name.endsWith(".dylib") || name.endsWith(".dll");
}

function stage(fromDir, toDir, serverName) {
  let count = 0;
  for (const name of readdirSync(fromDir)) {
    if (!isStaged(name, serverName)) {
      continue;
    }
    const from = join(fromDir, name);
    const to = join(toDir, name);
    rmSync(to, { force: true });
    // The macOS dylibs are version-suffixed files behind relative symlinks
    // (libggml.dylib -> libggml.0.dylib); recreate the links as they are.
    if (lstatSync(from).isSymbolicLink()) {
      symlinkSync(readlinkSync(from), to);
    } else {
      copyFileSync(from, to);
    }
    count++;
  }
  return count;
}

async function main() {
  const { goos, target, app } = parseOptions();
  const asset = ASSETS[target];
  const serverName = goos === "windows" ? "llama-server.exe" : "llama-server";
  const dest = resolveDestination(goos, target, app);
  const marker = join(dest, markerName);
  const stamp = `${RELEASE} ${target} ${asset.sha256}\n`;

  if (
    existsSync(join(dest, serverName)) &&
    existsSync(marker) &&
    readFileSync(marker, "utf8") === stamp
  ) {
    console.log(`sidecar: llama.cpp ${RELEASE} (${target}) is already staged in ${dest}`);
    return;
  }

  const archive = await fetchArchive(asset);
  mkdirSync(sidecarRoot, { recursive: true });
  const scratch = mkdtempSync(join(sidecarRoot, ".extract-"));
  try {
    extract(archive, scratch);
    const fromDir = findServerDir(scratch, serverName);
    // The dev directory belongs to this script, so it is cleared to drop a
    // previous release's differently-versioned dylibs. The app destinations hold
    // the app's own files and are rebuilt by `wails build -clean` instead.
    if (!app) {
      rmSync(dest, { recursive: true, force: true });
    }
    mkdirSync(dest, { recursive: true });
    rmSync(marker, { force: true });
    const count = stage(fromDir, dest, serverName);
    writeFileSync(marker, stamp);
    console.log(`sidecar: staged llama.cpp ${RELEASE} (${target}, ${count} files) into ${dest}`);
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
}

await main();
