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
  const generated = read("src/generated/api-schema.ts");
  const constants = read("src/lib/constants.ts");
  const match = /ProjectPermission:\s*([^;]+);/.exec(generated);

  assert.ok(match, "generated ProjectPermission union must exist");
  const permissions = [...match[1].matchAll(/"([^"]+)"/g)].map((item) => item[1]);

  assert.ok(permissions.length > 0);
  for (const permission of permissions) {
    assert.ok(constants.includes(`id: "${permission}"`), `${permission} is missing from projectPermissionGroups`);
  }
});

test("frontend API facade is bound to the generated contract", () => {
  const api = read("src/lib/api.ts");
  const client = read("src/lib/contractClient.ts");
  const packageJson = read("package.json");

  assert.ok(api.includes('import type * as Contract from "../generated/api-schema"'));
  assert.ok(api.includes('contractClient.GET("/api/v1/me/notifications"'));
  assert.ok(client.includes("createClient<paths>"));
  assert.ok(packageJson.includes('"check:api"'), "CI contract freshness command must remain available");
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

test("blue-green deploy keeps env-only compatibility and proxies public metadata", () => {
  const compose = readRoot("deploy/docker-compose.bluegreen.yml");
  const deploy = readRoot("deploy/bluegreen-deploy.sh");

  assert.equal(deploy.includes("PROGO_CONFIG_FILE"), false, "blue-green deploy must not require progo.yml for existing .env-only instances");
  assert.equal(compose.includes("PROGO_CONFIG"), false, "backend containers must not receive a mandatory runtime config mount");
  assert.ok(deploy.includes("/instance /health /ready"), "Caddy must proxy public instance metadata to the backend");
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
