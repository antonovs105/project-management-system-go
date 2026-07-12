import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PrivilegedAccountOnboarding } from "../src/features/account/PrivilegedAccountOnboarding";

describe("PrivilegedAccountOnboarding", () => {
  it("guides a newly bootstrapped owner through resilient setup", () => {
    render(<PrivilegedAccountOnboarding instanceRole="owner" enrollmentRequired />);

    expect(screen.getByRole("status")).toHaveTextContent("Complete privileged account setup");
    expect(screen.getByText(/save the recovery codes offline/i)).toBeInTheDocument();
    expect(screen.getByText(/create a second MFA-protected owner/i)).toBeInTheDocument();
  });

  it("does not render after enrollment or for a normal user", () => {
    const { rerender } = render(<PrivilegedAccountOnboarding instanceRole="owner" enrollmentRequired={false} />);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();

    rerender(<PrivilegedAccountOnboarding instanceRole="user" enrollmentRequired />);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});
