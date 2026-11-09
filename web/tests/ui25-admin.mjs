// Isolated staff onboarding and ADM role acceptance against the real backend.
import { createServer as createViteServer } from "vite";
import { chromium } from "playwright";
import { execFileSync, spawn } from "node:child_process";
import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import net from "node:net";
import crypto from "node:crypto";
import assert from "node:assert/strict";

const web = fileURLToPath(new URL("../", import.meta.url)),
  project = path.resolve(web, "..");
const out = path.join(project, "output", "ui25", "admin-browser");
await mkdir(out, { recursive: true });
const temp = await mkdtemp(path.join(out, "run-")),
  binary = path.join(out, "staff-check.exe");
execFileSync("go", ["build", "-buildvcs=false", "-o", binary, "."], {
  cwd: project,
  stdio: "pipe",
});
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
  STARHUB_CONFIG: "",
  STARHUB_DB: path.join(temp, "staff.db"),
  STARHUB_PORT: new URL(backend).port,
  STARHUB_COOKIE_SECURE: "false",
  STARHUB_STAFF_SETUP_MODE: "commercial",
  STARHUB_STAFF_ALLOWED_IPS: "127.0.0.1/32,::1/128",
  STARHUB_TRUSTED_PROXIES: "",
  STARHUB_RESOURCE_SNAPSHOT_URL: "",
  STARHUB_RESOURCE_SNAPSHOT_TOKEN: "",
  STARHUB_STAFF_PASSWORD: "Synthetic-root-password-123",
  STARHUB_ROOT_PASSWORD: "",
};
execFileSync(binary, ["staff", "init", "--username", "root"], {
  cwd: temp,
  env,
  stdio: "pipe",
});
const child = spawn(binary, [], {
  cwd: temp,
  env,
  windowsHide: true,
  stdio: ["ignore", "ignore", "pipe"],
});
let logs = "";
child.stderr.on("data", (x) => (logs += x));
const totp = (secret) => {
  let bits = "";
  for (const c of secret.trim())
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
};
let vite, browser, livePage;
const errors = [];
const results = [];
try {
  for (let i = 0; i < 120; i++) {
    try {
      if ((await fetch(backend + "/api/admin/install")).ok) break;
    } catch {}
    await new Promise((r) => setTimeout(r, 250));
    if (i === 119) throw Error("isolated staff backend did not start");
  }
  vite = await createViteServer({
    root: web,
    configFile: path.join(web, "vite.config.ts"),
    server: {
      port: await freePort(),
      host: "127.0.0.1",
      proxy: { "/api": backend, "/v1": backend },
    },
  });
  await vite.listen();
  const base = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ channel: "chrome", headless: true });
  const root = await browser.newContext();
  const api = async (method, url, data) => {
    const response = await root.request.fetch(backend + url, {
      method,
      data,
      headers: { Origin: backend, "X-StarHub-Portal": "staff" },
    });
    assert.equal(response.status(), 200, await response.text());
    return (await response.json()).data;
  };
  await api("POST", "/api/admin/auth/login", {
    username: "root",
    password: env.STARHUB_STAFF_PASSWORD,
  });
  await api("POST", "/api/admin/auth/password", {
    current_password: env.STARHUB_STAFF_PASSWORD,
    new_password: "Changed-synthetic-ui25-123",
  });
  const setup = await api("POST", "/api/admin/auth/enroll", {});
  await api("POST", "/api/admin/auth/verify", { code: totp(setup.secret) });
  const current = await api("GET", "/api/admin/region-profile");
  await api("PUT", "/api/admin/region-profile", {
    expected_revision: current.current.revision,
    profile: {
      ...current.current.profile,
      timezone: "Asia/Dubai",
      cookie_consent: true,
      cookie_consent_mode: "banner",
    },
  });
  for (let i = 1; i <= 23; i++)
    await api("POST", "/api/admin/legal", {
      kind: "terms",
      language: "en",
      version: "v" + i,
      content: "Synthetic historical legal " + i,
      effective_local: `2025-01-${String(i).padStart(2, "0")}T00:00`,
    });
  const state = await root.storageState();
  for (const language of ["zh", "en"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const name = `${language}-${theme}-${width}`;
        const context = await browser.newContext({
          storageState: state,
          viewport: { width, height: 1000 },
          reducedMotion: "reduce",
        });
        await context.addInitScript(
          ({ language, theme }) => {
            localStorage.setItem("xy-language", language);
            localStorage.setItem("xy-theme", theme);
          },
          { language, theme },
        );
        const page = await context.newPage();
        livePage = page;
        page.setDefaultTimeout(15000);
        page.on("pageerror", (e) => errors.push(e.message));
        await page.goto(base + "admin?tab=regions");
        const legal = page
          .locator(".adm-card.adm-page")
          .filter({ has: page.locator(".legal-history-filters") });
        await legal.locator("tbody tr").first().waitFor();
        await Promise.all([
          page.waitForResponse(
            (r) =>
              r.url().includes("/api/admin/legal?") &&
              r.url().includes("kind=terms"),
          ),
          legal
            .locator(".legal-history-filters select")
            .first()
            .selectOption("terms"),
        ]);
        assert.equal(await legal.locator("tbody tr").count(), 20);
        await legal.locator(".adm-pagination button").last().click();
        await legal.getByRole("cell", { name: "v1", exact: true }).waitFor();
        assert.equal(await legal.locator("tbody tr").count(), 3);
        for (const text of await legal
          .locator("tbody tr td:last-child")
          .allTextContents())
          assert.equal(text, language === "zh" ? "历史" : "Historical");
        await legal.locator(".adm-pagination button").first().click();
        await legal.locator("header button").click();
        await page.getByRole("dialog").waitFor();
        const dialog = page.getByRole("dialog");
        await dialog.locator("select").first().selectOption("refund");
        await dialog.locator("select").nth(1).selectOption(language);
        const version = "browser-" + name;
        await dialog.locator('input[maxlength="64"]').fill(version);
        await dialog
          .locator('input[type="datetime-local"]')
          .fill(`2026-01-01T00:${String(results.length).padStart(2, "0")}`);
        await dialog.locator("input[type=file]").setInputFiles({
          name: "legal.txt",
          mimeType: "text/plain",
          buffer: Buffer.from("Synthetic uploaded policy " + name),
        });
        await page.screenshot({
          path: path.join(out, name + "-upload.png"),
          fullPage: true,
        });
        await dialog.locator("button[type=submit]").click();
        await dialog.waitFor({ state: "hidden" });
        await legal
          .locator(".legal-history-filters select")
          .first()
          .selectOption("refund");
        await legal
          .locator(".legal-history-filters select")
          .nth(1)
          .selectOption(language);
        await legal.getByRole("cell", { name: version, exact: true }).waitFor();
        assert.ok(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
        );
        await page.screenshot({
          path: path.join(out, name + "-history.png"),
          fullPage: true,
        });
        await context.close();
        results.push({ name, passed: true });
      }
  // Public verification uses a fresh anonymous browser and actual persisted content.
  const anonymous = await browser.newContext();
  const page = await anonymous.newPage();
  await page.goto(base + "legal/refund");
  await page.locator(".published-legal").waitFor();
  assert.match(
    await page.locator(".legal-document-body").innerText(),
    /Synthetic uploaded policy/,
  );
  await anonymous.close();
  await root.close();
  assert.deepEqual(errors, []);
} catch (error) {
  if (livePage && !livePage.isClosed())
    await livePage
      .screenshot({ path: path.join(out, "failure.png"), fullPage: true })
      .catch(() => {});
  throw error;
} finally {
  await writeFile(
    path.join(out, "report.json"),
    JSON.stringify({ results, errors }, null, 2),
  );
  await browser?.close();
  await vite?.close();
  child.kill();
}
console.log(JSON.stringify({ results, errors }));
