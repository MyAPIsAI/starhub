// Verify platform channels against an isolated backend without an inventory snapshot.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import { spawn, execFileSync } from "node:child_process";
import { mkdtemp } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import os from "node:os";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const temp = await mkdtemp(
  path.join(os.tmpdir(), "opencode", "platform-channels-"),
);
let backend, vite, browser;
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
  const page = await context.newPage();
  page.setDefaultTimeout(30000);
  const errors = [];
  page.on("pageerror", (error) => errors.push(String(error)));
  await page.goto(base + "/console/channels");
  const panel = page.locator("#platform-channels");
  await panel
    .getByText("No channels have been created on this platform.", {
      exact: true,
    })
    .waitFor();
  for (let i = 1; i <= 21; i++) {
    await api("POST", "/api/channel", {
      name: `Platform fixture ${String(i).padStart(2, "0")}`,
      type: i === 21 ? 45 : 1,
      key: "fixture-key",
      models: i === 21 ? "deepseek-chat" : "gpt-4.1",
      group: "default",
      status: i === 21 ? 2 : 1,
    });
  }
  assert.equal((await api("GET", "/api/resource")).snapshot, null);
  await panel
    .getByRole("button", { name: "Refresh channels", exact: true })
    .click();
  await panel
    .getByRole("heading", { name: "Platform channels · 21", exact: true })
    .waitFor();
  assert.equal(await panel.locator("[data-platform-channel]").count(), 20);
  await panel.getByRole("button", { name: "Next", exact: true }).click();
  assert.equal(await panel.locator("[data-platform-channel]").count(), 1);
  await panel
    .getByLabel("Search platform channels", { exact: true })
    .fill("deepseek-chat");
  const row = panel.locator("[data-platform-channel]");
  await row.getByText("Platform fixture 21", { exact: true }).waitFor();
  await row.getByText("DeepSeek", { exact: true }).waitFor();
  await row.getByText("Closed", { exact: true }).waitFor();
  assert.equal(
    await panel
      .getByRole("link", { name: "Manage channels", exact: true })
      .getAttribute("href"),
    "/admin?tab=resources#channels",
  );
  assert.equal(await page.locator("#platform-channels").count(), 1);
  assert.ok(
    await page.evaluate(() => {
      const publication = document.querySelector("#gateway-publication");
      const channels = document.querySelector("#platform-channels");
      const bindings = document.querySelector("#gateway-bindings");
      return (
        publication.contains(channels) &&
        Boolean(
          channels.compareDocumentPosition(bindings) &
          Node.DOCUMENT_POSITION_FOLLOWING,
        )
      );
    }),
  );
  assert.equal(
    await page.getByLabel("Search source channels", { exact: true }).count(),
    0,
  );
  assert.equal(
    await page
      .getByRole("region", { name: "Source to gateway bindings", exact: true })
      .count(),
    0,
  );
  await panel
    .getByLabel("Search platform channels", { exact: true })
    .fill("missing-channel");
  await panel
    .getByText("No matching platform channels.", { exact: true })
    .waitFor();
  await page.route("**/api/channel", (route) =>
    route.fulfill({
      status: 500,
      contentType: "application/json",
      body: '{"success":false,"message":"Fixture unavailable"}',
    }),
  );
  await panel
    .getByRole("button", { name: "Refresh channels", exact: true })
    .click();
  await panel.getByRole("alert").waitFor();
  assert.equal(await panel.locator("[data-platform-channel]").count(), 0);
  await page.unroute("**/api/channel");
  await panel
    .getByRole("button", { name: "Refresh channels", exact: true })
    .click();
  await panel.getByLabel("Search platform channels", { exact: true }).fill("");
  // Seed inventory through the API, then exercise changes through the live UI.
  const gateway = (await api("GET", "/api/channel")).find(
    (channel) => channel.name === "Platform fixture 01",
  );
  const snapshot = {
    id: "platform-state-inventory",
    sequence: 1,
    publishedAt: new Date().toISOString(),
    source: "platform-state-fixture",
    channels: [
      { id: "supplier", name: "Supplier", provider: "", enabled: true },
    ],
    instances: [
      {
        id: "resource",
        channelId: "supplier",
        upstreamModel: "gpt-4.1",
        canonicalModel: "gpt-4.1",
        protocols: ["OpenAI"],
        capabilities: ["text"],
        contextLength: 128000,
        region: "",
        status: "available",
        latencyMs: 0,
      },
    ],
  };
  let catalog = await api("GET", "/api/resource");
  catalog = await api("PUT", "/api/resource", {
    expected_revision: catalog.revision,
    snapshot,
  });
  await api("PUT", "/api/placement/resource", {
    expected_revision: catalog.revision,
    placement: { modelId: "gpt-4.1", groupIds: ["default"], enabled: true },
  });
  await page.reload();
  await panel
    .getByLabel("Search platform channels", { exact: true })
    .fill(gateway.name);
  const gatewayRow = panel.locator(`[data-platform-channel="${gateway.id}"]`);
  const publication = page.locator("#gateway-publication");
  const dialog = page.getByRole("dialog");
  await publication
    .getByLabel("Gateway channel: supplier", { exact: true })
    .selectOption(String(gateway.id));
  const expectState = async (label) => {
    await gatewayRow.getByText(label, { exact: true }).waitFor();
    const current = (await api("GET", "/api/channel")).find(
      (channel) => channel.id === gateway.id,
    );
    assert.equal(
      current.resource_state.state,
      label === "Enabled" ? "enabled" : "closed",
    );
  };
  const publish = async () => {
    await publication
      .getByRole("button", { name: "Generate server preview", exact: true })
      .click();
    await publication
      .getByRole("button", { name: "Publish this review", exact: true })
      .click();
    await dialog
      .getByRole("button", { name: "Confirm publication", exact: true })
      .click();
    await dialog.waitFor({ state: "hidden" });
    await expectState("Enabled");
  };
  const changeBinding = async (operation) => {
    const label = operation === "revoke" ? "Revoke routes" : "Release binding";
    await publication
      .getByRole("button", { name: `${label}: ${gateway.id}`, exact: true })
      .click();
    await dialog
      .getByRole("button", {
        name:
          operation === "revoke"
            ? "Confirm route revocation"
            : "Confirm binding release",
        exact: true,
      })
      .click();
    await dialog.waitFor({ state: "hidden" });
    await expectState("Closed");
  };
  const placement = async (enabled) => {
    await page
      .locator('[data-resource-id="resource"]')
      .getByRole("button", { name: "Edit placement", exact: true })
      .click();
    await dialog
      .getByLabel("Enable inventory placement", { exact: true })
      .setChecked(enabled);
    await dialog
      .getByRole("button", { name: "Review changes", exact: true })
      .click();
    await dialog
      .getByRole("button", { name: "Confirm save", exact: true })
      .click();
    await dialog.waitFor({ state: "hidden" });
  };
  await publish();
  await changeBinding("revoke");
  await publish();
  await placement(false);
  await expectState("Closed");
  await placement(true);
  await publish();
  await page
    .getByRole("button", { name: "Import snapshot", exact: true })
    .click();
  await dialog.getByLabel("Snapshot JSON", { exact: true }).fill(
    JSON.stringify({
      ...snapshot,
      sequence: 2,
      publishedAt: new Date().toISOString(),
      instances: [{ ...snapshot.instances[0], contextLength: 64000 }],
    }),
  );
  await dialog
    .getByRole("button", { name: "Review changes", exact: true })
    .click();
  await dialog
    .getByRole("button", { name: "Confirm save", exact: true })
    .click();
  await dialog.waitFor({ state: "hidden" });
  await expectState("Closed");
  await publish();
  // An uncertain response must still trigger a read of the committed server state.
  await page.route(`**/api/resource/bindings/${gateway.id}`, async (route) => {
    // Preserve the browser's same-origin context when Playwright forwards the request.
    const response = await route.fetch({
      headers: {
        ...(await route.request().allHeaders()),
        "sec-fetch-site": "same-origin",
      },
    });
    assert.equal(response.status(), 200, await response.text());
    await route.fulfill({
      status: 500,
      contentType: "application/json",
      body: '{"success":false,"message":"Fixture response lost"}',
    });
  });
  await publication
    .getByRole("button", { name: `Revoke routes: ${gateway.id}`, exact: true })
    .click();
  await dialog
    .getByRole("button", { name: "Confirm route revocation", exact: true })
    .click();
  await dialog.getByRole("alert").waitFor();
  await expectState("Closed");
  await page.unroute(`**/api/resource/bindings/${gateway.id}`);
  await dialog.getByText("Close", { exact: true }).click();
  await publish();
  await changeBinding("release");
  console.log(
    "Platform channel state checks passed: publication, revocation, placement changes, snapshot changes, uncertain response and binding release refresh the mounted list without a manual refresh.",
  );
  await panel.getByLabel("Search platform channels", { exact: true }).fill("");
  if (!process.argv.includes("--state-only")) {
    for (const lang of ["en", "zh", "ar"]) {
      await page.evaluate(
        (language) => localStorage.setItem("xy-language", language),
        lang,
      );
      await page.reload();
      await panel.locator("[data-platform-channel]").first().waitFor();
      for (const width of [1440, 390, 320]) {
        await page.setViewportSize({ width, height: 1000 });
        await panel.scrollIntoViewIfNeeded();
        assert.ok(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
          `${lang}/${width} page overflow`,
        );
        await panel.locator(".rp-table").focus();
        assert.equal(
          await panel
            .locator(".rp-table")
            .evaluate((element) => element === document.activeElement),
          true,
        );
        await page.screenshot({
          path: path.join(temp, `platform-${lang}-${width}.png`),
        });
      }
    }
    await page.evaluate(
      () => (document.documentElement.dataset.theme = "dark"),
    );
    await page.screenshot({ path: path.join(temp, "platform-dark.png") });
    const accessibility = await new AxeBuilder({ page })
      .include("#platform-channels")
      .analyze();
    assert.deepEqual(
      accessibility.violations.map((entry) => entry.id),
      [],
    );
    await api("POST", "/api/user/logout");
    await page.reload();
    await page.waitForURL(base + "/");
    assert.equal(await page.locator("#platform-channels").count(), 0);
    assert.deepEqual(errors, []);
    console.log(
      `Platform channel checks passed: empty inventory, 21 real channels, pagination, search, disabled status, refresh/error recovery, management link, three languages, narrow screens, keyboard focus, dark theme, accessibility and logout. Screenshots: ${temp}`,
    );
  }
  assert.deepEqual(errors, []);
} finally {
  await browser?.close();
  await vite?.close();
  backend?.kill();
}
