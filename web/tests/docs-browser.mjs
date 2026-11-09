import assert from "node:assert/strict";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { spawn } from "node:child_process";
import { createServer as httpServer } from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { chromium } from "playwright";

const web = fileURLToPath(new URL("../", import.meta.url));
const out = path.resolve(
  web,
  process.env.DOCS_OUTPUT || "../output/ui24/browser",
);
await mkdir(out, { recursive: true });
const report = { cases: [], errors: [], snippets: [] };
const requests = [];
const fixture = httpServer(async (req, res) => {
  let body = "";
  for await (const chunk of req) body += chunk;
  requests.push({
    path: req.url,
    method: req.method,
    authorization: req.headers.authorization,
    body,
  });
  res.setHeader("Content-Type", "application/json");
  res.end(
    JSON.stringify(
      req.url === "/v1/models"
        ? {
            object: "list",
            data: [{ id: "docs-fixture-model", object: "model" }],
          }
        : {
            id: "chatcmpl-fixture",
            object: "chat.completion",
            model: "docs-fixture-model",
            choices: [
              {
                index: 0,
                message: { role: "assistant", content: "Hello!" },
                finish_reason: "stop",
              },
            ],
          },
    ),
  );
});
await new Promise((resolve) => fixture.listen(0, "127.0.0.1", resolve));
const fixtureBase = `http://127.0.0.1:${fixture.address().port}/v1`;
const vite = await createServer({
  root: web,
  server: { host: "127.0.0.1", port: 0, strictPort: false },
});
let browser;
const run = (file, args, env) =>
  new Promise((resolve, reject) => {
    const child = spawn(file, args, {
      env: { ...process.env, ...env },
      windowsHide: true,
    });
    let stdout = "",
      stderr = "";
    child.stdout.on("data", (data) => (stdout += data));
    child.stderr.on("data", (data) => (stderr += data));
    child.on("error", reject);
    child.on("close", (code) =>
      code === 0
        ? resolve(stdout)
        : reject(new Error(`${file}: ${code}: ${stderr}`)),
    );
  });
