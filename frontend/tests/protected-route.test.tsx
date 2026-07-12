import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { MemoryRouter, Outlet, Route, Routes } from "react-router-dom";
import { ProtectedRoute } from "../src/components/layout/ProtectedRoute";
import { useAuthStore } from "../src/store/auth";

function TestRoutes() {
  return (
    <MemoryRouter initialEntries={["/projects"]}>
      <Routes>
        <Route path="/login" element={<div>Sign in page</div>} />
        <Route element={<ProtectedRoute />}>
          <Route element={<Outlet />}>
            <Route path="/projects" element={<div>Private projects</div>} />
          </Route>
        </Route>
      </Routes>
    </MemoryRouter>
  );
}

describe("ProtectedRoute", () => {
  beforeEach(() => {
    localStorage.clear();
    useAuthStore.getState().logout();
  });

  it("redirects an anonymous browser session to login", () => {
    render(<TestRoutes />);

    expect(screen.getByText("Sign in page")).toBeInTheDocument();
    expect(screen.queryByText("Private projects")).not.toBeInTheDocument();
  });

  it("renders protected content for an authenticated cookie-backed session", () => {
    useAuthStore.getState().setSession({
      userId: "11111111-1111-4111-8111-111111111111",
      instanceRole: "user",
      email: "member@example.test",
    });

    render(<TestRoutes />);

    expect(screen.getByText("Private projects")).toBeInTheDocument();
    expect(localStorage.getItem("pms.session")).not.toContain("token");
  });
});
