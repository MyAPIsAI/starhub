// Uses isolated application processes, a temporary database and synthetic credentials.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import { execFileSync, spawn } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import net from "node:net";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url));
const project = path.resolve(web, "..");
const out = path.join(project, "output", "key-collapsed-check");
await mkdir(out, { recursive: true });
const temp = await mkdtemp(path.join(out, "run-"));
const binary = path.join(
  out,
  process.platform === "win32" ? "key-check.exe" : "key-check",
);
execFileSync("go", ["build", "-buildvcs=false", "-o", binary, "."], {
  cwd: project,
  stdio: "pipe",
});
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
const backendPort = await freePort();
const backend = `http://127.0.0.1:${backendPort}`;
const password = "KeyAcceptanceOnly2026";
const child = spawn(binary, [], {
  cwd: temp,
  env: {
    ...process.env,
    STARHUB_CONFIG: "",
    STARHUB_PORT: String(backendPort),
    STARHUB_DB: path.join(temp, "starhub.db"),
    STARHUB_COOKIE_SECURE: "false",
    STARHUB_ROOT_PASSWORD: password,
  },
  stdio: ["ignore", "ignore", "pipe"],
  windowsHide: true,
});
let backendLogs = "";
child.stderr.on("data", (data) => {
  backendLogs += data;
});
let vite, browser;
const report = { checks: [], screenshots: [], errors: [], networkErrors: [] };
const words = {
  zh: {
    create: "创建密钥",
    allowed: "明确授权的模型",
    candidates: "智能候选模型",
    empty: "开启智能入口时须选择至少一个已授权的文本模型。",
    close: "关闭",
  },
  en: {
    create: "Create key",
    allowed: "Explicitly allowed models",
    candidates: "Smart candidate models",
    empty: "Select at least one authorized text model for smart routing.",
    close: "Close",
  },
  ar: {
    create: "إنشاء مفتاح",
    allowed: "النماذج المسموحة صراحةً",
    candidates: "النماذج المرشحة للتوجيه الذكي",
    empty: "اختر نموذجًا نصيًا مصرحًا به على الأقل للتوجيه الذكي.",
    close: "إغلاق",
  },
};
try {
  let ready = false;
  for (let i = 0; i < 80; i++) {
    try {
      ready = (await fetch(`${backend}/api/plaza/models`)).ok;
    } catch {}
    if (ready) break;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  assert.ok(ready, `isolated backend did not start: ${backendLogs}`);
  vite = await createViteServer({
    root: web,
    server: {
      port: await freePort(),
      host: "127.0.0.1",
      proxy: { "/api": backend, "/v1": backend },
    },
    logLevel: "error",
  });
  await vite.listen();
  const base = vite.resolvedUrls.local[0].replace(/\/$/, "");
  browser = await chromium.launch({ channel: "chrome", headless: true });
  const seed = await browser.newContext();
  const login = await seed.request.post(`${base}/api/user/login`, {
    data: { username: "root", password },
  });
  assert.equal(login.status(), 200);
  const ownerID = (await login.json()).data.user.id;
  const rateLimit = await seed.request.put(`${base}/api/option`, {
    data: { key: "ip_rate_limit", value: "10000" },
  });
  assert.equal(rateLimit.status(), 200);
  const channel = await seed.request.post(`${base}/api/channel/`, {
    data: {
      name: "Key acceptance",
      type: 1,
      key: "synthetic-browser-key",
      base_url: "https://example.invalid",
      models: "gpt-4.1,gpt-4.1-mini,claude-sonnet-4",
      group: "default",
      status: 1,
    },
  });
  assert.equal(channel.status(), 200);
  const groups = await seed.request.put(`${base}/api/option`, {
    data: { key: "group_ratio", value: '{"default":1,"vip":1}' },
  });
  assert.equal(groups.status(), 200);
  const otherChannel = await seed.request.post(`${base}/api/channel/`, {
    data: {
      name: "Other group choices",
      type: 1,
      key: "synthetic-other-group-key",
      base_url: "https://example.invalid",
      models: "text-embedding-3-small,gpt-image-1,whisper-1",
      group: "vip",
      status: 1,
    },
  });
  assert.equal(otherChannel.status(), 200);
  for (const [model, vendor, tags] of [
    ["gpt-4.1", "openai", "vision,function_call"],
    ["gpt-4.1-mini", "openai", "function_call"],
    ["claude-sonnet-4", "anthropic", "vision,function_call"],
    ["text-embedding-3-small", "openai", "embedding"],
    ["gpt-image-1", "openai", "image"],
    ["whisper-1", "openai", "stt"],
  ]) {
    const metadata = await seed.request.put(`${base}/api/plaza/meta`, {
      data: {
        model,
        vendor,
        vendor_display: vendor === "openai" ? "OpenAI" : "Anthropic",
        tags,
      },
    });
    assert.equal(metadata.status(), 200);
  }
  const storageState = await seed.storageState();
  await seed.close();
  for (const language of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const text = words[language];
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
        page.setDefaultTimeout(12000);
        page.on("pageerror", (error) => report.errors.push(String(error)));
        page.on("response", (response) => {
          if (response.url().includes("/api/") && response.status() >= 400)
            report.networkErrors.push({
              path: new URL(response.url()).pathname,
              status: response.status(),
            });
        });
        await page.goto(`${base}/console/keys`);
        await page
          .locator(".key-manager-header")
          .getByRole("button", { name: text.create, exact: true })
          .click();
        const dialog = page.getByRole("dialog");
        await page.locator("#new-key-name").fill("Key acceptance");
        const policy = page.locator(".native-key-model-policy");
        assert.equal(await page.locator(".native-key-step").count(), 0);
        assert.equal(
          await page
            .locator(".native-key-policy-disclosure")
            .getAttribute("aria-expanded"),
          "false",
        );
        assert.equal(
          await page
            .locator("#native-key-access-toggle")
            .getAttribute("aria-expanded"),
          "false",
        );
        assert.equal(
          await page.locator("#native-key-policy-body").isVisible(),
          false,
        );
        assert.equal(
          await page.locator("#native-key-access-fields").isVisible(),
          false,
        );
        const collapsedShot = `key-collapsed-${language}-${theme}-${width}.png`;
        await page.screenshot({
          path: path.join(out, collapsedShot),
          fullPage: false,
        });
        report.screenshots.push(collapsedShot);
        await page.locator(".native-key-policy-disclosure").click();
        assert.equal(
          await page
            .locator("#native-key-scope-snapshot")
            .getAttribute("aria-pressed"),
          "true",
        );
        assert.ok((await policy.textContent()).includes("myapis-auto"));
        assert.equal(
          await policy.locator('input[type="checkbox"]:visible').count(),
          0,
        );
        const snapshotShot = `model-snapshot-${language}-${theme}-${width}.png`;
        await policy.screenshot({ path: path.join(out, snapshotShot) });
        report.screenshots.push(snapshotShot);
        await page.locator("#native-key-scope-custom").click();
        const allowed = policy.locator(".native-key-model-list");
        await dialog
          .getByRole("button", { name: text.create, exact: true })
          .click();
        await policy.locator(".native-key-policy-error").waitFor();
        assert.ok(
          await allowed
            .getByLabel("text-embedding-3-small", { exact: true })
            .isDisabled(),
        );
        assert.ok(
          await allowed.getByLabel("gpt-image-1", { exact: true }).isDisabled(),
        );
        await page.locator("#native-key-model-search").fill("gpt-4.1-mini");
        assert.equal(
          await allowed.locator('input[type="checkbox"]').count(),
          1,
        );
        await allowed.getByLabel("gpt-4.1-mini", { exact: true }).check();
        await page.locator("#native-key-model-search").fill("");
        assert.ok(
          await allowed.getByLabel("gpt-4.1-mini", { exact: true }).isChecked(),
        );
        await allowed.getByLabel("gpt-4.1-mini", { exact: true }).uncheck();
        await allowed.getByLabel("gpt-4.1", { exact: true }).check();
        await allowed.getByLabel("gpt-4.1-mini", { exact: true }).check();
        await page.locator("#native-key-candidates-toggle").click();
        const candidates = dialog.getByRole("group", {
          name: text.candidates,
          exact: true,
        });
        assert.equal(
          await candidates
            .getByLabel("claude-sonnet-4", { exact: true })
            .count(),
          0,
        );
        assert.equal(
          await candidates.getByLabel("gpt-image-1", { exact: true }).count(),
          0,
        );
        await candidates.getByLabel("gpt-4.1", { exact: true }).uncheck();
        await candidates.getByLabel("gpt-4.1-mini", { exact: true }).uncheck();
        await page.locator(".native-key-policy-disclosure").click();
        await dialog
          .getByRole("button", { name: text.create, exact: true })
          .click();
        await dialog
          .getByRole("alert")
          .filter({ hasText: text.empty })
          .waitFor();
        assert.equal(
          await page
            .locator(".native-key-policy-disclosure")
            .getAttribute("aria-expanded"),
          "true",
        );
        await candidates.getByLabel("gpt-4.1-mini", { exact: true }).check();
        await page.locator("#native-key-candidates-toggle").click();
        const customShot = `model-custom-${language}-${theme}-${width}.png`;
        await policy.screenshot({ path: path.join(out, customShot) });
        report.screenshots.push(customShot);
        let writes = 0;
        page.on("request", (request) => {
          if (
            request.method() === "POST" &&
            request.url().endsWith("/api/token/owned-action")
          )
            writes++;
        });
        await page.locator("#native-key-access-toggle").click();
        assert.ok(await page.locator("#native-key-expiry-never").isChecked());
        assert.equal(await page.locator("#native-key-expiry").count(), 0);
        assert.equal(await page.locator("#native-key-ips").inputValue(), "");
        assert.equal(
          await page.locator("#native-key-ip-count").textContent(),
          "0 / 50",
        );
        await page.locator("#native-key-access-toggle").click();
        assert.equal(
          await page.locator("#native-key-access-fields").isVisible(),
          false,
        );
        await page.locator("#native-key-access-toggle").press("Enter");
        assert.ok(await page.locator("#native-key-access-fields").isVisible());
        await page.locator("#native-key-expiry-date").check();
        await dialog
          .getByRole("button", { name: text.create, exact: true })
          .click();
        await page.locator("#native-key-expiry-error").waitFor();
        assert.equal(writes, 0, "empty scheduled expiry was submitted");
        await page.locator("#new-key-quota").fill("25");
        await page.locator("#native-key-period").selectOption("daily");
        await page.locator("#native-key-expiry").fill("2027-01-01T12:00");
        await page
          .locator("#native-key-ips")
          .fill(
            Array.from(
              { length: 51 },
              (_, index) => `203.0.113.${index + 1}`,
            ).join("\n"),
          );
        assert.equal(
          await page.locator("#native-key-ip-count").textContent(),
          "51 / 50",
        );
        await page.locator("#native-key-access-toggle").click();
        await dialog
          .getByRole("button", { name: text.create, exact: true })
          .click();
        await page.locator("#native-key-ip-error").waitFor();
        assert.ok(await page.locator("#native-key-access-fields").isVisible());
        assert.equal(writes, 0, "oversized allowlist was submitted");
        await page
          .locator("#native-key-ips")
          .fill("10.0.0.0/24\n\n2001:db8::/32\n");
        assert.equal(
          await page.locator("#native-key-ip-count").textContent(),
          "2 / 50",
        );
        await page.locator("#native-key-expiry-never").check();
        assert.equal(await page.locator("#native-key-expiry").count(), 0);
        const box = await dialog.boundingBox();
        assert.ok(
          box && box.x >= 0 && box.x + box.width <= width + 1,
          "dialog exceeds viewport",
        );
        assert.ok(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
          "horizontal page overflow",
        );
        await dialog.locator("[data-overlay-body]").evaluate((element) => {
          element.scrollTop = 200;
        });
        await page.waitForTimeout(180);
        const screenshot = `keys-${language}-${theme}-${width}.png`;
        await page.screenshot({
          path: path.join(out, screenshot),
          fullPage: false,
        });
        report.screenshots.push(screenshot);
        await dialog.locator("[data-overlay-body]").evaluate((element) => {
          element.scrollTop = element.scrollHeight;
        });
        const limitsScreenshot = `keys-limits-${language}-${theme}-${width}.png`;
        await page.screenshot({
          path: path.join(out, limitsScreenshot),
          fullPage: false,
        });
        report.screenshots.push(limitsScreenshot);
        const accessScreenshot = `access-control-${language}-${theme}-${width}.png`;
        const ipDraft = await page.locator("#native-key-ips").inputValue();
        await page.locator("#native-key-ips").fill("");
        assert.equal(
          await page.locator("#native-key-ip-count").textContent(),
          "0 / 50",
        );
        await page
          .locator(".native-key-access")
          .screenshot({ path: path.join(out, accessScreenshot) });
        await page.locator("#native-key-ips").fill(ipDraft);
        report.screenshots.push(accessScreenshot);
        assert.equal(
          await page
            .locator("#native-key-ips")
            .evaluate((element) => getComputedStyle(element).direction),
          "ltr",
        );
        await page.locator("#native-key-expiry-date").check();
        assert.equal(
          await page.locator("#native-key-expiry").inputValue(),
          "2027-01-01T12:00",
        );
        await page.locator("#native-key-smart-access").focus();
        await page.keyboard.press("Tab");
        assert.ok(
          await dialog.evaluate((element) =>
            element.contains(document.activeElement),
          ),
          "keyboard focus escaped dialog",
        );
        if (language === "zh" && theme === "light" && width === 1440) {
          const responsePromise = page.waitForResponse(
            (response) =>
              response.request().method() === "POST" &&
              response.url().endsWith("/api/token/owned-action"),
          );
          await dialog
            .getByRole("button", { name: text.create, exact: true })
            .click();
          const response = await responsePromise;
          assert.equal(response.status(), 200);
          const created = (await response.json()).data.token;
          const body = response.request().postDataJSON();
          assert.equal(body.payload.model_limits, "gpt-4.1,gpt-4.1-mini");
          assert.equal(body.payload.smart_candidates, "gpt-4.1-mini");
          assert.equal(body.payload.smart_access, true);
          assert.equal(body.payload.quota_limit, 12_500_000);
          assert.equal(body.payload.quota_period, "daily");
          const list = await context.request.get(`${base}/api/token/owned`);
          const token = (await list.json()).data.find(
            (row) => row.name === "Key acceptance",
          );
          assert.equal(token.smart_candidates, "gpt-4.1-mini");
          assert.equal(token.smart_access, true);
          assert.equal(token.model_limits, "gpt-4.1,gpt-4.1-mini");
          assert.equal(token.quota_period, "daily");
          assert.equal(token.quota_limit, 12_500_000);
          assert.equal(token.remain_quota, 12_500_000);
          assert.equal(token.allow_ips, "10.0.0.0/24,2001:db8::/32");
          assert.ok(token.expired_time > Date.now() / 1000);
          report.checks.push(
            "real API preserves separate authorization and smart candidate lists",
          );
          await dialog
            .getByRole("button", { name: text.close, exact: true })
            .last()
            .click();
          await page.reload();
          const row = page.locator(`[data-key-id="${created.id}"]`);
          await row.waitFor();
          assert.notEqual(
            await row.locator(".key-list-credential code").textContent(),
            created.key,
          );
          await row.locator('[data-key-action="reveal"]').click();
          await page.locator("#key-auth-password").fill(password);
          await page
            .getByRole("dialog")
            .locator('button[type="submit"]')
            .click();
          await page.waitForFunction(
            ({ id, secret }) =>
              document.querySelector(
                `[data-key-id="${id}"] .key-list-credential code`,
              )?.textContent === secret,
            { id: created.id, secret: created.key },
          );
          await row.locator('[data-key-action="reveal"]').click();
          const repeat = page.waitForResponse((response) =>
            response.url().endsWith(`/api/token/${created.id}/reveal`),
          );
          await row.locator('[data-key-action="reveal"]').click();
          assert.equal((await repeat).status(), 200);
          await row.locator('[data-key-action="reveal"]').click();
          report.checks.push(
            "reload loses plaintext; password retrieves the encrypted key; repeated view uses audited server grant",
          );
          await row.locator('[data-key-action="edit"]').click();
          await page.locator("#native-key-access-toggle").click();
          assert.ok(await page.locator("#native-key-expiry-date").isChecked());
          assert.equal(
            await page.locator("#native-key-ip-count").textContent(),
            "2 / 50",
          );
          assert.equal(
            await page.locator("#native-key-period").inputValue(),
            "daily",
          );
          assert.equal(await page.locator("#new-key-quota").inputValue(), "25");
          await page.locator("#native-key-period").selectOption("monthly");
          const update = page.waitForResponse(
            (response) =>
              response.request().method() === "POST" &&
              response.url().endsWith("/api/token/owned-action"),
          );
          await page
            .getByRole("dialog")
            .getByRole("button", { name: "保存", exact: true })
            .click();
          assert.equal((await update).status(), 200);
          report.checks.push(
            "quota period and expiry persist through the real edit form",
          );
          await page.getByRole("dialog").waitFor({ state: "hidden" });
          await row.locator('[data-key-action="edit"]').click();
          await page.locator("#native-key-access-toggle").click();
          await page.locator("#native-key-expiry-never").check();
          await page.locator("#native-key-ips").fill("");
          assert.equal(
            await page.locator("#native-key-ip-count").textContent(),
            "0 / 50",
          );
          const unrestrictedUpdate = page.waitForResponse(
            (response) =>
              response.request().method() === "POST" &&
              response.url().endsWith("/api/token/owned-action"),
          );
          await page
            .getByRole("dialog")
            .getByRole("button", { name: "保存", exact: true })
            .click();
          assert.equal((await unrestrictedUpdate).status(), 200);
          const unrestricted = (
            await (await context.request.get(`${base}/api/token/owned`)).json()
          ).data.find((token) => token.id === created.id);
          assert.equal(unrestricted.expired_time, -1);
          assert.equal(unrestricted.allow_ips, "");
          report.checks.push(
            "permanent expiry saves -1 and an empty allowlist saves no sample addresses",
          );
        } else {
          await page.keyboard.press("Escape");
        }
        await dialog.waitFor({ state: "hidden" });
        report.checks.push(
          `${language}/${theme}/${width}: collapsed defaults without step number, candidate boundary, access disclosure, expiry selection, 50-entry limit, draft preservation, keyboard and viewport`,
        );
        console.log(`PASS ${language}/${theme}/${width}`);
        await context.close();
      }
  const finalContext = await browser.newContext({
    storageState,
    viewport: { width: 1440, height: 1000 },
  });
  await finalContext.addInitScript(() => {
    localStorage.setItem("xy-language", "zh");
    localStorage.setItem("xy-theme", "light");
  });
  const finalPage = await finalContext.newPage();
  finalPage.on("pageerror", (error) => report.errors.push(String(error)));
  await finalPage.goto(`${base}/console/keys`);
  await finalPage
    .locator(".key-manager-header")
    .getByRole("button", { name: "创建密钥", exact: true })
    .click();
  await finalPage.locator("#new-key-name").fill("Snapshot authorization");
  const initialChoices = (
    await (
      await finalContext.request.get(
        `${base}/api/token/owned-choices?expected_user_id=${ownerID}`,
      )
    ).json()
  ).data.choices.filter((choice) => choice.group === "default");
  const expectedSnapshot = initialChoices.map((choice) => choice.model).sort();
  const createSnapshot = finalPage.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      response.url().endsWith("/api/token/owned-action"),
  );
  await finalPage
    .getByRole("dialog")
    .getByRole("button", { name: "创建密钥", exact: true })
    .click();
  const snapshotResponse = await createSnapshot;
  assert.equal(snapshotResponse.status(), 200);
  assert.deepEqual(
    snapshotResponse
      .request()
      .postDataJSON()
      .payload.model_limits.split(",")
      .sort(),
    expectedSnapshot,
  );
  const snapshotID = (await snapshotResponse.json()).data.token.id;
  await finalPage
    .getByRole("dialog")
    .getByRole("button", { name: "关闭", exact: true })
    .last()
    .click();
  const addedModel = "gpt-after-snapshot";
  const newModelChannel = await finalContext.request.post(
    `${base}/api/channel/`,
    {
      data: {
        name: "Later model",
        type: 1,
        key: "synthetic-later-key",
        base_url: "https://example.invalid",
        models: addedModel,
        group: "default",
        status: 1,
      },
    },
  );
  assert.equal(newModelChannel.status(), 200);
  await finalPage.reload();
  const snapshotRow = finalPage.locator(`[data-key-id="${snapshotID}"]`);
  await snapshotRow.locator('[data-key-action="edit"]').click();
  await finalPage.locator(".native-key-policy-disclosure").click();
  await finalPage.locator("#edit-key-name").fill("Snapshot metadata edit");
  assert.equal(
    await finalPage
      .locator(".native-key-model-list")
      .getByLabel(addedModel, { exact: true })
      .isChecked(),
    false,
  );
  const metadataUpdate = finalPage.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      response.url().endsWith("/api/token/owned-action"),
  );
  await finalPage
    .getByRole("dialog")
    .getByRole("button", { name: "保存", exact: true })
    .click();
  const metadataResponse = await metadataUpdate;
  assert.equal(metadataResponse.status(), 200);
  assert.equal(
    Object.hasOwn(
      metadataResponse.request().postDataJSON().payload,
      "model_limits",
    ),
    false,
  );
  let storedSnapshot = (
    await (await finalContext.request.get(`${base}/api/token/owned`)).json()
  ).data.find((key) => key.id === snapshotID);
  assert.deepEqual(
    storedSnapshot.model_limits.split(",").sort(),
    expectedSnapshot,
  );
  await finalPage.getByRole("dialog").waitFor({ state: "hidden" });
  await snapshotRow.locator('[data-key-action="edit"]').click();
  await finalPage.locator(".native-key-policy-disclosure").click();
  await finalPage.locator("#native-key-scope-snapshot").click();
  const explicitUpdate = finalPage.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      response.url().endsWith("/api/token/owned-action"),
  );
  await finalPage
    .getByRole("dialog")
    .getByRole("button", { name: "保存", exact: true })
    .click();
  assert.equal((await explicitUpdate).status(), 200);
  storedSnapshot = (
    await (await finalContext.request.get(`${base}/api/token/owned`)).json()
  ).data.find((key) => key.id === snapshotID);
  assert.deepEqual(
    storedSnapshot.model_limits.split(",").sort(),
    [...expectedSnapshot, addedModel].sort(),
  );
  await finalPage.getByRole("dialog").waitFor({ state: "hidden" });
  await snapshotRow.locator('[data-key-action="edit"]').click();
  await finalPage.locator(".native-key-policy-disclosure").click();
  await finalPage.locator("#native-key-smart-access").click();
  const disableSmart = finalPage.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      response.url().endsWith("/api/token/owned-action"),
  );
  await finalPage
    .getByRole("dialog")
    .getByRole("button", { name: "保存", exact: true })
    .click();
  assert.equal((await disableSmart).status(), 200);
  storedSnapshot = (
    await (await finalContext.request.get(`${base}/api/token/owned`)).json()
  ).data.find((key) => key.id === snapshotID);
  assert.equal(storedSnapshot.smart_access, false);
  assert.deepEqual(
    storedSnapshot.model_limits.split(",").sort(),
    [...expectedSnapshot, addedModel].sort(),
  );
  report.checks.push(
    "all-at-save stores explicit models; later additions do not expand permissions on metadata edits; explicit snapshot update adds them",
  );
  report.checks.push(
    "disabling myapis-auto preserves direct model authorization",
  );
  await finalContext.close();
  assert.deepEqual(report.errors, []);
} catch (error) {
  report.errors.push(String(error));
  console.error(error);
  process.exitCode = 1;
} finally {
  await writeFile(
    path.join(out, "browser-report.json"),
    JSON.stringify(report, null, 2),
  );
  await browser?.close();
  await vite?.close();
  child.kill();
}
