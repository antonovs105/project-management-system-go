import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const userEmail = process.env.E2E_USER_EMAIL || "e2e-user@example.test";
const userPassword = process.env.E2E_USER_PASSWORD || "e2e-user-password123";

async function login(page: Page, email: string, password: string) {
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
}

test("a bootstrapped owner receives privileged setup guidance", async ({ page }) => {
  const ownerEmail = process.env.E2E_OWNER_EMAIL;
  const ownerPassword = process.env.E2E_OWNER_PASSWORD;
  test.skip(!ownerEmail || !ownerPassword, "set owner credentials to exercise bootstrap onboarding");

  await login(page, ownerEmail!, ownerPassword!);

  await expect(page).toHaveURL(/\/account$/);
  await expect(page.getByRole("status")).toContainText("Complete privileged account setup");
  await expect(page.getByText(/create a second MFA-protected owner/i)).toBeVisible();
});

test("a verified user creates a project and ticket without accessibility violations", async ({ page }) => {
  await login(page, userEmail, userPassword);
  await expect(page).toHaveURL(/\/projects$/);

  const suffix = Date.now().toString(36);
  const projectName = `E2E project ${suffix}`;
  const ticketTitle = `E2E ticket ${suffix}`;

	await page.getByRole("button", { name: "Project", exact: true }).click();
  const projectDialog = page.getByRole("dialog", { name: "Create Project" });
  await projectDialog.getByLabel("Name").fill(projectName);
  await projectDialog.getByLabel("Description").fill("Created by the browser release gate.");
  await projectDialog.getByRole("button", { name: "Create" }).click();

  await expect(page).toHaveURL(/\/projects\/[0-9a-f-]+$/);
  await expect(page.getByRole("heading", { name: projectName })).toBeVisible();
  await page.getByRole("button", { name: "Ticket", exact: true }).click();
  const ticketDialog = page.getByRole("dialog", { name: "Create Ticket" });
  await ticketDialog.getByLabel("Title").fill(ticketTitle);
  await ticketDialog.getByRole("button", { name: "Create" }).click();

  await expect(page.getByRole("button", { name: `Open ticket ${ticketTitle}` })).toBeVisible();
  const accessibility = await new AxeBuilder({ page }).analyze();
  expect(accessibility.violations, accessibility.violations.map((violation) => `${violation.id}: ${violation.help}`).join("\n")).toEqual([]);

	const sourceProjectID = page.url().split("/").at(-1)!;
	const apiURL = process.env.E2E_API_URL || "http://localhost:8080";
	const exportResponse = await page.request.get(`${apiURL}/api/v1/projects/${sourceProjectID}/export`);
	expect(exportResponse.ok()).toBeTruthy();
	const bundle = await exportResponse.json();
	bundle.project.name = `${projectName} import`;

	await page.goto("/projects");
	const fileChooserPromise = page.waitForEvent("filechooser");
	await page.getByRole("button", { name: "Import" }).click();
	const fileChooser = await fileChooserPromise;
	await fileChooser.setFiles({ name: "project.progo.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(bundle)) });
	await expect(page).toHaveURL(/\/projects\/[0-9a-f-]+$/);
	await expect(page.getByRole("heading", { name: `${projectName} import` })).toBeVisible();

	await page.goto("/account");
	const downloadPromise = page.waitForEvent("download");
	await page.getByRole("button", { name: "Export data" }).click();
	const download = await downloadPromise;
	expect(download.suggestedFilename()).toBe("progo-account-export.json");

	const tokenName = `E2E automation ${suffix}`;
	await page.getByLabel("Token name").fill(tokenName);
	await page.getByRole("button", { name: "Create token" }).click();
	const secret = await page.getByRole("status").locator("code").textContent();
	expect(secret).toMatch(/^progo_/);
	const tokenRead = await page.request.get(`${apiURL}/api/v1/projects`, { headers: { Authorization: `Bearer ${secret}` } });
	expect(tokenRead.status()).toBe(200);
	const tokenWrite = await page.request.post(`${apiURL}/api/v1/projects`, { headers: { Authorization: `Bearer ${secret}` }, data: { name: "forbidden", description: "" } });
	expect(tokenWrite.status()).toBe(403);
	await page.getByRole("button", { name: "I saved it" }).click();
	await expect(page.getByText(tokenName)).toBeVisible();
	await page.getByRole("button", { name: "Revoke" }).first().click();
	await expect.poll(async () => (await page.request.get(`${apiURL}/api/v1/projects`, { headers: { Authorization: `Bearer ${secret}` } })).status()).toBe(401);
});