try {
  await vite.listen();
  browser = await chromium.launch({
    ...(process.platform === "win32" ? { channel: "chrome" } : {}),
    headless: true,
  });
  const origin = vite.resolvedUrls.local[0].replace(/\/$/, "");
  for (const lang of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const name = `${lang}-${theme}-${width}`;
        const labels = JSON.parse(
          await readFile(
            path.join(web, `src/i18n/messages.${lang}.json`),
            "utf8",
          ),
        );
        const context = await browser.newContext({
          viewport: { width, height: 1100 },
          reducedMotion: "reduce",
          permissions: ["clipboard-read", "clipboard-write"],
        });
        await context.addInitScript(
          ({ lang, theme }) => {
            localStorage.setItem("xy-language", lang);
            localStorage.setItem("xy-theme", theme);
          },
          { lang, theme },
        );
        await context.route(
          (url) => url.pathname.startsWith("/api/"),
          (route) => route.fulfill({ json: { success: true, data: {} } }),
        );
        const page = await context.newPage();
        const openMenu = async () => {
          if (
            width <= 900 &&
            (await page
              .locator(".docs-menu-toggle")
              .getAttribute("aria-expanded")) !== "true"
          )
            await page.locator(".docs-menu-toggle").click();
        };
        page.on("pageerror", (error) =>
          report.errors.push({ name, error: error.message }),
        );
        await page.goto(origin + "/docs");
        await page.locator(".docs-quickstart").waitFor();
        assert.equal(await page.locator("html").getAttribute("lang"), lang);
        assert.equal(
          await page.locator("html").getAttribute("dir"),
          lang === "ar" ? "rtl" : "ltr",
        );
        assert.equal(
          await page
            .getByLabel(labels.docs24_baseLabel, { exact: true })
            .inputValue(),
          "https://api.myapis.ai/v1",
        );
        await page
          .getByLabel(labels.docs24_baseLabel, { exact: true })
          .fill("https://gateway.example/v1");
        await page.locator(".docs-article h1").click();
        await page.evaluate(() => document.fonts.ready);
        await page.screenshot({
          path: path.join(out, `${name}.png`),
          animations: "disabled",
        });
        if (width <= 900) {
          assert.equal(
            await page
              .locator(".docs-menu-toggle")
              .getAttribute("aria-expanded"),
            "false",
          );
          await openMenu();
          await page.keyboard.press("Escape");
          assert.equal(
            await page
              .locator(".docs-menu-toggle")
              .getAttribute("aria-expanded"),
            "false",
          );
        } else {
          assert.equal(await page.locator(".docs-outline a").count(), 4);
          await page.locator(".docs-outline a").nth(2).click();
          assert.ok(
            new URL(page.url()).hash.startsWith("#docs-quickstart-section-"),
          );
        }
        const ids = await page.locator(".docs-sidebar nav button").count();
        assert.equal(ids, 22);
        await openMenu();
        assert.equal(
          await page
            .locator(".docs-nav-group")
            .filter({
              has: page.getByRole("heading", {
                name: labels.docsDesign_groupTools,
                exact: true,
              }),
            })
            .getByRole("button", {
              name: labels.docsDesign_codexTitle,
              exact: true,
            })
            .count(),
          1,
        );
        assert.ok(
          await page
            .getByText(labels.docs24_fundBoundary, { exact: true })
            .count(),
        );
        assert.equal(
          await page
            .getByRole("link", {
              name: labels.docs24_registerLink,
              exact: true,
            })
            .getAttribute("href"),
          "/register",
        );
        for (let index = 0; index < ids; index++) {
          await openMenu();
          await page.locator(".docs-sidebar nav button").nth(index).click();
          await page.waitForFunction(
            (index) =>
              document
                .querySelectorAll(".docs-sidebar nav button")
                [index]?.getAttribute("aria-current") === "page",
            index,
          );
          await page.locator(".docs-code pre:visible").first().waitFor();
          assert.ok(
            await page.locator(".docs-code button").count(),
            `${name} article ${index}: no copy action`,
          );
          assert.ok(
            await page
              .locator(".docs-code pre:visible")
              .evaluateAll((nodes) =>
                nodes.every(
                  (node) => getComputedStyle(node).direction === "ltr",
                ),
              ),
            name +
              " code direction " +
              JSON.stringify(
                await page
                  .locator(".docs-code pre:visible")
                  .evaluateAll((nodes) =>
                    nodes.map((node) => ({
                      dir: getComputedStyle(node).direction,
                      attribute: node.getAttribute("dir"),
                      text: node.textContent.slice(0, 40),
                    })),
                  ),
              ),
          );
          assert.ok(
            await page.evaluate(
              () => document.documentElement.scrollWidth <= innerWidth + 1,
            ),
            `${name} article ${index}: page overflow`,
          );
          const articleID = new URL(page.url()).searchParams.get("article");
          if (articleID === "codex") {
            const config = page
              .locator(".docs-code")
              .filter({ hasText: "config.toml" });
            const code = await config.locator("pre").innerText();
            assert.ok(code.includes('base_url = "https://gateway.example/v1"'));
            assert.ok(code.includes('env_key = "MYAPIS_API_KEY"'));
            assert.ok(code.includes('wire_api = "responses"'));
            await config.getByRole("button").click();
            assert.equal(
              (
                await page.evaluate(() => navigator.clipboard.readText())
              ).replaceAll("\r\n", "\n"),
              code,
            );
            await page.locator(".docs-article h1").scrollIntoViewIfNeeded();
            await page.screenshot({
              path: path.join(out, `${name}-codex.png`),
              animations: "disabled",
            });
          }
          if (articleID === "responses") {
            assert.equal(
              await page
                .locator(".docs-code")
                .filter({ hasText: "config.toml" })
                .count(),
              0,
            );
          }
          if (["audio", "images", "video"].includes(articleID)) {
            const operationTabs = page.locator(
              ".docs-operation-examples [role='tab']",
            );
            for (
              let tabIndex = 0;
              tabIndex < (await operationTabs.count());
              tabIndex++
            ) {
              await operationTabs.nth(tabIndex).click();
              assert.equal(
                await page
                  .locator(".docs-operation-examples pre:visible")
                  .count(),
                1,
              );
              await page
                .locator(
                  ".docs-operation-examples [role='tabpanel']:visible button",
                )
                .click();
              const copied = (
                await page.evaluate(() => navigator.clipboard.readText())
              ).replaceAll("\r\n", "\n");
              assert.equal(
                copied,
                await page
                  .locator(".docs-operation-examples pre:visible")
                  .innerText(),
              );
            }
            await operationTabs.first().click();
          }
          if (articleID === "audio" && width === 1440) {
            await page.locator(".docs-base > summary").click();
            await page
              .getByLabel(labels.docs24_baseLabel, { exact: true })
              .fill("https://gateway.example/proxy/v1");
            const anchor = page.locator(".docs-outline a").first();
            const target = await anchor.getAttribute("href");
            assert.equal(
              await page.locator(target).count(),
              1,
              "outline remains attached after endpoint edit",
            );
            await anchor.click();
            await page
              .getByLabel(labels.docs24_baseLabel, { exact: true })
              .fill("https://gateway.example/v1");
            await page.locator(".docs-base > summary").click();
          }
          if (
            lang === "zh" &&
            theme === "light" &&
            ["audio", "protocols"].includes(articleID)
          ) {
            await page.locator(".docs-article h1").scrollIntoViewIfNeeded();
            await page.screenshot({
              path: path.join(out, `${name}-${articleID}.png`),
              animations: "disabled",
            });
          }
          assert.ok(
            !(await page.locator("main").innerText()).includes(
              "https://api.myapis.ai",
            ),
            name + " stale demo endpoint",
          );
        }
        await page.goto(origin + "/docs?article=codex");
        await page
          .getByRole("heading", {
            level: 1,
            name: labels.docsDesign_codexTitle,
            exact: true,
          })
          .waitFor();
        await page
          .locator("main")
          .getByRole("link", {
            name: labels.docsDesign_responsesTitle,
            exact: true,
          })
          .click();
        await page
          .getByRole("heading", {
            level: 1,
            name: labels.docsDesign_responsesTitle,
            exact: true,
          })
          .waitFor();
        await page
          .locator("main")
          .getByRole("link", {
            name: labels.docsDesign_codexTitle,
            exact: true,
          })
          .click();
        await page.reload();
        await page
          .getByRole("heading", {
            level: 1,
            name: labels.docsDesign_codexTitle,
            exact: true,
          })
          .waitFor();
        await page.goto(origin + "/docs?article=video");
        await page
          .getByRole("heading", {
            level: 1,
            name: labels.docs24_video,
            exact: true,
          })
          .waitFor();
        await openMenu();
        await page.locator(".docs-sidebar nav button").first().click();
        await page.goBack();
        await page
          .getByRole("heading", {
            level: 1,
            name: labels.docs24_video,
            exact: true,
          })
          .waitFor();
        await openMenu();
        const search = page.getByRole("searchbox");
        await search.fill("Codex");
        await page
          .getByRole("heading", {
            level: 1,
            name: labels.docsDesign_codexTitle,
            exact: true,
          })
          .waitFor();
        assert.equal(await page.locator(".docs-sidebar nav button").count(), 1);
        await search.fill("GOOGLE_GEMINI_BASE_URL");
        await page
          .getByRole("heading", {
            level: 1,
            name: labels.docs24_geminiCli,
            exact: true,
          })
          .waitFor();
        await search.fill("no-result-fixture-123");
        assert.equal(await page.locator(".docs-sidebar nav button").count(), 0);
        await search.fill("");
        await openMenu();
        await page.locator(".docs-sidebar nav button").first().click();
        const baseField = page.getByLabel(labels.docs24_baseLabel, {
          exact: true,
        });
        await baseField.fill("https://gateway.example/v1?secret=invalid");
        assert.equal(
          await page.locator(".docs-code button:enabled").count(),
          0,
        );
        await baseField.fill(fixtureBase);
        await page
          .getByLabel(labels.docs24_modelLabel, { exact: true })
          .fill("docs-fixture-model");
        const requestBlock = () =>
          page.locator(".docs-code").filter({
            has: page.getByText(labels.docs24_send, { exact: true }),
          });
        await page.getByRole("tab", { name: "cURL", exact: true }).focus();
        await page.keyboard.press("End");
        await page.waitForFunction(() =>
          [...document.querySelectorAll('[role="tab"]')].some(
            (node) =>
              node.textContent === "PowerShell" &&
              node.getAttribute("aria-selected") === "true",
          ),
        );
        await page.keyboard.press("Home");
        await page.waitForFunction(() =>
          [...document.querySelectorAll('[role="tab"]')].some(
            (node) =>
              node.textContent === "cURL" &&
              node.getAttribute("aria-selected") === "true",
          ),
        );
        await requestBlock().getByRole("button").click();
        const bash = (
          await page.evaluate(() => navigator.clipboard.readText())
        ).replaceAll("\r\n", "\n");
        assert.equal(bash, await requestBlock().locator("pre").innerText());
        await page
          .getByRole("tab", { name: "PowerShell", exact: true })
          .click();
        await requestBlock().getByRole("button").click();
        const powershell = await page.evaluate(() =>
          navigator.clipboard.readText(),
        );
        assert.ok(powershell.includes("Invoke-RestMethod"));
        await requestBlock().scrollIntoViewIfNeeded();
        await page.screenshot({
          path: path.join(out, `${name}-request.png`),
          animations: "disabled",
        });
        await requestBlock().locator("pre").focus();
        assert.ok(
          await requestBlock()
            .locator("pre")
            .evaluate((node) => node === document.activeElement),
        );
        if (lang === "en" && theme === "light" && width === 1440) {
          const env = {
            MYAPIS_BASE_URL: fixtureBase,
            MYAPIS_API_KEY: "synthetic-docs-key",
            MYAPIS_MODEL: "docs-fixture-model",
          };
          const bashPath =
            process.env.DOCS_BASH ||
            (process.platform === "win32"
              ? "C:/Program Files/Git/bin/bash.exe"
              : "bash");
          assert.ok(
            process.platform !== "win32" || existsSync(bashPath),
            "Bash is needed to execute copied cURL",
          );
          assert.equal(
            JSON.parse(await run(bashPath, ["-c", bash], env)).choices[0]
              .message.content,
            "Hello!",
          );
          report.snippets.push(
            "copied Bash/cURL executed against local fixture",
          );
          if (process.platform === "win32") {
            const script = path.join(out, "copied-request.ps1");
            await writeFile(script, powershell);
            assert.equal(
              JSON.parse(
                await run(
                  "powershell.exe",
                  ["-NoProfile", "-Command", powershell],
                  env,
                ),
              ).choices[0].message.content,
              "Hello!",
            );
            report.snippets.push(
              "copied PowerShell executed against local fixture",
            );
          }
          await page.getByRole("tab", { name: "Python", exact: true }).click();
          const python = await page
            .locator(".docs-code")
            .filter({ has: page.getByText("first_call.py", { exact: true }) })
            .locator("pre")
            .innerText();
          await writeFile(path.join(out, "first_call.py"), python);
          const pythonPath = process.env.DOCS_PYTHON || "python";
          assert.equal(
            (
              await run(pythonPath, [path.join(out, "first_call.py")], env)
            ).trim(),
            "Hello!",
          );
          report.snippets.push(
            "official Python SDK example executed against local fixture",
          );
          for (const request of requests) {
            assert.equal(request.path, "/v1/chat/completions");
            assert.equal(request.authorization, "Bearer synthetic-docs-key");
            assert.deepEqual(JSON.parse(request.body), {
              ...JSON.parse(
                await readFile(
                  path.join(web, "src/pages/docs/quickstart-request.json"),
                  "utf8",
                ),
              ),
              model: "docs-fixture-model",
            });
          }
        }
        report.cases.push({ name, articles: ids, result: "passed" });
        await context.close();
      }
  assert.deepEqual(report.errors, []);
  console.log(JSON.stringify(report));
} finally {
  await writeFile(
    path.join(out, "report.json"),
    JSON.stringify(report, null, 2),
  );
  await browser?.close();
  await vite.close();
  fixture.close();
}
