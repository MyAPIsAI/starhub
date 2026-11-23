// Exercise persistent binding creation from the actual page button against an isolated backend.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import { spawn, execFileSync } from "node:child_process";
import { mkdtemp } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import os from "node:os";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const temp = await mkdtemp(
  path.join(os.tmpdir(), "opencode", "current-channel-binding-"),
);
let backend, vite, browser, page;
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
try {
  const binary = path.join(
    temp,
    process.platform === "win32" ? "starhub.exe" : "starhub",
  );
  execFileSync("go", ["build", "-o", binary, "."], {
    cwd: path.resolve(web, ".."),
    stdio: "pipe",
  });
  const port = await freePort();
  backend = spawn(binary, [], {
    cwd: temp,
    windowsHide: true,
    stdio: "ignore",
    env: {
      ...process.env,
      STARHUB_CONFIG: "",
      STARHUB_PORT: String(port),
      STARHUB_BIND_ADDRESS: "127.0.0.1",
      STARHUB_DB: path.join(temp, "starhub.db"),
      STARHUB_COOKIE_SECURE: "false",
      STARHUB_ROOT_PASSWORD: "ChannelFixture92",
      STARHUB_ENV: "development",
      STARHUB_CRYPTO_SECRET: "platform-channel-browser-fixture-secret",
      STARHUB_CREDENTIAL_KEYS: JSON.stringify({ fixture: "11".repeat(32) }),
      STARHUB_CREDENTIAL_ACTIVE_KEY: "fixture",
      RESOURCE_CATALOG_NOTIFY_URL: "",
      STARHUB_REGION_PROFILE: "",
    },
  });
  let ready = false;
  for (let attempt = 0; attempt < 80; attempt++) {
    try {
      ready = (await fetch(`http://127.0.0.1:${port}/api/health`)).ok;
    } catch {}
    if (ready) break;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  assert.ok(ready, "isolated backend did not start");
  vite = await createViteServer({
    root: web,
    cacheDir: path.join(temp, "vite-cache"),
    logLevel: "error",
    server: {
      host: "127.0.0.1",
      port: await freePort(),
      proxy: { "/api": `http://127.0.0.1:${port}` },
    },
  });
  await vite.listen();
  const base = `http://127.0.0.1:${vite.httpServer.address().port}`;
  browser = await chromium.launch({
    headless: true,
    ...(process.platform === "win32" ? { channel: "chrome" } : {}),
  });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
    reducedMotion: "reduce",
  });
  await context.addInitScript(() => {
    if (!localStorage.getItem("xy-language"))
      localStorage.setItem("xy-language", "en");
  });
  const api = async (method, route, data) => {
    const response = await context.request.fetch(base + route, {
      method,
      data,
    });
    assert.equal(response.status(), 200, await response.text());
    const body = await response.json();
    assert.equal(body.success, true);
    return body.data;
  };
  await api("POST", "/api/user/login", {
    username: "root",
    password: "ChannelFixture92",
  });
  await api("PUT", "/api/user/self", {
    current_password: "ChannelFixture92",
    new_password: "ChannelChanged93",
  });
  await api("POST", "/api/user/2fa/skip");
  await api("PUT", "/api/option", { key: "ip_rate_limit", value: "10000" });
  page = await context.newPage();
  page.setDefaultTimeout(20000);
  const errors = [];
  page.on("pageerror", (error) => errors.push(String(error)));
  const openCreation = async () => {
    await page
      .locator("#gateway-bindings")
      .getByRole("button", { name: "New persistent binding", exact: true })
      .click();
    const dialog = page.getByRole("dialog", {
      name: "New persistent binding",
      exact: true,
    });
    await dialog.waitFor();
    return dialog;
  };
  await page.goto(base + "/console/channels");
  let dialog = await openCreation();
  await dialog
    .getByText(
      "No channels are configured. Add a channel and its models first.",
      { exact: true },
    )
    .waitFor();
  await page.keyboard.press("Escape");
  await dialog.waitFor({ state: "hidden" });
  await page.waitForFunction(
    () => document.activeElement?.textContent === "New persistent binding",
  );
  console.log("PASS empty channels and keyboard focus return");
  for (const name of ["Current source", "Current gateway"]) {
    await api("POST", "/api/channel", {
      name,
      type: 1,
      key: "fixture-key",
      models: "gpt-4.1,gpt-4.1-mini",
      group: "default",
      status: 1,
    });
  }
  const channels = await api("GET", "/api/channel");
  const source = channels.find((ch) => ch.name === "Current source");
  const gateway = channels.find((ch) => ch.name === "Current gateway");
  assert.equal((await api("GET", "/api/resource")).snapshot, null);
  dialog = await openCreation();
  await dialog
    .getByLabel("Source channel", { exact: true })
    .selectOption(String(source.id));
  await dialog
    .getByLabel("Gateway channel", { exact: true })
    .selectOption(String(gateway.id));
  await dialog.getByRole("checkbox", { name: "gpt-4.1", exact: true }).check();
  await dialog
    .getByRole("button", { name: "Generate server preview", exact: true })
    .click();
  await dialog
    .getByText("Preview checks passed. Click Publish binding in the footer and confirm to add it to the published list.", {
      exact: true,
    })
    .waitFor();
  assert.equal(
    await dialog
      .getByRole("region", { name: "Proposed pairs", exact: true })
      .locator("tbody tr")
      .count(),
    1,
  );
  await dialog
    .getByRole("checkbox", { name: "gpt-4.1-mini", exact: true })
    .check();
  assert.equal(
    await dialog
      .getByRole("button", { name: "Publish binding", exact: true })
      .count(),
    0,
  );
  await dialog.getByLabel("Search models", { exact: true }).fill("mini");
  assert.equal(await dialog.getByRole("checkbox").count(), 1);
  await dialog.getByLabel("Search models", { exact: true }).fill("");
  await dialog
    .getByRole("button", { name: "Generate server preview", exact: true })
    .click();
  await dialog
    .getByText("Preview checks passed. Click Publish binding in the footer and confirm to add it to the published list.", {
      exact: true,
    })
    .waitFor();
  assert.equal(
    await dialog
      .getByRole("region", { name: "Proposed pairs", exact: true })
      .locator("tbody tr")
      .count(),
    2,
  );
  await page.screenshot({
    path: path.join(temp, "current-binding-desktop.png"),
  });
  await dialog
    .getByRole("button", { name: "Publish binding", exact: true })
    .click();
  const confirmation = page.getByRole("dialog", {
    name: "Confirm route replacement for selected channels",
    exact: true,
  });
  const committed = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/resource/publication") &&
      response.request().method() === "PUT",
  );
  await confirmation
    .getByRole("button", { name: "Confirm publication", exact: true })
    .click();
  assert.equal((await committed).status(), 200);
  await dialog.waitFor({ state: "hidden" });
  const binding = page.locator(`[data-binding-gateway="${gateway.id}"]`);
  await binding.waitFor();
  assert.equal((await api("GET", "/api/resource/publication-state")).total, 1);
  assert.equal((await api("GET", "/api/resource")).snapshot, null);
  console.log(
    "PASS no-snapshot selection, model search, stale preview reset, and real publication",
  );

  await page.route("**/api/resource/publication-context", (route) =>
    route.fulfill({
      status: 500,
      contentType: "application/json",
      body: JSON.stringify({ success: false, message: "fixture failure" }),
    }),
  );
  dialog = await openCreation();
  await dialog
    .getByText("Could not load channel data. Refresh to retry.", {
      exact: true,
    })
    .waitFor();
  assert.equal(
    await dialog.getByLabel("Source channel", { exact: true }).count(),
    0,
  );
  await page.unroute("**/api/resource/publication-context");
  await dialog
    .getByRole("button", { name: "Refresh channel data", exact: true })
    .click();
  await dialog.getByLabel("Source channel", { exact: true }).waitFor();
  await dialog
    .getByText("Other source: resource snapshot", { exact: true })
    .click();
  await dialog
    .getByRole("button", {
      name: "Use the resource snapshot workflow",
      exact: true,
    })
    .click();
  await dialog.waitFor({ state: "hidden" });
  await page
    .locator("#binding-creation-guide")
    .getByRole("button", {
      name: "Import optional resource snapshot",
      exact: true,
    })
    .waitFor();
  console.log("PASS failed context retry and optional snapshot workflow");

  for (const [lang, title, sourceLabel, targetLabel, file] of [
    [
      "zh",
      "新建持久绑定",
      "来源渠道",
      "目标渠道",
      "current-binding-zh-mobile.png",
    ],
    [
      "ar",
      "إنشاء ربط دائم",
      "قناة المصدر",
      "قناة البوابة",
      "current-binding-ar-mobile.png",
    ],
  ]) {
    await page.evaluate((language) => {
      localStorage.setItem("xy-language", language);
      localStorage.setItem("xy-theme", "dark");
    }, lang);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.reload();
    await page
      .locator("#gateway-bindings")
      .getByRole("button", { name: title, exact: true })
      .click();
    const localized = page.getByRole("dialog", { name: title, exact: true });
    await localized
      .getByLabel(sourceLabel, { exact: true })
      .selectOption(String(source.id));
    await localized
      .getByLabel(targetLabel, { exact: true })
      .selectOption(String(gateway.id));
    await localized
      .getByRole("checkbox", { name: "gpt-4.1", exact: true })
      .check();
    assert.equal(await localized.getByRole("checkbox").count(), 2);
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
    );
    await page.screenshot({ path: path.join(temp, file) });
    await page.keyboard.press("Escape");
    await localized.waitFor({ state: "hidden" });
  }
  assert.deepEqual(errors, []);
  console.log("PASS Chinese/Arabic mobile dialogs and no page errors");
  console.log(`Screenshots: ${temp}`);
} catch (error) {
  if (page) {
    await page.screenshot({ path: path.join(temp, "failure.png") });
    console.error(await page.getByRole("dialog").allTextContents());
    console.error(`Failure screenshot: ${temp}`);
  }
  throw error;
} finally {
  await browser?.close();
  await vite?.close();
  backend?.kill();
}
