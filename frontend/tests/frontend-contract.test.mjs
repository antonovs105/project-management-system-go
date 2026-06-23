import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

function read(path) {
  return readFileSync(new URL(`../${path}`, import.meta.url), "utf8");
}

function readRoot(path) {
  return readFileSync(new URL(`../../${path}`, import.meta.url), "utf8");
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
    "/auth/oauth/providers",
    "/auth/oauth/exchange",
    "/auth/${provider}/start",
    "/instance",
    "getPublicInstance",
    "getInstanceCapabilities",
    "/me/invites",
    "/admin/users",
    "/admin/audit-events",
    "/projects/${projectId}/members",
    "/projects/${projectId}/github/repositories",
    "/projects/${projectId}/github/commits",
    "/repositories/${repositoryId}/sync",
    "linkTicketGitHubCommit",
    "/projects/${projectId}/tickets/events",
    "projectTicketEventsURL",
    "/tickets/${ticketId}/move",
    "/me/notifications",
    "/me/notifications/events",
    "notificationsEventsURL",
    "/me/notifications/${notificationId}/read",
    "/me/notifications/read-all",
    "/projects/${projectId}/invites",
    "/members/${userId}",
    "updateProjectMemberRole",
    "/invites/${inviteId}/accept",
    "/invites/${inviteId}/reject",
    "/invites/${inviteId}/revoke",
    "/tickets/${ticketId}/links",
    "/tickets/${ticketId}/github/commits",
    "/tickets/${ticketId}/github/commits/${commitId}",
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

test("remote project ticket list auto-refreshes while realtime federation is absent", () => {
  const workspace = read("src/pages/RemoteProjectWorkspace.tsx");

  assert.ok(workspace.includes("const remoteTicketPollMs = 5000"), "remote ticket polling interval must be explicit");
  assert.ok(workspace.includes("refetchInterval: remoteTicketPollMs"), "remote tickets must poll automatically");
  assert.ok(workspace.includes("refetchOnWindowFocus: true"), "remote tickets must refresh when the user returns to the tab");
  assert.ok(workspace.includes("refetchOnReconnect: true"), "remote tickets must refresh after network reconnect");
});

test("frontend exposes a persistent light and dark theme switch", () => {
  const app = read("src/App.tsx");
  const provider = read("src/components/ThemeProvider.tsx");
  const toggle = read("src/components/ThemeToggle.tsx");
  const layout = read("src/components/layout/AppLayout.tsx");
  const css = read("src/index.css");

  assert.ok(app.includes("<ThemeProvider>"), "App must mount the theme provider");
  assert.ok(provider.includes('const storageKey = "progo.theme"'), "theme choice must persist under a stable key");
  assert.ok(provider.includes('classList.toggle("dark"'), "theme provider must toggle the document dark class");
  assert.ok(toggle.includes("useTheme"), "theme toggle must use the shared theme state");
  assert.ok(layout.includes("<ThemeToggle />"), "workspace header must expose the theme toggle");
  assert.ok(css.includes("html.dark"), "global CSS must define dark mode surface styles");
});

test("auth page queries optional OAuth providers directly", () => {
  const authPage = read("src/pages/AuthPage.tsx");

  assert.ok(authPage.includes("api.listOAuthProviders"), "auth page must not rely only on /instance metadata for OAuth buttons");
  assert.ok(authPage.includes("queryKeys.oauthProviders"), "OAuth provider query must use a stable query key");
  assert.ok(authPage.includes("oauthProviders.data ?? publicInstance.data?.oauth_providers"), "auth page must keep /instance OAuth metadata as fallback");
});

test("blue-green deploy mounts runtime config into backend containers", () => {
  const compose = readRoot("deploy/docker-compose.bluegreen.yml");
  const deploy = readRoot("deploy/bluegreen-deploy.sh");

  assert.ok(deploy.includes('PROGO_CONFIG_FILE=${PROGO_CONFIG_FILE:-"$APP_DIR/progo.yml"}'), "deploy script must default to the runtime YAML config");
  assert.ok(deploy.includes("env-only compatibility config"), "deploy script must remain compatible with existing .env-only installs");
  assert.ok(deploy.includes("printf '{}\\n'"), "deploy script must create a valid empty YAML fallback");
  assert.ok(compose.includes("PROGO_CONFIG: /etc/progo/progo.yml"), "backend containers must receive PROGO_CONFIG");
  assert.ok(compose.includes("${PROGO_CONFIG_FILE:?PROGO_CONFIG_FILE is required}:/etc/progo/progo.yml:ro"), "runtime config must be mounted read-only");
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
