import { expect, test } from "@playwright/test";

const shellHeaderText = "WeWork Console";
const routeMatrix = [
  { path: "/", expectText: "客服", selector: "nav" },
  { path: "/admin", expectText: "管理", selector: "nav" },
  { path: "/login", expectText: "客服免密登录", selector: "h1" },
  { path: "/cs-login", expectText: "客服工作台登录", selector: "h1" },
  { path: "/admin-login", expectText: "管理中心登录", selector: "h1" },
];

for (const route of routeMatrix) {
  test(`smoke route ${route.path}`, async ({ page }) => {
    await page.goto(route.path, { waitUntil: "domcontentloaded" });
    await expect(page.locator("text=" + shellHeaderText)).toBeVisible();
    const marker = route.selector === "nav"
      ? page.locator("nav", { hasText: route.expectText })
      : page.locator("h1", { hasText: route.expectText });
    await expect(marker).toBeVisible();
    await expect(page).toHaveURL(new RegExp(`${route.path.replace("/", "\\/")}($|\\?)`));
  });
}

test("smoke version endpoint", async ({ request }) => {
  const response = await request.get("/version.txt");
  expect(response.ok()).toBeTruthy();
  const text = (await response.text()).trim();
  expect(text.length).toBeGreaterThan(0);
});
