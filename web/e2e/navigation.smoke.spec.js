import { expect, test } from "@playwright/test";

const shellHeaderText = "IM Console";
const routeMatrix = [
  { path: "/", expectText: "消息端", selector: "nav" },
  { path: "/admin", expectText: "管理", selector: "nav" },
  { path: "/login", expectText: "消息端免密登录", selector: "h1" },
  { path: "/cs-login", expectText: "消息端登录", selector: "h1" },
  { path: "/admin-login", expectText: "运营端登录", selector: "h1" },
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
