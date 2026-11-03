// Isolated UI-3.3 acceptance against the real gateway and a local synthetic provider.
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
const out = path.join(project, "output", process.env.STARHUB_PLAYGROUND_BROWSER_OUTPUT ?? path.join("playground-20261103", "browser"));
await mkdir(out, { recursive: true });
const temp = await mkdtemp(path.join(out, "run-"));
const binary = path.join(out, "playground-check.exe");
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
  for (const key of ["ip_rate_limit", "token_rate_limit", "user_rate_limit"])
    await option(key, "10000");
  await option(
    "model_price",
    '{"gpt-4.1":0.001,"gpt-4.1-mini":0.002,"gemini-test":0.001}',
  );
  for (const [type, models] of [
    [1, "gpt-4.1,gpt-4.1-mini"],
    [25, "gemini-test"],
  ]) {
    const res = await seed.request.post(base + "/api/channel/", {
      data: {
        name: "Synthetic playground",
        type,
        models,
        group: "default",
        base_url: supplier,
        key: "synthetic-provider-key",
        status: 1,
      },
    });
    assert.equal(res.status(), 200);
  }
  for (const [model, vendor] of [
    ["gpt-4.1", "OpenAI"],
    ["gpt-4.1-mini", "OpenAI"],
    ["gemini-test", "Google"],
  ]) {
    const res = await seed.request.put(base + "/api/plaza/meta", {
      data: { model, display_name: model, vendor, tags: "text" },
    });
    assert.equal(res.status(), 200);
  }
  const create = async (name, fields = {}, client = seed) => {
    const res = await client.request.post(base + "/api/token", {
      data: {
        name,
        group: "default",
        model_limits: "gpt-4.1,gpt-4.1-mini,gemini-test",
        smart_access: true,
        smart_candidates: "gpt-4.1,gpt-4.1-mini",
        unlimited_quota: true,
        ...fields,
      },
    });
    assert.equal(res.status(), 200);
    return (await res.json()).data;
  };
  const key = await create("Active playground key");
  const disabled = await create("Disabled hidden key");
  const stopped = await seed.request.put(base + "/api/token", {
    data: { id: disabled.id, status: 2 },
  });
  assert.equal(stopped.status(), 200);
  const expired = await create("Expired hidden key", {
    expired_time: Math.floor(Date.now() / 1000) - 60,
  });
  const storageState = await seed.storageState();
  const fixtures = { zh: { key, disabled, expired, storageState } };
  // Separate owners exercise customer sessions and preserve the real password rate limit.
  for (const language of ["en", "ar"]) {
    const client = await browser.newContext();
    const username = `playground-${language}`;
    const registered = await client.request.post(base + "/api/user/register", { data: { username, password } });
    assert.equal(registered.status(), 200);
    const user = (await registered.json()).data;
    const funded = await seed.request.put(base + "/api/user", { data: { id: user.id, quota: 1000000, expected_quota: user.quota } });
    assert.equal(funded.status(), 200);
    const signedIn = await client.request.post(base + "/api/user/login", { data: { username, password } });
    assert.equal(signedIn.status(), 200);
    const active = await create("Active playground key", {}, client);
    const stoppedKey = await create("Disabled hidden key", {}, client);
    const paused = await client.request.put(base + "/api/token", { data: { id: stoppedKey.id, status: 2 } });
    assert.equal(paused.status(), 200);
    const expiredKey = await create("Expired hidden key", { expired_time: Math.floor(Date.now() / 1000) - 60 }, client);
    fixtures[language] = { key: active, disabled: stoppedKey, expired: expiredKey, storageState: await client.storageState() };
    await client.close();
  }
  const labels = {
    zh: {
      send: "发送请求",
      verify: "验证并调用",
      history: "查看调用明细",
      source: "调试台",
    },
    en: {
      send: "Send request",
      verify: "Verify and send",
      history: "View request history",
      source: "Playground",
    },
    ar: {
      send: "إرسال الطلب",
      verify: "تحقق وأرسل",
      history: "عرض سجل الطلبات",
      source: "مساحة التجربة",
    },
  };
  for (const language of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const { key, disabled, expired, storageState } = fixtures[language];
        const context = await browser.newContext({
          storageState,
          viewport: { width, height: width === 390 ? 844 : 1000 },
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
        const text = labels[language];
        await page.goto(
          `${base}/console/playground?key=${key.id}&model=myapis-auto`,
        );
        await page
          .locator("#verify-model option[value='myapis-auto']")
          .waitFor({ state: "attached" });
        const keys = await page.locator("#verify-key option").allTextContents();
        assert.ok(keys.includes(key.name));
        assert.ok(!keys.includes(disabled.name));
        assert.ok(!keys.includes(expired.name));
        assert.equal(
          await page
            .locator("#verify-model option")
            .first()
            .getAttribute("value"),
          "myapis-auto",
        );
        const marker = `Synthetic private draft ${language}-${theme}-${width}`;
        await page.locator("#verify-prompt").fill(marker);
        const before = upstreamCalls;
        await page
          .getByRole("button", { name: text.send, exact: true })
          .click();
        await page.locator("#playground-password").waitFor();
        assert.equal(upstreamCalls, before);
        if (language === "zh" && theme === "light" && width === 1440) {
          await page
            .locator("#playground-password")
            .fill("synthetic-wrong-password");
          await page
            .getByRole("button", { name: text.verify, exact: true })
            .click();
          await page.locator("#playground-password-help").waitFor();
          assert.equal(upstreamCalls, before);
        }
        await page.locator("#playground-password").fill(password);
        await page
          .getByRole("button", { name: text.verify, exact: true })
          .click();
        await page.waitForFunction(
          () =>
            document.querySelector(".verify-output")?.textContent ===
              "Hello " &&
            document.querySelector(".verify-send button")?.disabled,
        );
        await page
          .getByRole("button", { name: text.send, exact: true })
          .waitFor({ state: "visible" });
        await page.waitForFunction(
          () =>
            document.querySelector(".verify-metrics") &&
            !document.querySelector(".verify-send button")?.disabled,
        );
        assert.equal(
          await page.locator(".verify-output").innerText(),
          "Hello world",
        );
        assert.equal(upstreamCalls, before + 1);
        assert.equal(lastBody.messages.at(-1).content, marker);
        const trace = (await page.locator(".verify-trace").innerText()).replace(
          "Trace ID: ",
          "",
        );
        assert.match(trace, /^[a-f0-9]{64}$/);
        const metadata = await page.request.get(
          base + "/api/playground/results/" + trace,
        );
        assert.equal(metadata.status(), 200);
        const result = (await metadata.json()).data;
        assert.equal(result.cost_nano, "1000000");
        assert.equal(result.status, 1);
        assert.ok(result.ttft_ms > 0);
        assert.ok(result.duration_ms >= 400);
        assert.equal(result.source, "playground");
        const usage = await page.request.get(
          base + `/api/token/${key.id}/usage`,
        );
        assert.ok((await usage.json()).data.used_quota > 0);
        const storage = await page.evaluate(() =>
          [JSON.stringify(localStorage), JSON.stringify(sessionStorage)].join(
            "\n",
          ),
        );
        for (const secret of [key.key, disabled.key, expired.key, password])
          assert.ok(!storage.includes(secret));
        const overflow = await page.evaluate(
          () => document.documentElement.scrollWidth > innerWidth + 1,
        );
        assert.equal(overflow, false);
        const shot = `playground-${language}-${theme}-${width}.png`;
        await page.screenshot({ path: path.join(out, shot), fullPage: true });
        report.screenshots.push(shot);
        await page.locator(".verify-code summary").click();
        for (const codeLanguage of ["Python", "Node.js", "cURL"]) {
          await page
            .getByRole("button", { name: codeLanguage, exact: true })
            .click();
          const code = await page.locator(".verify-code code").innerText();
          assert.ok(code.includes("MYAPIS_API_KEY"));
          assert.ok(!code.includes(key.key));
        }
        // A second request reuses the password window and performs a fresh audited reveal.
        await page
          .getByRole("button", { name: text.send, exact: true })
          .click();
        await page.waitForFunction(
          (old) =>
            document.querySelector(".verify-trace")?.textContent &&
            !document
              .querySelector(".verify-trace")
              .textContent.includes(old) &&
            !document.querySelector(".verify-send button")?.disabled,
          trace,
        );
        assert.equal(await page.locator("#playground-password").count(), 0);
        assert.equal(upstreamCalls, before + 2);
        if (language === "zh" && theme === "light" && width === 1440) {
          await page.locator(".verify-advanced summary").click();
          await page.locator("#verify-stream").uncheck();
          await page.locator("#verify-model").selectOption("gpt-4.1");
          await page.locator("#verify-protocol").selectOption("Anthropic");
          await page
            .getByRole("button", { name: text.send, exact: true })
            .click();
          await page.waitForFunction(
            () =>
              document.querySelector(".verify-metrics") &&
              !document.querySelector(".verify-send button")?.disabled,
          );
          assert.equal(
            await page.locator(".verify-output").innerText(),
            "Hello world",
          );
          await page.locator("#verify-protocol").selectOption("Gemini");
          await page.locator("#verify-model").selectOption("gemini-test");
          await page.locator("#verify-stream").check();
          await page
            .getByRole("button", { name: text.send, exact: true })
            .click();
          await page.waitForFunction(
            () =>
              document.querySelector(".verify-metrics") &&
              !document.querySelector(".verify-send button")?.disabled,
          );
          assert.equal(
            await page.locator(".verify-output").innerText(),
            "Hello world",
          );
          await page
            .getByRole("link", { name: text.history, exact: true })
            .click();
          await page
            .locator(".request-history")
            .waitFor()
            .catch(() => {});
          const history = await page.request.get(
            base + "/api/log?page_size=100",
          );
          const rows = (await history.json()).data.items;
          assert.ok(
            rows.some(
              (row) =>
                row.trace_id === trace && row.request_source === "playground",
            ),
          );
          assert.ok(!JSON.stringify(rows).includes(marker));
          await page.getByRole("button", { name: trace, exact: true }).click();
          const drawer = page.getByRole("dialog");
          await drawer.waitFor();
          assert.equal(
            await drawer
              .locator("dt")
              .filter({ hasText: /^来源$/ })
              .first()
              .locator("..")
              .locator("dd")
              .innerText(),
            "调试台",
          );
          report.checks.push(
            "Anthropic JSON, Gemini streaming, settled history and content privacy",
          );
        }
        report.checks.push(
          `${language}/${theme}/${width}: effective key list, first-use password gate, incremental output, server trace/fee, quota, memory-only credential, five-minute reuse, code previews and viewport`,
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
