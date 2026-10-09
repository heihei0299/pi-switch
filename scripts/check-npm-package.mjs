import { spawnSync } from "node:child_process";

const pack = spawnSync("npm", ["pack", "--dry-run", "--json"], { encoding: "utf8" });
if (pack.error || pack.status !== 0) {
  process.stderr.write(pack.stderr || pack.error?.message || "npm pack dry-run failed\n");
  process.exit(pack.status || 1);
}

let report;
try {
  report = JSON.parse(pack.stdout);
} catch (error) {
  console.error(`cannot parse npm pack output: ${error.message}`);
  process.exit(1);
}

const files = report.flatMap((entry) => entry.files || []).map((entry) => entry.path);
const required = [
  "package.json",
  "bin/pi-switch.js",
  "bin/pi-switch-linux-amd64",
  "bin/pi-switch-linux-arm64",
];
const missing = required.filter((file) => !files.includes(file));
const supportedBinaries = new Set(required.slice(2));
const unsupportedBinaries = files.filter((file) => file.startsWith("bin/pi-switch-") && !supportedBinaries.has(file));
const forbidden = files.filter((file) => /(^|\/)(\.env|.*\.key$|.*secret.*|node_modules|\.git)(\/|$)/i.test(file));

if (missing.length || unsupportedBinaries.length || forbidden.length) {
  console.error({ missing, unsupportedBinaries, forbidden });
  process.exit(1);
}

console.log(`validated ${files.length} npm package files`);
