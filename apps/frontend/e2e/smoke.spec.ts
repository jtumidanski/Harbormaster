import { test, expect } from "@playwright/test";

// Golden-path smoke: drives the first-run wizard, signs in, and lands on the
// bucket list. Requires HARBORMASTER_E2E_URL pointing at a freshly-stood-up
// stack with no setup completed. Run as part of T6.13 acceptance and on-demand.

test("setup -> login -> buckets golden path", async ({ page }) => {
  await page.goto("/");

  // Setup wizard: admin step
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password", { exact: true }).fill("correct horse battery staple!");
  await page.getByLabel("Confirm password").fill("correct horse battery staple!");
  await page.getByRole("button", { name: /next/i }).click();

  // Setup wizard: MinIO step
  await page.getByLabel("Endpoint URL").fill("http://minio:9000");
  await page.getByLabel("Access key").fill("admin");
  await page.getByLabel("Secret key").fill("admin12345");
  await page.getByRole("button", { name: /finish|submit/i }).click();

  // Login
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password", { exact: true }).fill("correct horse battery staple!");
  await page.getByRole("button", { name: /sign in/i }).click();

  // Land on dashboard (default redirect) and confirm the buckets nav link is wired.
  await expect(page.getByRole("link", { name: /buckets/i })).toBeVisible();
});
