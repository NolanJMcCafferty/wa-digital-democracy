import { expect, test } from "@playwright/test";

test("fixture bill can be found in the UI and opened", async ({ page }) => {
  await page.goto("/bills?q=Fixture%20Housing%20Stability&limit=5");
  await expect(page).toHaveTitle(/WA Digital Democracy|Bills/i);

  const fixtureBill = page.getByRole("link", { name: /HB 9001/i }).first();
  await expect(page.getByText("Fixture Housing Stability Act")).toBeVisible();
  await expect(fixtureBill).toBeVisible();
  await fixtureBill.click();

  await expect(page).toHaveURL(/\/bills\/2099-00\/HB9001$/);
  await expect(page.getByRole("heading", { name: "HB 9001" })).toBeVisible();
  await expect(page.getByRole("heading", { name: /Fixture Housing Stability Act/i })).toBeVisible();
  await expect(page.getByRole("link", { name: /House Committee on Housing/i })).toBeVisible();
});

test("invalid bill slug renders through the frontend without leaking backend auth", async ({ page }) => {
  const response = await page.goto("/bills/2025-26/not-a-bill");
  expect(response?.status()).toBe(404);
  await expect(page.locator("body")).toContainText(/404|not found/i);
});
