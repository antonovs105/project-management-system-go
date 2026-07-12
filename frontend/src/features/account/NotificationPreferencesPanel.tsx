import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BellRing } from "lucide-react";
import { toast } from "sonner";
import { ErrorState, LoadingState, Panel } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";
import type { NotificationPreference, NotificationType } from "../../types";

const preferenceLabels: Record<NotificationType, string> = {
  "ticket.assigned": "Ticket assignments",
  "ticket.status_changed": "Ticket status changes",
  "ticket.due_soon": "Tickets due soon",
  "ticket.overdue": "Overdue tickets",
  "comment.created": "New ticket comments",
  "comment.mentioned": "Comment mentions",
  "project.invited": "Project invitations",
  "project.role_changed": "Project role changes",
  "federation.delivery_failed": "Federation delivery failures",
  "security.event": "Account security events",
};

export function NotificationPreferencesPanel() {
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
    onError: (error) => toast.error(errorMessage(error, "Could not update notification preference.")),
  });

  function change(value: NotificationPreference, field: "in_app_enabled" | "email_enabled", enabled: boolean) {
    update.mutate({ ...value, [field]: enabled });
  }

  return (
    <Panel className="p-5 xl:col-span-2">
      <div className="mb-1 flex items-center gap-2">
        <BellRing size={18} className="text-zinc-500" />
        <h2 className="font-semibold text-zinc-950">Notification preferences</h2>
      </div>
      <p className="mb-4 text-sm text-zinc-500">
        Email delivery requires a verified address and is queued through the instance worker.
      </p>
      {preferences.isLoading ? <LoadingState label="Loading notification preferences" /> : null}
      {preferences.isError ? (
        <ErrorState title="Could not load notification preferences" body={errorMessage(preferences.error, "Preference request failed.")} />
      ) : null}
      {preferences.data ? (
        <div className="overflow-x-auto rounded-xl border border-zinc-200">
          <table className="w-full min-w-[32rem] text-left text-sm">
            <thead className="bg-zinc-50 text-xs uppercase tracking-wide text-zinc-500">
              <tr>
                <th className="px-3 py-2 font-semibold">Event</th>
                <th className="px-3 py-2 text-center font-semibold">In app</th>
                <th className="px-3 py-2 text-center font-semibold">Email</th>
              </tr>
            </thead>
            <tbody>
              {preferences.data.map((value) => (
                <tr key={value.type} className="border-t border-zinc-200">
                  <td className="px-3 py-2 font-medium text-zinc-800">{preferenceLabels[value.type]}</td>
                  <td className="px-3 py-2 text-center">
                    <input
                      type="checkbox"
                      aria-label={`In-app ${preferenceLabels[value.type]}`}
                      checked={value.in_app_enabled}
                      disabled={update.isPending}
                      onChange={(event) => change(value, "in_app_enabled", event.target.checked)}
                    />
                  </td>
                  <td className="px-3 py-2 text-center">
                    <input
                      type="checkbox"
                      aria-label={`Email ${preferenceLabels[value.type]}`}
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
