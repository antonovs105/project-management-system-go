import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Toaster } from "sonner";
import { AppLayout } from "./components/layout/AppLayout";
import { ProtectedRoute } from "./components/layout/ProtectedRoute";
import { AccountPage } from "./pages/AccountPage";
import { AdminAuditPage } from "./pages/AdminAuditPage";
import { AdminFederationPage } from "./pages/AdminFederationPage";
import { AdminUsersPage } from "./pages/AdminUsersPage";
import { AuthPage } from "./pages/AuthPage";
import { FederationPage } from "./pages/FederationPage";
import { InvitationsPage } from "./pages/InvitationsPage";
import { ProjectWorkspace } from "./pages/ProjectWorkspace";
import { ProjectsPage } from "./pages/ProjectsPage";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
      staleTime: 15_000,
    },
  },
});

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<AuthPage mode="login" />} />
          <Route path="/register" element={<AuthPage mode="register" />} />

          <Route element={<ProtectedRoute />}>
            <Route element={<AppLayout />}>
              <Route index element={<Navigate to="/projects" replace />} />
              <Route path="/projects" element={<ProjectsPage />} />
              <Route path="/invitations" element={<InvitationsPage />} />
              <Route path="/federation" element={<FederationPage />} />
              <Route path="/projects/:projectId" element={<ProjectWorkspace />} />
              <Route path="/projects/:projectId/graph" element={<ProjectWorkspace />} />
              <Route path="/projects/:projectId/deliveries" element={<ProjectWorkspace />} />
              <Route path="/projects/:projectId/settings" element={<ProjectWorkspace />} />
              <Route path="/account" element={<AccountPage />} />
              <Route path="/admin/users" element={<AdminUsersPage />} />
              <Route path="/admin/federation" element={<AdminFederationPage />} />
              <Route path="/admin/audit" element={<AdminAuditPage />} />
            </Route>
          </Route>

          <Route path="*" element={<Navigate to="/projects" replace />} />
        </Routes>
        <Toaster richColors closeButton />
      </BrowserRouter>
    </QueryClientProvider>
  );
}
