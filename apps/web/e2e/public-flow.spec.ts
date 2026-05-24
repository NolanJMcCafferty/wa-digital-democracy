import { expect, test } from "@playwright/test";

test("bill search opens fixture bill detail", async ({ page }) => {
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

test("hearing search opens fixture hearing detail", async ({ page }) => {
  await page.goto("/hearings?bill=HB%209001&limit=5");
  await expect(page.getByRole("heading", { name: "Hearings" })).toBeVisible();

  const fixtureHearing = page.getByRole("link", { name: /House Committee on Housing/i }).first();
  await expect(fixtureHearing).toBeVisible();
  const hearingRow = page.getByRole("row", { name: /House Committee on Housing.*HB 9001/i });
  await expect(hearingRow).toBeVisible();
  await fixtureHearing.click();

  await expect(page).toHaveURL(/\/hearings\/\d+$/);
  await expect(page.getByRole("heading", { name: "House Committee on Housing" })).toBeVisible();
  await expect(page.getByRole("link", { name: "HB 9001" })).toBeVisible();
  await expect(page.getByText(/2 signed in/i)).toBeVisible();
});

test("organization search opens fixture organization detail", async ({ page }) => {
  await page.goto("/organizations?q=Fixture%20Housing%20Coalition");
  await expect(page.getByRole("heading", { name: "Organizations" })).toBeVisible();

  const fixtureOrg = page.getByRole("link", { name: "Fixture Housing Coalition" }).first();
  await expect(fixtureOrg).toBeVisible();
  await expect(page.getByText(/aka .*FHC/i)).toBeVisible();
  await fixtureOrg.click();

  await expect(page).toHaveURL(/\/organizations\/fixture-housing-coalition$/);
  await expect(page.getByRole("heading", { name: "Fixture Housing Coalition" })).toBeVisible();
  await expect(page.getByText(/Synthetic organization used by deterministic test fixtures/i)).toBeVisible();
  await expect(page.getByRole("link", { name: /House Committee on Housing/i })).toBeVisible();
});

test("housing issue page aggregates fixture bill, hearing, and organization", async ({ page }) => {
  await page.goto("/issues/housing");
  await expect(page.getByRole("heading", { name: "Housing" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Bills" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Hearings" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Organizations" })).toBeVisible();
  await expect(page.getByRole("link", { name: /HB 9001/i })).toBeVisible();
  await expect(page.getByRole("link", { name: /House Committee on Housing/i }).first()).toBeVisible();
  await expect(page.getByRole("link", { name: "Fixture Housing Coalition" })).toBeVisible();
});

test("home page legislator mosaic opens fixture legislator detail", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1, name: "WA Digital Democracy" })).toBeVisible();

  const legislator = page.getByRole("link", { name: "Fixture Sponsor", exact: true });
  await expect(legislator).toBeVisible();
  await legislator.click();

  await expect(page).toHaveURL(/\/legislators\/representative-fixture-sponsor$/);
  await expect(page.getByRole("heading", { name: "Fixture Sponsor" })).toBeVisible();
  await expect(page.getByText(/House · District 99 · D/i)).toBeVisible();
  await expect(page.getByRole("heading", { name: "Sponsored bills" })).toBeVisible();
  await expect(page.getByRole("link", { name: /HB 9001/i })).toBeVisible();
});

test("home page address lookup matches fixture legislator", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Find Your Legislators" })).toBeVisible();

  await page.getByPlaceholder("600 4th Ave, Seattle, WA 98104").fill("600 4th Ave, Seattle, WA 98104");
  await page.getByRole("button", { name: "Search" }).click();

  await expect(page.getByText("Legislative District 99")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("Matched: 600 4th Ave, Seattle, WA 98104")).toBeVisible();
  await expect(page.getByRole("link", { name: /Fixture Sponsor State Representative/i })).toBeVisible();
});

test("invalid bill slug renders through the frontend without leaking backend auth", async ({ page }) => {
  const response = await page.goto("/bills/2025-26/not-a-bill");
  expect(response?.status()).toBe(404);
  await expect(page.locator("body")).toContainText(/404|not found/i);
});
