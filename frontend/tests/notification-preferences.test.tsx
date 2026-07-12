import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { NotificationPreferencesPanel } from "../src/features/account/NotificationPreferencesPanel";
import { I18nProvider } from "../src/components/I18nProvider";
import type { NotificationPreference } from "../src/types";

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  update: vi.fn(),
}));

vi.mock("../src/lib/api", () => ({
  api: {
    listNotificationPreferences: mocks.list,
    updateNotificationPreference: mocks.update,
  },
  errorMessage: (_error: unknown, fallback: string) => fallback,
}));

const preferences: NotificationPreference[] = [
  { type: "ticket.assigned", in_app_enabled: true, email_enabled: true },
  { type: "ticket.status_changed", in_app_enabled: true, email_enabled: false },
];

describe("NotificationPreferencesPanel", () => {
  beforeEach(() => {
    mocks.list.mockReset().mockResolvedValue(preferences);
    mocks.update.mockReset().mockImplementation(async (value: NotificationPreference) => value);
  });

  it("renders server preferences and persists channel changes", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <I18nProvider><QueryClientProvider client={client}>
        <NotificationPreferencesPanel />
      </QueryClientProvider></I18nProvider>,
    );

    expect(await screen.findByText("Ticket assignments")).toBeInTheDocument();
    const emailStatus = screen.getByRole("checkbox", { name: "Email Ticket status changes" });
    expect(emailStatus).not.toBeChecked();
    fireEvent.click(emailStatus);

    await waitFor(() => {
      expect(mocks.update).toHaveBeenCalledWith({
        type: "ticket.status_changed",
        in_app_enabled: true,
        email_enabled: true,
      }, expect.anything());
    });
    await waitFor(() => expect(emailStatus).toBeChecked());
  });
});
