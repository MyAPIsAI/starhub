import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { mkdir, readFile, writeFile } from 'node:fs/promises';

// This suite requires the isolated TestP6BrowserServer fixture.
const base = process.env.STARHUB_AUTH_E2E_URL || 'http://127.0.0.1:39781';
const out = process.env.STARHUB_AUTH_EVIDENCE_DIR || '../output/public-auth-check';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const report = { checks: [], errors: [], screenshots: [] };

async function newPage(language = 'zh', theme = 'light', username) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addInitScript(({ language, theme }) => {
    localStorage.setItem('xy-language', language);
    localStorage.setItem('xy-theme', theme);
  }, { language, theme });
  if (username) {
    const response = await context.request.post(`${base}/api/user/login`, {
      data: { username, password: 'p6-password' },
    });
    assert.equal(response.status(), 200);
  }
  const page = await context.newPage();
  page.setDefaultTimeout(10000);
  page.on('pageerror', error => report.errors.push(String(error)));
  return page;
}

async function visit(page, route, status) {
  const self = page.waitForResponse(response => response.url().endsWith('/api/user/self'));
  await page.goto(`${base}${route}`);
  assert.equal((await self).status(), status);
  await page.locator('.veyra-public-header').waitFor();
}

async function signedInHeader(page, username = 'member') {
  const accounts = page.locator('.veyra-public-header a[href="/console/settings"]');
  await accounts.first().waitFor({ state: 'attached' });
  assert.equal(await accounts.count(), 2);
  assert.deepEqual(await accounts.allTextContents(), [username, username]);
  assert.equal(await page.locator('.veyra-public-header a[href="/login"]').count(), 0);
}

async function signIn(page) {
  await page.locator('#auth-account').fill('member');
  await page.locator('#auth-password').fill('p6-password');
  await page.locator('#auth-consent').check();
  await page.locator('.auth-form button[type="submit"]').click();
  await page.waitForURL(`${base}/console`);
  await page.locator('.xy-sidebar-nav a[href="/models"]').click();
  await signedInHeader(page);
}

async function noOverflow(page) {
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
}

async function capture(page, name) {
  await page.screenshot({ path: `${out}/${name}.png` });
  report.screenshots.push(`${name}.png`);
}

