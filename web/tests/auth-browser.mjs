import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';

const base = process.env.VEYRA_URL || 'http://127.0.0.1:5196';
const out = process.env.VEYRA_EVIDENCE_DIR || 'docs/verification-2026-09-27';
const prefix = process.env.VEYRA_EVIDENCE_PREFIX || process.env.EVIDENCE_PREFIX || 'auth';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const report = {
  startedAt: new Date().toISOString(), base, independentBrowserContext: true,
  businessStateInjection: false, clockSimulation: 'Playwright clock advances only countdown and expiry cases',
  checks: [], errors: [], screenshots: [],
};
let page;
const contexts = [];
async function newPage({ width = 1440, height = 1000, clock = false } = {}) {
  const context = await browser.newContext({ viewport: { width, height } });
  contexts.push(context);
  page = await context.newPage();
  page.setDefaultTimeout(10000);
  page.on('pageerror', error => report.errors.push(String(error)));
  if (clock) await page.clock.install();
  return page;
}
const path = () => new URL(page.url()).pathname;
async function go(route = '/login') {
  await page.goto(`${base}${route}`);
  if (['/login', '/register', '/forgot-password'].includes(route.split('?')[0])) await page.locator('.auth-dialog').waitFor();
  else await page.locator('.xy-hero').waitFor();
}
async function capture(name) {
  const file = `${prefix}-${name}.png`;
  await page.locator('.auth-dialog, .auth-backdrop').evaluateAll(nodes => Promise.all(nodes.flatMap(node => node.getAnimations().map(animation => animation.finished.catch(() => {})))));
  await page.screenshot({ path: `${out}/${file}`, fullPage: false });
  report.screenshots.push(file);
}
async function check(name, fn) {
  try {
    await fn(); report.checks.push({ name, passed: true }); console.log('PASS', name);
  } catch (error) {
    report.checks.push({ name, passed: false, error: String(error) });
    console.log('FAIL', name, error.message);
    await capture(`failure-${report.checks.length}`).catch(() => {});
  }
}
const submit = () => page.locator('.auth-form button[type="submit"]');
async function consent() {
  await page.locator('#auth-consent').check();
}
async function fillDemo() {
  await page.getByRole('button', { name: '填入演示账号', exact: true }).click();
}
async function phoneMode() {
  await page.locator('.auth-methods').getByRole('button', { name: '手机号验证码', exact: true }).click();
}
async function ledger() {
  return page.evaluate(() => JSON.parse(localStorage.getItem('veyra-billing-demo-v1')));
}
async function financialState() {
  const state = await ledger();
  return { balance: state.balance, ledger: state.ledger, orders: state.rechargeOrders, keys: state.keys, requests: state.liveRequests, samples: state.sampleRequestsEnabled };
}
async function noOverflow() {
  const dimensions = await page.evaluate(() => {
    const dialog = document.querySelector('.auth-dialog');
    const body = document.querySelector('.auth-dialog-body');
    const box = dialog.getBoundingClientRect();
    return {
      width: innerWidth, height: innerHeight, pageWidth: document.documentElement.scrollWidth,
      x: box.x, right: box.right, y: box.y, bottom: box.bottom,
      dialogWidth: dialog.clientWidth, dialogScrollWidth: dialog.scrollWidth,
      bodyWidth: body.clientWidth, bodyScrollWidth: body.scrollWidth,
    };
  });
  assert.ok(dimensions.pageWidth <= dimensions.width + 1, `page overflow: ${JSON.stringify(dimensions)}`);
  assert.ok(dimensions.x >= -1 && dimensions.right <= dimensions.width + 1, `dialog horizontal overflow: ${JSON.stringify(dimensions)}`);
  assert.ok(dimensions.y >= -1 && dimensions.bottom <= dimensions.height + 1, `dialog vertical overflow: ${JSON.stringify(dimensions)}`);
  assert.ok(dimensions.bodyScrollWidth <= dimensions.bodyWidth + 1, `dialog content overflow: ${JSON.stringify(dimensions)}`);
}
async function close() {
  await page.locator('.auth-close').click();
  await page.locator('.auth-dialog').waitFor({ state: 'hidden' });
  assert.equal(path(), '/');
}

