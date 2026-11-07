import assert from "node:assert/strict";
import { mkdir, writeFile, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { chromium } from "playwright";

const web = fileURLToPath(new URL("../", import.meta.url));
const out = path.resolve(web, "../output/res-boundaries-browser");
await mkdir(out, { recursive: true });
const vite = await createServer({
  root: web,
  server: { host: "127.0.0.1", port: 0, strictPort: false },
});
const report = { cases: [], errors: [] };
let browser;
try {
  await vite.listen();
  browser = await chromium.launch({ channel: "chrome", headless: true });
  for (const lang of ["zh", "en", "ar"])
    for (const theme of ["light", "dark"])
      for (const width of [1440, 390]) {
        const labels = JSON.parse(
          await readFile(
            path.join(
              web,
              `src/i18n/messages.${lang === "ar" ? "en" : lang}.json`,
            ),
            "utf8",
          ),
        );
        const context = await browser.newContext({
          viewport: { width, height: 1000 },
          reducedMotion: "reduce",
        });
        await context.addInitScript(
          ({ lang, theme }) => {
            localStorage.setItem("xy-language", lang);
            localStorage.setItem("xy-theme", theme);
          },
          { lang, theme },
        );
        const before = {
          channel_id: "supplier",
          upstream_model: "alpha-v1",
          canonical_model: "alpha",
          provider: "Example vendor",
        };
        const after = { ...before, upstream_model: "alpha-v2" };
        let catalog = {
          revision: 3,
          updated_at: 1793145601,
          affects_gateway_routing: true,
          regions: {},
          snapshot: {
            id: "fixture",
            sequence: 3,
            publishedAt: "2026-11-07T00:00:00Z",
            receivedAt: "2026-11-07T00:00:01Z",
            source: "Acceptance fixture",
            channels: [
              {
                id: "supplier",
                name: "Example supplier",
                provider: "Example vendor",
                enabled: true,
              },
            ],
            instances: [
              {
                id: "resource-active",
                channelId: "supplier",
                upstreamModel: "alpha-v2",
                canonicalModel: "alpha",
                protocols: ["OpenAI"],
                capabilities: ["text"],
                contextLength: 128000,
                region: "",
                status: "available",
                latencyMs: 0,
              },
            ],
          },
          placements: {
            "resource-active": {
              modelId: "alpha",
              groupIds: ["default"],
              enabled: false,
            },
          },
          placement_reviews: { "resource-active": { before, after } },
          removed_placement_total: 1,
          removed_placements: [
            {
              id: 1,
              resource_id: "resource-removed",
              model_id: "alpha",
              group_ids: ["default"],
              identity: before,
              removed_at: 1793145601,
              sequence: 3,
            },
          ],
        };
        let writes = 0,
          rejectCleanup = true;
        await context.route(
          (url) => url.pathname.startsWith("/api/"),
          async (route) => {
            const request = route.request(),
              pathname = new URL(request.url()).pathname;
            let data = {};
            if (["/api/admin/auth/self", "/api/user/self"].includes(pathname))
              data = {
                id: 1,
                username: "root-fixture",
                role: width === 390 ? 20 : 100,
                account_kind: "staff",
                platform_admin: true,
                status: 1,
                group: "default",
                quota: 0,
                used_quota: 0,
              };
            else if (pathname === "/api/resource") data = catalog;
            else if (pathname === "/api/placement/resource-active") {
              const body = request.postDataJSON();
              writes++;
              assert.equal(body.expected_revision, catalog.revision);
              assert.equal(body.placement.confirmIdentity, true);
              assert.equal(body.placement.enabled, true);
              const { confirmIdentity, ...placement } = body.placement;
              catalog = {
                ...catalog,
                revision: catalog.revision + 1,
                placements: { "resource-active": placement },
                placement_reviews: {},
              };
              data = catalog;
            } else if (pathname === "/api/resource/removed-placements/1") {
              const body = request.postDataJSON();
              writes++;
              assert.equal(request.method(), "DELETE");
              assert.equal(body.confirm, true);
              if (rejectCleanup) {
                rejectCleanup = false;
                await route.fulfill({
                  status: 409,
                  json: {
                    success: false,
                    message: "Synthetic revision conflict",
                  },
                });
                return;
              }
              assert.equal(body.expected_revision, catalog.revision);
              catalog = {
                ...catalog,
                revision: catalog.revision + 1,
                removed_placements: [],
                removed_placement_total: 0,
              };
              data = catalog;
            } else if (pathname === "/api/resource/publication-context")
              data = {
                schema: "exact_model_pairs_preview_v1",
                profile: "openai_text_exact_v1",
                supported_endpoints: ["/v1/chat/completions", "/v1/messages"],
                resource_revision: catalog.revision,
                channel_revision: 1,
                option_version: 1,
                preview_only: true,
                affects_gateway_routing: false,
                channels: [],
              };
            else if (pathname === "/api/resource/publication-state")
              data = {
                schema: "managed_gateway_bindings_v1",
                resource_revision: catalog.revision,
                channel_revision: 1,
                option_version: 1,
                page: 1,
                page_size: 20,
                total: 0,
                read_only: true,
                bindings: [],
              };
            else if (pathname === "/api/channel") data = [];
            else if (pathname === "/api/status")
              data = { title: "Fixture", setup: true };
            else if (pathname === "/api/option") data = [];
            await route.fulfill({ json: { success: true, data } });
          },
        );
        const page = await context.newPage();
        page.on("pageerror", (error) =>
          report.errors.push(`${lang}-${theme}-${width}: ${error.message}`),
        );
        await page.goto(
          new URL("/admin?tab=resources", vite.resolvedUrls.local[0]).href,
        );
        const inventory = page.locator(".resource-inventory"),
          dialog = page.getByRole("dialog");
        const button = (key, root = page) =>
          root.getByRole("button", { name: labels[key], exact: true });
        await page.getByTestId("identity-review-required").waitFor();
        assert.ok(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
        );
        await inventory.screenshot({
          path: path.join(out, `${lang}-${theme}-${width}-inventory.png`),
        });
        await button("edit_placement_ffca10c028", inventory).click();
        await dialog.getByRole("checkbox").last().check();
        await button("review_changes_0594a17c6d", dialog).click();
        await dialog.getByRole("alert").waitFor();
        assert.equal(writes, 0);
        await page.getByTestId("confirm-resource-identity").focus();
        await page.keyboard.press("Space");
        await dialog.screenshot({
          path: path.join(out, `${lang}-${theme}-${width}-review.png`),
        });
        await button("review_changes_0594a17c6d", dialog).click();
        await button("confirm_save_f90ef9ab0e", dialog).click();
        await dialog.waitFor({ state: "hidden" });
        await page
          .getByTestId("identity-review-required")
          .waitFor({ state: "hidden" });
        assert.equal(writes, 1);
        const cleanup = button("resource_review_cleanup", inventory);
        await cleanup.click();
        await button("cancel_0828e4ce08", dialog).click();
        await dialog.waitFor({ state: "hidden" });
        await page.waitForFunction(
          (label) => document.activeElement?.textContent?.trim() === label,
          labels.resource_review_cleanup,
        );
        assert.equal(
          await cleanup.evaluate(
            (element) => element === document.activeElement,
          ),
          true,
        );
        await cleanup.click();
        await button("review_changes_0594a17c6d", dialog).click();
        await button("confirm_save_f90ef9ab0e", dialog).click();
        await dialog.getByRole("alert").waitFor();
        assert.equal(catalog.removed_placements.length, 1);
        await button("cancel_0828e4ce08", dialog).click();
        await cleanup.click();
        await button("review_changes_0594a17c6d", dialog).click();
        await button("confirm_save_f90ef9ab0e", dialog).click();
        await dialog.waitFor({ state: "hidden" });
        await page
          .getByTestId("removed-placements")
          .waitFor({ state: "hidden" });
        assert.equal(catalog.placements["resource-active"].enabled, true);
        await button("import_snapshot_9fbcc55332", inventory).click();
        const invalid = structuredClone(catalog.snapshot);
        invalid.sequence++;
        invalid.channels[0].enabled = null;
        await page.getByLabel("Snapshot JSON").fill(JSON.stringify(invalid));
        await button("review_changes_0594a17c6d", dialog).click();
        await dialog.getByRole("alert").waitFor();
        assert.equal(writes, 3);
        assert.equal(catalog.snapshot.sequence, 3);
        await button("cancel_0828e4ce08", dialog).click();
        report.cases.push({ lang, theme, width, passed: true });
        await context.close();
      }
  assert.deepEqual(report.errors, []);
} catch (error) {
  report.errors.push(String(error.stack || error));
  process.exitCode = 1;
} finally {
  await browser?.close();
  await vite.close();
  await writeFile(
    path.join(out, "report.json"),
    JSON.stringify(report, null, 2),
  );
  console.log(JSON.stringify(report));
}
