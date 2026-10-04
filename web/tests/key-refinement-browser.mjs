import { chromium } from "playwright";
import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
const base = process.env.VEYRA_URL || "http://127.0.0.1:5196";
const out = "docs/verification-2026-09-26-round2";
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: "chrome", headless: true });
const context = await browser.newContext({
  viewport: { width: 1440, height: 1000 },
});
const page = await context.newPage();
const report = {
  checks: [],
  errors: [],
  screenshots: [],
  startedAt: new Date().toISOString(),
};
page.on("pageerror", (e) => report.errors.push(String(e)));
async function check(name, fn) {
  try {
    await fn();
    report.checks.push({ name, passed: true });
    console.log("PASS", name);
  } catch (e) {
    report.checks.push({ name, passed: false, error: String(e) });
    console.log("FAIL", name, e.stack);
  }
}
async function bind() {
  await page.evaluate(async () => {
    const r = performance
      .getEntriesByType("resource")
      .find((r) => r.name.includes("/src/lib/product-preview.ts"));
    window.__keyStore = (await import(r.name)).useProductPreview;
  });
}
async function go(path) {
  await page.goto(base + path);
  await page.locator("main").first().waitFor();
  await bind();
}
const state = () => page.evaluate(() => window.__keyStore.getState());
const screenshot = async (name) => {
  await page.waitForTimeout(240);
  await page.screenshot({ path: `${out}/${name}.png`, fullPage: true });
  report.screenshots.push(`${name}.png`);
};
const advanced = () => page.locator(".key-task-advanced > summary");
const modelSwitch = () =>
  page.getByRole("switch", { name: "启用模型白名单", exact: true });
const ipSwitch = () =>
  page.getByRole("switch", { name: "启用 IP（网络地址）白名单", exact: true });
const submit = () =>
  page.locator("#new-key-form").evaluate((f) => f.requestSubmit());
