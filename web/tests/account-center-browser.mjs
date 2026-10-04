import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';

const base = process.env.VEYRA_URL || 'http://127.0.0.1:5196';
const out = process.env.VEYRA_EVIDENCE_DIR || 'docs/verification-2026-09-27';
const prefix = process.env.VEYRA_EVIDENCE_PREFIX || 'account-center';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const report = {
  base, startedAt: new Date().toISOString(), independentBrowserContext: true,
  businessStateInjection: false,
  clockSimulation: 'Only advance real UI verification resend cooldowns; no injected authentication results.',
  checks: [], errors: [], screenshots: [],
};
let page;
async function newPage({ width = 1440, height = 1000, clock = true } = {}) {
  const context = await browser.newContext({ viewport: { width, height }, reducedMotion: 'reduce' });
  page = await context.newPage();
  page.setDefaultTimeout(10000);
  page.on('pageerror', error => report.errors.push(String(error)));
  if (clock) await page.clock.install();
}
const path = () => new URL(page.url()).pathname;
const snapshot = () => page.evaluate(() => JSON.parse(localStorage.getItem('veyra-billing-demo-v1')));
const role = () => page.evaluate(() => localStorage.getItem('veyra-demo-account-role-v1'));
async function financialState() {
  const state = await snapshot();
  return { balance: state.balance, ledger: state.ledger, orders: state.rechargeOrders, keys: state.keys, requests: state.liveRequests, scenario: state.scenario, role: await role() };
}
async function go(route = '/console/settings') {
  await page.goto(`${base}${route}`);
  if (route.startsWith('/console/settings')) await page.locator('#account-id').waitFor();
  else if (/^\/(login|register|forgot-password)/.test(route)) await page.locator('.auth-dialog').waitFor();
  else await page.locator('main').waitFor();
}
async function capture(name) {
  const file = `${prefix}-${name}.png`;
  await page.screenshot({ path: `${out}/${file}`, fullPage: false });
  report.screenshots.push(file);
}
async function check(name, run) {
  try { await run(); report.checks.push({ name, passed: true }); console.log('PASS', name); }
  catch (error) {
    report.checks.push({ name, passed: false, error: String(error) });
    console.log('FAIL', name, error.message);
    await capture(`failure-${report.checks.length}`).catch(() => {});
  }
}
async function fieldValue(selector) {
  return page.locator(selector).evaluate(node => 'value' in node ? node.value : node.textContent.trim());
}
const identityId = () => fieldValue('#account-id');
const securityForm = () => page.locator('#account-security-form');
const securitySubmit = () => page.locator('button[type="submit"][form="account-security-form"], #account-security-form button[type="submit"]').first();
const securityAlert = () => page.getByRole('dialog').getByRole('alert');
async function openSecurity(action) {
  await page.locator(`[data-account-action="${action}"]`).click();
  await securityForm().waitFor();
}
async function closeSecurity() {
  await page.keyboard.press('Escape');
  await securityForm().waitFor({ state: 'hidden' });
}
async function finishSecurity() {
  await securitySubmit().click();
  await securityForm().waitFor({ state: 'hidden' });
}
async function requestCode() {
  await page.clock.fastForward(61000);
  await page.locator('[data-account-send-code]').click();
}
async function bindContact(action, target, password) {
  await openSecurity(action);
  await page.locator('#security-current-password').fill(password);
  await page.locator('#security-target').fill(target);
  await requestCode();
  await page.locator('#security-code').fill('829104');
  await finishSecurity();
}
async function login(account, password, { success = true } = {}) {
  await go('/login');
  await page.locator('#auth-account').fill(account);
  await page.locator('#auth-password').fill(password);
  await page.locator('#auth-consent').check();
  await page.locator('.auth-form button[type="submit"]').click();
  if (success) await page.waitForURL('**/console');
  else {
    await page.locator('.auth-error').waitFor();
    assert.equal(path(), '/login');
    assert.match(await page.locator('.auth-error').innerText(), /账号或密码不正确|无法登录|未找到/);
  }
}
async function register({ username, phone, password }) {
  await go('/register');
  await page.clock.fastForward(61000);
  await page.locator('#auth-username').fill(username);
  await page.locator('#auth-register-phone').fill(phone);
  await page.locator('#auth-new-password').fill(password);
  await page.locator('#auth-confirm-password').fill(password);
  await page.getByRole('button', { name: '获取验证码', exact: true }).click();
  await page.locator('#auth-code').fill('829104');
  await page.locator('#auth-consent').check();
  await page.locator('.auth-human input').check();
  await page.locator('.auth-form button[type="submit"]').click();
  await page.waitForURL('**/console/connect');
  await go();
}
async function noOverflow({ modal = false } = {}) {
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), '整页不能横向溢出');
  for (const selector of modal ? ['#security-current-password', '#security-target', '#security-code'] : ['#account-id', '#account-username', '#account-phone', '#account-email']) {
    const locator = page.locator(selector);
    if (await locator.count() === 0) continue;
    const box = await locator.boundingBox();
    assert.ok(box && box.x >= -1 && box.x + box.width <= page.viewportSize().width + 1, `${selector}超出横向屏幕`);
  }
  if (modal) {
    const box = await page.getByRole('dialog').boundingBox();
    assert.ok(box.y >= -1 && box.y + box.height <= page.viewportSize().height + 1, '账号安全弹窗不能超出屏幕高度');
  }
}
const alpha = { username: 'account_alpha', phone: '13900139000', password: 'AccountAlpha12345' };
const beta = { username: 'account_beta', phone: '13700137000', password: 'AccountBeta12345' };
const alphaEmail = 'alpha-account@example.test';
const betaEmail = 'beta-account@example.test';
const alphaNewPhone = '13600136000';
const nextPassword = 'AccountUpdated67890';
let alphaId;
let betaId;
let moneyBefore;
try {
  await newPage();
  await check('默认账号中心显示不可编辑的用户标识及明确的安全操作入口', async () => {
    await go();
    const id = await identityId();
    assert.ok(id.length >= 8 && !id.includes('@') && !id.includes('13800138000'), '用户标识应独立于联系方式');
    assert.ok(await page.locator('#account-id').evaluate(node => !('readOnly' in node) || node.readOnly));
    for (const action of ['password', 'phone', 'email']) assert.equal(await page.locator(`[data-account-action="${action}"]`).count(), 1);
    assert.equal(await page.locator('#account-username').inputValue(), 'veyra_demo');
    assert.equal(await page.locator('#ent-name-input').count(), 0);
    await noOverflow(); await capture('default-desktop');
    await page.reload(); await page.locator('#account-id').waitFor();
    assert.equal(await identityId(), id, '刷新不能重建账号标识');
  });
  await check('两个真实注册账号获得不同稳定标识，邮箱绑定需要验证后才生效', async () => {
    await register(beta); betaId = await identityId();
    const before = await financialState();
    await openSecurity('email');
    await page.locator('#security-current-password').fill(beta.password);
    await page.locator('#security-target').fill(betaEmail);
    await requestCode();
    assert.equal((await snapshot()).user.email, '', '仅发送验证码不能绑定邮箱');
    assert.equal(await page.locator('#security-code').inputValue(), '', '不能自动填写演示验证码');
    assert.match(await page.getByRole('dialog').innerText(), /演示|模拟/);
    await page.locator('#security-code').fill('829104'); await finishSecurity();
    assert.equal((await snapshot()).user.email, betaEmail);
    assert.equal((await snapshot()).user.emailVerified, true, '确认后邮箱应具有真实演示验证状态');
    assert.equal(await identityId(), betaId);
    assert.deepEqual(await financialState(), before);
    const betaProfile = (await snapshot()).user;
    assert.ok(betaProfile.securityEvents.length > 0, '邮箱绑定应产生本账号安全事件');
    await register(alpha); alphaId = await identityId();
    assert.notEqual(alphaId, betaId);
    const alphaProfile = (await snapshot()).user;
    assert.deepEqual(alphaProfile.securityEvents, [], '新账号不能带入其他账号安全记录');
    assert.notEqual(alphaProfile.createdAt, betaProfile.createdAt);
    await login(beta.username, beta.password); await go(); assert.equal(await identityId(), betaId);
    const restoredBeta = (await snapshot()).user;
    assert.deepEqual(restoredBeta.securityEvents, betaProfile.securityEvents);
    assert.equal(restoredBeta.createdAt, betaProfile.createdAt);
    await login(alpha.username, alpha.password); await go(); assert.equal(await identityId(), alphaId);
    assert.deepEqual((await snapshot()).user.securityEvents, []);
    assert.equal((await snapshot()).user.createdAt, alphaProfile.createdAt);
    moneyBefore = await financialState();
  });
  await check('绑定已占用邮箱明确失败，保留表单、原身份及账本', async () => {
    await openSecurity('email');
    await page.locator('#security-current-password').fill(alpha.password);
    await page.locator('#security-target').fill(betaEmail);
    await requestCode(); await securityAlert().waitFor();
    assert.match(await securityAlert().innerText(), /邮箱.*(?:已|占用|存在)|(?:已|占用|存在).*邮箱/);
    assert.equal(await page.locator('#security-target').inputValue(), betaEmail);
    assert.equal((await snapshot()).user.email, '');
    assert.deepEqual(await financialState(), moneyBefore);
    await closeSecurity();
  });
  await check('邮箱绑定错误验证码可修复，取消不变更账号，重新打开不残留密码', async () => {
    await openSecurity('email');
    await page.locator('#security-current-password').fill(alpha.password);
    await page.locator('#security-target').fill(alphaEmail);
    await requestCode();
    await page.locator('#security-code').fill('000000'); await securitySubmit().click();
    await securityAlert().waitFor(); assert.match(await securityAlert().innerText(), /验证码/);
    assert.equal(await page.locator('#security-target').inputValue(), alphaEmail);
    assert.equal((await snapshot()).user.email, '');
    await page.reload(); await securityForm().waitFor();
    assert.equal(await page.locator('#security-target').inputValue(), alphaEmail, '刷新保留非敏感的联系方式草稿');
    assert.equal(await page.locator('#security-current-password').inputValue(), '', '刷新不持久化密码');
    assert.equal(await page.locator('#security-code').inputValue(), '', '刷新不持久化用户填写的验证码');
    await closeSecurity();
    await openSecurity('email');
    assert.equal(await page.locator('#security-current-password').inputValue(), '');
    await closeSecurity();
    assert.equal(await identityId(), alphaId); assert.deepEqual(await financialState(), moneyBefore);
  });
  await check('邮箱验证码与目标绑定，修改目标后不能沿用旧码，正确验证后可用邮箱登录', async () => {
    await openSecurity('email');
    await page.locator('#security-current-password').fill(alpha.password);
    await page.locator('#security-target').fill('pending-alpha@example.test');
    await requestCode();
    await page.locator('#security-target').fill(alphaEmail);
    await page.locator('#security-code').fill('829104'); await securitySubmit().click();
    await securityAlert().waitFor(); assert.match(await securityAlert().innerText(), /验证码|重新|先/);
    assert.equal((await snapshot()).user.email, '');
    await requestCode(); await page.locator('#security-code').fill('829104'); await finishSecurity();
    assert.equal((await snapshot()).user.email, alphaEmail);
    assert.equal((await snapshot()).user.emailVerified, true);
    assert.equal(await identityId(), alphaId);
    await page.reload(); await page.locator('#account-id').waitFor();
    assert.equal((await snapshot()).user.email, alphaEmail);
    await login(alphaEmail, alpha.password); await go();
    assert.equal(await identityId(), alphaId); assert.deepEqual(await financialState(), moneyBefore);
    await capture('email-bound-desktop');
  });
  await check('换绑手机号拒绝其他账号已用号码，失败保留原手机号', async () => {
    await openSecurity('phone');
    await page.locator('#security-current-password').fill(alpha.password);
    await page.locator('#security-target').fill(beta.phone);
    await requestCode(); await securityAlert().waitFor();
    assert.match(await securityAlert().innerText(), /手机号.*(?:已|占用|存在)|(?:已|占用|存在).*手机号/);
    assert.equal((await snapshot()).user.phone.replace(/^\+86/, ''), alpha.phone);
    await closeSecurity(); assert.deepEqual(await financialState(), moneyBefore);
  });
  await check('新手机号完成验证才替换原号码，旧号码失效而新号码仍对应同一用户标识', async () => {
    await openSecurity('phone');
    await page.locator('#security-current-password').fill(alpha.password);
    await page.locator('#security-target').fill(alphaNewPhone);
    await requestCode();
    assert.equal((await snapshot()).user.phone.replace(/^\+86/, ''), alpha.phone);
    await page.locator('#security-code').fill('829104'); await finishSecurity();
    assert.equal((await snapshot()).user.phone.replace(/^\+86/, ''), alphaNewPhone);
    assert.equal(await identityId(), alphaId);
    await login(alpha.phone, alpha.password, { success: false });
    await login(alphaNewPhone, alpha.password); await go();
    assert.equal(await identityId(), alphaId); assert.deepEqual(await financialState(), moneyBefore);
  });
  await check('修改密码先校验当前密码及两次新密码；失败保留可修复输入', async () => {
    await openSecurity('password');
    await page.locator('#security-current-password').fill('IncorrectCurrent123');
    await page.locator('#security-new-password').fill(nextPassword);
    await page.locator('#security-confirm-password').fill(nextPassword);
    await securitySubmit().click(); await securityAlert().waitFor();
    assert.match(await securityAlert().innerText(), /密码/);
    assert.equal(await page.locator('#security-new-password').inputValue(), nextPassword);
    await page.locator('#security-current-password').fill(alpha.password);
    await page.locator('#security-confirm-password').fill('NotMatchingPassword');
    await securitySubmit().click(); await securityAlert().waitFor();
    assert.match(await securityAlert().innerText(), /一致|匹配/);
    assert.deepEqual(await financialState(), moneyBefore);
    await page.locator('#security-confirm-password').fill(nextPassword); await finishSecurity();
    assert.equal(await identityId(), alphaId); assert.deepEqual(await financialState(), moneyBefore);
  });
  await check('改密后旧密码拒绝，新密码支持用户名、新手机和已验证邮箱且账本权限不变', async () => {
    await login(alpha.username, alpha.password, { success: false });
    for (const identifier of [alpha.username, alphaNewPhone, alphaEmail]) {
      await login(identifier, nextPassword); await go();
      assert.equal(await identityId(), alphaId); assert.deepEqual(await financialState(), moneyBefore);
    }
    const text = await page.locator('main').innerText();
    assert.match(text, /密码|安全/);
    await page.reload(); await page.locator('#account-id').waitFor();
    assert.equal(await identityId(), alphaId); assert.deepEqual(await financialState(), moneyBefore);
    await capture('security-changes-completed');
  });

  await newPage();
  await check('已有余额和密钥的默认账号绑定邮箱后原账本、密钥及权限完整保留', async () => {
    await login('veyra_demo', 'securepass123'); await go();
    const before = await financialState(); const id = await identityId();
    await bindContact('email', 'existing-demo@example.test', 'securepass123');
    assert.equal(await identityId(), id);
    assert.deepEqual(await financialState(), before);
    await page.reload(); await page.locator('#account-id').waitFor();
    assert.equal(await identityId(), id); assert.deepEqual(await financialState(), before);
  });

  await check('账号改密后密钥验证同步使用新密码，旧演示密码不能继续解锁密钥', async () => {
    await go(); const before = await financialState();
    const changedPassword = 'ExistingDemoUpdated123';
    await openSecurity('password');
    await page.locator('#security-current-password').fill('securepass123');
    await page.locator('#security-new-password').fill(changedPassword);
    await page.locator('#security-confirm-password').fill(changedPassword);
    await finishSecurity();
    await go('/console/keys');
    await page.getByRole('button', { name: '验证查看', exact: true }).first().click();
    await page.locator('#key-auth-password').fill('securepass123');
    await page.getByRole('dialog').locator('button[type="submit"]').click();
    await page.getByRole('dialog').getByText(/密码错误|密码不正确/).waitFor();
    assert.ok(await page.locator('#key-auth-password').isVisible());
    assert.match(await page.locator('.xy-keys-list > div').first().innerText(), /验证查看/);
    await page.locator('#key-auth-password').fill(changedPassword);
    await page.getByRole('dialog').locator('button[type="submit"]').click();
    await page.locator('#key-auth-password').waitFor({ state: 'hidden' });
    assert.equal(await page.locator('.xy-keys-list > div').first().getByRole('button', { name: '验证查看', exact: true }).count(), 0);
    await page.locator('.xy-keys-list > div').first().getByRole('button', { name: /复制/ }).waitFor();
    assert.ok(await page.locator('.xy-keys-list > div').first().getByRole('button', { name: /复制/ }).isVisible());
    assert.deepEqual(await financialState(), before);
  });

  for (const width of [390, 320]) {
    await newPage({ width, height: width === 320 ? 568 : 844 });
    await check(`${width}像素账号中心及安全弹窗不溢出，提交取消和导航可操作`, async () => {
      await go(); await noOverflow(); await capture(`mobile-${width}`);
      await openSecurity('phone'); await noOverflow({ modal: true });
      await securitySubmit().scrollIntoViewIfNeeded();
      const box = await securitySubmit().boundingBox();
      assert.ok(box.y >= 0 && box.y + box.height <= page.viewportSize().height + 1);
      await capture(`mobile-${width}-security`); await closeSecurity();
      await page.locator('.xy-menu-toggle').click();
      await page.locator('.xy-sidebar-nav a[href="/console/settings"]').waitFor();
      assert.ok(await page.locator('.xy-sidebar-nav a[href="/console/settings"]').isVisible());
      await page.locator('.xy-mobile-close').click();
      await noOverflow();
    });
  }
  for (const language of ['en', 'ar']) {
    await newPage({ width: 390, height: 844 });
    await check(`${language === 'en' ? '英文' : '阿拉伯文从右到左'}深色账号中心及安全弹窗适配窄屏`, async () => {
      await go('/');
      // Only display preferences are set; account, credential, billing and role state are never injected.
      await page.evaluate(lang => { localStorage.setItem('xy-language', lang); localStorage.setItem('xy-theme', 'dark'); }, language);
      await go(); await noOverflow();
      assert.equal(await page.locator('html').getAttribute('dir'), language === 'ar' ? 'rtl' : 'ltr');
      assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
      await capture(`${language}-dark`);
      await openSecurity('email'); await noOverflow({ modal: true });
      await securitySubmit().scrollIntoViewIfNeeded(); await noOverflow({ modal: true });
      await capture(`${language}-dark-security`); await closeSecurity();
      await noOverflow();
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
