// Uses an isolated database and a local upstream; no real credentials are used.
import { createServer as createViteServer } from "vite";
import { chromium, expect } from "playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { spawn, execFileSync } from "node:child_process";
import { createServer } from "node:http";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import os from "node:os";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const repo = path.resolve(web, "..");
const temp = await mkdtemp(path.join(os.tmpdir(), "starhub-playground-"));
const out = path.join(web, "playwright-report", "playground");
await mkdir(out, { recursive: true });
const report = { checks: [], screenshots: [], pageErrors: [] };
let backend, vite, browser, upstream;
let backendLogs = "";
const upstreamCalls = [];
let upstreamFailure = false;
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
try {
  upstream = createServer(async (req, res) => {
    let body = "";
    for await (const chunk of req) body += chunk;
    const input = JSON.parse(body);
    upstreamCalls.push({ input, authorization: req.headers.authorization });
    res.setHeader("Content-Type", "application/json");
    if (upstreamFailure) {
      res.writeHead(401);
      res.end(
        JSON.stringify({
          error: {
            message: "fixture upstream rejected the credential",
            type: "authentication_error",
            code: "invalid_api_key",
          },
        }),
      );
      return;
    }
    res.end(
      JSON.stringify({
        id: "chatcmpl-playground-fixture",
        object: "chat.completion",
        model: input.model,
        choices: [
          {
            index: 0,
            message: { role: "assistant", content: "Playground upstream OK" },
            finish_reason: "stop",
          },
        ],
        usage: { prompt_tokens: 7, completion_tokens: 3, total_tokens: 10 },
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
      STARHUB_ROOT_PASSWORD: "PlaygroundInitial92",
      STARHUB_ENV: "development",
      STARHUB_CRYPTO_SECRET: "playground-fixture-secret",
      STARHUB_CREDENTIAL_KEYS: JSON.stringify({ fixture: "11".repeat(32) }),
      STARHUB_CREDENTIAL_ACTIVE_KEY: "fixture",
      RESOURCE_CATALOG_NOTIFY_URL: "",
      STARHUB_REGION_PROFILE: "",
    },
  });
  for (const stream of [backend.stdout, backend.stderr])
    stream.on("data", (chunk) => {
      backendLogs = (backendLogs + chunk).slice(-16000);
    });
  let ready = false;
  for (let i = 0; i < 80; i++) {
    try {
      ready = (await fetch(`http://127.0.0.1:${backendPort}/api/health`)).ok;
    } catch {}
    if (ready) break;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  assert.ok(ready, "isolated backend failed to start: " + backendLogs);
  vite = await createViteServer({
    root: web,
    logLevel: "error",
    server: {
      host: "127.0.0.1",
      port: await freePort(),
      proxy: {
        "/api": `http://127.0.0.1:${backendPort}`,
        "/v1": `http://127.0.0.1:${backendPort}`,
      },
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
  await api("POST", "/api/user/login", {
    username: "root",
    password: "PlaygroundInitial92",
  });
  await api("PUT", "/api/user/self", {
    current_password: "PlaygroundInitial92",
    new_password: "PlaygroundChanged93",
  });
  await api("POST", "/api/user/2fa/skip");
  await api("POST", "/api/channel", {
    name: "Playground fixture",
    type: 1,
    key: "fixture-upstream-key",
    base_url: upstreamBase,
    models: "glm-5.2",
    group: "default",
    status: 1,
  });
  const token = await api("POST", "/api/token", {
    name: "Persisted playground key",
    group: "default",
    model_limits: "glm-5.2",
    remain_quota: 1000000,
  });
  const page = await context.newPage();
  page.setDefaultTimeout(15000);
  page.on("pageerror", (error) => report.pageErrors.push(String(error)));
  const send = () =>
    page.getByRole("button", { name: "Send request", exact: true });
  const usage = async () =>
    (await api("GET", "/api/token/owned")).find((row) => row.id === token.id)
      .used_quota_nano;
  const before = await usage();
  await page.goto(base + "/console/playground");
  await expect(page.locator("#verify-key")).toHaveValue(String(token.id));
  await expect(page.locator("#verify-model")).toHaveValue("glm-5.2");
  await expect(send()).toBeDisabled();
  assert.equal(upstreamCalls.length, 0);
  await page.locator("#verify-password").fill("WrongPassword93");
  await page.locator("#verify-password").press("Enter");
  await expect(page.locator("#verify-unlock-error")).toBeVisible();
  await expect(send()).toBeDisabled();
  assert.equal(upstreamCalls.length, 0);
  assert.equal(await usage(), before);
  report.checks.push(
    "persisted masked keys and backend-only models load; locked keys and wrong passwords cannot send or charge",
  );
  await page.locator("#verify-password").fill("PlaygroundChanged93");
  await page.locator("#verify-password").press("Enter");
  await expect(send()).toBeEnabled();
  await expect(page.locator("#verify-password")).toHaveCount(0);
  assert.equal(upstreamCalls.length, 0);
  await page.locator("#verify-prompt").fill("Reply with the fixture response.");
  await send().click();
  await expect(page.locator(".verify-output")).toHaveText(
    "Playground upstream OK",
  );
  assert.equal(upstreamCalls.length, 1);
  assert.equal(upstreamCalls[0].authorization, "Bearer fixture-upstream-key");
  assert.equal(upstreamCalls[0].input.model, "glm-5.2");
  assert.equal(
    upstreamCalls[0].input.messages[0].content,
    "Reply with the fixture response.",
  );
  assert.ok(BigInt(await usage()) > BigInt(before));
  const calls = (await api("GET", "/api/log")).items;
  assert.equal(calls.length, 1);
  assert.equal(calls[0].model, "glm-5.2");
  assert.equal(calls[0].prompt_tokens, 7);
  assert.equal(calls[0].completion_tokens, 3);
  const storage = await page.evaluate(() =>
    JSON.stringify({
      local: { ...localStorage },
      session: { ...sessionStorage },
    }),
  );
  assert.ok(!storage.includes(token.key));
  assert.ok(!storage.includes("PlaygroundChanged93"));
  report.checks.push(
    "password unlock sends one real gateway request with correct credentials; the backend records usage and charges, with no secret persistence",
  );
  const successQuota = await usage();
  await page.route("**/v1/chat/completions", (route) => route.abort("failed"));
  await send().click();
  await expect(page.locator(".verify-outcome")).toContainText(
    "Request outcome unknown",
  );
  await expect(send()).toBeEnabled();
  assert.equal(upstreamCalls.length, 1);
  assert.equal(await usage(), successQuota);
  assert.ok(
    !(await page.locator(".verify-result").innerText()).includes("SIMULATION"),
  );
  await page.unroute("**/v1/chat/completions");
  report.checks.push(
    "transport failures show an unknown outcome without simulation, automatic retries or local charges",
  );
  await page.reload();
  await expect(page.locator("#verify-password")).toBeVisible();
  await expect(send()).toBeDisabled();
  await expect(page.locator("#verify-prompt")).toHaveValue(
    "Reply with the fixture response.",
  );
  report.checks.push(
    "refresh restores the request draft but locks the credential again",
  );
  for (const language of ["en", "zh", "ar"]) {
    for (const theme of ["light", "dark"]) {
      await page.setViewportSize({ width: 320, height: 900 });
      await page.reload();
      await expect(page.locator("#verify-password")).toBeVisible();
      await page.evaluate(
        async ({ language, theme }) => {
          const { usePreferences } = await import("/src/lib/preferences.ts");
          usePreferences.getState().setLanguage(language);
          usePreferences.getState().setTheme(theme);
        },
        { language, theme },
      );
      await expect(page.locator("html")).toHaveAttribute("lang", language);
      assert.ok(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
        "horizontal overflow",
      );
      const name = `locked-${language}-${theme}-320.png`;
      await page.screenshot({ path: path.join(out, name), fullPage: true });
      report.screenshots.push(name);
      const accessibility = await new AxeBuilder({ page })
        .include(".verify-workspace")
        .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
        .analyze();
      assert.deepEqual(
        accessibility.violations.map((v) => ({
          id: v.id,
          targets: v.nodes.map((n) => n.target),
        })),
        [],
      );
    }
  }
  report.checks.push(
    "locked-key flow passes three languages, both themes, 320px layout, keyboard unlock and accessibility checks",
  );
  await page.evaluate(() => {
    localStorage.setItem("xy-language", "en");
    localStorage.setItem("xy-theme", "light");
  });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.reload();
  await page.locator("#verify-password").fill("PlaygroundChanged93");
  await page.locator("#verify-password").press("Enter");
  await expect(send()).toBeEnabled();
  upstreamFailure = true;
  await send().click();
  await expect(page.locator(".verify-outcome.is-error")).toBeVisible();
  assert.equal(upstreamCalls.length, 2);
  assert.equal(await usage(), successQuota);
  assert.ok(
    !(await page.locator(".verify-result").innerText()).includes("Not charged"),
  );
  report.checks.push(
    "upstream credential failures remain real API errors and do not fall back to simulated results",
  );
  assert.deepEqual(report.pageErrors, []);
  console.log(JSON.stringify(report, null, 2));
} catch (error) {
  report.failure = String(error);
  console.error(error);
  process.exitCode = 1;
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
