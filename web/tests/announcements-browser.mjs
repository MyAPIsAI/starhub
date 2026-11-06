import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import crypto from "node:crypto";
import net from "node:net";
import { createServer } from "vite";
import { chromium } from "playwright";

const web = fileURLToPath(new URL("../", import.meta.url)),
  project = path.resolve(web, "..");
const out = path.join(project, "output", "announcements-browser");
await mkdir(out, { recursive: true });
const temp = await mkdtemp(path.join(out, "run-")),
  binary = path.join(temp, "announcement-check.exe");
const freePort = async () => {
  const s = net.createServer();
  await new Promise((r) => s.listen(0, "127.0.0.1", r));
  const port = s.address().port;
  await new Promise((r) => s.close(r));
  return port;
};
const backend = `http://127.0.0.1:${await freePort()}`;
const env = {
  ...process.env,
  GOCACHE: path.join(project, "output", "go-build"),
  GOFLAGS: "-buildvcs=false",
  STARHUB_CONFIG: path.join(temp, "fixture.yaml"),
  STARHUB_DB: path.join(temp, "notices.db"),
  STARHUB_PORT: new URL(backend).port,
  STARHUB_COOKIE_SECURE: "false",
  STARHUB_STAFF_SETUP_MODE: "commercial",
  STARHUB_STAFF_ALLOWED_IPS: "127.0.0.1/32,::1/128",
  STARHUB_TRUSTED_PROXIES: "",
  STARHUB_STAFF_PASSWORD: "Synthetic-notice-root-123",
  STARHUB_ROOT_PASSWORD: "",
};
// The matrix shares one loopback IP; raise only the isolated fixture's request budget.
await writeFile(
  env.STARHUB_CONFIG,
  "version: 1\ninitial_options:\n  ip_rate_limit: 10000\n  user_rate_limit: 10000\n",
);
for (const name of [
  "SNAPSHOT_URL",
  "SNAPSHOT_TOKEN",
  "EVENTS_URL",
  "HEALTH_SNAPSHOT_URL",
  "INCIDENTS_URL",
  "CA_FILE",
  "CLIENT_CERT_FILE",
  "CLIENT_KEY_FILE",
])
  env[`STARHUB_RESOURCE_${name}`] = "";
execFileSync("go", ["build", "-buildvcs=false", "-o", binary, "."], {
  cwd: project,
  env,
  stdio: "pipe",
  windowsHide: true,
});
execFileSync(binary, ["staff", "init", "--username", "root"], {
  cwd: temp,
  env,
  stdio: "pipe",
  windowsHide: true,
});
const seed = path.join(temp, "seed.go");
await writeFile(
  seed,
  `package main
import (
  "fmt"
  "os"
  "starhub/model"
)
func main() {
  if err := model.InitDB(os.Getenv("STARHUB_DB")); err != nil { panic(err) }
  defer model.CloseDB()
  rows := []model.StandardModel{
    {Model: "fixture-model", Name: "Fixture model", Vendor: "Fixture vendor", Kind: "chat", Protocols: []string{"OpenAI"}},
    {Model: "secondary-model", Name: "Secondary model", Vendor: "Second vendor", Kind: "chat", Protocols: []string{"OpenAI"}},
  }
  for i := 0; i < 42; i++ {
    rows = append(rows, model.StandardModel{Model: fmt.Sprintf("fixture-bulk-%02d", i), Name: fmt.Sprintf("Bulk fixture %02d", i), Vendor: "Fixture vendor", Kind: "chat", Protocols: []string{"OpenAI"}})
  }
  if err := model.DB.Create(&rows).Error; err != nil { panic(err) }
}
`,
);
execFileSync("go", ["run", seed], {
  cwd: project,
  env,
  stdio: "pipe",
  windowsHide: true,
});
const child = spawn(binary, [], {
  cwd: temp,
  env,
  windowsHide: true,
  stdio: ["ignore", "ignore", "pipe"],
});
let serverErrors = "";
child.stderr.on("data", (v) => (serverErrors += v));
function totp(secret) {
  let bits = "";
  for (const c of secret)
    bits += "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
      .indexOf(c)
      .toString(2)
      .padStart(5, "0");
  const bytes = [];
  for (let i = 0; i + 8 <= bits.length; i += 8)
    bytes.push(parseInt(bits.slice(i, i + 8), 2));
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30000)));
  const sum = crypto
      .createHmac("sha1", Buffer.from(bytes))
      .update(counter)
      .digest(),
    offset = sum.at(-1) & 15;
  return String((sum.readUInt32BE(offset) & 0x7fffffff) % 1000000).padStart(
    6,
    "0",
  );
}
let vite, browser, lastPage;
const results = [],
  errors = [];
