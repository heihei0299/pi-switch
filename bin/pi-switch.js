#!/usr/bin/env node
import { spawnSync } from "child_process";
import { existsSync, readFileSync } from "fs";
import { dirname, resolve, join } from "path";
import { fileURLToPath } from "url";

const dir = dirname(fileURLToPath(import.meta.url));
const projectRoot = resolve(dir, "..");

const archMap = { x64: "amd64", arm64: "arm64" };

if (process.platform !== "linux" || !archMap[process.arch]) {
  console.error(`Error: pi-switch CLI supports Linux amd64/arm64 only (got ${process.platform}/${process.arch}).`);
  process.exit(1);
}

function resolveBin() {
  if (process.env.PI_SWITCH_GO_BIN && existsSync(process.env.PI_SWITCH_GO_BIN)) {
    return process.env.PI_SWITCH_GO_BIN;
  }
  const candidates = [
    join(dir, `pi-switch-linux-${archMap[process.arch]}`),
    join(projectRoot, "pi-switch"),
  ];
  for (const p of candidates) {
    if (existsSync(p)) return p;
  }
  return null;
}

const bin = resolveBin();
if (!bin) {
  console.error(`Error: Go binary not found for Linux/${process.arch}.`);
  console.error(`Expected: bin/pi-switch-linux-${archMap[process.arch]}`);
  console.error(`Build with: npm run build:webui && npm run build:go:current`);
  console.error(`Or set PI_SWITCH_GO_BIN=/path/to/binary`);
  process.exit(1);
}

const args = process.argv.slice(2);
const result = spawnSync(bin, args, { stdio: "inherit" });
if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}
process.exit(result.status ?? 0);
