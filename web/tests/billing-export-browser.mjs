// Isolated UI-3.6 and BIL-AC-27 acceptance against the real gateway and a local synthetic provider.
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
  process.env.STARHUB_BILL_BROWSER_OUTPUT ??
    path.join(
      "billing-export-repair-20261103",
      alternateRegion ? "browser-new-york" : "browser",
    ),
);
await mkdir(out, { recursive: true });
const temp = await mkdtemp(path.join(out, "run-"));
const binary = path.join(out, "billing-export-check.exe");
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
let upstreamCalls = 0,
  lastBody;
const upstream = http.createServer(async (req, res) => {
  const buffers = [];
  for await (const chunk of req) buffers.push(chunk);
  const body = JSON.parse(Buffer.concat(buffers).toString());
  lastBody = body;
  upstreamCalls++;
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
      }),
    );
  const summary = await seed.request.get(base + "/api/wallet");
  const wallet = (await summary.json()).data;
  assert.equal(wallet.month_spend_nano, "300000000");
  assert.equal(wallet.api_balance_nano, "199700000000");
  const state = await seed.storageState();
  const words = {
    zh: {
      csv: "导出记录",
      ledger: "账本流水",
      month: "本月消费",
      file: "下载 CSV",
      fields: "字段说明",
      create: "创建导出",
      generate: "生成 CSV（表格文件）",
    },
    en: {
      csv: "Export",
      ledger: "Ledger",
      month: "Usage this month",
      file: "Download CSV",
      fields: "Field descriptions",
      create: "Create export",
      generate: "Generate CSV export",
    },
    ar: {
      csv: "تصدير",
      ledger: "السجل المالي",
      month: "استهلاك الشهر",
      file: "تنزيل CSV",
      fields: "أوصاف الحقول",
      create: "إنشاء تصدير",
      generate: "إنشاء ملف CSV",
    },
  };
  for (const language of alternateRegion ? ["zh"] : ["zh", "en", "ar"])
    for (const theme of alternateRegion ? ["light"] : ["light", "dark"])
      for (const width of alternateRegion ? [1440] : [1440, 390]) {
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
        page.setDefaultTimeout(15000);
        page.on("pageerror", (error) => report.errors.push(String(error)));
        await page.goto(base + "/console/billing?tab=ledger");
        await page.locator(".bc-table-scroll tbody tr").first().waitFor();
        const usage = page
          .locator(".bc-month-metric")
          .filter({ hasText: words[language].month });
        assert.ok((await usage.innerText()).includes("0.30"));
        assert.ok(
          (await page.locator(".bc-balance").innerText()).includes("199.70"),
        );
        await page.locator(".bc-filters select").selectOption("recharge");
        await page.waitForFunction(
          () =>
            document.querySelectorAll(".bc-table-scroll tbody tr").length === 2,
        );
        const table = await page.locator(".bc-table-scroll").innerText();
        assert.ok(!table.includes("call_summary"));
        assert.equal(
          await page.evaluate(
            () => document.documentElement.scrollWidth > innerWidth + 1,
          ),
          false,
        );
        const billingShot = `billing-${language}-${theme}-${width}.png`;
        await page.screenshot({
          path: path.join(out, billingShot),
          fullPage: true,
        });
        report.screenshots.push(billingShot);
        const downloaded = page.waitForEvent("download");
        await page
          .getByRole("button", { name: words[language].csv, exact: true })
          .click();
        const download = await downloaded;
        const file = await download.path();
        const bytes = await (await import("node:fs/promises")).readFile(file);
        assert.ok(bytes.subarray(0, 3).equals(Buffer.from([239, 187, 191])));
        const csv = bytes.toString("utf8");
        assert.equal(csv.trim().split(/\r?\n/).length, 3);
        assert.ok(csv.includes("10.000000000") && csv.includes("20.000000000"));
        const headers = { zh: "流水编号", en: "Ledger ID", ar: "معرّف الحركة" };
        assert.ok(csv.includes(headers[language]));
        await page.goto(base + "/console/exports");
        await page.locator(".export-history article").first().waitFor();
        const record = page.locator(".export-history article").first();
        const dictionaryDownload = page.waitForEvent("download");
        await record
          .getByRole("button", { name: words[language].fields, exact: true })
          .click();
        const dictionaryFile = await (await dictionaryDownload).path();
        const dictionary = await (
          await import("node:fs/promises")
        ).readFile(dictionaryFile, "utf8");
        assert.ok(dictionary.includes("nine decimal places"));
        assert.equal(
          await page.evaluate(
            () => document.documentElement.scrollWidth > innerWidth + 1,
          ),
          false,
        );
        const exportShot = `exports-${language}-${theme}-${width}.png`;
        await page.screenshot({
          path: path.join(out, exportShot),
          fullPage: true,
        });
        report.screenshots.push(exportShot);
        await page.goto(base + "/console/requests?range=30d");
        await page.waitForFunction(
          (zone) =>
            document
              .querySelector(".request-history-summary")
              ?.textContent.includes("300") &&
            document
              .querySelector(".request-history-summary")
              ?.textContent.includes(zone),
          alternateRegion ?? "Asia/Dubai",
        );
        assert.equal(
          await page.locator(".request-history-table tbody tr").count(),
          20,
        );
        assert.equal(
          await page.evaluate(
            () => document.documentElement.scrollWidth > innerWidth + 1,
          ),
          false,
        );
        const requestsShot = `requests-${language}-${theme}-${width}.png`;
        await page.screenshot({
          path: path.join(out, requestsShot),
          fullPage: true,
        });
        report.screenshots.push(requestsShot);
        await page.goto(base + "/console/exports");
        await page.locator(".export-history article").first().waitFor();
        if (language === "zh" && theme === "light" && width === 1440) {
          await page
            .getByRole("button", { name: words[language].create, exact: true })
            .click();
          const dialog = page.getByRole("dialog");
          await page.locator("#export-task-type").selectOption("requests");
          const date = new Date();
          const end = new Intl.DateTimeFormat("en-CA", {
            timeZone: wallet.timezone,
            year: "numeric",
            month: "2-digit",
            day: "2-digit",
          }).format(date);
          const start = new Date(Date.now() - 44 * 86400000)
            .toISOString()
            .slice(0, 10);
          await page.locator("#export-task-start").fill(start);
          await page.locator("#export-task-end").fill(end);
          await dialog
            .getByRole("button", {
              name: words[language].generate,
              exact: true,
            })
            .click();
          assert.equal(await dialog.isVisible(), true);
          assert.ok((await dialog.innerText()).includes("31"));
          await page.locator("#export-task-start").fill(end);
          await dialog
            .getByRole("button", {
              name: words[language].generate,
              exact: true,
            })
            .click();
          await dialog.waitFor({ state: "hidden" });
          await page.waitForFunction(
            () =>
              document.querySelectorAll(".export-history article").length >= 2,
          );
          const requests = page.locator(".export-history article").first();
          const requestDownload = page.waitForEvent("download");
          await requests
            .getByRole("button", { name: words[language].file, exact: true })
            .click();
          const requestFile = await (await requestDownload).path();
          const contents = await (
            await import("node:fs/promises")
          ).readFile(requestFile, "utf8");
          assert.ok(contents.includes("'=HYPERLINK"));
          assert.equal(contents.trim().split(/\r?\n/).length, 301);
          report.checks.push(
            "45-day range rejected; real 300-row request export guards formula-prefixed key names",
          );
          await page.goto(base + "/console/requests?range=30d");
          await page.waitForFunction(() =>
            document
              .querySelector(".request-history-summary")
              ?.textContent.includes("300"),
          );
          assert.equal(
            await page.locator(".request-history-table tbody tr").count(),
            20,
          );
          await page
            .getByRole("button", { name: "下一页", exact: true })
            .click();
          await page.waitForFunction(() =>
            document
              .querySelector(".request-history-pagination")
              ?.textContent.includes("21–40"),
          );
          await page
            .getByRole("button", { name: "创建导出任务", exact: true })
            .click();
          await page
            .getByRole("dialog")
            .getByRole("button", {
              name: words[language].generate,
              exact: true,
            })
            .click();
          await page.getByRole("dialog").waitFor({ state: "hidden" });
          await page.locator(".export-history article").first().waitFor();
          assert.ok(
            (
              await page.locator(".export-history article").first().innerText()
            ).includes("300"),
          );
          report.checks.push(
            "server-paged request history shows 300 total rows, advances by 20 and exports the same 300 filtered rows",
          );
        }
        report.checks.push(
          `${language}/${theme}/${width}: real 300-call monthly usage, available API balance, two-row recharge filter/export, exact money, BOM, language, dictionary, export history and viewport`,
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
