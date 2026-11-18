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
const temp = await mkdtemp(path.join(os.tmpdir(), "starhub-model-discovery-"));
const out = path.join(web, "playwright-report", "channel-model-discovery");
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
    hits.push({ path: req.url, authorization: req.headers.authorization });
    if (req.url.startsWith("/slow/"))
      await new Promise((resolve) => setTimeout(resolve, 500));
    res.setHeader("Content-Type", "application/json");
    if (req.url.startsWith("/auth/")) {
      res.writeHead(401);
      res.end(JSON.stringify({ error: "fixture authentication failed" }));
    } else if (req.url.startsWith("/video/"))
      res.end(
        '{"data":[{"id":"sora-2"},{"id":"sora-2-pro"},{"id":"chat-model"}]}',
      );
    else if (req.url.startsWith("/empty/")) res.end('{"data":[]}');
    else
      res.end(
        JSON.stringify({
          data: [
            "model-a",
            "Model-B",
            "model-a",
            " ",
            "very-long-model-" + "x".repeat(85),
          ].map((id) => ({ id })),
        }),
      );
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
  const dialog = page.getByRole("dialog");
  const discover = () =>
    dialog.getByRole("button", { name: "Fetch upstream models", exact: true });
  const capture = async (name) => {
    await page.screenshot({
      path: path.join(out, name + ".png"),
      mask: [page.locator('input[type="password"]')],
    });
    report.screenshots.push(name + ".png");
  };
  const openNew = async () => {
    await page
      .getByRole("button", { name: "New channel", exact: true })
      .click();
    await dialog.locator("#ch-name").fill("Discovery channel");
    await dialog.locator("#ch-key").fill("fixture-key");
    await dialog.locator("#ch-base-url").fill(upstreamBase);
  };
  const searchWithoutSaving = async () => {
    const writes = channelWrites;
    const search = dialog.getByRole("searchbox", {
      name: "Search upstream models",
    });
    await search.fill("model-a");
    await search.press("Enter");
    assert.equal(channelWrites, writes);
    assert.equal(await dialog.isVisible(), true);
    assert.equal(await dialog.getByRole("checkbox").count(), 1);
    for (const init of [{ isComposing: true }, { keyCode: 229 }]) {
      const prevented = await search.evaluate((node, init) => {
        const event = new KeyboardEvent("keydown", {
          key: "Enter",
          bubbles: true,
          cancelable: true,
          ...init,
        });
        node.dispatchEvent(event);
        return event.defaultPrevented;
      }, init);
      assert.equal(prevented, false, "IME confirmation must remain available");
    }
    assert.equal(channelWrites, writes);
    await search.fill("");
  };
  await openNew();
  await dialog.locator("#ch-type").selectOption("61");
  await dialog.getByText(/No plugin versions yet/).waitFor();
  assert.equal(await dialog.locator("#ch-plugin").isDisabled(), true);
  assert.equal(
    await dialog
      .getByRole("button", { name: "Create", exact: true })
      .isDisabled(),
    true,
  );
  assert.equal(await discover().isDisabled(), true);
  await dialog
    .getByRole("button", { name: "Add OpenAI video plugin", exact: true })
    .click();
  await page.waitForFunction(
    () =>
      Boolean(document.querySelector("#ch-plugin")?.value) ||
      document.querySelector('[role="dialog"] [role="alert"]'),
  );
  assert.deepEqual(await dialog.getByRole("alert").allTextContents(), []);
  const pluginId = await dialog.locator("#ch-plugin").inputValue();
  assert.equal(
    await dialog.locator("#ch-name").inputValue(),
    "Discovery channel",
  );
  assert.equal(await dialog.locator("#ch-key").inputValue(), "fixture-key");
  await capture("plugin-installed-desktop");
  await dialog
    .getByRole("button", { name: "Add OpenAI video plugin", exact: true })
    .click();
  await page.waitForFunction(
    () => !document.querySelector("#ch-plugin")?.disabled,
  );
  assert.equal(await dialog.locator("#ch-plugin").inputValue(), pluginId);
  assert.equal((await api("GET", "/api/task-plugin")).length, 1);
  await dialog
    .getByRole("button", { name: "Use plugin models", exact: true })
    .click();
  assert.equal(
    await dialog.locator("#ch-models").inputValue(),
    "sora-2,sora-2-pro",
  );
  await dialog.locator("#ch-models").fill("");
  await dialog.locator("#ch-base-url").fill(upstreamBase + "/video");
  await discover().click();
  await dialog.getByRole("checkbox", { name: "sora-2", exact: true }).waitFor();
  assert.equal(await dialog.getByRole("checkbox").count(), 2);
  assert.equal(hits.at(-1).authorization, "Bearer fixture-key");
  await dialog
    .getByRole("button", { name: "Select search results", exact: true })
    .click();
  await dialog
    .getByRole("button", { name: "Add selected models", exact: true })
    .click();
  assert.equal(
    await dialog.locator("#ch-models").inputValue(),
    "sora-2,sora-2-pro",
  );
  const source = await readFile(
    path.join(repo, "plugins/tasks/openai-video.js"),
    "utf8",
  );
  let releaseUpload, uploadStarted;
  const uploadGate = new Promise((resolve) => {
    releaseUpload = resolve;
  });
  const started = new Promise((resolve) => {
    uploadStarted = resolve;
  });
  await page.route("**/api/task-plugin", async (route) => {
    if (route.request().method() === "POST") {
      uploadStarted();
      await uploadGate;
    }
    await route.continue();
  });
  await dialog.locator("#ch-plugin-file").setInputFiles({
    name: "video-v2.js",
    mimeType: "text/javascript",
    buffer: Buffer.from(source.replace('version: "1.1.0"', 'version: "1.2.0"')),
  });
  await started;
  assert.equal(
    await dialog
      .getByRole("button", { name: "Create", exact: true })
      .isDisabled(),
    true,
  );
  const writesBeforeUpload = channelWrites;
  await dialog
    .locator("#admin-channel-form")
    .evaluate((form) => form.requestSubmit());
  assert.equal(channelWrites, writesBeforeUpload);
  releaseUpload();
  await page.waitForFunction((previous) => {
    const select = document.querySelector("#ch-plugin");
    return select?.value && select.value !== previous;
  }, pluginId);
  await page.waitForFunction(
    () => !document.querySelector("#ch-plugin")?.disabled,
  );
  await page.unroute("**/api/task-plugin");
  assert.equal(
    await dialog
      .getByRole("button", { name: "Create", exact: true })
      .isEnabled(),
    true,
  );
  report.checks.push(
    "pending plugin upload blocks both button and programmatic channel submission",
  );
  await dialog
    .getByRole("button", { name: "Refresh versions", exact: true })
    .click();
  assert.equal((await api("GET", "/api/task-plugin")).length, 2);
  for (const [language, theme] of [
    ["zh", "light"],
    ["en", "dark"],
    ["ar", "light"],
  ]) {
    await page.evaluate(
      async ({ language, theme }) => {
        const { usePreferences } = await import("/src/lib/preferences.ts");
        usePreferences.getState().setLanguage(language);
        usePreferences.getState().setTheme(theme);
      },
      { language, theme },
    );
    await page.setViewportSize({ width: 320, height: 844 });
    await dialog.locator("#ch-plugin").scrollIntoViewIfNeeded();
    assert.equal(
      await dialog
        .locator(".task-plugin-channel-field")
        .last()
        .evaluate((node) => node.scrollWidth > node.clientWidth + 1),
      false,
    );
    await capture(`plugin-${language}-${theme}-320`);
  }
  await page.evaluate(async () => {
    const { usePreferences } = await import("/src/lib/preferences.ts");
    usePreferences.getState().setLanguage("en");
    usePreferences.getState().setTheme("light");
  });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await dialog.waitFor({ state: "hidden" });
  assert.equal(channelWrites, 0);
  report.checks.push(
    "empty plugin list, built-in installation, idempotent registration, inline upload, plugin model fallback, filtered upstream discovery and narrow layouts pass without saving a channel",
  );
  await openNew();
  await dialog.locator("#ch-name").fill("Plugin discovery channel");
  await dialog.locator("#ch-type").selectOption("61");
  await dialog.locator("#ch-plugin").selectOption(pluginId);
  await dialog
    .getByRole("button", { name: "Use plugin models", exact: true })
    .click();
  await dialog.getByRole("button", { name: "Create", exact: true }).click();
  await dialog.waitFor({ state: "hidden" });
  const pluginChannel = (await api("GET", "/api/channel"))[0];
  assert.equal(pluginChannel.task_plugin_id, Number(pluginId));
  assert.equal(pluginChannel.models, "sora-2,sora-2-pro");
  await api("DELETE", `/api/channel/${pluginChannel.id}`);
  report.checks.push(
    "plugin channel creation persists the selected version and its supported models",
  );
  await openNew();
  await discover().click();
  await dialog
    .getByRole("checkbox", { name: "model-a", exact: true })
    .waitFor();
  assert.equal(await dialog.locator("#ch-models").inputValue(), "");
  assert.equal((await api("GET", "/api/channel")).length, 0);
  assert.equal(await dialog.getByRole("checkbox").count(), 3);
  assert.equal(hits.at(-1).authorization, "Bearer fixture-key");
  await dialog
    .getByRole("searchbox", { name: "Search upstream models" })
    .fill("Model-B");
  assert.equal(await dialog.getByRole("checkbox").count(), 1);
  await dialog
    .getByRole("button", { name: "Select search results", exact: true })
    .click();
  assert.equal(
    await dialog
      .getByRole("checkbox", { name: "Model-B", exact: true })
      .isChecked(),
    true,
  );
  await dialog
    .getByRole("searchbox", { name: "Search upstream models" })
    .fill("");
  await dialog
    .getByRole("button", { name: "Deselect all", exact: true })
    .click();
  await dialog.locator("#ch-models").fill("manual-model");
  await dialog.getByRole("checkbox", { name: "model-a", exact: true }).check();
  await dialog
    .getByRole("button", { name: "Add selected models", exact: true })
    .click();
  assert.equal(
    await dialog.locator("#ch-models").inputValue(),
    "manual-model,model-a",
  );
  report.checks.push(
    "unsaved discovery bypasses model required validation, deduplicates and merges only selected models",
  );
  await capture("create-desktop");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await dialog.waitFor({ state: "hidden" });
  assert.equal((await api("GET", "/api/channel")).length, 0);
  report.checks.push(
    "cancel after fetching and selection does not create a channel",
  );

  await openNew();
  await dialog.locator("#ch-models").fill("manual-model");
  await discover().click();
  await dialog.getByRole("checkbox", { name: "model-a", exact: true }).check();
  await dialog
    .getByRole("button", { name: "Add selected models", exact: true })
    .click();
  await searchWithoutSaving();
  assert.equal((await api("GET", "/api/channel")).length, 0);
  await dialog.getByRole("button", { name: "Create", exact: true }).click();
  await dialog.waitFor({ state: "hidden" });
  const saved = (await api("GET", "/api/channel"))[0];
  assert.equal(saved.models, "manual-model,model-a");
  await page
    .getByRole("row")
    .filter({ hasText: "Discovery channel" })
    .getByRole("button", { name: "Edit", exact: true })
    .click();
  assert.equal(await dialog.locator("#ch-key").inputValue(), "");
  await discover().click();
  await dialog
    .getByRole("checkbox", { name: "model-a", exact: true })
    .waitFor();
  assert.equal(hits.at(-1).authorization, "Bearer fixture-key");
  await dialog.getByText("manual-model", { exact: false }).last().waitFor();
  await dialog.locator("#ch-name").fill("Unsaved channel name");
  await searchWithoutSaving();
  assert.equal((await api("GET", "/api/channel"))[0].name, saved.name);
  await dialog.locator("#ch-name").fill(saved.name);
  report.checks.push(
    "search Enter does not create or save channels, IME confirmation remains available, and explicit creation still works",
  );
  report.checks.push(
    "outer create persists selected models; editing fetches with the saved key and retains unlisted models",
  );
  const beforeBlocked = hits.length;
  await dialog.locator("#ch-base-url").fill(upstreamBase + "/alternate");
  await discover().click();
  await dialog
    .getByRole("alert")
    .filter({ hasText: "Enter a key for the new connection" })
    .waitFor();
  assert.equal(hits.length, beforeBlocked);
  await dialog.locator("#ch-key").fill("replacement-fixture-key");
  await discover().click();
  await dialog
    .getByRole("checkbox", { name: "Model-B", exact: true })
    .waitFor();
  assert.equal(hits.at(-1).authorization, "Bearer replacement-fixture-key");
  assert.equal((await api("GET", "/api/channel"))[0].base_url, upstreamBase);
  report.checks.push(
    "changed URLs cannot reuse stored credentials; draft key and URL preview remain unsaved",
  );

  await dialog.locator("#ch-base-url").fill(upstreamBase + "/auth");
  await discover().click();
  await dialog
    .getByRole("alert")
    .filter({ hasText: "Upstream authentication failed" })
    .waitFor();
  assert.equal(new URL(page.url()).pathname, "/admin");
  assert.equal(await dialog.locator("#ch-models").inputValue(), saved.models);
  await api("GET", "/api/channel");
  await dialog.locator("#ch-base-url").fill(upstreamBase + "/v1");
  await discover().click();
  await dialog
    .getByRole("alert")
    .filter({ hasText: "Remove the trailing /v1" })
    .waitFor();
  await dialog.locator("#ch-base-url").fill(upstreamBase + "/empty");
  await discover().click();
  await dialog
    .getByText("Upstream returned no models.", { exact: false })
    .waitFor();
  assert.equal(await dialog.locator("#ch-models").inputValue(), saved.models);
  report.checks.push(
    "upstream authentication, invalid roots and empty lists preserve form and local session",
  );

  await dialog.locator("#ch-base-url").fill(upstreamBase + "/slow");
  await discover().click();
  await dialog
    .getByRole("button", { name: "Fetching…", exact: true })
    .waitFor();
  await dialog.locator("#ch-base-url").fill(upstreamBase);
  await page.waitForTimeout(650);
  assert.equal(await dialog.getByRole("checkbox").count(), 0);
  await dialog
    .getByText("Connection settings changed.", { exact: false })
    .waitFor();
  await dialog.locator("#ch-base-url").fill(upstreamBase + "/slow");
  await discover().click();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await dialog.waitFor({ state: "hidden" });
  await page.waitForTimeout(650);
  await page
    .getByRole("row")
    .filter({ hasText: "Discovery channel" })
    .getByRole("button", { name: "Edit", exact: true })
    .click();
  assert.equal(await dialog.getByRole("checkbox").count(), 0);
  report.checks.push("late requests cannot populate changed or reopened forms");
  await dialog.locator("#ch-key").fill("replacement-fixture-key");
  await dialog.locator("#ch-base-url").fill(upstreamBase + "/slow");
  await discover().click();
  await dialog
    .getByRole("button", { name: "Fetching…", exact: true })
    .waitFor();
  await page.evaluate(async () => {
    const { useSystemAccess } = await import("/src/lib/system-access.ts");
    const state = useSystemAccess.getState();
    state.setSession({
      id: state.session.accountId,
      group: state.session.group,
      role: state.role,
      username: state.session.username,
    });
  });
  await page.waitForTimeout(650);
  assert.equal(await dialog.getByRole("checkbox").count(), 0);
  await dialog.locator("#ch-base-url").fill(upstreamBase);
  await dialog.locator("#ch-key").fill("");
  report.checks.push(
    "confirmed session identity changes discard pending discovery results",
  );
  await dialog.locator("#ch-type").selectOption("14");
  assert.equal(await discover().count(), 0);
  await dialog.locator("#ch-models").fill("manual-model,model-a");
  await dialog.locator("#ch-type").selectOption("1");
  await discover().click();
  await dialog
    .getByRole("checkbox", { name: "Model-B", exact: true })
    .waitFor();
  await dialog.getByRole("checkbox", { name: "Model-B", exact: true }).check();
  await dialog
    .getByRole("button", { name: "Add selected models", exact: true })
    .click();
  assert.equal((await api("GET", "/api/channel"))[0].models, saved.models);
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  await dialog.waitFor({ state: "hidden" });
  assert.equal(
    (await api("GET", "/api/channel"))[0].models,
    saved.models + ",Model-B",
  );
  report.checks.push(
    "unsupported types allow manual models; only outer save persists edits",
  );

  await page
    .getByRole("row")
    .filter({ hasText: "Discovery channel" })
    .getByRole("button", { name: "Edit", exact: true })
    .click();
  await discover().click();
  await dialog
    .getByRole("checkbox", { name: "model-a", exact: true })
    .waitFor();
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
      await page.setViewportSize({ width: 320, height: 844 });
      await dialog
        .locator(".channel-model-heading button")
        .scrollIntoViewIfNeeded();
      assert.ok(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
      );
      const box = await dialog.boundingBox();
      assert.ok(box.x >= -1 && box.x + box.width <= 321);
      const overflow = await dialog
        .locator(".channel-model-discovery")
        .evaluate((node) => node.scrollWidth > node.clientWidth + 1);
      assert.equal(overflow, false);
      await capture(`selection-${language}-${theme}-320`);
    }
  }
  const scan = await new AxeBuilder({ page })
    .include(".channel-model-discovery")
    .withTags(["wcag2a", "wcag2aa"])
    .analyze();
  assert.deepEqual(scan.violations, []);
  await page.evaluate(async () => {
    const { usePreferences } = await import("/src/lib/preferences.ts");
    usePreferences.getState().setLanguage("en");
  });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await dialog.locator(".channel-model-heading button").focus();
  await page.keyboard.press("Tab");
  assert.equal(
    await dialog
      .locator("#ch-models")
      .evaluate((node) => node === document.activeElement),
    true,
  );
  report.checks.push(
    "three languages, two themes, 320px layout, accessibility and keyboard navigation pass",
  );
  assert.deepEqual(report.pageErrors, []);
  assert.equal(logs.includes("replacement-fixture-key"), false);
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
