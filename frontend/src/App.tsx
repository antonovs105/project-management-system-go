import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { lazy, Suspense } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Toaster } from "sonner";
import { AppLayout } from "./components/layout/AppLayout";
import { ProtectedRoute } from "./components/layout/ProtectedRoute";
import { I18nProvider } from "./components/I18nProvider";
import { ThemeProvider } from "./components/ThemeProvider";
import { useTheme } from "./lib/theme-context";

const AccountPage = lazy(() => import("./pages/AccountPage").then((module) => ({ default: module.AccountPage })));
const AccountRecoveryPage = lazy(() => import("./pages/AccountRecoveryPage").then((module) => ({ default: module.AccountRecoveryPage })));
const AdminAuditPage = lazy(() => import("./pages/AdminAuditPage").then((module) => ({ default: module.AdminAuditPage })));
const AdminFederationPage = lazy(() => import("./pages/AdminFederationPage").then((module) => ({ default: module.AdminFederationPage })));
const AdminUsersPage = lazy(() => import("./pages/AdminUsersPage").then((module) => ({ default: module.AdminUsersPage })));
const AuthPage = lazy(() => import("./pages/AuthPage").then((module) => ({ default: module.AuthPage })));
const FederationPage = lazy(() => import("./pages/FederationPage").then((module) => ({ default: module.FederationPage })));
const InvitationsPage = lazy(() => import("./pages/InvitationsPage").then((module) => ({ default: module.InvitationsPage })));
const LegalPage = lazy(() => import("./pages/LegalPage").then((module) => ({ default: module.LegalPage })));
const OAuthCallbackPage = lazy(() => import("./pages/OAuthCallbackPage").then((module) => ({ default: module.OAuthCallbackPage })));
const ProjectWorkspace = lazy(() => import("./pages/ProjectWorkspace").then((module) => ({ default: module.ProjectWorkspace })));
const ProjectsPage = lazy(() => import("./pages/ProjectsPage").then((module) => ({ default: module.ProjectsPage })));
const RemoteProjectWorkspace = lazy(() => import("./pages/RemoteProjectWorkspace").then((module) => ({ default: module.RemoteProjectWorkspace })));

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
      staleTime: 15_000,
    },
  },
});

function AppToaster() {
  const { theme } = useTheme();
  return <Toaster richColors closeButton theme={theme} />;
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <I18nProvider>
          <BrowserRouter>
            <Suspense fallback={<div className="p-6" role="status">Loading…</div>}>
            <Routes>
              <Route path="/login" element={<AuthPage mode="login" />} />
              <Route path="/register" element={<AuthPage mode="register" />} />
              <Route path="/oauth/callback" element={<OAuthCallbackPage />} />
              <Route path="/auth/forgot-password" element={<AccountRecoveryPage mode="forgot" />} />
              <Route path="/auth/reset-password" element={<AccountRecoveryPage mode="reset" />} />
              <Route path="/auth/verify-email" element={<AccountRecoveryPage mode="verify" />} />
              <Route path="/terms" element={<LegalPage />} />

              <Route element={<ProtectedRoute />}>
                <Route element={<AppLayout />}>
                  <Route index element={<Navigate to="/projects" replace />} />
                  <Route path="/projects" element={<ProjectsPage />} />
                  <Route path="/invitations" element={<InvitationsPage />} />
                  <Route path="/federation" element={<FederationPage />} />
                  <Route path="/projects/:projectId" element={<ProjectWorkspace />} />
                  <Route path="/projects/:projectId/graph" element={<ProjectWorkspace />} />
                  <Route path="/projects/:projectId/deliveries" element={<ProjectWorkspace />} />
				  <Route path="/projects/:projectId/activity" element={<ProjectWorkspace />} />
                  <Route path="/projects/:projectId/settings" element={<ProjectWorkspace />} />
                  <Route path="/remote-projects/:projectId" element={<RemoteProjectWorkspace />} />
                  <Route path="/account" element={<AccountPage />} />
                  <Route path="/admin/users" element={<AdminUsersPage />} />
                  <Route path="/admin/federation" element={<AdminFederationPage />} />
                  <Route path="/admin/audit" element={<AdminAuditPage />} />
                </Route>
              </Route>

              <Route path="*" element={<Navigate to="/projects" replace />} />
            </Routes>
            </Suspense>
            <AppToaster />
          </BrowserRouter>
        </I18nProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
