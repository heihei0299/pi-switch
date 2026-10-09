import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { chmodSync, copyFileSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import test from "node:test";

const launcher = fileURLToPath(new URL("../bin/pi-switch.js", import.meta.url));

function makeFixture(t) {
  const root = mkdtempSync(join(tmpdir(), "pi-switch-launcher-"));
  const binDir = join(root, "bin");
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(binDir, { recursive: true });
  copyFileSync(launcher, join(binDir, "pi-switch.js"));
  return { root, binDir };
}

function addBinary(binDir, name, output) {
  const path = join(binDir, name);
  writeFileSync(path, `#!/usr/bin/env node\nprocess.stdout.write(${JSON.stringify(output)});\n`);
  chmodSync(path, 0o755);
}

function runLauncher(binDir, platform, arch, extraEnv = {}) {
  const source = [
    `Object.defineProperty(process, "platform", { value: ${JSON.stringify(platform)} });`,
    `Object.defineProperty(process, "arch", { value: ${JSON.stringify(arch)} });`,
    `await import(${JSON.stringify(pathToFileURL(join(binDir, "pi-switch.js")).href)});`,
  ].join("\n");
  const env = { ...process.env, ...extraEnv };
  if (!("PI_SWITCH_GO_BIN" in extraEnv)) delete env.PI_SWITCH_GO_BIN;
  return spawnSync(process.execPath, ["--input-type=module", "--eval", source], { encoding: "utf8", env });
}

test("selects the matching Linux binary", async (t) => {
  for (const [arch, target] of [["x64", "amd64"], ["arm64", "arm64"]]) {
    await t.test(`linux/${arch}`, (t) => {
      const { binDir } = makeFixture(t);
      addBinary(binDir, `pi-switch-linux-${target}`, `linux-${target}`);
      addBinary(binDir, "pi-switch-darwin-amd64", "wrong-platform");
      const result = runLauncher(binDir, "linux", arch);
      assert.equal(result.status, 0, result.stderr);
      assert.equal(result.stdout, `linux-${target}`);
    });
  }
});

test("rejects unsupported platform and architecture before using an override", async (t) => {
  for (const [platform, arch] of [["darwin", "x64"], ["win32", "x64"], ["linux", "ia32"]]) {
    await t.test(`${platform}/${arch}`, (t) => {
      const { binDir } = makeFixture(t);
      const result = runLauncher(binDir, platform, arch, { PI_SWITCH_GO_BIN: process.execPath });
      assert.equal(result.status, 1);
      assert.match(result.stderr, /supports Linux amd64\/arm64 only/);
    });
  }
});

test("does not fall back to a binary for another platform", (t) => {
  const { binDir } = makeFixture(t);
  addBinary(binDir, "pi-switch-darwin-amd64", "wrong-platform");
  const result = runLauncher(binDir, "linux", "x64");
  assert.equal(result.status, 1);
  assert.match(result.stderr, /Go binary not found/);
  assert.equal(result.stdout, "");
});
