// Isolated UI-3.4, UI-3.5 and UI-AC-08/09/10 acceptance against the real gateway and a local synthetic provider.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import { execFileSync, spawn } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import http from "node:http";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const project = path.resolve(web, "..");
const alternateRegion = process.env.STARHUB_BILL_BROWSER_REGION;
const out = path.join(
  project,
  "output",
  process.env.STARHUB_USAGE_BROWSER_OUTPUT ??
    path.join(
      "ui-usage-repair-20261103",
      alternateRegion ? "browser-new-york" : "browser",
    ),
);
await mkdir(out, { recursive: true });
const temp = await mkdtemp(path.join(out, "run-"));
const binary = path.join(out, "usage-check.exe");
execFileSync("go", ["build", "-buildvcs=false", "-o", binary, "."], {
  cwd: project,
  stdio: "pipe",
});
const freePort = async () => {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
};
const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
let firstTrace = "";
let upstreamCalls = 0,
  lastBody;
const upstream = http.createServer(async (req, res) => {
  const buffers = [];
  for await (const chunk of req) buffers.push(chunk);
  const body = JSON.parse(Buffer.concat(buffers).toString());
  lastBody = body;
  upstreamCalls++;
  if (body.messages?.[0]?.content === "Synthetic report failure") {
    res.writeHead(503, { "Content-Type": "application/json" });
    res.end(
      JSON.stringify({
        error: { message: "Synthetic failure", type: "upstream_error" },
      }),
    );
    return;
  }
  const gemini = req.url.includes("/v1beta/");
  const stream = gemini
    ? req.url.includes("streamGenerateContent")
    : body.stream;
  if (stream) {
    res.writeHead(200, { "Content-Type": "text/event-stream" });
    res.write(
      "data: " +
        JSON.stringify(
          gemini
            ? { candidates: [{ content: { parts: [{ text: "Hello " }] } }] }
            : { choices: [{ delta: { content: "Hello " } }] },
        ) +
        "\n\n",
    );
    await delay(450);
    res.write(
      "data: " +
        JSON.stringify(
          gemini
            ? {
                candidates: [
                  {
                    content: { parts: [{ text: "world" }] },
                    finishReason: "STOP",
                  },
                ],
                usageMetadata: {
                  promptTokenCount: 12,
                  candidatesTokenCount: 4,
                },
              }
            : {
                choices: [
                  { delta: { content: "world" }, finish_reason: "stop" },
                ],
                usage: { prompt_tokens: 12, completion_tokens: 4 },
              },
        ) +
        "\n\n",
    );
    if (!gemini) res.write("data: [DONE]\n\n");
    res.end();
  } else {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(
      JSON.stringify(
        gemini
          ? {
              candidates: [
                {
                  content: { parts: [{ text: "Hello world" }] },
                  finishReason: "STOP",
                },
              ],
              usageMetadata: { promptTokenCount: 12, candidatesTokenCount: 4 },
            }
          : {
              model: body.model,
              choices: [{ message: { content: "Hello world" } }],
              usage: { prompt_tokens: 12, completion_tokens: 4 },
            },
      ),
    );
  }
});
await new Promise((resolve) => upstream.listen(0, "127.0.0.1", resolve));
const supplier = `http://127.0.0.1:${upstream.address().port}`;
const backend = `http://127.0.0.1:${await freePort()}`;
const password = "PlaygroundSynthetic2026";
const child = spawn(binary, [], {
  cwd: temp,
  windowsHide: true,
  env: {
    ...process.env,
    STARHUB_CONFIG: "",
    STARHUB_PORT: new URL(backend).port,
    STARHUB_DB: path.join(temp, "starhub.db"),
    STARHUB_COOKIE_SECURE: "false",
    STARHUB_ROOT_PASSWORD: password,
    STARHUB_REGION_TIMEZONE: alternateRegion ?? "Asia/Dubai",
  },
  stdio: ["ignore", "ignore", "pipe"],
});
let logs = "";
child.stderr.on("data", (data) => {
  logs += data;
});
let vite, browser;
const report = { checks: [], screenshots: [], errors: [] };
try {
  let ready = false;
  for (let i = 0; i < 100; i++) {
    try {
      ready = (await fetch(backend + "/api/plaza/models")).ok;
    } catch {}
    if (ready) break;
    await delay(200);
  }
  assert.ok(ready, `backend did not start: ${logs}`);
  vite = await createViteServer({
    root: web,
    server: {
      host: "127.0.0.1",
      port: await freePort(),
      proxy: { "/api": backend, "/v1": backend },
    },
    logLevel: "error",
  });
  await vite.listen();
  const base = vite.resolvedUrls.local[0].replace(/\/$/, "");
  browser = await chromium.launch({ channel: "chrome", headless: true });
  const seed = await browser.newContext();
  const login = await seed.request.post(base + "/api/user/login", {
    data: { username: "root", password },
  });
  assert.equal(login.status(), 200);
  const owner = (await login.json()).data.user.id;
  const option = async (key, value) => {
    const res = await seed.request.put(base + "/api/option", {
      data: { key, value },
    });
    assert.equal(res.status(), 200);
  };

  for (const key of [
    "ip_rate_limit",
    "token_rate_limit",
    "user_rate_limit",
    "model_rate_limit",
  ])
    await option(key, "10000");
  await option("model_price", '{"gpt-4.1":0.001}');
  const channel = await seed.request.post(base + "/api/channel/", {
    data: {
      name: "Billing acceptance",
      type: 1,
      models: "gpt-4.1",
      group: "default",
      base_url: supplier,
      key: "synthetic-supplier",
      status: 1,
    },
  });
  assert.equal(channel.status(), 200);
  for (const amount of ["10.00", "20.00"]) {
    const made = await seed.request.post(base + "/api/recharge", {
      data: { amount_usd: amount, channel: "card" },
    });
    assert.equal(made.status(), 200);
    const order = (await made.json()).data;
    const begun = await seed.request.post(
      base + `/api/recharge/${order.id}/begin`,
      { data: {} },
    );
    assert.equal(begun.status(), 200);
    const paid = await seed.request.post(
      base + `/api/recharge/${order.id}/resolve`,
      { data: { outcome: "success" } },
    );
    assert.equal(paid.status(), 200);
  }
  const created = await seed.request.post(base + "/api/token", {
    data: {
      name: '=HYPERLINK("http://x")',
      group: "default",
      unlimited_quota: true,
      model_limits: "gpt-4.1",
      smart_access: false,
    },
  });
  assert.equal(created.status(), 200);
  const key = (await created.json()).data;
  for (let batch = 0; batch < 15; batch++)
    await Promise.all(
      Array.from({ length: 20 }, async () => {
        const response = await seed.request.post(
          base + "/v1/chat/completions",
          {
            headers: { Authorization: "Bearer " + key.key },
            data: {
              model: "gpt-4.1",
              max_tokens: 8,
              messages: [{ role: "user", content: "Synthetic billing check" }],
            },
          },
        );
        assert.equal(response.status(), 200);
        firstTrace ||= response.headers()["x-request-id"];
      }),
    );
  const summary = await seed.request.get(base + "/api/wallet");
  const wallet = (await summary.json()).data;
  assert.equal(wallet.month_spend_nano, "300000000");
  assert.equal(wallet.api_balance_nano, "199700000000");
  const state = await seed.storageState();
  const made = await seed.request.post(base + "/api/token", {
    data: {
      name: "Second usage key",
      group: "default",
      unlimited_quota: true,
      model_limits: "gpt-4.1",
      smart_access: false,
    },
  });
  assert.equal(made.status(), 200);
  const second = (await made.json()).data;
  const call = (content) =>
    seed.request.post(base + "/v1/chat/completions", {
      headers: { Authorization: "Bearer " + second.key },
      data: { model: "gpt-4.1", messages: [{ role: "user", content }] },
    });
  for (let i = 0; i < 10; i++)
    assert.equal((await call("Synthetic report check")).status(), 200);
  assert.equal((await call("Synthetic report failure")).status(), 502);
  assert.match(firstTrace, /^[a-f0-9]{64}$/);
  await option("model_price", '{"gpt-4.1":0.099}');
  const totals = (
    await (await seed.request.get(base + "/api/user/reports")).json()
  ).data;
  assert.equal(totals.totals.requests, 311);
  assert.equal(totals.totals.successful, 310);
  assert.equal(totals.totals.amount_nano, "310000000");
  assert.equal(totals.rows.length, 2);
  const exact = (
    await (
      await seed.request.get(
        base + "/api/log?scope=owned&trace_id=" + firstTrace,
      )
    ).json()
  ).data.items;
  assert.equal(exact.length, 1);
  assert.equal(exact[0].amount_nano, "1000000");
  assert.equal(
    JSON.parse(exact[0].billing_detail).parts[0].unit_price_usd,
    "0.001",
  );
  for (const field of ["channel_id", "used_channels", "prompt", "response"])
    assert.equal(exact[0][field], undefined);
  report.checks.push(
    "311 gateway calls, two keys, one uncharged failure, 310000000 nanoUSD; .001 historical tariff survives .099 repricing; exact header trace resolves outside first 100 records",
  );
  for (const language of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const context = await browser.newContext({
          storageState: state,
          viewport: { width, height: width === 390 ? 844 : 1000 },
          acceptDownloads: true,
        });
        await context.addInitScript(
          ({ language, theme }) => {
            localStorage.setItem("xy-language", language);
            localStorage.setItem("xy-theme", theme);
          },
          { language, theme },
        );
        const page = await context.newPage();
        page.setDefaultTimeout(20000);
        page.on("pageerror", (e) => report.errors.push(String(e)));
        await page.route("**/api/user/reports**", (route) =>
          route.fulfill({
            status: 200,
            contentType: "application/json",
            body: JSON.stringify({
              success: true,
              data: {
                items: [{ username: "private-legacy-account" }],
                total: 1,
              },
            }),
          }),
        );
        await page.goto(base + "/console/usage");
        const incompatibility = page.locator('.usage-reports [role="alert"]');
        await incompatibility.waitFor();
        const notice = await incompatibility.innerText();
        assert.ok(
          notice.includes(
            language === "zh"
              ? "当前后端版本不兼容"
              : language === "ar"
                ? "غير متوافقة"
                : "incompatible",
          ),
        );
        assert.equal(await page.locator(".usage-summary-value").count(), 0);
        assert.ok(
          !(await page.locator("body").innerText()).includes(
            "private-legacy-account",
          ),
        );
        const incompatibleShot = `incompatible-${language}-${theme}-${width}.png`;
        await page.screenshot({
          path: path.join(out, incompatibleShot),
          fullPage: true,
          animations: "disabled",
        });
        report.screenshots.push(incompatibleShot);
        await page.unroute("**/api/user/reports**");
        await incompatibility.getByRole("button").click();
        await page.waitForFunction(
          () =>
            document.querySelector(".usage-summary-value")?.textContent ===
            "311",
        );
        assert.ok(
          (
            await page.locator(".usage-summary-value").nth(1).innerText()
          ).includes("0.31"),
        );
        assert.ok(
          (
            await page.locator(".usage-summary-value").nth(2).innerText()
          ).includes("99.68"),
        );
        const filters = page.locator(".usage-filters select");
        for (const dimension of [
          "model",
          "vendor",
          "call_type",
          "group",
          "key",
        ]) {
          await filters.nth(6).selectOption(dimension);
          await page.waitForFunction(
            () => !document.querySelector('.usage-reports [role="status"]'),
          );
          await page.waitForFunction(
            (expected) =>
              document.querySelectorAll(".usage-table-scroll tbody tr")
                .length === expected,
            dimension === "key" ? 2 : 1,
          );
        }
        await filters.nth(7).selectOption("hour");
        await page.locator(".recharts-surface").first().waitFor();
        for (const value of ["requests", "tokens", "cost"])
          await page.locator(".usage-heading select").selectOption(value);
        await filters.nth(1).selectOption(String(second.id));
        await page.waitForFunction(
          () =>
            document.querySelector(".usage-summary-value")?.textContent ===
            "11",
        );
        assert.ok(
          (
            await page.locator(".usage-summary-value").nth(1).innerText()
          ).includes("0.01"),
        );
        await filters.nth(1).selectOption("");
        await page.waitForFunction(
          () =>
            document.querySelector(".usage-summary-value")?.textContent ===
            "311",
        );
        const screenshot = async (name) => {
          const file = `${name}-${language}-${theme}-${width}.png`;
          await page.screenshot({
            path: path.join(out, file),
            fullPage: true,
            animations: "disabled",
          });
          report.screenshots.push(file);
        };
        assert.equal(
          await page.evaluate(
            () => document.documentElement.scrollWidth > innerWidth + 1,
          ),
          false,
        );
        await screenshot("usage");
        if (language === "zh" && theme === "light" && width === 1440) {
          await page.locator(".usage-heading > button").click();
          const article = page.locator(".export-history article").first();
          await article.waitFor();
          const link = article.getByRole("button", {
            name: "下载 CSV",
            exact: true,
          });
          await link.waitFor();
          const waiting = page.waitForEvent("download");
          await link.click();
          const download = await waiting;
          const bytes = await (
            await import("node:fs/promises")
          ).readFile(await download.path());
          assert.ok(bytes.subarray(0, 3).equals(Buffer.from([239, 187, 191])));
          const csv = bytes.toString("utf8");
          assert.ok(csv.includes("300000000"));
          assert.ok(csv.includes("10000000"));
          assert.ok(csv.includes("'=HYPERLINK"));
          report.checks.push(
            "Filtered Key report CSV matches both exact amounts, BOM and formula-safe key name",
          );
          await page.goto(
            base +
              "/console/usage?range=custom&start_date=2024-01-01&end_date=2026-11-03",
          );
          await page.locator('.usage-reports [role="alert"]').waitFor();
          assert.equal(await page.locator(".usage-summary-value").count(), 0);
          report.checks.push(
            "Over-one-year range rejected without stale totals",
          );
        }
        await page.goto(base + "/console/requests?range=30d");
        await page.locator(".request-history-table tbody tr").first().waitFor();
        assert.equal(
          await page.locator(".request-history-table tbody tr").count(),
          20,
        );
        assert.ok(
          (
            await page.locator(".request-history-pagination").innerText()
          ).includes("311"),
        );
        await page
          .locator(".request-history-toolbar select")
          .nth(0)
          .selectOption(String(second.id));
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".request-history-table tbody tr")
              .length === 11,
        );
        await page
          .locator(".request-history-toolbar select")
          .nth(0)
          .selectOption("");
        await page.locator(".request-history-search input").fill(firstTrace);
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".request-history-table tbody tr")
              .length === 1,
        );
        assert.equal(
          (await page.locator(".request-trace-button").innerText()).trim(),
          firstTrace,
        );
        assert.equal(
          await page.evaluate(
            () => document.documentElement.scrollWidth > innerWidth + 1,
          ),
          false,
        );
        await screenshot("history");
        await page.locator(".request-trace-button").click();
        const dialog = page.getByRole("dialog");
        await dialog.waitFor();
        await page.waitForFunction(() => {
          const r = document
            .querySelector('[role="dialog"]')
            .getBoundingClientRect();
          return r.left >= -1 && r.right <= innerWidth + 1;
        });
        assert.equal(
          await dialog.evaluate((el) => el.scrollWidth > el.clientWidth + 1),
          false,
        );
        const text = await dialog.innerText();
        assert.ok(text.includes("0.001000000"));
        assert.ok(text.includes("default"));
        for (const privateText of [
          "synthetic-supplier",
          "Synthetic billing check",
          "Hello world",
        ])
          assert.ok(!text.includes(privateText));
        await screenshot("detail");
        await dialog.locator(".settled-billing").scrollIntoViewIfNeeded();
        await screenshot("billing-detail");
        await page.keyboard.press("Escape");
        await dialog.waitFor({ state: "hidden" });
        await page.goto(base + "/console/requests?trace=" + firstTrace);
        await page.getByRole("dialog").waitFor();
        assert.ok(
          (await page.getByRole("dialog").innerText()).includes(firstTrace),
        );
        report.checks.push(
          `${language}/${theme}/${width}: legacy API contract rejection and retry recovery; five dimensions, hourly trend, three metrics, Key filters, 311-row pagination, exact trace, private frozen-rate drawer and old-trace direct link; no viewport overflow`,
        );
        await context.close();
        console.log(`PASS ${language}/${theme}/${width}`);
      }
  assert.deepEqual(report.errors, []);
  await seed.close();
} finally {
  await writeFile(
    path.join(out, "browser-report.json"),
    JSON.stringify(report, null, 2),
  );
  await browser?.close();
  await vite?.close();
  child.kill();
  await new Promise((resolve) => upstream.close(resolve));
}