try {
  await newPage();
  await check('首页登录在真实首页之上打开半透明遮罩，背景不可交互', async () => {
    await go('/');
    const heading = await page.locator('.xy-hero h1').innerText();
    await page.locator('.veyra-public-login').click();
    await page.locator('.auth-dialog').waitFor();
    assert.equal(path(), '/login');
    assert.equal(await page.locator('.xy-hero h1').innerText(), heading);
    assert.equal(await page.locator('.auth-dialog').count(), 1);
    const background = await page.locator('.auth-backdrop').evaluate(node => getComputedStyle(node).backgroundColor);
    assert.match(background, /rgba\(.+, 0\.\d+\)/);
    assert.equal(await page.locator('.xy-hero').evaluate(node => Boolean(node.closest('[aria-hidden="true"]'))), true);
    assert.ok((await page.locator('.auth-dialog-footer').innerText()).includes('本地演示'));
    await noOverflow(); await capture('login-desktop');
  });
  await check('登录关闭返回主页及登录入口焦点；后退、前进、直接刷新均保持合理状态', async () => {
    await close();
    await page.waitForFunction(() => document.activeElement?.classList.contains('veyra-public-login'));
    await page.locator('.veyra-public-login').click();
    await page.locator('.auth-dialog').waitFor();
    await page.goBack();
    await page.locator('.auth-dialog').waitFor({ state: 'hidden' });
    assert.equal(path(), '/');
    await page.goForward();
    await page.locator('.auth-dialog').waitFor(); assert.equal(path(), '/login');
    await page.reload(); await page.locator('.auth-dialog').waitFor();
    assert.ok(await page.locator('.xy-hero').isVisible());
    await page.keyboard.press('Escape'); await page.locator('.auth-dialog').waitFor({ state: 'hidden' });
    assert.equal(path(), '/');
  });
  await check('协议区域只有一个默认未选的勾选框，正确账号不能绕过；失败保留输入及账本', async () => {
    await go('/login'); const before = await financialState();
    assert.equal(await page.locator('.auth-consent input[type="checkbox"]').count(), 1);
    assert.equal(await page.locator('#auth-consent').isChecked(), false);
    await fillDemo(); await submit().click();
    await page.getByRole('alert').waitFor();
    assert.ok((await page.getByRole('alert').innerText()).includes('请阅读并勾选'));
    assert.equal(path(), '/login');
    assert.equal(await page.locator('#auth-account').inputValue(), 'veyra_demo');
    assert.equal((await page.locator('#auth-password').inputValue()).length, 13);
    assert.deepEqual(await financialState(), before);
  });
  await check('两份协议独立阅读不改变单一勾选，返回保留输入和焦点，Escape先退协议再关登录', async () => {
    const passwordLength = (await page.locator('#auth-password').inputValue()).length;
    for (const checked of [false, true]) {
      if (checked) await consent();
      for (const policy of ['terms', 'privacy']) {
        await page.locator(`[data-auth-policy="${policy}"]`).click();
        await page.locator('.auth-policy').waitFor();
        assert.equal(new URL(page.url()).searchParams.get('document'), policy);
        if (policy === 'privacy') {
          assert.ok((await page.locator('.auth-policy').innerText()).includes('明文密码不会持久保存'));
          await page.keyboard.press('Escape');
        } else {
          await page.getByRole('button', { name: '返回继续填写', exact: true }).click();
        }
        await page.locator('.auth-form').waitFor();
        assert.equal(path(), '/login');
        assert.equal(await page.locator('.auth-consent input[type="checkbox"]').count(), 1);
        assert.equal(await page.locator('#auth-consent').isChecked(), checked, '阅读链接不得自动勾选或取消勾选');
        assert.equal((await page.locator('#auth-password').inputValue()).length, passwordLength);
        assert.equal(await page.locator('#auth-account').inputValue(), 'veyra_demo');
        await page.waitForFunction(value => document.activeElement?.getAttribute('data-auth-policy') === value, policy);
      }
    }
    for (let index = 0; index < 22; index++) {
      await page.keyboard.press('Tab');
      assert.ok(await page.evaluate(() => Boolean(document.activeElement?.closest('.auth-dialog'))));
    }
    await page.keyboard.press('Escape'); await page.locator('.auth-dialog').waitFor({ state: 'hidden' });
  });
  await check('账号密码真正校验：错误保留输入和账本，改正确后进入工作台', async () => {
    await go('/login'); const before = await financialState();
    await fillDemo(); await consent(); await page.locator('#auth-password').fill('IncorrectDemo123');
    await submit().click(); await page.getByRole('alert').waitFor();
    assert.ok((await page.getByRole('alert').innerText()).includes('账号或密码不正确'));
    assert.equal(path(), '/login');
    assert.equal((await page.locator('#auth-password').inputValue()).length, 16);
    assert.deepEqual(await financialState(), before);
    await fillDemo(); await submit().click(); await page.waitForURL('**/console');
    assert.equal((await ledger()).user.username, 'veyra_demo');
    assert.equal((await ledger()).user.phone.replace(/^\+86/, ''), '13800138000');
    assert.equal((await ledger()).user.isLoggedIn, true);
    assert.deepEqual(await financialState(), before);
  });

  await newPage({ clock: true });
  await check('手机号校验、验证码必须先获取；获取仅显示模拟码且不自动填写', async () => {
    await go('/login'); await phoneMode(); await consent();
    await page.locator('#auth-phone').fill('123');
    await page.getByRole('button', { name: '获取验证码', exact: true }).click();
    assert.ok((await page.getByRole('alert').innerText()).includes('有效手机号'));
    await page.getByRole('button', { name: '填入演示手机号', exact: true }).click();
    await page.locator('#auth-code').fill('829104'); await submit().click();
    assert.ok((await page.getByRole('alert').innerText()).includes('请先'));
    await page.locator('#auth-code').fill('');
    await page.getByRole('button', { name: '获取验证码', exact: true }).click();
    assert.equal(await page.locator('#auth-code').inputValue(), '');
    assert.ok((await page.locator('.auth-code-hint').innerText()).includes('未发送真实消息'));
    assert.equal(await page.locator('.auth-code-input button').isDisabled(), true);
    await capture('phone-desktop');
  });
  await check('倒计时刷新后仍有效，切换号码不能沿用旧码，错误验证码可修复', async () => {
    await page.reload(); await page.locator('.auth-dialog').waitFor();
    assert.equal(new URL(page.url()).searchParams.get('method'), 'phone');
    assert.ok(await page.locator('#auth-phone').isVisible());
    assert.equal(await page.locator('.auth-code-input button').isDisabled(), true);
    await consent(); await page.locator('#auth-phone').fill('13900139000');
    await page.locator('#auth-code').fill('829104'); await submit().click();
    assert.equal(path(), '/login');
    assert.ok((await page.getByRole('alert').innerText()).includes('未找到'));
    await page.locator('#auth-phone').fill('13800138000');
    assert.equal(await page.locator('#auth-code').inputValue(), '');
    await page.locator('#auth-code').fill('000000'); await submit().click();
    assert.ok((await page.getByRole('alert').innerText()).includes('验证码不正确'));
    assert.equal(await page.locator('#auth-phone').inputValue(), '13800138000');
    assert.equal(await page.locator('#auth-code').inputValue(), '000000');
  });
  await check('验证码连续五次错误明确用尽，正确码也不可继续；冷却后重新获取恢复', async () => {
    for (let index = 0; index < 4; index++) {
      await submit().click();
      await page.locator('.auth-error').waitFor();
      await page.waitForFunction(() => document.querySelector('.auth-form button[type="submit"]')?.getAttribute('aria-busy') !== 'true');
    }
    assert.match(await page.getByRole('alert').innerText(), /连续输错 5 次/);
    await page.locator('#auth-code').fill('829104'); await submit().click();
    assert.match(await page.getByRole('alert').innerText(), /连续输错 5 次/); assert.equal(path(), '/login');
    await page.clock.fastForward(61000);
    await page.getByRole('button', { name: '获取验证码', exact: true }).click();
    assert.equal(await page.getByRole('alert').count(), 0);
  });
  await check('验证码到期明确报错，重新获取后正确手机号能登录', async () => {
    const before = await financialState();
    await page.clock.fastForward(601000);
    await page.locator('#auth-code').fill('829104'); await submit().click();
    assert.ok((await page.getByRole('alert').innerText()).includes('过期'));
    assert.equal(path(), '/login'); assert.deepEqual(await financialState(), before);
    await page.getByRole('button', { name: '获取验证码', exact: true }).click();
    await submit().click(); await page.waitForURL('**/console');
    assert.equal((await ledger()).user.username, 'veyra_demo');
    assert.equal((await ledger()).user.phone.replace(/^\+86/, ''), '13800138000');
    assert.deepEqual(await financialState(), before);
  });

  await newPage({ clock: true });
  const registeredUsername = 'fictional_user';
  const registeredPhone = '13900139000';
  async function assertRegisteredIdentity() {
    const state = await ledger();
    assert.equal(state.user.username, registeredUsername);
    assert.equal(state.user.phone.replace(/^\+86/, ''), registeredPhone);
    assert.equal(state.user.email, '');
    return state;
  }
  async function fillRegistration({ username = registeredUsername, phone = registeredPhone, confirm = 'FictionalRegister123' } = {}) {
    await page.locator('#auth-username').fill(username);
    await page.locator('#auth-register-phone').fill(phone);
    await page.locator('#auth-new-password').fill('FictionalRegister123');
    await page.locator('#auth-confirm-password').fill(confirm);
  }
  await check('注册仅需用户名手机号验证码和两次密码；确认不一致保留表单且不创建账户', async () => {
    await go('/register'); const before = await financialState();
    assert.equal(await page.locator('.auth-consent input[type="checkbox"]').count(), 1);
    assert.equal(await page.locator('#auth-consent').isChecked(), false);
    assert.equal(await page.locator('#auth-company, #auth-email').count(), 0);
    for (const selector of ['#auth-username', '#auth-register-phone', '#auth-code', '#auth-new-password', '#auth-confirm-password']) {
      assert.equal(await page.locator(selector).getAttribute('required'), '');
    }
    await fillRegistration({ confirm: 'FictionalMismatch456' });
    await page.getByRole('button', { name: '获取验证码', exact: true }).click();
    assert.equal(await page.locator('#auth-code').inputValue(), '');
    await page.locator('#auth-code').fill('829104');
    await consent(); await page.locator('.auth-human input').check(); await submit().click();
    await page.getByRole('alert').waitFor();
    assert.match(await page.getByRole('alert').innerText(), /一致|匹配/);
    assert.equal(path(), '/register');
    assert.equal(await page.locator('#auth-username').inputValue(), registeredUsername);
    assert.equal(await page.locator('#auth-register-phone').inputValue(), registeredPhone);
    assert.equal((await page.locator('#auth-new-password').inputValue()).length, 20);
    assert.equal((await page.locator('#auth-confirm-password').inputValue()).length, 20);
    assert.deepEqual(await financialState(), before);
    const savedAuth = await page.evaluate(() => sessionStorage.getItem('veyra.demo-auth.v2'));
    assert.ok(!savedAuth.includes(registeredUsername), '确认密码错误不能创建凭据');
  });
  await check('修复确认密码后可注册，保留模拟人机验证，创建零余额普通账户', async () => {
    await page.locator('#auth-confirm-password').fill('FictionalRegister123');
    await page.locator('.auth-human input').uncheck(); await submit().click();
    assert.ok((await page.getByRole('alert').innerText()).includes('人机验证'));
    await page.locator('.auth-human input').check();
    assert.equal(await page.getByRole('alert').count(), 0, '完成人机验证后应立即清除已修复的错误提示');
    await capture('register-desktop');
    await submit().click(); await page.waitForURL('**/console/connect');
    const state = await assertRegisteredIdentity();
    assert.equal(state.balance, '0'); assert.deepEqual(state.keys, []);
    assert.deepEqual(state.liveRequests, []); assert.equal(state.sampleRequestsEnabled, false);
    assert.deepEqual(state.ledger, []); assert.deepEqual(state.rechargeOrders, []);
    assert.equal(state.scenario, 'first');
    assert.equal(await page.evaluate(() => localStorage.getItem('veyra-demo-account-role-v1')), 'customer');
    assert.equal(await page.locator('a[href="/console/model-pricing"]').count(), 0);
    const savedAuth = await page.evaluate(() => sessionStorage.getItem('veyra.demo-auth.v2'));
    assert.ok(!savedAuth.includes('FictionalRegister123'));
    assert.ok(!savedAuth.includes('FictionalMismatch456'));
    assert.ok(savedAuth.includes('digest'));
  });
  await check('注册账号刷新后用户名密码可登录，账户资料显示用户名手机号且保持零余额', async () => {
    await page.reload(); await assertRegisteredIdentity(); await go('/login');
    assert.equal(await page.locator('#auth-account').getAttribute('type'), 'text');
    await page.locator('#auth-account').fill(registeredUsername);
    await page.locator('#auth-password').fill('FictionalRegister123');
    await consent(); await submit().click(); await page.waitForURL('**/console');
    const state = await assertRegisteredIdentity();
    assert.equal(state.balance, '0'); assert.deepEqual(state.keys, []);
    await page.goto(`${base}/console/settings`); await page.locator('main').waitFor();
    assert.equal((await page.locator('main h1').innerText()).includes('企业'), false, '个人账号设置标题不得沿用企业称谓');
    const settingsLabels = await page.locator('.xy-sidebar-nav a[href="/console/settings"]').allTextContents();
    assert.equal(settingsLabels.length, 1);
    assert.equal(settingsLabels[0].includes('企业'), false, '个人账号侧栏不得沿用企业称谓');
    const deactivate = page.locator('main').getByRole('button', { name: '申请注销', exact: true });
    assert.ok(await deactivate.isVisible());
    assert.equal(await page.locator('main').getByRole('button', { name: /注销企业/ }).count(), 0);
    assert.equal(await page.locator('#account-username').inputValue(), registeredUsername);
    const maskedPhone = await page.locator('#account-phone').inputValue();
    assert.match(maskedPhone, /139 \*{4} 9000$/);
    assert.ok(!maskedPhone.includes(registeredPhone), '账户资料应脱敏展示手机号');
    await page.reload(); await page.locator('#account-username').waitFor();
    assert.equal(await page.locator('#account-username').inputValue(), registeredUsername);
    assert.equal(await page.locator('#account-phone').inputValue(), maskedPhone);
    await capture('registered-account-profile');
  });
  await check('新注册手机号也可搭配同一密码登录，不额外创建账户', async () => {
    await go('/login'); const before = await financialState();
    await page.locator('#auth-account').fill(registeredPhone);
    await page.locator('#auth-password').fill('FictionalRegister123');
    await consent(); await submit().click(); await page.waitForURL('**/console');
    await assertRegisteredIdentity(); assert.deepEqual(await financialState(), before);
  });
  await check('重复用户名和重复手机号分别明确拒绝，失败不更改当前账户或账本', async () => {
    await go('/register'); const before = await financialState();
    await page.clock.fastForward(61000);
    await fillRegistration({ phone: '13700137000' });
    await page.getByRole('button', { name: '获取验证码', exact: true }).click();
    await page.locator('#auth-code').fill('829104'); await consent(); await page.locator('.auth-human input').check();
    await submit().click(); await page.getByRole('alert').waitFor();
    assert.match(await page.getByRole('alert').innerText(), /用户名/);
    assert.match(await page.getByRole('alert').innerText(), /已|存在|使用|重复/);
    assert.equal(path(), '/register'); assert.equal(await page.locator('#auth-username').inputValue(), registeredUsername);
    assert.deepEqual(await financialState(), before); await assertRegisteredIdentity();
    await page.locator('#auth-username').fill('fictional_other');
    await page.locator('#auth-register-phone').fill(registeredPhone);
    assert.equal(await page.locator('#auth-code').inputValue(), '');
    await page.clock.fastForward(61000);
    await page.getByRole('button', { name: '获取验证码', exact: true }).click(); await page.getByRole('alert').waitFor();
    assert.match(await page.getByRole('alert').innerText(), /手机号/);
    assert.match(await page.getByRole('alert').innerText(), /已|存在|使用|重复/);
    assert.equal(path(), '/register'); assert.deepEqual(await financialState(), before); await assertRegisteredIdentity();
  });
  await check('新注册手机号可获取验证码登录，身份与用户名密码登录一致', async () => {
    await go('/login?method=phone'); const before = await financialState();
    await page.clock.fastForward(61000);
    await page.locator('#auth-phone').fill(registeredPhone); await consent();
    await page.getByRole('button', { name: '获取验证码', exact: true }).click();
    assert.equal(await page.locator('#auth-code').inputValue(), '');
    await page.locator('#auth-code').fill('829104'); await submit().click(); await page.waitForURL('**/console');
    await assertRegisteredIdentity(); assert.deepEqual(await financialState(), before);
  });
  await check('手机号验证重设密码真正更新凭据；旧密码拒绝，新密码支持用户名与手机号', async () => {
    await go('/forgot-password'); await page.clock.fastForward(61000);
    assert.equal(await page.locator('#auth-reset-email').count(), 0);
    await page.locator('#auth-reset-phone').fill(registeredPhone);
    await page.getByRole('button', { name: '获取验证码', exact: true }).click();
    await page.locator('#auth-code').fill('829104');
    await page.locator('#auth-reset-password').fill('FictionalReset456');
    await submit().click(); await page.locator('.auth-reset-success').waitFor();
    assert.ok((await page.locator('.auth-reset-success').innerText()).includes('演示密码已更新'));
    await capture('reset-success');
    await close(); await page.locator('.veyra-public-login').click();
    await page.locator('.auth-form').waitFor();
    assert.equal(await page.locator('.auth-reset-success').count(), 0);
    assert.equal(await page.locator('#auth-password').inputValue(), '');
    await page.locator('#auth-account').fill(registeredUsername);
    await page.locator('#auth-password').fill('FictionalRegister123'); await consent(); await submit().click();
    assert.ok((await page.getByRole('alert').innerText()).includes('账号或密码不正确'));
    await page.locator('#auth-password').fill('FictionalReset456'); await submit().click();
    await page.waitForURL('**/console'); await assertRegisteredIdentity();
    await go('/login'); await page.locator('#auth-account').fill(registeredPhone);
    await page.locator('#auth-password').fill('FictionalReset456'); await consent(); await submit().click();
    await page.waitForURL('**/console'); assert.equal((await assertRegisteredIdentity()).balance, '0');
  });

  await newPage({ clock: true });
  await check('密码五次失败产生真实倒计时；刷新不能绕过，手机号可作为独立恢复方式', async () => {
    await go('/login'); await fillDemo(); await consent(); await page.locator('#auth-password').fill('IncorrectDemo123');
    for (let index = 0; index < 5; index++) {
      await submit().click();
      await page.locator('.auth-error').waitFor();
      await page.waitForFunction(() => document.querySelector('.auth-form button[type="submit"]')?.getAttribute('aria-busy') !== 'true');
    }
    assert.equal(await submit().isDisabled(), true);
    assert.ok((await page.locator('.auth-lock').innerText()).includes('密码登录锁定还剩'));
    await page.reload(); await page.locator('.auth-dialog').waitFor();
    assert.equal(await submit().isDisabled(), true);
    await phoneMode(); await consent();
    await page.getByRole('button', { name: '填入演示手机号', exact: true }).click();
    assert.equal(await page.locator('.auth-code-input button').isDisabled(), false);
    await page.getByRole('button', { name: '获取验证码', exact: true }).click();
    await page.locator('#auth-code').fill('829104'); await submit().click(); await page.waitForURL('**/console');
  });

  for (const width of [390, 320]) {
    await newPage({ width, height: width === 320 ? 568 : 844 });
    await check(`${width}像素手机注册弹窗内容适配高度，必要时内部滚动，提交关闭可达`, async () => {
      await go('/register'); await noOverflow();
      const scrolling = await page.locator('.auth-dialog-body').evaluate(node => ({ scroll: node.scrollHeight, client: node.clientHeight, overflow: getComputedStyle(node).overflowY }));
      assert.equal(scrolling.overflow, 'auto');
      assert.ok(scrolling.scroll >= scrolling.client);
      if (width === 320) assert.ok(scrolling.scroll > scrolling.client, '小高度屏幕的超高表单应在弹窗内滚动');
      await submit().scrollIntoViewIfNeeded();
      const button = await submit().boundingBox();
      assert.ok(button.y >= 0 && button.y + button.height <= (width === 320 ? 568 : 844));
      await noOverflow(); await capture(`register-mobile-${width}`);
      await page.locator('.auth-close').click(); await page.locator('.auth-dialog').waitFor({ state: 'hidden' });
      assert.equal(path(), '/');
      await go('/login'); await phoneMode(); await noOverflow();
      await page.getByRole('button', { name: '填入演示手机号', exact: true }).click();
      await page.getByRole('button', { name: '获取验证码', exact: true }).click();
      await noOverflow(); await capture(`phone-mobile-${width}`); await close();
      await page.waitForFunction(() => document.activeElement?.classList.contains('veyra-public-menu'));
    });
  }

  for (const language of ['en', 'ar']) {
    await newPage({ width: 390, height: 844 });
    await check(`${language === 'en' ? '英文' : '阿拉伯文从右到左'}深色登录注册在窄屏完整可操作`, async () => {
      await go('/');
      // Only presentation preferences are written; authentication and business state are never injected.
      await page.evaluate(lang => { localStorage.setItem('xy-language', lang); localStorage.setItem('xy-theme', 'dark'); }, language);
      await go('/login');
      assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
      assert.equal(await page.locator('html').getAttribute('dir'), language === 'ar' ? 'rtl' : 'ltr');
      await noOverflow(); await capture(`${language}-dark-login`);
      await page.locator('.auth-methods button').nth(1).click(); await noOverflow();
      await page.locator('.auth-switch button').click(); assert.equal(path(), '/register');
      await noOverflow(); await submit().scrollIntoViewIfNeeded();
      await noOverflow(); await capture(`${language}-dark-register`);
      await page.locator('.auth-consent button').last().click(); await page.locator('.auth-policy').waitFor(); await noOverflow();
      await page.keyboard.press('Escape'); await page.locator('.auth-form').waitFor(); assert.equal(path(), '/register');
      assert.equal(await page.locator('.auth-form').count(), 1);
      await close();
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
