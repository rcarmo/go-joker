import { chromium } from "playwright";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawn, spawnSync, type ChildProcess } from "node:child_process";

const root = new URL("..", import.meta.url).pathname.replace(/\/$/, "");
const joker = process.env.JOKER_BIN || join(root, ".cache", "tmp", "go-joker-smoke");
const port = Number(process.env.NOTEBOOK_SMOKE_PORT || 18080 + Math.floor(Math.random() * 1000));
const addr = `127.0.0.1:${port}`;
const url = `http://${addr}/`;

function run(args: string[], cwd = root) {
  const result = spawnSync(joker, args, {
    cwd,
    encoding: "utf8",
    env: { ...process.env, TMPDIR: join(root, ".cache", "tmp"), GOTMPDIR: join(root, ".cache", "gotmp") },
  });
  if (result.status !== 0) {
    throw new Error(`${joker} ${args.join(" ")} failed\nstdout:\n${result.stdout}\nstderr:\n${result.stderr}`);
  }
  return result.stdout;
}

async function waitForServer(child: ChildProcess) {
  const deadline = Date.now() + 10_000;
  let last = "";
  while (Date.now() < deadline) {
    if (child.exitCode !== null) throw new Error(`notebook server exited early with ${child.exitCode}`);
    try {
      const res = await fetch(url, { cache: "no-store" });
      if (res.ok) return;
      last = `HTTP ${res.status}`;
    } catch (err) {
      last = String(err);
    }
    await new Promise((resolve) => setTimeout(resolve, 150));
  }
  throw new Error(`notebook server did not start at ${url}: ${last}`);
}

const work = await mkdtemp(join(tmpdir(), "joker-notebook-smoke-"));
let server: ChildProcess | undefined;
let browser: Awaited<ReturnType<typeof chromium.launch>> | undefined;
try {
  const notebook = join(work, "rich-demo.edn");
  run(["notebook", "demo", notebook]);
  const summary = run(["notebook", "run", notebook, "--summary", "--fail-on-error"]);
  if (!summary.includes('"success":true')) throw new Error(`rich demo run failed: ${summary}`);

  server = spawn(joker, ["notebook", notebook, "--addr", addr, "--token", "smoke"], {
    cwd: root,
    stdio: ["ignore", "pipe", "pipe"],
    env: { ...process.env, TMPDIR: join(root, ".cache", "tmp"), GOTMPDIR: join(root, ".cache", "gotmp") },
  });
  let stderr = "";
  server.stderr?.on("data", (chunk) => { stderr += String(chunk); });
  await waitForServer(server);

  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  page.on("pageerror", (err) => { throw err; });
  await page.goto(url, { waitUntil: "domcontentloaded" });
  await page.waitForSelector("#cells .cell");
  const cellCount = await page.locator("#cells .cell").count();
  if (cellCount !== 8) throw new Error(`expected 8 cells, got ${cellCount}`);

  await page.waitForSelector(".table-output table");
  await page.locator(".table-output .table-filter").fill("Ada");
  await page.waitForFunction(() => document.querySelector(".table-output")?.textContent?.includes("filtered from"));
  await page.locator(".table-output th").first().click();

  await page.locator("button", { hasText: "Export Markdown" }).click();
  await page.waitForFunction(() => document.querySelector("#raw")?.textContent?.includes("Joker notebook rich demo"));

  await page.locator("button", { hasText: "Show dependency graph" }).click();
  await page.waitForSelector("#dependency-graph svg");

  await page.locator("#notebook-title").fill("Smoke title");
  await page.locator("button", { hasText: "Save" }).click();
  await page.waitForFunction(() => document.querySelector("#notebook-dirty")?.textContent === "Saved");

  const status = await (await fetch(`${url}api/status`)).json();
  if (status.cellCount !== 8) throw new Error(`bad status from server: ${JSON.stringify(status)}`);
  console.log(JSON.stringify({ ok: true, url, cells: cellCount, stderr: stderr.trim() }));
} finally {
  if (browser) await browser.close();
  if (server && server.exitCode === null) server.kill("SIGTERM");
  await rm(work, { recursive: true, force: true });
}