await go("/console/keys");
await page.evaluate(() => {
  window.__keyStore.getState().resetScenario("first");
  sessionStorage.removeItem("veyra-key-draft-v2");
});
await go("/console/keys?create=1");
await check(
  "新草稿默认不重置；全宽预览不侵入弹窗底栏，仅模型列表滚动",
  async () => {
    assert.equal(await page.locator("#new-key-period").inputValue(), "never");
    await page.locator('[data-tier="pro"] .tier-picker-info').click();
    const geometry = await page
      .locator(".tier-picker-preview")
      .evaluate((el) => {
        const rect = el.getBoundingClientRect(),
          form = el.closest("form").getBoundingClientRect(),
          body = el.closest("[data-overlay-body]"),
          footer = body.nextElementSibling.getBoundingClientRect(),
          list = el.querySelector("ul");
        return {
          width: rect.width,
          formWidth: form.width,
          left: rect.left,
          formLeft: form.left,
          bottom: rect.bottom,
          footerTop: footer.top,
          scrollHeight: list.scrollHeight,
          clientHeight: list.clientHeight,
          bodyScroll: body.scrollTop,
        };
      });
    assert.ok(Math.abs(geometry.width - geometry.formWidth) < 2);
    assert.ok(Math.abs(geometry.left - geometry.formLeft) < 2);
    assert.ok(geometry.bottom <= geometry.footerTop - 7);
    assert.ok(geometry.scrollHeight > geometry.clientHeight);
    await page.locator(".tier-preview-models").hover();
    await page.mouse.wheel(0, 250);
    await page.waitForTimeout(100);
    assert.ok(
      (await page
        .locator(".tier-preview-models")
        .evaluate((el) => el.scrollTop)) > 0,
    );
    assert.equal(
      await page.locator("[data-overlay-body]").evaluate((el) => el.scrollTop),
      geometry.bodyScroll,
    );
    await screenshot("key-desktop-preview");
    await page.keyboard.press("Escape");
    assert.equal(await page.locator(".tier-picker-preview").count(), 0);
    assert.equal(await page.getByRole("dialog").count(), 1);
  },
);
await check(
  "高级设置展开收起、图标状态；模型白名单胶囊开关保留选择草稿",
  async () => {
    assert.equal(
      await advanced().innerText(),
      "高级设置\n模型白名单与网络地址限制\n展开",
    );
    await advanced().click();
    assert.ok((await advanced().innerText()).includes("收起"));
    assert.equal(await modelSwitch().getAttribute("aria-checked"), "false");
    await modelSwitch().click();
    const boxes = page.locator(".key-model-list input");
    await boxes.nth(0).check();
    await boxes.nth(1).check();
    await modelSwitch().click();
    assert.equal(await page.locator(".key-model-list").count(), 0);
    await modelSwitch().click();
    assert.equal(
      await page.locator(".key-model-list input:checked").count(),
      2,
    );
    await advanced().click();
    assert.ok((await advanced().innerText()).includes("展开"));
    await advanced().click();
    assert.equal(
      await page.locator(".key-model-list input:checked").count(),
      2,
    );
    await page.locator(".key-ip-allowlist").scrollIntoViewIfNeeded();
    await screenshot("key-desktop-advanced");
  },
);
await check(
  "IP白名单启用空列表阻止创建；关闭/开启恢复地址，取消与刷新保留草稿",
  async () => {
    await page.locator("#new-key-name").fill("白名单切换验证");
    await ipSwitch().click();
    await submit();
    assert.ok(
      (await page.getByRole("alert").innerText()).includes("至少填写一个地址"),
    );
    assert.equal((await state()).keys.length, 0);
    await page.locator("#new-key-ips").fill("127.0.0.1\n10.0.0.0/24");
    await ipSwitch().click();
    assert.equal(await page.locator("#new-key-ips").count(), 0);
    await ipSwitch().click();
    assert.equal(
      await page.locator("#new-key-ips").inputValue(),
      "127.0.0.1\n10.0.0.0/24",
    );
    await page
      .getByRole("button", { name: "取消，保留草稿", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    await go("/console/keys?create=1");
    assert.equal(
      await page.locator("#new-key-name").inputValue(),
      "白名单切换验证",
    );
    await advanced().click();
    assert.equal(await modelSwitch().getAttribute("aria-checked"), "true");
    assert.equal(await ipSwitch().getAttribute("aria-checked"), "true");
    assert.equal(
      await page.locator(".key-model-list input:checked").count(),
      2,
    );
    assert.equal(
      await page.locator("#new-key-ips").inputValue(),
      "127.0.0.1\n10.0.0.0/24",
    );
  },
);
await check(
  "关闭白名单创建保存真实不限范围状态；编辑取消不改数据且不扩张原智能入口权限",
  async () => {
    await modelSwitch().click();
    await ipSwitch().click();
    await submit();
    await page
      .getByRole("heading", { name: "密钥已创建", exact: true })
      .waitFor();
    let key = (await state()).keys[0];
    assert.equal(key.modelAccess, "all");
    assert.equal(key.allowedModels, undefined);
    assert.deepEqual(key.ipWhitelist, []);
    assert.equal(key.quotaPeriod, "never");
    await page.getByRole("button", { name: "关闭", exact: true }).click();
    await page.evaluate(
      (id) =>
        window.__keyStore.getState().updateKey(id, {
          smartAccess: false,
          modelAccess: "custom",
          allowedModels: ["gpt-4.1"],
          quotaPeriod: "monthly",
          ipWhitelist: ["127.0.0.1"],
        }),
      key.id,
    );
    await page
      .getByRole("button", { name: "编辑", exact: true })
      .first()
      .click();
    assert.equal(
      await page.locator("#edit-key-period").inputValue(),
      "monthly",
    );
    await advanced().click();
    assert.equal(await modelSwitch().getAttribute("aria-checked"), "true");
    assert.equal(await ipSwitch().getAttribute("aria-checked"), "true");
    await modelSwitch().click();
    await ipSwitch().click();
    await page.getByRole("button", { name: "取消", exact: true }).click();
    key = (await state()).keys[0];
    assert.equal(key.modelAccess, "custom");
    assert.deepEqual(key.ipWhitelist, ["127.0.0.1"]);
    assert.equal(key.smartAccess, false);
    await page
      .getByRole("button", { name: "编辑", exact: true })
      .first()
      .click();
    await advanced().click();
    await modelSwitch().click();
    await ipSwitch().click();
    await page.getByRole("button", { name: "保存修改", exact: true }).click();
    key = (await state()).keys[0];
    assert.equal(key.modelAccess, "all");
    assert.deepEqual(key.ipWhitelist, []);
    assert.equal(key.smartAccess, false);
    assert.equal(key.quotaPeriod, "monthly");
  },
);
await check(
  "全局原生下拉箭头右距12px且文本预留38px；Radix箭头有独立内距",
  async () => {
    for (const path of [
      "/console/keys?create=1",
      "/console/connect",
      "/console/playground",
      "/models",
    ]) {
      await go(path);
      await page.locator('select:not([aria-hidden="true"])').first().waitFor();
      const controls = await page
        .locator('select:not([aria-hidden="true"])')
        .evaluateAll((els) =>
          els.map((el) => ({
            appearance: getComputedStyle(el).appearance,
            padding: getComputedStyle(el).paddingInlineEnd,
            position: getComputedStyle(el).backgroundPositionX,
            image: getComputedStyle(el).backgroundImage,
          })),
        );
      assert.ok(controls.length);
      for (const c of controls) {
        assert.equal(c.appearance, "none");
        assert.equal(c.padding, "38px");
        assert.ok(c.position.includes("12px"));
        assert.ok(c.image.includes("svg"));
      }
    }
    const trigger = page.locator("button.select-trigger:visible").first();
    const metric = await trigger.evaluate((el) => ({
      padding: getComputedStyle(el).paddingInlineEnd,
      icon: !!el.querySelector(".select-trigger-icon"),
    }));
    assert.equal(metric.padding, "12px");
    assert.equal(metric.icon, true);
  },
);
await check(
  "390px窄屏预览全宽受底栏约束；键盘Escape关闭预览，空格切换高级设置",
  async () => {
    await page.setViewportSize({ width: 390, height: 844 });
    await go("/console/keys?create=1");
    await page.locator('[data-tier="pro"] .tier-picker-info').click();
    const g = await page.locator(".tier-picker-preview").evaluate((el) => {
      const r = el.getBoundingClientRect(),
        f = el.closest("form").getBoundingClientRect(),
        b = el.closest("[data-overlay-body]").getBoundingClientRect();
      return {
        w: r.width,
        fw: f.width,
        bottom: r.bottom,
        end: b.bottom,
        top: r.top,
        start: b.top,
      };
    });
    assert.ok(Math.abs(g.w - g.fw) < 2);
    assert.ok(g.bottom <= g.end - 7);
    assert.ok(g.top >= g.start);
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth > innerWidth,
      ),
      false,
    );
    await screenshot("key-mobile-preview");
    await page.keyboard.press("Escape");
    assert.equal(await page.locator(".tier-picker-preview").count(), 0);
    await advanced().focus();
    await page.keyboard.press("Space");
    assert.equal(
      await page.locator(".key-task-advanced").getAttribute("open"),
      "",
    );
    await modelSwitch().click();
    await screenshot("key-mobile-advanced");
  },
);
await check(
  "英文与阿拉伯文深色窄屏，无横向溢出且箭头遵循书写方向",
  async () => {
    for (const lang of ["en", "ar"]) {
      await page.evaluate((l) => {
        localStorage.setItem("xy-language", l);
        localStorage.setItem("xy-theme", "dark");
      }, lang);
      await page.reload();
      await page.locator("#new-key-name").waitFor();
      assert.equal(
        await page.evaluate(
          () => document.documentElement.scrollWidth > innerWidth,
        ),
        false,
      );
      const pos = await page
        .locator("#new-key-period")
        .evaluate((el) => getComputedStyle(el).backgroundPositionX);
      assert.ok(lang === "ar" ? pos === "12px" : pos.includes("12px"));
      await page.locator('[data-tier="pro"] .tier-picker-info').click();
      await screenshot(`key-mobile-${lang}-dark`);
    }
  },
);
report.finishedAt = new Date().toISOString();
await writeFile(
  `${out}/key-browser-results.json`,
  JSON.stringify(report, null, 2),
);
await browser.close();
if (report.errors.length || report.checks.some((c) => !c.passed))
  process.exitCode = 1;
