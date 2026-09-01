#!/usr/bin/env node
import { spawnSync } from "child_process";
import { existsSync, readFileSync, readdirSync } from "fs";
import { dirname, resolve, join } from "path";
import { fileURLToPath } from "url";

const dir = dirname(fileURLToPath(import.meta.url));
const projectRoot = resolve(dir, "..");

const platformMap = { darwin: "darwin", linux: "linux", win32: "windows" };
const archMap = { x64: "amd64", arm64: "arm64", ia32: "386" };

function resolveBin() {
  if (process.env.PI_SWITCH_GO_BIN && existsSync(process.env.PI_SWITCH_GO_BIN)) {
    return process.env.PI_SWITCH_GO_BIN;
  }
  const goos = platformMap[process.platform] || process.platform;
  const goarch = archMap[process.arch] || process.arch;
  const ext = goos === "windows" ? ".exe" : "";
  const candidates = [
    join(dir, `pi-switch-${goos}-${goarch}${ext}`),
    join(dir, `pi-switch-${process.platform}-${process.arch}${ext}`),
    join(projectRoot, `pi-switch${ext}`),
    join(projectRoot, `pi-switch-go${ext}`),
  ];
  for (const p of candidates) {
    if (existsSync(p)) return p;
  }
  try {
    const files = readdirSync(dir);
    for (const f of files) {
      if (f.startsWith("pi-switch-") && !f.endsWith(".js") && !f.endsWith(".map")) {
        const full = join(dir, f);
        if (existsSync(full)) return full;
      }
    }
  } catch {}
  return null;
}

const bin = resolveBin();
if (!bin) {
  console.error(`Error: Go binary not found for ${process.platform}/${process.arch}.`);
  console.error(`Expected: bin/pi-switch-${platformMap[process.platform]||process.platform}-${archMap[process.arch]||process.arch}${(platformMap[process.platform]==='windows' || process.platform==='win32') ? '.exe' : ''}`);
  console.error(`Build with: npm run build:webui && go build -ldflags "-s -w" -o bin/pi-switch-${platformMap[process.platform]||process.platform}-${archMap[process.arch]||process.arch} ./cmd/pi-switch`);
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
