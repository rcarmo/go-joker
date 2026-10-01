// One-time, idempotent import-path migration, including generated Go imports.
// This does not regenerate bootstrap payloads or modify repository URLs.
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../../", import.meta.url));
const oldPath = "github.com/rcarmo/go-joker";
const newPath = `${oldPath}/v42`;
const files = execFileSync("git", ["ls-files", "-z"], { cwd: root, encoding: "utf8" }).split("\0");
let changed = 0;
for (const file of files) {
  if (!(file.endsWith(".go") || file === "go.mod" || file === "std/generate-std.joke")) continue;
  const path = `${root}/${file}`;
  const before = readFileSync(path, "utf8");
  const after = file === "go.mod"
    ? before.replace(/^module github\.com\/rcarmo\/go-joker$/m, `module ${newPath}`)
    : before.replace(/(?<!https?:\/\/)github\.com\/rcarmo\/go-joker\/(?!v42\/)/g, `${newPath}/`);
  if (before !== after) {
    writeFileSync(path, after);
    changed++;
  }
}
console.log(`Migrated ${changed} module/import files to ${newPath}`);
