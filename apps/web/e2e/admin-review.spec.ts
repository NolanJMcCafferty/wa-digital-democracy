import { expect, test } from "@playwright/test";

// These tests intentionally do not bypass or mock Clerk. The e2e environment
// must provide a real authenticated Clerk admin session before these pages load.
// If auth is not wired correctly, navigation should land on Clerk sign-in/404
// and these assertions will fail for the right reason.
test.describe.configure({ mode: "serial", retries: 0 });

test("admin speaker review accepts fixture speaker candidate", async ({ page }) => {
  await page.goto("/admin/review/speakers");
  await expect(page.getByRole("heading", { name: "Speaker identity review" })).toBeVisible();

  await page.getByRole("link", { name: "fixture-tvw-event-9001" }).click();
  await expect(page.getByRole("heading", { name: "TVW fixture-tvw-event-9001" })).toBeVisible();

  await page.getByRole("link", { name: "speaker-fixture-review" }).click();
  await expect(page.getByRole("heading", { name: "speaker-fixture-review" })).toBeVisible();
  await expect(page.getByText("Alex Fixture").first()).toBeVisible();
  await expect(page.getByText(/Self-introduction: Alex Fixture/i)).toBeVisible();

  await page.getByRole("button", { name: "Accept" }).click();
  await expect(page.getByText(/Current accepted assignment:/i)).toBeVisible();
  await expect(page.getByText(/Alex Fixture/i).first()).toBeVisible();
  await expect(page.getByText(/accepted/i).first()).toBeVisible();
});

test("admin entity review confirms fixture organization candidate", async ({ page }) => {
  await page.goto("/admin/review/entities");
  await expect(page.getByRole("heading", { name: "Organization entity-match review" })).toBeVisible();

  await expect(page.getByText("Fixture Housing Coalition PAC")).toBeVisible();
  await expect(page.getByText("Synthetic admin review fixture candidate")).toBeVisible();

  await page.getByRole("button", { name: "Confirm" }).click();
  await expect(page.getByText("Fixture Housing Coalition PAC")).toBeVisible();

  await page.goto("/admin/review/entities?decision=confirmed");
  await expect(page.getByText("Fixture Housing Coalition PAC")).toBeVisible();
  await expect(page.getByText(/confirmed/i).first()).toBeVisible();
});
