import { chromium } from "playwright";
import { mkdtemp, mkdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import { spawn, spawnSync, type ChildProcess } from "node:child_process";

const root = new URL("..", import.meta.url).pathname.replace(/\/$/, "");
const joker = process.env.JOKER_BIN || join(root, ".cache", "tmp", "go-joker-screenshot");
const sourceNotebook = process.env.NOTEBOOK_SOURCE || "demo";
const defaultName = sourceNotebook === "demo" ? "rich-demo" : basename(sourceNotebook).replace(/\.edn$/, "");
const out = process.env.NOTEBOOK_SCREENSHOT || join(root, ".cache", "screenshots", `${defaultName}-full-page.png`);
const port = Number(process.env.NOTEBOOK_SCREENSHOT_PORT || 19180 + Math.floor(Math.random() * 1000));
const addr = `127.0.0.1:${port}`;
const url = `http://${addr}/`;

function run(args: string[], cwd = root) {
  const result = spawnSync(joker, args, {
    cwd,
    encoding: "utf8",
    env: { ...process.env, TMPDIR: join(root, ".cache", "tmp"), GOTMPDIR: join(root, ".cache", "gotmp") },
  });
  if (result.status !== 0) throw new Error(`${joker} ${args.join(" ")} failed\n${result.stdout}\n${result.stderr}`);
  return result.stdout;
}

async function waitForServer(child: ChildProcess) {
  const deadline = Date.now() + 10_000;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) throw new Error(`notebook server exited early with ${child.exitCode}`);
    try {
      const res = await fetch(url, { cache: "no-store" });
      if (res.ok) return;
    } catch {}
    await new Promise((resolve) => setTimeout(resolve, 150));
  }
  throw new Error(`notebook server did not start at ${url}`);
}

const work = await mkdtemp(join(tmpdir(), "joker-notebook-shot-"));
let server: ChildProcess | undefined;
let browser: Awaited<ReturnType<typeof chromium.launch>> | undefined;
try {
  const notebook = join(work, `${defaultName}.edn`);
  if (sourceNotebook === "demo") {
    run(["notebook", "demo", notebook]);
  } else {
    run(["notebook", "run", sourceNotebook, "--no-save", "--summary", "--fail-on-error"]);
    const cp = spawnSync("cp", [sourceNotebook, notebook], { cwd: root, encoding: "utf8" });
    if (cp.status !== 0) throw new Error(`copy failed\n${cp.stdout}\n${cp.stderr}`);
  }
  run(["notebook", "run", notebook, "--summary", "--fail-on-error"]);
  server = spawn(joker, ["notebook", notebook, "--addr", addr, "--token", "shot"], {
    cwd: root,
    stdio: ["ignore", "pipe", "pipe"],
    env: { ...process.env, TMPDIR: join(root, ".cache", "tmp"), GOTMPDIR: join(root, ".cache", "gotmp") },
  });
  await waitForServer(server);
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1440, height: 1800 }, deviceScaleFactor: 1 });
  await page.goto(url, { waitUntil: "domcontentloaded" });
  await page.waitForSelector("#cells .cell");
  await page.locator("button", { hasText: "Show dependency graph" }).click();
  await page.waitForSelector("#dependency-graph svg");
  await mkdir(join(root, ".cache", "screenshots"), { recursive: true });
  await mkdir(join(root, "docs", "images"), { recursive: true });
  await page.screenshot({ path: out, fullPage: true });
  console.log(out);
} finally {
  if (browser) await browser.close();
  if (server && server.exitCode === null) server.kill("SIGTERM");
  await rm(work, { recursive: true, force: true });
}
