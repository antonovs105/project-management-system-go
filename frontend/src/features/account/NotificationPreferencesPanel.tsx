import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BellRing } from "lucide-react";
import { toast } from "sonner";
import { ErrorState, LoadingState, Panel } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { useI18n } from "../../lib/i18n-context";
import { queryKeys } from "../../lib/queryKeys";
import type { NotificationPreference, NotificationType } from "../../types";

export function NotificationPreferencesPanel() {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const preferences = useQuery({
    queryKey: queryKeys.notificationPreferences,
    queryFn: api.listNotificationPreferences,
  });
  const update = useMutation({
    mutationFn: api.updateNotificationPreference,
    onSuccess: (value) => {
      queryClient.setQueryData<NotificationPreference[]>(queryKeys.notificationPreferences, (current = []) =>
        current.map((item) => (item.type === value.type ? value : item)),
      );
    },
    onError: (error) => toast.error(errorMessage(error, t("account.notificationsUpdateFailed"))),
  });

  const preferenceLabels: Record<NotificationType, string> = {
    "ticket.assigned": t("account.notificationTicketAssigned"),
    "ticket.status_changed": t("account.notificationTicketStatus"),
    "ticket.due_soon": t("account.notificationDueSoon"),
    "ticket.overdue": t("account.notificationOverdue"),
    "comment.created": t("account.notificationComment"),
    "comment.mentioned": t("account.notificationMention"),
    "project.invited": t("account.notificationInvite"),
    "project.role_changed": t("account.notificationRole"),
    "federation.delivery_failed": t("account.notificationFederation"),
    "security.event": t("account.notificationSecurity"),
  };

  function change(value: NotificationPreference, field: "in_app_enabled" | "email_enabled", enabled: boolean) {
    update.mutate({ ...value, [field]: enabled });
  }

  return (
    <Panel className="p-5 xl:col-span-2">
      <div className="mb-1 flex items-center gap-2">
        <BellRing size={18} className="text-zinc-500" />
        <h2 className="font-semibold text-zinc-950">{t("account.notificationsTitle")}</h2>
      </div>
      <p className="mb-4 text-sm text-zinc-500">
        {t("account.notificationsBody")}
      </p>
      {preferences.isLoading ? <LoadingState label={t("account.notificationsLoading")} /> : null}
      {preferences.isError ? (
        <ErrorState title={t("account.notificationsLoadFailed")} body={errorMessage(preferences.error, t("account.notificationsRequestFailed"))} />
      ) : null}
      {preferences.data ? (
        <div className="overflow-x-auto rounded-xl border border-zinc-200">
          <table className="w-full min-w-[32rem] text-left text-sm">
            <thead className="bg-zinc-50 text-xs uppercase tracking-wide text-zinc-500">
              <tr>
                <th className="px-3 py-2 font-semibold">{t("account.notificationsEvent")}</th>
                <th className="px-3 py-2 text-center font-semibold">{t("account.notificationsInApp")}</th>
                <th className="px-3 py-2 text-center font-semibold">{t("account.notificationsEmail")}</th>
              </tr>
            </thead>
            <tbody>
              {preferences.data.map((value) => (
                <tr key={value.type} className="border-t border-zinc-200">
                  <td className="px-3 py-2 font-medium text-zinc-800">{preferenceLabels[value.type]}</td>
                  <td className="px-3 py-2 text-center">
                    <input
                      type="checkbox"
                      aria-label={`${t("account.notificationsInApp")} ${preferenceLabels[value.type]}`}
                      checked={value.in_app_enabled}
                      disabled={update.isPending}
                      onChange={(event) => change(value, "in_app_enabled", event.target.checked)}
                    />
                  </td>
                  <td className="px-3 py-2 text-center">
                    <input
                      type="checkbox"
                      aria-label={`${t("account.notificationsEmail")} ${preferenceLabels[value.type]}`}
                      checked={value.email_enabled}
                      disabled={update.isPending}
                      onChange={(event) => change(value, "email_enabled", event.target.checked)}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </Panel>
  );
}
