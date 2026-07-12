import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PrivilegedAccountOnboarding } from "../src/features/account/PrivilegedAccountOnboarding";
import { I18nProvider } from "../src/components/I18nProvider";

describe("PrivilegedAccountOnboarding", () => {
  it("guides a newly bootstrapped owner through resilient setup", () => {
    render(<I18nProvider><PrivilegedAccountOnboarding instanceRole="owner" enrollmentRequired /></I18nProvider>);

    expect(screen.getByRole("status")).toHaveTextContent("Complete privileged account setup");
    expect(screen.getByText(/save the recovery codes offline/i)).toBeInTheDocument();
    expect(screen.getByText(/create a second MFA-protected owner/i)).toBeInTheDocument();
  });

  it("does not render after enrollment or for a normal user", () => {
    const { rerender } = render(<I18nProvider><PrivilegedAccountOnboarding instanceRole="owner" enrollmentRequired={false} /></I18nProvider>);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();

    rerender(<I18nProvider><PrivilegedAccountOnboarding instanceRole="user" enrollmentRequired /></I18nProvider>);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});
