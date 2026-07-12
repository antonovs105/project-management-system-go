import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ProjectWebhooksPanel } from "../src/features/projects/ProjectWebhooksPanel";

const mocks = vi.hoisted(() => ({ list: vi.fn(), deliveries: vi.fn(), create: vi.fn(), remove: vi.fn(), retry: vi.fn() }));

vi.mock("../src/lib/api", () => ({
  api: {
    listProjectWebhooks: mocks.list,
    listProjectWebhookDeliveries: mocks.deliveries,
    createProjectWebhook: mocks.create,
    deleteProjectWebhook: mocks.remove,
    retryProjectWebhookDelivery: mocks.retry,
  },
  errorMessage: (_error: unknown, fallback: string) => fallback,
}));

describe("ProjectWebhooksPanel", () => {
  beforeEach(() => {
    mocks.list.mockReset().mockResolvedValue([]);
    mocks.deliveries.mockReset().mockResolvedValue([]);
    mocks.create.mockReset().mockResolvedValue({ id: "webhook-1", name: "Automation", target_url: "https://example.test/hook", events: ["ticket.created", "ticket.updated"], secret: "whsec_once_only" });
    mocks.remove.mockReset().mockResolvedValue(undefined);
    mocks.retry.mockReset().mockResolvedValue(undefined);
  });

  it("creates a signed webhook and presents the secret once", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(<QueryClientProvider client={client}><ProjectWebhooksPanel projectId="project-1" /></QueryClientProvider>);

    fireEvent.change(screen.getByLabelText("Webhook name"), { target: { value: "Automation" } });
    fireEvent.change(screen.getByLabelText("HTTPS target URL"), { target: { value: "https://example.test/hook" } });
    fireEvent.click(screen.getByRole("button", { name: "Create webhook" }));

    await waitFor(() => expect(mocks.create).toHaveBeenCalledWith("project-1", { name: "Automation", target_url: "https://example.test/hook", events: ["ticket.created", "ticket.updated"] }));
    expect(await screen.findByText("whsec_once_only")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "I saved it" }));
    expect(screen.queryByText("whsec_once_only")).not.toBeInTheDocument();
  });
});
