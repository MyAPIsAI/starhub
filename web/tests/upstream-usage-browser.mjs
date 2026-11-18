// Uses an isolated database and a local upstream; no real credentials are used.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import { spawn, execFileSync } from "node:child_process";
import { createServer } from "node:http";
import { mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import os from "node:os";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const repo = path.resolve(web, "..");
const temp = await mkdtemp(path.join(os.tmpdir(), "starhub-upstream-usage-"));
const out = path.join(web, "playwright-report", "upstream-usage");
await mkdir(out, { recursive: true });
const report = { checks: [], screenshots: [], pageErrors: [] };
let backend, vite, browser, upstream;
let logs = "";
const hits = [];
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
try {
  upstream = createServer(async (req, res) => {
    const hit = { path: req.url, authorization: req.headers.authorization };
    hits.push(hit);
    res.setHeader("Content-Type", "application/json");
    if (req.url.startsWith("/unsupported/")) {
      res.writeHead(404);
      res.end("{}");
    } else if (req.url.startsWith("/auth/") || (req.url.startsWith("/partial/") && req.headers.authorization.endsWith("failed"))) {
      res.writeHead(403);
      res.end("{}");
    } else if (req.url.startsWith("/invalid/")) {
      res.end('{"success":false,"message":"fixture-private-upstream-error"}');
    } else {
      res.end(
        JSON.stringify({
          success: true,
          data: {
            total_used: 1595625560,
            total_available: 500000,
            total_granted: 1596125560,
          unlimited_quota: req.url.startsWith("/unlimited/"),
            expires_at: 0,
          },
        }),
      );
    }
  });
  await new Promise((resolve) => upstream.listen(0, "127.0.0.1", resolve));
  const upstreamBase = `http://127.0.0.1:${upstream.address().port}`;
  const binary = path.join(
    temp,
    process.platform === "win32" ? "starhub.exe" : "starhub",
  );
  execFileSync("go", ["build", "-o", binary, "."], {
    cwd: repo,
    stdio: "pipe",
  });
  const backendPort = await freePort();
  backend = spawn(binary, [], {
    cwd: temp,
    windowsHide: true,
    stdio: ["ignore", "pipe", "pipe"],
    env: {
      ...process.env,
      STARHUB_CONFIG: "",
      STARHUB_PORT: String(backendPort),
      STARHUB_BIND_ADDRESS: "127.0.0.1",
      STARHUB_DB: path.join(temp, "starhub.db"),
      STARHUB_COOKIE_SECURE: "false",
      STARHUB_ROOT_PASSWORD: "DiscoveryInitial92",
      STARHUB_ENV: "development",
      STARHUB_CRYPTO_SECRET: "model-discovery-fixture-secret",
      STARHUB_CREDENTIAL_KEYS: JSON.stringify({ fixture: "11".repeat(32) }),
      STARHUB_CREDENTIAL_ACTIVE_KEY: "fixture",
      RESOURCE_CATALOG_NOTIFY_URL: "",
      STARHUB_REGION_PROFILE: "",
    },
  });
  for (const stream of [backend.stdout, backend.stderr])
    stream.on("data", (chunk) => {
      logs = (logs + chunk).slice(-16000);
    });
  let ready = false;
  for (let i = 0; i < 80; i++) {
    try {
      ready = (await fetch(`http://127.0.0.1:${backendPort}/api/health`)).ok;
    } catch {}
    if (ready) break;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  assert.ok(ready, "isolated backend failed to start: " + logs);
  vite = await createViteServer({
    root: web,
    cacheDir: path.join(temp, "vite-cache"),
    logLevel: "error",
    server: {
      host: "127.0.0.1",
      port: await freePort(),
      proxy: { "/api": `http://127.0.0.1:${backendPort}` },
    },
  });
  await vite.listen();
  const base = `http://127.0.0.1:${vite.httpServer.address().port}`;
  browser = await chromium.launch({
    headless: true,
    ...(process.platform === "win32" ? { channel: "msedge" } : {}),
  });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
    reducedMotion: "reduce",
  });
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await context.addInitScript(() => localStorage.setItem("xy-language", "en"));
  const api = async (method, route, data) => {
    const response = await context.request.fetch(base + route, {
      method,
      data,
    });
    const body = await response.json();
    assert.equal(response.status(), 200, JSON.stringify(body));
    assert.equal(body.success, true, JSON.stringify(body));
    return body.data;
  };
  assert.equal(
    (
      await context.request.post(base + "/api/channel/fetch_models", {
        data: { type: 1, base_url: upstreamBase, key: "fixture-key" },
      })
    ).status(),
    401,
  );
  await api("POST", "/api/user/login", {
    username: "root",
    password: "DiscoveryInitial92",
  });
  assert.equal(
    (
      await context.request.post(base + "/api/channel/fetch_models", {
        data: { type: 1, base_url: upstreamBase, key: "fixture-key" },
      })
    ).status(),
    403,
  );
  await api("PUT", "/api/user/self", {
    current_password: "DiscoveryInitial92",
    new_password: "DiscoveryChanged93",
  });
  await api("POST", "/api/user/2fa/skip");
  report.checks.push(
    "discovery endpoint rejects anonymous and unverified staff sessions",
  );
  const page = await context.newPage();
  let channelWrites = 0;
  page.on("request", (request) => {
    if (
      ["POST", "PUT"].includes(request.method()) &&
      new URL(request.url()).pathname === "/api/channel"
    )
      channelWrites++;
  });
  page.setDefaultTimeout(15000);
  page.on("pageerror", (error) => report.pageErrors.push(String(error)));
  await page.goto(base + "/admin?tab=resources");
  await api("POST", "/api/channel", {
    name: "Usage fixture",
    type: 1,
    key: "sk-fixture" + "x".repeat(160),
    base_url: upstreamBase,
    models: "fixture-model",
    group: "default",
  });
  await page.reload();
  const row = page.getByRole("row").filter({ has: page.getByRole("cell", { name: "Usage fixture", exact: true }) });
  const dialog = page.getByRole("dialog");
  await row.getByRole("button", { name: "Reconcile", exact: true }).click();
  await dialog.waitFor();
  assert.match(await dialog.innerText(), /\$3,191\.2511/);
  await dialog.getByText("View exact amounts", {exact: true}).click();
  assert.match(await dialog.innerText(), /\$3,191\.251120000/);
  await dialog.getByText("View exact amounts", {exact: true}).click();
  assert.ok((await dialog.locator(".usage-keys tbody td").first().innerText()).length < 30);
  for (const language of ["en", "zh", "ar"]) {
    for (const theme of ["light", "dark"]) {
      await page.evaluate(
        async ({ language, theme }) => {
          const { usePreferences } = await import("/src/lib/preferences.ts");
          usePreferences.getState().setLanguage(language);
          usePreferences.getState().setTheme(theme);
        },
        { language, theme },
      );
      for (const width of [1440, 320]) {
        await page.setViewportSize({ width, height: 900 });
        await page.waitForFunction(() => {
          const box = document
            .querySelector('[role="dialog"]')
            .getBoundingClientRect();
          return box.x >= -1 && box.right <= innerWidth + 1;
        });
        const box = await dialog.boundingBox();
        assert.ok(
          box.x >= -1 && box.x + box.width <= width + 1,
          JSON.stringify({ language, theme, width, box }),
        );
        assert.notEqual(
          await dialog.evaluate(
            (node) => getComputedStyle(node).backgroundColor,
          ),
          "rgba(0, 0, 0, 0)",
        );
        assert.equal(await dialog.locator(".task-dialog-body").evaluate(node => node.scrollWidth > node.clientWidth + 1), false);
        await dialog.locator(".usage-note").scrollIntoViewIfNeeded();
        assert.ok(await dialog.getByRole("button").first().isVisible());
        await page.screenshot({
          path: path.join(out, `usage-${language}-${theme}-${width}.png`),
        });
      }
    }
  }
  const scan = await new AxeBuilder({page}).include(".upstream-usage").withTags(["wcag2a", "wcag2aa"]).analyze();
  assert.deepEqual(scan.violations, []);
  await page.keyboard.press("Escape");
  await dialog.waitFor({ state: "hidden" });
  assert.ok(await row.getByRole("button").first().count());
  report.checks.push(
    "usage dialog has an opaque background in three languages, two themes and desktop/narrow layouts; Escape closes it",
  );
  await page.evaluate(async () => {
    const { usePreferences } = await import("/src/lib/preferences.ts");
    usePreferences.getState().setLanguage("en");
  });
  await api("POST", "/api/channel", {
    name: "Ark usage fixture", type: 32, key: "fixture-ark-key",
    base_url: upstreamBase, models: "doubao-seedream-4-5-251128", group: "default",
  });
  await page.reload();
  const usageHits = hits.length;
  await page.getByRole("row").filter({hasText: "Ark usage fixture"}).getByRole("button", {name: "Reconcile", exact: true}).click();
  await dialog.getByText(/Ark usage queries are not integrated/).waitFor();
  assert.equal(hits.length, usageHits);
  assert.equal(await dialog.locator(".usage-summary strong").count(), 1);
  assert.equal(await dialog.locator(".usage-keys").count(), 0);
  for (const language of ["en", "zh", "ar"]) {
    await page.evaluate(async language => {
      const {usePreferences} = await import("/src/lib/preferences.ts");
      usePreferences.getState().setLanguage(language);
    }, language);
    assert.equal(await dialog.locator(".task-dialog-body").evaluate(node => node.scrollWidth > node.clientWidth + 1), false);
    await page.screenshot({path: path.join(out, `usage-ark-${language}-320.png`)});
  }
  await page.keyboard.press("Escape");
  await dialog.waitFor({state: "hidden"});
  await page.evaluate(async () => {
    const {usePreferences} = await import("/src/lib/preferences.ts");
    usePreferences.getState().setLanguage("en");
  });
  report.checks.push("Ark usage explains the capability boundary in three languages, preserves local usage and sends no upstream request");
  for (const [name, prefix, key] of [
    ["Unsupported fixture", "unsupported", "fixture-key"],
    ["Authentication fixture", "auth", "fixture-key"],
    ["Invalid usage fixture", "invalid", "fixture-key"],
    ["Partial fixture", "partial", "fixture-ok\nfixture-failed"],
    ["Unlimited fixture", "unlimited", "fixture-key"],
  ]) {
    await api("POST", "/api/channel", {name, type: 1, key, base_url: `${upstreamBase}/${prefix}`, models: "fixture-model", group: "default"});
  }
  await page.reload();
  for (const [name, expected, amountCount] of [
    ["Unsupported fixture", "compatible usage endpoint", 1],
    ["Authentication fixture", "authentication or permission failed", 1],
    ["Invalid usage fixture", "rejected the usage query", 1],
    ["Partial fixture", "Only some keys", 2],
    ["Unlimited fixture", "Unlimited", 3],
  ]) {
    const fixtureRow = page.getByRole("row").filter({has: page.getByRole("cell", {name, exact: true})});
    await fixtureRow.getByRole("button", {name: "Reconcile", exact: true}).click();
    await dialog.getByText(new RegExp(expected)).first().waitFor();
    assert.equal(await dialog.locator(".usage-summary strong").count(), amountCount);
    assert.equal((await dialog.innerText()).includes("fixture-private-upstream-error"), false);
    assert.equal(await dialog.locator(".task-dialog-body").evaluate(node => node.scrollWidth > node.clientWidth + 1), false);
    await page.screenshot({path: path.join(out, `usage-${name.replaceAll(" ", "-")}-320.png`)});
    await page.keyboard.press("Escape");
    await dialog.waitFor({state: "hidden"});
  }
  report.checks.push("unsupported endpoints, authentication failures, business errors, partial totals without differences and unlimited quota are rendered distinctly");
  await page.route("**/upstream-usage", (route) =>
    route.fulfill({
      status: 500,
      contentType: "application/json",
      body: JSON.stringify({
        success: false,
        message: "Fixture query failure",
      }),
    }),
  );
  await row.getByRole("button", { name: "Reconcile", exact: true }).click();
  await dialog.getByRole("alert").waitFor();
  assert.notEqual(
    await dialog.evaluate((node) => getComputedStyle(node).backgroundColor),
    "rgba(0, 0, 0, 0)",
  );
  await page.keyboard.press("Escape");
  await dialog.waitFor({ state: "hidden" });
  assert.equal(
    await row
      .getByRole("button", { name: "Reconcile", exact: true })
      .evaluate((node) => node === document.activeElement),
    true,
  );
  report.checks.push(
    "query error uses the shared modal and restores focus to the triggering button",
  );
  assert.deepEqual(report.pageErrors, []);
  console.log(JSON.stringify(report, null, 2));
} catch (error) {
  report.failure = String(error.stack || error);
  throw error;
} finally {
  await writeFile(
    path.join(out, "report.json"),
    JSON.stringify(report, null, 2),
  );
  await browser?.close();
  await vite?.close();
  backend?.kill();
  await new Promise((resolve) =>
    upstream ? upstream.close(resolve) : resolve(),
  );
}
