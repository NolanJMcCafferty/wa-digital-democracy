import { expect, test } from "@playwright/test";

test("public frontend proxies through the authenticated backend API", async ({ page, request }) => {
  const browserAPI = await request.get("/api/v1/bills?q=Fixture%20Housing%20Stability&limit=1");
  expect(browserAPI.status()).toBe(200);
  expect(browserAPI.headers()["content-type"]).toMatch(/application\/json/);
  const body = await browserAPI.json();
  expect(body.total).toBeGreaterThanOrEqual(1);
  expect(body.bills[0].bill_id).toBe("HB 9001");

  await page.goto("/bills?q=Fixture%20Housing%20Stability&limit=5");
  await expect(page).toHaveTitle(/WA Digital Democracy|Bills/i);
  await expect(page.getByText("Fixture Housing Stability Act")).toBeVisible();
});

test("fixture bill detail renders frontend data from the backend", async ({ page }) => {
  const response = await page.goto("/bills/2099-00/HB9001");
  expect(response?.status()).toBe(200);
  await expect(page.getByRole("heading", { name: /Fixture Housing Stability Act/i })).toBeVisible();
  await expect(page.getByRole("link", { name: /House Committee on Housing/i })).toBeVisible();
  await expect(page.getByRole("heading", { name: "HB 9001" })).toBeVisible();
});

test("invalid bill slug renders through the frontend without leaking backend auth", async ({ page }) => {
  const response = await page.goto("/bills/2025-26/not-a-bill");
  expect(response?.status()).toBe(404);
  await expect(page.locator("body")).toContainText(/404|not found/i);
});
