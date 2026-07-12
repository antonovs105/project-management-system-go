import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { APITokenPanel } from "../src/features/account/APITokenPanel";
import { I18nProvider } from "../src/components/I18nProvider";

const mocks = vi.hoisted(() => ({ list: vi.fn(), create: vi.fn(), revoke: vi.fn() }));

vi.mock("../src/lib/api", () => ({
  api: { listAPITokens: mocks.list, createAPIToken: mocks.create, revokeAPIToken: mocks.revoke },
  errorMessage: (_error: unknown, fallback: string) => fallback,
}));

describe("APITokenPanel", () => {
  beforeEach(() => {
    mocks.list.mockReset().mockResolvedValue([]);
    mocks.create.mockReset().mockResolvedValue({ id: "token-1", user_id: "user-1", name: "Build", prefix: "progo_example", scopes: ["projects:read"], created_at: new Date().toISOString(), token: "progo_once_only_secret" });
    mocks.revoke.mockReset().mockResolvedValue(undefined);
  });

  it("creates a scoped token and presents its secret once", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(<I18nProvider><QueryClientProvider client={client}><APITokenPanel /></QueryClientProvider></I18nProvider>);

    fireEvent.change(screen.getByLabelText("Token name"), { target: { value: "Build" } });
    fireEvent.click(screen.getByRole("button", { name: "Create token" }));

    await waitFor(() => expect(mocks.create).toHaveBeenCalledWith({ name: "Build", scopes: ["projects:read"] }, expect.anything()));
    expect(await screen.findByText("progo_once_only_secret")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "I saved it" }));
    expect(screen.queryByText("progo_once_only_secret")).not.toBeInTheDocument();
  });
});
