import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
const base = process.env.VEYRA_URL || 'http://127.0.0.1:5196';
const out = process.env.VEYRA_EVIDENCE_DIR || 'docs/verification-2026-09-27';
const prefix = process.env.VEYRA_EVIDENCE_PREFIX || 'account-center-accessibility';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const report = { base, startedAt: new Date().toISOString(), independentBrowserContext: true, businessStateInjection: false, scans: [] };
try {
  for (const [name, width, language, mode] of [
    ['桌面账号中心', 1440, 'zh', null], ['桌面改密弹窗', 1440, 'zh', 'password'],
    ['320像素手机换绑弹窗', 320, 'zh', 'phone'], ['阿拉伯文深色邮箱弹窗', 390, 'ar', 'email'],
  ]) {
    const context = await browser.newContext({ viewport: { width, height: width === 320 ? 568 : 1000 }, reducedMotion: 'reduce' });
    const page = await context.newPage();
    await page.goto(`${base}/console/settings`); await page.locator('#account-id').waitFor();
    if (language === 'ar') {
      await page.evaluate(() => { localStorage.setItem('xy-language', 'ar'); localStorage.setItem('xy-theme', 'dark'); });
      await page.reload(); await page.locator('#account-id').waitFor();
    }
    if (mode) {
      await page.locator(`[data-account-action="${mode}"]`).click();
      await page.locator('#account-security-form').waitFor();
    }
    const results = await new AxeBuilder({ page }).include(mode ? '[role="dialog"]' : '.account-center').withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze();
    report.scans.push({ name, violations: results.violations.map(v => ({ id: v.id, impact: v.impact, description: v.description, help: v.help, nodes: v.nodes.map(n => ({ target: n.target, failureSummary: n.failureSummary })) })) });
    console.log(name, results.violations.length, 'violations');
    await context.close();
  }
} finally {
  report.completedAt = new Date().toISOString();
  await writeFile(`${out}/${prefix}-report.json`, JSON.stringify(report, null, 2));
  await browser.close();
}
assert.equal(report.scans.reduce((n, scan) => n + scan.violations.length, 0), 0);
