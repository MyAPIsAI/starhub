import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';

const base = process.env.VEYRA_URL || 'http://127.0.0.1:5196';
const out = process.env.VEYRA_EVIDENCE_DIR || 'docs/verification-2026-09-27';
const prefix = process.env.VEYRA_EVIDENCE_PREFIX || 'account-scope';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const report = {
  base, startedAt: new Date().toISOString(), independentBrowserContext: true,
  ordinaryChecksUseBusinessStateInjection: false,
  migrationFixture: 'Only the explicitly named legacy migration check writes an old billing snapshot in a fresh independent context. Registration and scenario checks use normal UI actions.',
  checks: [], errors: [], screenshots: [],
};
let page;
async function newPage({ width = 1440, height = 1000 } = {}) {
  const context = await browser.newContext({ viewport: { width, height } });
  page = await context.newPage();
  page.setDefaultTimeout(10000);
  page.on('pageerror', error => report.errors.push(String(error)));
}
async function go(route = '/console/settings') {
  await page.goto(`${base}${route}`);
  if (route === '/console/settings') await page.locator('#account-username').waitFor();
  else if (route.startsWith('/console/')) await page.locator('main h1').first().waitFor();
  else if (route === '/') await page.locator('.xy-hero').waitFor();
  else await page.locator('main').waitFor();
}
async function capture(name) {
  const file = `${prefix}-${name}.png`;
  await page.screenshot({ path: `${out}/${file}`, fullPage: false });
  report.screenshots.push(file);
}
async function check(name, fn) {
  try { await fn(); report.checks.push({ name, passed: true }); console.log('PASS', name); }
  catch (error) {
    report.checks.push({ name, passed: false, error: String(error) });
    console.log('FAIL', name, error.message);
    await capture(`failure-${report.checks.length}`).catch(() => {});
  }
}
const snapshot = () => page.evaluate(() => JSON.parse(localStorage.getItem('veyra-billing-demo-v1')));
const forbiddenAccountPhrases = /企业(?:账号|账户|名称|工作空间|工作区|用户|客户)|个人工作空间|我的工作空间|我的企业|演示企业|企业两把密钥|Enterprise\s+(?:account|workspace)|Company name|Primary Business Email/i;
async function assertNoAccountSplit() {
  const text = await page.locator('body').innerText();
  const match = text.match(forbiddenAccountPhrases);
  assert.equal(match, null, match ? `仍存在旧账号分类文案：${match[0]}` : undefined);
  assert.equal(await page.locator('#ent-name-input, #auth-company').count(), 0, '不得保留企业名称输入入口');
}
async function assertUnifiedProfile({ username, phone = true } = {}) {
  await page.locator('#account-username').waitFor();
  assert.equal(await page.locator('main h1').innerText(), '账号中心');
  const navigationClosed = !(await page.locator('.xy-sidebar-nav a[href="/console/settings"]').isVisible());
  if (navigationClosed) await page.locator('.xy-menu-toggle').click();
  assert.equal(await page.locator('.xy-sidebar-nav a[href="/console/settings"]').innerText(), '账号中心');
  assert.equal(await page.locator('.xy-workspace strong').innerText(), '工作台');
  assert.ok(await page.locator('.ac-profile').getByText('账号资料', { exact: true }).isVisible());
  assert.ok(await page.locator('main').getByRole('button', { name: '申请注销', exact: true }).isVisible());
  assert.equal(await page.locator('#account-username').getAttribute('readonly'), '');
  assert.equal(await page.locator('#account-phone').getAttribute('readonly'), '');
  const currentUsername = await page.locator('#account-username').inputValue();
  assert.ok(currentUsername.length > 0, '默认和历史账号都必须有可解释的登录标识展示');
  if (username) assert.equal(currentUsername, username);
  if (phone) assert.match(await page.locator('#account-phone').inputValue(), /\*{4}/, '已绑定手机号应脱敏展示');
  await assertNoAccountSplit();
  if (navigationClosed) await page.locator('.xy-mobile-close').click();
}
async function noOverflow() {
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), '不得出现整页横向溢出');
  for (const selector of ['#account-username', '#account-phone']) {
    const rect = await page.locator(selector).boundingBox();
    assert.ok(rect && rect.x >= 0 && rect.x + rect.width <= page.viewportSize().width + 1, `${selector}超出屏幕`);
  }
}
async function selectScenario(index, id) {
  const trigger = page.locator('.demo-controls > button').first();
  if (await trigger.getAttribute('aria-expanded') !== 'true') await trigger.click();
  await page.locator('.demo-controls-panel .demo-scenario-card').nth(index).click();
  await page.locator('.demo-controls-panel').waitFor({ state: 'hidden' });
  await page.waitForFunction(expected => JSON.parse(localStorage.getItem('veyra-billing-demo-v1')).scenario === expected, id);
}

