import { test, expect, type Page } from "@playwright/test";

async function login(page: Page, username = "root", password = "123456") {
  await page.goto("/login");
  await page.getByLabel("Language").selectOption("en-US");
  await page.getByLabel("Username", { exact: true }).fill(username);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page).toHaveURL(/\/console\/overview$/);
}

test.beforeEach(async ({ context }) => {
  await context.addInitScript(() =>
    localStorage.setItem("starhub.locale", "en-US"),
  );
});

test("all console routes load from the embedded Go server, including reloads", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await login(page);
  await expect(
    page.getByText("Daily collected usage", { exact: true }),
  ).toBeVisible();
  for (const section of [
    "tokens",
    "logs",
    "profile",
    "channels",
    "users",
    "options",
    "redemptions",
    "audit",
  ]) {
    await page.goto(`/console/${section}`);
    await expect(page.locator("h1")).toBeVisible();
    await expect(
      page.getByText("Unable to load data", { exact: true }),
    ).toHaveCount(0);
  }
  await page.goto("/console/logs");
  await expect(page.locator("tbody tr")).toHaveCount(20);
  await expect(page.locator("tbody tr").first()).toContainText("$0.000002");
  await expect(page.locator("tbody tr").first()).toContainText("$0.000004");
  await page.getByRole("button", { name: "Next", exact: true }).click();
  await expect(page.locator("tbody tr")).toHaveCount(4);
  await page.getByLabel("Exact model name").fill("missing-model");
  await expect(
    page.getByText("No matching records", { exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(page.getByLabel("Exact model name")).toHaveValue(
    "missing-model",
  );
  const response = await page.request.get("/api/does-not-exist");
  expect(response.status()).toBe(404);
  expect(await response.text()).not.toContain("<html");
  expect((await page.request.get("/console/missing.js")).status()).toBe(404);
  expect(errors).toEqual([]);
});

test("token creation exposes the key once and editing clears restrictions", async ({
  page,
}) => {
  await login(page);
  await page.goto("/console/tokens");
  await page
    .getByRole("button", { name: "Create", exact: true })
    .first()
    .click();
  const form = page.getByRole("dialog");
  await form
    .getByRole("textbox", { name: "Name", exact: true })
    .fill("browser-zero-token");
  await form.getByLabel("Token balance", { exact: true }).fill("0");
  await form.getByText("Advanced restrictions", { exact: true }).click();
  await form.getByLabel("Allowed models", { exact: true }).fill("gpt-4o-mini");
  await form.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Save your new API key" }),
  ).toBeVisible();
  const secret = await page.locator("pre.secret").innerText();
  expect(secret).toMatch(/^sk-/);
  expect(secret).not.toMatch(/^sk-sk-/);
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Close", exact: true })
    .first()
    .click();
  await expect(page.locator("body")).not.toContainText(secret);
  await page
    .getByRole("button", { name: "Edit browser-zero-token", exact: true })
    .click();
  await form.getByText("Advanced restrictions", { exact: true }).click();
  await form.getByLabel("Allowed models", { exact: true }).fill("");
  await form.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(form).toHaveCount(0);
  const result = await (await page.request.get("/api/token/")).json();
  const token = result.data.find(
    (row: { name: string }) => row.name === "browser-zero-token",
  );
  expect(token.remain_quota).toBe(0);
  expect(token.model_limits).toBe("");
  expect(token.key).not.toBe(secret);
  const storage = await page.evaluate(() =>
    JSON.stringify({
      local: { ...localStorage },
      session: { ...sessionStorage },
    }),
  );
  expect(storage).not.toContain(secret);
  expect(storage).not.toContain("password");
  await page.reload();
  await expect(page.locator("body")).not.toContainText(secret);
});

test("root balance edits reject stale values and operator accounts are read only", async ({
  page,
}) => {
  await login(page);
  await page.goto("/console/users");
  await page.getByRole("button", { name: "Edit member", exact: true }).click();
  const form = page.getByRole("dialog");
  await form.getByLabel("Quota", { exact: true }).fill("500");
  await expect(
    form.getByRole("button", { name: "Save changes" }),
  ).toBeDisabled();
  await form
    .getByLabel("I confirm setting the current balance to this amount.")
    .check();
  const result = await (await page.request.get("/api/user/")).json();
  const member = result.data.find(
    (row: { username: string }) => row.username === "member",
  );
  const concurrent = await page.request.put("/api/user/", {
    data: {
      id: member.id,
      quota: member.quota + 1,
      expected_quota: member.quota,
    },
  });
  expect(concurrent.status()).toBe(200);
  await form.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(form.getByRole("alert")).toContainText("Balance changed");
  await form.getByRole("button", { name: "Cancel", exact: true }).click();
  await page
    .getByRole("button", { name: "Discard changes", exact: true })
    .click();
  await page.getByLabel("Sign out", { exact: true }).click();
  await login(page, "operator", "p6-password");
  await page.goto("/console/users");
  await expect(page.locator("tbody tr")).toHaveCount(3);
  await expect(page.getByRole("button", { name: /^Edit / })).toHaveCount(0);
  expect(
    (
      await page.request.put("/api/user/", {
        data: { id: member.id, quota: 50, expected_quota: 1 },
      })
    ).status(),
  ).toBe(403);
});

test("channel probes show success and failure; settings validate and audit records actors", async ({
  page,
}) => {
  await login(page);
  await page.goto("/console/channels");
  await page
    .getByRole("button", { name: "Test channel Local provider", exact: true })
    .click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Test channel", exact: true })
    .click();
  await expect(page.getByText(/Connection successful/)).toBeVisible();
  await page
    .getByRole("button", { name: "Test channel Failing provider", exact: true })
    .click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Test channel", exact: true })
    .click();
  await expect(page.getByText(/Connection failed/)).toBeVisible();
  await page.goto("/console/options");
  await page.getByRole("tab", { name: "Groups", exact: true }).click();
  const card = page.locator(".option-card").filter({
    has: page.getByRole("heading", { name: "Group ratios", exact: true }),
  });
  await card.getByLabel("Group ratios", { exact: true }).fill('{"default":-1}');
  await card.getByRole("button", { name: "Save changes" }).click();
  await expect(card.getByRole("alert")).toBeVisible();
  await card
    .getByLabel("Group ratios", { exact: true })
    .fill('{"default":1,"browser":2}');
  await card.getByRole("button", { name: "Save changes" }).click();
  await expect(
    card.getByRole("button", { name: "Save changes" }),
  ).toBeDisabled();
  await page.goto("/console/audit");
  await expect(page.locator("tbody")).toContainText("root #1");
  await expect(page.locator("tbody")).toContainText("/api/option");
});

test("redemption credits a member and logout propagates across tabs", async ({
  page,
  context,
}) => {
  await login(page);
  await page.goto("/console/redemptions");
  await page
    .getByRole("button", { name: "Create", exact: true })
    .first()
    .click();
  await page
    .getByRole("dialog")
    .getByRole("textbox", { name: "Quota", exact: true })
    .fill("7");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Save changes" })
    .click();
  const code = await page.locator("pre.secret").innerText();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Close", exact: true })
    .first()
    .click();
  await page.getByLabel("Sign out", { exact: true }).click();
  await login(page, "member", "p6-password");
  await expect(
    page
      .locator(".sidebar")
      .getByRole("link", { name: "Channels", exact: true }),
  ).toHaveCount(0);
  await page.goto("/console/channels");
  await expect(
    page.getByRole("heading", { name: "Access restricted", exact: true }),
  ).toBeVisible();
  await page.goto("/console/profile");
  await page.getByLabel("Code", { exact: true }).fill(code);
  await page.getByRole("button", { name: "Redeem code", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("$0.000014");
  await page.getByLabel("Code", { exact: true }).fill(code);
  await page.getByRole("button", { name: "Redeem code", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("already used");
  const other = await context.newPage();
  await other.goto("/console/tokens");
  await expect(
    other.getByRole("heading", { name: "API tokens", exact: true }),
  ).toBeVisible();
  await page.getByLabel("Sign out", { exact: true }).click();
  await expect(other).toHaveURL(/\/login/);
  await expect(other.locator(".sidebar")).toHaveCount(0);
});

test("mobile navigation, dark mode, language switching, and keyboard dialogs work", async ({
  page,
}) => {
  await login(page);
  await page.getByLabel("Appearance").selectOption("dark");
  await expect(page.locator("html")).toHaveClass(/dark/);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByLabel("Open navigation").click();
  await page
    .getByRole("dialog")
    .getByRole("link", { name: "API tokens", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "API tokens", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Create", exact: true })
    .first()
    .click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByLabel("Language").selectOption("zh-CN");
  await expect(page.locator("html")).toHaveAttribute("lang", "zh-CN");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "test-results/console-mobile-dark.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/console/overview");
  await page.locator(".dashboard-chart svg").waitFor();
  await page.screenshot({
    path: "test-results/console-overview-dark.png",
    fullPage: true,
  });
});

test("session bootstrap network failure offers retry instead of a false logout", async ({
  page,
}) => {
  await login(page);
  await page.route("**/api/user/self", (route) => route.abort());
  await page.reload();
  await expect(
    page.getByText("Unable to verify your session", { exact: true }),
  ).toBeVisible();
  await page.unroute("**/api/user/self");
  await page.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Your gateway, at a glance" }),
  ).toBeVisible();
});

test("registration requires a subsequent login and expired sessions clear cached pages", async ({
  page,
  context,
}) => {
  await page.goto("/register");
  await page.getByLabel("Username", { exact: true }).fill("browser-new-member");
  await page.getByLabel("Password", { exact: true }).fill("p6-password");
  await page
    .getByRole("button", { name: "Create account", exact: true })
    .click();
  await expect(page).toHaveURL(/\/login\?/);
  expect(
    (await context.cookies()).find((cookie) => cookie.name === "session"),
  ).toBeUndefined();
  await login(page, "browser-new-member", "p6-password");
  await expect(
    page.getByText("Your balance is empty", { exact: false }),
  ).toBeVisible();
  await page.goto("/console/tokens");
  await expect(
    page.getByText("Nothing here yet", { exact: true }),
  ).toBeVisible();
  await expect(page.locator("body")).not.toContainText("browser-zero-token");
  await context.clearCookies();
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page).toHaveURL(/\/login/);
  await expect(page.locator(".sidebar")).toHaveCount(0);
});

test("large local lists remain paged and layouts work at tablet and enlarged scale", async ({
  page,
}) => {
  await login(page);
  await page.route("**/api/token", (route) =>
    route.fulfill({
      json: {
        success: true,
        data: Array.from({ length: 1000 }, (_, index) => ({
          id: index + 1,
          user_id: 1,
          name: `bulk-token-${String(index + 1).padStart(4, "0")}`,
          key: "sk-***",
          remain_quota: 1,
          used_quota: 0,
          status: 1,
          group: "",
          expired_time: -1,
          created_at: 1790000000,
        })),
      },
    }),
  );
  await page.goto("/console/tokens");
  await expect(page.locator("tbody tr")).toHaveCount(20);
  await page.getByLabel("Rows per page").selectOption("100");
  await expect(page.locator("tbody tr")).toHaveCount(100);
  await page.getByRole("button", { name: "Next", exact: true }).click();
  await expect(page.locator("tbody tr").first()).toContainText(
    "bulk-token-0900",
  );
  await page.getByLabel("Filter by name…").fill("bulk-token-0001");
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await page.setViewportSize({ width: 768, height: 1024 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.getByLabel("Appearance").selectOption("light");
  await page.screenshot({
    path: "test-results/console-desktop-light.png",
    fullPage: true,
  });
  await page.evaluate(() => {
    document.documentElement.style.zoom = "2";
  });
  expect(
    await page.evaluate(
      () => document.body.getBoundingClientRect().right <= innerWidth + 1,
    ),
  ).toBe(true);
});

test("browser back between resource pages asks before discarding a form", async ({
  page,
}) => {
  await login(page);
  await page.goto("/console/channels");
  await page
    .locator(".sidebar")
    .getByRole("link", { name: "API tokens", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Create", exact: true })
    .first()
    .click();
  await page
    .getByRole("dialog")
    .getByRole("textbox", { name: "Name", exact: true })
    .fill("unsaved-token");
  await page.evaluate(() => history.back());
  await expect(
    page.getByRole("heading", { name: "Leave without saving?", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Discard changes", exact: true })
    .click();
  await expect(page).toHaveURL(/\/console\/channels$/);
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