try {
  const page = await newPage();
  await visit(page, '/models', 401);
  await page.locator('[data-model-id="gpt-4o-mini"]').waitFor();
  assert.equal(new URL(page.url()).pathname, '/models');
  assert.equal(await page.locator('.veyra-public-actions a[href="/login"]').count(), 1);
  await visit(page, '/models/gpt-4o-mini', 401);
  await page.locator('.access-detail-heading').waitFor();
  assert.equal(new URL(page.url()).pathname, '/models/gpt-4o-mini');
  report.checks.push('Anonymous visitors can browse the plaza and model details without a login redirect');

  await page.locator('.veyra-public-login').click();
  await signIn(page);
  const self = await page.request.get(`${base}/api/user/self`);
  assert.equal(self.status(), 200);
  assert.equal((await self.json()).data.username, 'member');
  await capture(page, 'after-desktop');
  report.checks.push('Password login followed by the console plaza link preserves the real session and displays the account');

  if (process.env.STARHUB_AUTH_BASELINE) {
    const baseline = JSON.parse(await readFile(process.env.STARHUB_AUTH_BASELINE, 'utf8'));
    const current = await page.locator('.veyra-public-header').evaluate(node => {
      const style = getComputedStyle(node);
      const linkStyle = getComputedStyle(node.querySelector('.veyra-public-login'));
      return { height: node.getBoundingClientRect().height, background: style.backgroundColor,
        color: style.color, border: style.borderBottom, linkFont: linkStyle.font, linkColor: linkStyle.color };
    });
    assert.deepEqual(current, baseline);
    report.checks.push('Header height, colors, border and account typography match the original visual baseline');
  }

  for (const route of ['/models', '/models/gpt-4o-mini', '/', '/docs', '/status']) {
    await visit(page, route, 200);
    await signedInHeader(page);
    assert.equal(new URL(page.url()).pathname, route);
  }
  report.checks.push('Refresh and direct navigation restore the same account on every public page');
  // Reproduce the legacy optional-field representation without changing money or permissions.
  await page.evaluate(() => {
    const key = 'veyra-billing-demo-v1';
    const snapshot = JSON.parse(localStorage.getItem(key));
    snapshot.user.phone = '';
    localStorage.setItem(key, JSON.stringify(snapshot));
  });
  const secondTab = await page.context().newPage();
  await visit(secondTab, '/models', 200);
  await signedInHeader(secondTab);
  await secondTab.close();
  await signedInHeader(page);
  report.checks.push('A new tab restores the account and an older empty-phone snapshot without logging out the original tab');

  await page.locator('.veyra-public-actions a[href="/console/settings"]').focus();
  await page.keyboard.press('Enter');
  await page.waitForURL(`${base}/console/settings`);
  await page.getByRole('button', { name: '退出当前账号', exact: true }).click();
  await page.waitForURL(url => ['/', '/login'].includes(url.pathname));
  await visit(page, '/models', 401);
  assert.equal(await page.locator('.veyra-public-account').count(), 0);
  assert.equal(await page.locator('.veyra-public-login').count(), 1);
  report.checks.push('The keyboard account link works; sign-out revokes the session and restores the login link');

  for (const language of ['zh', 'en', 'ar']) {
    for (const theme of ['light', 'dark']) {
      const localized = await newPage(language, theme, 'member');
      await visit(localized, '/models', 200);
      await signedInHeader(localized);
      await noOverflow(localized);
      for (const width of [390, 320]) {
        await localized.setViewportSize({ width, height: 844 });
        await localized.locator('.veyra-public-menu').click();
        assert.ok(await localized.locator('.veyra-public-mobile-actions a[href="/console/settings"]').isVisible());
        await noOverflow(localized);
        if (width === 390) await capture(localized, `${language}-${theme}-mobile`);
        await localized.keyboard.press('Escape');
        assert.equal(await localized.locator('.veyra-public-menu').getAttribute('aria-expanded'), 'false');
        assert.ok(await localized.locator('.veyra-public-menu').evaluate(node => node === document.activeElement));
      }
      await localized.locator('.veyra-public-menu').click();
      await localized.locator('.veyra-public-mobile-actions a[href="/console/settings"]').click();
      await localized.waitForURL(`${base}/console/settings`);
      report.checks.push(`${language}/${theme}: desktop and 390/320px account navigation works without overflow`);
      await localized.context().close();
    }
  }

  const longName = `auth_header_${'a'.repeat(44)}`;
  const created = await page.request.post(`${base}/api/user/register`, {
    data: { username: longName, password: 'p6-password', email: '' },
  });
  assert.ok([200, 409].includes(created.status()));
  const longPage = await newPage('zh', 'light', longName);
  await visit(longPage, '/models', 200);
  await signedInHeader(longPage, longName);
  assert.ok(await longPage.locator('.veyra-public-actions .veyra-public-account').evaluate(node => node.scrollWidth > node.clientWidth));
  await noOverflow(longPage);
  await longPage.setViewportSize({ width: 320, height: 844 });
  await longPage.locator('.veyra-public-menu').click();
  await noOverflow(longPage);
  await capture(longPage, 'long-account-mobile');
  report.checks.push('Long account names truncate within the existing desktop and mobile layout');

  const delayed = await newPage('zh', 'light', 'member');
  let release;
  const gate = new Promise(resolve => { release = resolve; });
  await delayed.context().route('**/api/user/self', async route => {
    await gate;
    await route.continue().catch(() => {});
  });
  await delayed.goto(`${base}/models`);
  await delayed.locator('.veyra-public-actions [role="status"]').waitFor();
  assert.equal(await delayed.locator('.veyra-public-header a[href="/login"]').count(), 0);
  await capture(delayed, 'session-checking');
  release();
  await signedInHeader(delayed);
  report.checks.push('A delayed startup probe shows a neutral placeholder without flashing login');

  const retryPage = await newPage('zh', 'light', 'member');
  await retryPage.context().route('**/api/user/self', route => route.fulfill({
    status: 503, contentType: 'application/json', body: JSON.stringify({ success: false, message: 'Temporary test failure' }),
  }));
  await retryPage.goto(`${base}/models`);
  const retryLink = retryPage.locator('.veyra-public-actions a[aria-label="暂时无法确认登录状态，点击重试"]');
  await retryLink.waitFor();
  assert.equal(await retryPage.locator('.veyra-public-header a[href="/login"]').count(), 0);
  assert.equal(new URL(retryPage.url()).pathname, '/models');
  await retryPage.context().unroute('**/api/user/self');
  await retryLink.click();
  await signedInHeader(retryPage);
  report.checks.push('A transient server failure leaves the public page open and supports session retry');

  const consoleRetry = await newPage('zh', 'light', 'member');
  await consoleRetry.context().route('**/api/user/self', route => route.abort('failed'));
  await consoleRetry.goto(`${base}/console`);
  await consoleRetry.locator('.route-loading[role="alert"]').waitFor();
  assert.equal(new URL(consoleRetry.url()).pathname, '/console');
  await consoleRetry.context().unroute('**/api/user/self');
  await consoleRetry.locator('.route-loading button').click();
  await consoleRetry.locator('.xy-sidebar').waitFor();
  report.checks.push('A startup network failure keeps the protected route gated and allows retry');

  const expired = await newPage('zh', 'light', 'member');
  await expired.context().clearCookies();
  await expired.goto(`${base}/console`);
  await expired.waitForURL(`${base}/`);
  await expired.locator('.veyra-public-actions a[href="/login"]').waitFor();
  assert.equal(await expired.locator('.veyra-public-actions a[href="/login"]').count(), 1);
  report.checks.push('An expired session cannot open the console');

  for (const staleStatus of [200, 401]) {
    const racePage = await newPage();
    let finishProbe;
    const probeGate = new Promise(resolve => { finishProbe = resolve; });
    const completions = [];
    await racePage.context().route('**/api/user/self', async route => {
      const done = (async () => {
        await probeGate;
        await route.fulfill({ status: staleStatus, contentType: 'application/json', body: JSON.stringify(
          staleStatus === 200 ? { success: true, data: { id: 999, username: 'stale_operator', email: '', role: 100 } }
            : { success: false, message: 'login required' },
        ) }).catch(() => {});
      })();
      completions.push(done);
      await done;
    });
    await racePage.goto(`${base}/login`);
    await signIn(racePage);
    finishProbe();
    await Promise.all(completions);
    await signedInHeader(racePage);
    report.checks.push(`An older ${staleStatus} startup response cannot overwrite a successful interactive login`);
    await racePage.context().close();
  }

  await page.setViewportSize({ width: 390, height: 844 });
  await page.locator('.veyra-public-menu').click();
  await page.locator('.veyra-public-mobile-actions a[href="/login"]').click();
  await page.locator('.auth-dialog').waitFor();
  report.checks.push('The anonymous mobile navigation still opens password login');
  assert.deepEqual(report.errors, []);
  console.log(JSON.stringify(report, null, 2));
} catch (error) {
  report.failure = String(error);
  report.pages = [];
  for (const context of browser.contexts()) {
    for (const page of context.pages()) {
      report.pages.push({ url: page.url(), header: await page.locator('.veyra-public-header').innerText().catch(() => '') });
      await capture(page, `failure-${report.pages.length}`).catch(() => {});
    }
  }
  throw error;
} finally {
  await writeFile(`${out}/report.json`, JSON.stringify(report, null, 2));
  await browser.close();
}