try {
  await newPage();
  await check('不经注册直接进入默认演示账号，资料和导航统一为账号中心及工作台', async () => {
    await go(); await assertUnifiedProfile({ username: 'veyra_demo' });
    assert.equal(await page.locator('.auth-dialog').count(), 0);
    await noOverflow(); await capture('default-profile-desktop');
  });
  await check('默认账号刷新保留身份、钱包和统一账号入口', async () => {
    const before = await snapshot();
    const userValue = await page.locator('#account-username').inputValue();
    const phoneValue = await page.locator('#account-phone').inputValue();
    await page.reload(); await assertUnifiedProfile({ username: userValue });
    assert.equal(await page.locator('#account-phone').inputValue(), phoneValue);
    assert.deepEqual(await snapshot(), before);
  });
  for (const [index, id, label] of [[0, 'first', '首次接入'], [1, 'enterprise', '多用途密钥'], [2, 'pricing', '运营改价']]) {
    await check(`${label}真实场景切换后账号展示一致，系统设置仅管理员场景可见`, async () => {
      await go(); await selectScenario(index, id);
      const state = await snapshot();
      assert.equal(state.scenario, id);
      assert.equal(BigInt(state.balance), state.ledger.reduce((total, row) => total + BigInt(row.amount_nano), 0n), '场景余额应与实际流水（含已发生的示例调用）一致');
      if (id === 'first') assert.equal(state.balance, '0');
      else assert.ok(BigInt(state.balance) > 0n, '已有用户场景应保留可操作的模拟余额；运营场景可能已计入示例调用');
      assert.equal(state.keys.length, id === 'first' ? 0 : id === 'enterprise' ? 2 : 1);
      const administrator = id === 'pricing';
      assert.equal(await page.evaluate(() => localStorage.getItem('veyra-demo-account-role-v1')), administrator ? 'superAdmin' : 'customer');
      await go(); await assertUnifiedProfile({ username: 'veyra_demo' });
      for (const route of ['model-pricing', 'channels', 'service-groups']) {
        assert.equal(await page.locator(`.xy-sidebar-nav a[href="/console/${route}"]`).count(), administrator ? 1 : 0);
      }
      assert.equal((await page.locator('.xy-sidebar-nav').innerText()).includes('系统设置'), administrator);
      await capture(`scenario-${id}`);
    });
  }
  for (const [route, label] of [
    ['/', '主页'], ['/models', '模型目录'], ['/models/gpt-4.1', '模型详情'],
    ['/console/billing', '账务'], ['/console/service-quality', '服务质量'], ['/console/service-groups', '服务分组'],
  ]) {
    await check(`${label}可见页面不再出现企业账号分类和企业资料入口`, async () => {
      await go(route); await assertNoAccountSplit();
      if (route === '/console/service-groups') {
        assert.ok(await page.getByRole('heading', { name: '服务分组', exact: true }).isVisible());
      }
      await capture(`neutral-${label}`);
    });
  }

  // Only this migration check writes business state. It models an actual old
  // persisted schema without username/phone, never a successful registration.
  await check('旧版无用户名快照通过真实启动迁移，保留旧邮箱和账本且不恢复企业表单', async () => {
    await go(); const legacy = structuredClone(await snapshot());
    legacy.user = {
      enterpriseName: 'Legacy Profile Archive', email: 'legacy-account@example.test',
      isLoggedIn: true, onboardingCompleted: true, accountStatus: 'normal',
    };
    await newPage(); await go('/');
    await page.evaluate(value => localStorage.setItem('veyra-billing-demo-v1', JSON.stringify(value)), legacy);
    await go(); await assertUnifiedProfile({ phone: false });
    const username = await page.locator('#account-username').inputValue();
    assert.ok(!username.includes('Legacy Profile Archive'), '历史机构显示名不能被冒充为用户名');
    assert.match(await page.locator('#account-phone').inputValue(), /未绑定|未设置|—/);
    const profile = await page.locator('main').evaluate(node => `${node.textContent} ${Array.from(node.querySelectorAll('input')).map(input => input.value).join(' ')}`);
    assert.ok(profile.includes('legacy-account@example.test'), '应保留可解释的旧邮箱登录标识');
    const restored = await snapshot();
    assert.equal(restored.user.email, legacy.user.email);
    assert.equal(restored.balance, legacy.balance);
    for (const field of ['ledger', 'rechargeOrders', 'keys', 'liveRequests']) assert.deepEqual(restored[field], legacy[field]);
    await page.reload(); await assertUnifiedProfile({ username, phone: false });
    assert.equal((await snapshot()).user.email, legacy.user.email);
    await capture('legacy-profile');
  });

  for (const width of [390, 320]) {
    await newPage({ width, height: width === 320 ? 568 : 844 });
    await check(`${width}像素默认账号资料和导航可操作，输入框不横向溢出`, async () => {
      await go(); await assertUnifiedProfile({ username: 'veyra_demo' }); await noOverflow();
      await page.locator('.xy-menu-toggle').click();
      assert.equal(await page.locator('.xy-sidebar').evaluate(node => node.classList.contains('is-open')), true);
      assert.ok(await page.locator('.xy-sidebar-nav a[href="/console/settings"]').isVisible());
      assert.equal(await page.locator('.xy-workspace strong').innerText(), '工作台');
      await page.locator('.xy-mobile-close').click();
      assert.equal(await page.locator('.xy-sidebar').evaluate(node => node.classList.contains('is-open')), false);
      await noOverflow(); await capture(`profile-mobile-${width}`);
      await page.locator('main').getByRole('button', { name: '申请注销', exact: true }).scrollIntoViewIfNeeded();
      const button = await page.locator('main').getByRole('button', { name: '申请注销', exact: true }).boundingBox();
      assert.ok(button.y >= 0 && button.y + button.height <= page.viewportSize().height + 1);
    });
  }
  for (const [language, title] of [['en', 'Account center'], ['ar', 'مركز الحساب']]) {
    await newPage({ width: 390, height: 844 });
    await check(`${language === 'en' ? '英文' : '阿拉伯文'}深色默认账号保持统一标题与窄屏布局`, async () => {
      await go('/');
      // Presentation preferences only; authentication/business state is untouched.
      await page.evaluate(value => { localStorage.setItem('xy-language', value); localStorage.setItem('xy-theme', 'dark'); }, language);
      await go();
      assert.equal(await page.locator('main h1').innerText(), title);
      await page.locator('.xy-menu-toggle').click();
      assert.equal(await page.locator('.xy-sidebar-nav a[href="/console/settings"]').innerText(), title);
      await page.locator('.xy-mobile-close').click();
      assert.equal(await page.locator('html').getAttribute('dir'), language === 'ar' ? 'rtl' : 'ltr');
      assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
      assert.equal(await page.locator('#account-username').inputValue(), 'veyra_demo');
      await assertNoAccountSplit(); await noOverflow(); await capture(`profile-${language}-dark`);
    });
  }
} finally {
  report.completedAt = new Date().toISOString();
  report.passed = report.checks.filter(item => item.passed).length;
  report.total = report.checks.length;
  await writeFile(`${out}/${prefix}-browser-report.json`, JSON.stringify(report, null, 2));
  await browser.close();
}
if (report.errors.length || report.checks.some(item => !item.passed)) process.exitCode = 1;
console.log(`${report.passed}/${report.total} checks passed; ${report.errors.length} page errors`);