try {
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(backend + "/api/admin/install")).ok) break;
    } catch {}
    if (i === 99) throw Error("isolated backend failed to start");
    await new Promise((r) => setTimeout(r, 200));
  }
  vite = await createServer({
    root: web,
    server: {
      host: "127.0.0.1",
      port: await freePort(),
      proxy: { "/api": backend, "/v1": backend },
    },
  });
  await vite.listen();
  const base = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ channel: "chrome", headless: true });
  const post = async (context, endpoint, data) => {
    const response = await context.request.post(backend + endpoint, {
      data,
      headers: { Origin: backend, "X-StarHub-Portal": "staff" },
    });
    assert.equal(response.status(), 200, await response.text());
    const value = await response.json();
    assert.equal(value.success, true);
    return value.data;
  };
  const enroll = async (context, username, password) => {
    await post(context, "/api/admin/auth/login", { username, password });
    await post(context, "/api/admin/auth/password", {
      current_password: password,
      new_password: "Changed-synthetic-notice-123",
    });
    const enrolled = await post(context, "/api/admin/auth/enroll", {});
    await post(context, "/api/admin/auth/verify", {
      code: totp(enrolled.secret),
    });
  };
  const root = await browser.newContext();
  await enroll(root, "root", env.STARHUB_STAFF_PASSWORD);
  await post(root, "/api/admin/staff", {
    username: "notice-operator",
    temporary_password: "Synthetic-notice-ops-123",
    role: 10,
    reason: "Isolated browser acceptance",
  });
  const ops = await browser.newContext();
  await enroll(ops, "notice-operator", "Synthetic-notice-ops-123");
  const storageState = await ops.storageState();
  let sequence = 0;
  for (const lang of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const context = await browser.newContext({
          storageState,
          viewport: { width, height: 1100 },
          reducedMotion: "reduce",
        });
        const visitor = await browser.newContext({
          viewport: { width, height: 1100 },
          reducedMotion: "reduce",
        });
        for (const c of [context, visitor])
          await c.addInitScript(
            ({ lang, theme }) => {
              localStorage.setItem("xy-language", lang);
              localStorage.setItem("xy-theme", theme);
            },
            { lang, theme },
          );
        const page = await context.newPage(),
          publicPage = await visitor.newPage();
        lastPage = page;
        for (const p of [page, publicPage]) {
          p.on("pageerror", (e) => errors.push(e.message));
          p.on("response", (response) => {
            if (response.status() === 429)
              errors.push(
                "Unexpected rate limit: " + new URL(response.url()).pathname,
              );
          });
        }
        const label = {
          zh: {
            publish: "发布公告",
            edit: "编辑",
            save: "保存更新",
            end: "结束公告",
            confirm: "确认结束",
            retry: "重试",
          },
          en: {
            publish: "Publish notice",
            edit: "Edit",
            save: "Save changes",
            end: "Resolve notice",
            confirm: "Confirm resolution",
            retry: "Retry",
          },
          ar: {
            publish: "نشر إعلان",
            edit: "تعديل",
            save: "حفظ التغييرات",
            end: "إنهاء الإعلان",
            confirm: "تأكيد الإنهاء",
            retry: "إعادة المحاولة",
          },
        }[lang];
        await page.goto(base + "admin?tab=incidents");
        await page.locator(".announcement-admin").waitFor();
        await page
          .getByRole("button", { name: label.publish, exact: true })
          .click();
        const dialog = page.getByRole("dialog");
        await dialog.waitFor();
        const number = ++sequence;
        const titles = {
          zh: `服务延迟通知 ${number}`,
          en: `Service latency notice ${number}`,
          ar: `إشعار تأخر الخدمة ${number}`,
        };
        const descriptions = {
          zh: "部分模型响应较慢，我们正在调查。\n请稍后重试。",
          en: "Some models are responding slowly. We are investigating.\nPlease retry later.",
          ar: "بعض النماذج تستجيب ببطء. نحن نحقق في السبب.\nيرجى إعادة المحاولة لاحقًا.",
        };
        for (const locale of ["zh", "en", "ar"]) {
          await dialog.locator(`#notice-title-${locale}`).fill(titles[locale]);
          await dialog
            .locator(`#notice-content-${locale}`)
            .fill(descriptions[locale]);
        }
        const vendors = dialog.locator('[data-target-picker="vendors"]');
        const models = dialog.locator('[data-target-picker="models"]');
        const vendorBox = vendors.locator(
          'input[type="checkbox"][value="Fixture vendor"]',
        );
        const modelBox = models.locator(
          'input[type="checkbox"][value="fixture-model"]',
        );
        await vendorBox.waitFor();
        assert.equal(
          await vendors
            .locator('input[type="checkbox"][value="Fixture vendor"]')
            .count(),
          1,
        );
        if (number === 1) {
          await dialog
            .getByRole("button", { name: label.publish, exact: true })
            .click();
          await dialog
            .getByRole("alert")
            .filter({ hasText: "请勾选至少一个" })
            .waitFor();
          await models.getByRole("searchbox").fill("fixture-bulk-");
          assert.equal(await models.getByRole("checkbox").count(), 42);
          assert.equal(
            await models.getByRole("button").first().isDisabled(),
            true,
          );
          await models.getByRole("searchbox").fill("fixture-bulk-0");
          await models.getByRole("button").first().click();
          assert.equal(await models.locator("input:checked").count(), 10);
          await models.getByRole("button").last().click();
          assert.equal(await models.locator("input:checked").count(), 0);
          await models.getByRole("searchbox").fill("nonexistent-model");
          await models.getByText("没有匹配项", { exact: true }).waitFor();
          await page.route("**/api/admin/models", (route) =>
            route.fulfill({
              status: 503,
              contentType: "application/json",
              body: JSON.stringify({
                success: false,
                message: "Synthetic catalog failure",
              }),
            }),
          );
          await dialog
            .getByRole("button", { name: "刷新目录", exact: true })
            .click();
          await dialog
            .getByRole("alert")
            .filter({ hasText: "无法读取模型目录" })
            .waitFor();
          assert.equal(await models.getByRole("checkbox").count(), 0);
          await page.unroute("**/api/admin/models");
          await dialog
            .getByRole("button", { name: "刷新目录", exact: true })
            .click();
          await vendorBox.waitFor();
        }
        await vendors.getByRole("searchbox").fill("Fixture vendor");
        await vendorBox.check();
        assert.equal(await models.locator("input:checked").count(), 0);
        await models.getByRole("searchbox").fill("Fixture model");
        assert.equal(await models.getByRole("checkbox").count(), 1);
        await modelBox.focus();
        await page.keyboard.press("Space");
        assert.equal(await modelBox.isChecked(), true);
        await models.getByRole("searchbox").fill("secondary-model");
        await models
          .locator('input[type="checkbox"][value="secondary-model"]')
          .check();
        await models.getByRole("searchbox").fill("");
        assert.equal(await models.locator("input:checked").count(), 2);
        await models.getByRole("searchbox").fill("secondary-model");
        await models
          .locator('input[type="checkbox"][value="secondary-model"]')
          .uncheck();
        await models.getByRole("searchbox").fill("fixture-model");
        assert.equal(await modelBox.isChecked(), true);
        await dialog
          .locator(".announcement-target-section")
          .scrollIntoViewIfNeeded();
        assert.ok(
          await dialog.evaluate(
            (element) => element.scrollWidth <= element.clientWidth + 1,
          ),
        );
        await page.screenshot({
          path: path.join(out, `targets-${lang}-${theme}-${width}.png`),
        });
        await dialog
          .getByRole("button", { name: label.publish, exact: true })
          .click();
        await dialog.waitFor({ state: "hidden" });
        const card = page.locator("[data-announcement-id]").filter({
          has: page.getByRole("heading", { name: titles[lang], exact: true }),
        });
        await card.waitFor();
        const id = await card.getAttribute("data-announcement-id");
        await page.reload();
        await page.locator(`[data-announcement-id="${id}"]`).waitFor();
        await publicPage.goto(base + "status");
        const publicCard = publicPage.locator(
          `[data-public-announcement="${id}"]`,
        );
        await publicCard.waitFor();
        assert.ok((await publicCard.innerText()).includes(titles[lang]));
        assert.ok((await publicCard.innerText()).includes(descriptions[lang]));
        assert.ok((await publicCard.innerText()).includes("fixture-model"));
        assert.equal(
          await publicPage
            .locator("main > section")
            .first()
            .getAttribute("class"),
          "public-announcements",
        );
        assert.equal(
          await publicPage.locator("html").getAttribute("dir"),
          lang === "ar" ? "rtl" : "ltr",
        );
        for (const p of [page, publicPage])
          assert.ok(
            await p.evaluate(
              () => document.documentElement.scrollWidth <= innerWidth + 1,
            ),
          );
        await page.screenshot({
          path: path.join(out, `admin-${lang}-${theme}-${width}.png`),
        });
        await publicPage.screenshot({
          path: path.join(out, `public-${lang}-${theme}-${width}.png`),
        });
        if (number === 1) {
          await page.route("**/api/admin/models", (route) =>
            route.fulfill({
              status: 200,
              contentType: "application/json",
              body: JSON.stringify({ success: true, data: { models: [] } }),
            }),
          );
        }
        await page
          .locator(`[data-announcement-id="${id}"]`)
          .getByRole("button", { name: label.edit, exact: true })
          .click();
        await dialog.waitFor();
        if (number === 1) {
          await dialog
            .getByText("已不在当前目录，保留原有选择", { exact: true })
            .nth(1)
            .waitFor();
          await page.unroute("**/api/admin/models");
        }
        assert.equal(
          await vendors
            .locator('input[type="checkbox"][value="Fixture vendor"]')
            .isChecked(),
          true,
        );
        assert.equal(
          await models
            .locator('input[type="checkbox"][value="fixture-model"]')
            .isChecked(),
          true,
        );
        await dialog
          .locator(`#notice-content-${lang}`)
          .fill(descriptions[lang] + " Updated.");
        await dialog.locator("#notice-status").selectOption("monitoring");
        await dialog
          .getByRole("button", { name: label.save, exact: true })
          .click();
        await dialog.waitFor({ state: "hidden" });
        await publicPage.reload();
        await publicCard.waitFor();
        await publicPage.waitForFunction(
          (id) =>
            document
              .querySelector(`[data-public-announcement="${id}"]`)
              ?.textContent.includes("Updated."),
          id,
        );
        assert.ok((await publicCard.innerText()).includes("Fixture vendor"));
        assert.ok((await publicCard.innerText()).includes("fixture-model"));
        if (number === 1) {
          const payload = {
            expected_version: 1,
            operation_id: "stale",
            title: titles,
            content: descriptions,
            vendors: ["Fixture vendor"],
            models: ["fixture-model"],
            status: "investigating",
          };
          const stale = await context.request.put(
            backend + `/api/admin/announcements/${id}`,
            {
              data: payload,
              headers: {
                Origin: backend,
                "X-StarHub-Portal": "staff",
              },
            },
          );
          assert.equal(stale.status(), 409);
          await publicPage.route("**/api/status/announcements", (route) =>
            route.fulfill({
              status: 503,
              contentType: "application/json",
              body: JSON.stringify({
                success: false,
                message: "Synthetic failure",
              }),
            }),
          );
          await publicPage.reload();
          await publicPage
            .getByRole("alert")
            .filter({ hasText: "暂时无法读取事件公告" })
            .waitFor();
          assert.equal(
            await publicPage.locator("[data-public-announcement]").count(),
            0,
          );
          await publicPage.screenshot({
            path: path.join(out, "public-error.png"),
          });
          await publicPage.unroute("**/api/status/announcements");
          await publicPage
            .getByRole("button", { name: label.retry, exact: true })
            .click();
          await publicCard.waitFor();
        }
        await page
          .locator(`[data-announcement-id="${id}"]`)
          .getByRole("button", { name: label.end, exact: true })
          .click();
        await dialog.waitFor();
        await dialog
          .getByRole("button", { name: label.confirm, exact: true })
          .click();
        await dialog.waitFor({ state: "hidden" });
        await page
          .locator(`[data-announcement-id="${id}"] [data-status="resolved"]`)
          .waitFor();
        await Promise.all([
          publicPage.waitForResponse(
            (r) =>
              r.url().endsWith("/api/status/announcements") &&
              r.status() === 200,
          ),
          publicPage.reload(),
        ]);
        await publicCard.waitFor({ state: "hidden" });
        assert.equal(
          await publicPage
            .locator(`[data-public-announcement="${id}"]`)
            .count(),
          0,
        );
        results.push({ lang, theme, width, passed: true });
        await context.close();
        await visitor.close();
      }
  assert.deepEqual(errors, []);
  await writeFile(
    path.join(out, "report.json"),
    JSON.stringify({ results, errors }, null, 2),
  );
  console.log(JSON.stringify({ passed: results.length, errors }));
} catch (error) {
  if (lastPage && !lastPage.isClosed())
    await lastPage
      .screenshot({ path: path.join(out, "failure.png") })
      .catch(() => {});
  await writeFile(
    path.join(out, "failure.txt"),
    String(error) + "\n" + serverErrors,
  );
  throw error;
} finally {
  await browser?.close();
  await vite?.close();
  child.kill();
}
