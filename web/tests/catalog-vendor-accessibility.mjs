import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';

const base = process.env.VEYRA_URL || 'http://127.0.0.1:5196';
const out = process.env.VEYRA_EVIDENCE_DIR || 'docs/verification-2026-09-27';
const prefix = process.env.VEYRA_EVIDENCE_PREFIX || 'catalog-vendor-accessibility';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const report = { base, startedAt: new Date().toISOString(), independentBrowserContext: true, businessStateInjection: false, scans: [] };
try {
  for (const [name, width, language, view] of [
    ['桌面厂商分类卡片', 1440, 'zh', 'cards'],
    ['桌面厂商分类列表', 1440, 'zh', 'list'],
    ['阿拉伯文深色窄屏卡片', 390, 'ar', 'cards'],
    ['阿拉伯文深色窄屏列表', 390, 'ar', 'list'],
  ]) {
    const context = await browser.newContext({ viewport: { width, height: 1000 }, reducedMotion: 'reduce' });
    const page = await context.newPage();
    await page.addInitScript(language => {
      localStorage.setItem('xy-language', language);
      localStorage.setItem('xy-theme', language === 'ar' ? 'dark' : 'light');
    }, language);
    await page.goto(`${base}/models?view=${view}`);
    await page.locator('[data-model-id]').first().waitFor();
    await page.waitForFunction(value => document.querySelector(`[data-catalog-view="${value}"]`)?.getAttribute('aria-pressed') === 'true', view);
    const results = await new AxeBuilder({ page }).include('main').withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze();
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
