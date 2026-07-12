import { ShieldCheck } from "lucide-react";
import { Panel } from "../../components/ui";
import { useI18n } from "../../lib/i18n-context";
import type { InstanceRole } from "../../types";

interface PrivilegedAccountOnboardingProps {
  instanceRole?: InstanceRole;
  enrollmentRequired?: boolean;
}

export function PrivilegedAccountOnboarding({ instanceRole, enrollmentRequired }: PrivilegedAccountOnboardingProps) {
  const { t } = useI18n();
  if (!enrollmentRequired || (instanceRole !== "owner" && instanceRole !== "admin")) {
    return null;
  }

  return (
    <Panel className="border-amber-300 bg-amber-50 p-5 xl:col-span-2" role="status" aria-live="polite">
      <div className="flex items-start gap-3">
        <ShieldCheck className="mt-0.5 shrink-0 text-amber-700" size={20} aria-hidden="true" />
        <div>
          <h2 className="font-semibold text-amber-950">{t("account.privilegedTitle")}</h2>
          <p className="mt-1 text-sm text-amber-900">
            {t("account.privilegedBody")}
          </p>
          {instanceRole === "owner" ? (
            <p className="mt-2 text-sm text-amber-900">
              {t("account.privilegedOwnerBody")}
            </p>
          ) : null}
        </div>
      </div>
    </Panel>
  );
}
