import { ShieldCheck } from "lucide-react";
import { Panel } from "../../components/ui";
import type { InstanceRole } from "../../types";

interface PrivilegedAccountOnboardingProps {
  instanceRole?: InstanceRole;
  enrollmentRequired?: boolean;
}

export function PrivilegedAccountOnboarding({ instanceRole, enrollmentRequired }: PrivilegedAccountOnboardingProps) {
  if (!enrollmentRequired || (instanceRole !== "owner" && instanceRole !== "admin")) {
    return null;
  }

  return (
    <Panel className="border-amber-300 bg-amber-50 p-5 xl:col-span-2" role="status" aria-live="polite">
      <div className="flex items-start gap-3">
        <ShieldCheck className="mt-0.5 shrink-0 text-amber-700" size={20} aria-hidden="true" />
        <div>
          <h2 className="font-semibold text-amber-950">Complete privileged account setup</h2>
          <p className="mt-1 text-sm text-amber-900">
            Multi-factor authentication is required before you can use instance administration. Start setup below, save the recovery codes offline, and sign in again after enrollment.
          </p>
          {instanceRole === "owner" ? (
            <p className="mt-2 text-sm text-amber-900">
              After setup, create a second MFA-protected owner so account recovery never depends on one person or device.
            </p>
          ) : null}
        </div>
      </div>
    </Panel>
  );
}
