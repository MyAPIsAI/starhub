import assert from "node:assert/strict";
import { writeFile } from "node:fs/promises";
import { chromium } from "playwright";
const out = new URL(
  "../../output/catalog-demo-refresh/update/",
  import.meta.url,
);
const browser = await chromium.launch({ channel: "chrome", headless: true });
try {
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
    reducedMotion: "reduce",
  });
  const page = await context.newPage();
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const response = await page.request.get(
    "http://127.0.0.1:5173/api/catalog/models",
  );
  assert.equal(response.status(), 200);
  const body = await response.json();
  assert.equal(body.success, true);
  assert.equal(body.data.items[0]?.id, "myapis-auto");
  assert.equal(body.data.items[0]?.name, "MyAPIs Auto");
  for (const route of ["/api/catalog/rank", "/api/plaza/rank"]) {
    assert.equal(
      (await page.request.get("http://127.0.0.1:5173" + route)).status(),
      404,
    );
  }
  const legacy = await page.request.get(
    "http://127.0.0.1:5173/api/catalog/models/veyra-auto",
  );
  assert.equal(legacy.status(), 200);
  assert.equal((await legacy.json()).data.model.id, "myapis-auto");
  await page.goto("http://127.0.0.1:5173/models");
  if (body.data.total > 0)
    await page.locator("[data-model-id]").first().waitFor();
  else await page.locator('.catalog-empty[aria-busy="false"]').waitFor();
  assert.equal(await page.locator('.catalog-empty[role="alert"]').count(), 0);
  assert.equal(await page.locator(".catalog-ranking").count(), 0);
  assert.equal(await page.getByText("Veyra Auto", { exact: true }).count(), 0);
  await page.screenshot({
    path: new URL("live-marketplace.png", out).pathname.replace(
      /^\/([A-Z]:)/,
      "$1",
    ),
    fullPage: true,
  });
  const model =
    body.data.items.find((row) => row.kind !== "smart") ?? body.data.items[0];
  if (model) {
    await page.goto(
      "http://127.0.0.1:5173/models/" + encodeURIComponent(model.id),
    );
    await page.locator(".catalog-group-prices").waitFor();
    assert.equal(
      await page.locator("[data-group-id]").count(),
      model.group_prices.length,
    );
    await page.screenshot({
      path: new URL("live-detail.png", out).pathname.replace(
        /^\/([A-Z]:)/,
        "$1",
      ),
      fullPage: true,
    });
  }
  assert.deepEqual(errors, []);
  const result = {
    passed: true,
    fixtureData: false,
    anonymous: true,
    modelCount: body.data.total,
    groupTableVerified: Boolean(model),
    errors,
  };
  await writeFile(
    new URL("live-smoke.json", out),
    JSON.stringify(result, null, 2),
  );
  console.log(JSON.stringify(result));
} finally {
  await browser.close();
}
