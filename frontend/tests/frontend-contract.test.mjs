import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

function read(path) {
  return readFileSync(new URL(`../${path}`, import.meta.url), "utf8");
}

function sourceFiles(dir) {
  return readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      return sourceFiles(path);
    }
    return path.endsWith(".ts") || path.endsWith(".tsx") ? [path] : [];
  });
}

test("frontend does not expose owner bootstrap setup", () => {
  const root = fileURLToPath(new URL("../src", import.meta.url));
  const combined = sourceFiles(root).map((path) => readFileSync(path, "utf8")).join("\n");

  assert.equal(combined.includes("/setup/admin"), false);
  assert.equal(combined.toLowerCase().includes("owner setup"), false);
});

test("project permission labels cover every backend permission type", () => {
  const types = read("src/types.ts");
  const constants = read("src/lib/constants.ts");
  const match = /export type ProjectPermission =([\s\S]*?);/.exec(types);

  assert.ok(match, "ProjectPermission union must exist");
  const permissions = [...match[1].matchAll(/"([^"]+)"/g)].map((item) => item[1]);

  assert.ok(permissions.length > 0);
  for (const permission of permissions) {
    assert.ok(constants.includes(`id: "${permission}"`), `${permission} is missing from projectPermissionGroups`);
  }
});

test("api client covers current backend feature routes", () => {
  const api = read("src/lib/api.ts");
  const requiredFragments = [
    "/me/password",
    "/me/invites",
    "/admin/users",
    "/admin/audit-events",
    "/projects/${projectId}/members",
    "/projects/${projectId}/invites",
    "/members/${userId}",
    "updateProjectMemberRole",
    "/invites/${inviteId}/accept",
    "/invites/${inviteId}/reject",
    "/invites/${inviteId}/revoke",
    "/tickets/${ticketId}/links",
    "/links/${linkId}",
    "/deliveries/summary",
    "/deliveries/${deliveryId}/retry",
    "/me/federation/inbox",
    "/me/federation/discover",
    "/me/federation/follows",
    "/admin/federation/domain-blocks",
    "/admin/federation/remote-actors",
    "/admin/federation/deliveries",
  ];

  for (const fragment of requiredFragments) {
    assert.ok(api.includes(fragment), `${fragment} is not wired in api.ts`);
  }
});

test("frontend exposes bilingual locale dictionaries and copyright notice", () => {
  const app = read("src/App.tsx");
  const i18n = read("src/lib/i18n-messages.ts");
  const combined = `${app}\n${i18n}`;

  assert.ok(app.includes("I18nProvider"), "App must mount the locale provider");
  assert.ok(i18n.includes('supportedLocales = ["en", "uk"]'), "English and Ukrainian locales must be supported");
  assert.ok(combined.includes("Sviatoslav Antonov"), "English copyright owner name is missing");
  assert.ok(combined.includes("Святослав Антонов"), "Ukrainian copyright owner name is missing");
  assert.ok(i18n.includes("Умови використання"), "Ukrainian legal copy is missing");
});
