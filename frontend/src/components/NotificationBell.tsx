import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, CheckCheck } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { api, errorMessage, notificationsEventsURL } from "../lib/api";
import { relativeDate } from "../lib/format";
import { queryKeys } from "../lib/queryKeys";
import { useAuthStore } from "../store/auth";
import type { Notification as AppNotification } from "../types";
import { Button, IconButton } from "./ui";

function parseSSEMessage(raw: string): { event: string; data: string } | null {
  let event = "message";
  const data: string[] = [];
  for (const line of raw.split(/\r?\n/)) {
    if (line.startsWith("event:")) {
      event = line.slice("event:".length).trim();
    } else if (line.startsWith("data:")) {
      data.push(line.slice("data:".length).trimStart());
    }
  }
  return data.length > 0 ? { event, data: data.join("\n") } : null;
}

function isNotification(value: unknown): value is AppNotification {
  if (!value || typeof value !== "object") {
    return false;
  }
  const item = value as Partial<AppNotification>;
  return typeof item.id === "string" && typeof item.user_id === "string" && typeof item.type === "string";
}

function notificationLink(notification: AppNotification): string {
  return notification.project_id ? `/projects/${notification.project_id}` : "/account";
}

export function NotificationBell() {
  const [open, setOpen] = useState(false);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const queryClient = useQueryClient();

  const notifications = useQuery({
    queryKey: queryKeys.notifications,
    queryFn: () => api.listNotifications(),
    enabled: isAuthenticated,
  });

  const unreadCount = useMemo(
    () => (notifications.data || []).filter((notification) => !notification.read_at).length,
    [notifications.data],
  );

  const markRead = useMutation({
    mutationFn: api.markNotificationRead,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.notifications });
    },
  });

  const markAllRead = useMutation({
    mutationFn: api.markAllNotificationsRead,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.notifications });
    },
  });

  useEffect(() => {
    if (!isAuthenticated) {
      return;
    }

    const controller = new AbortController();
    const decoder = new TextDecoder();
    let reconnectTimer: number | undefined;

    function upsertNotification(notification: AppNotification) {
      queryClient.setQueryData<AppNotification[]>(queryKeys.notifications, (current = []) => {
        if (current.some((item) => item.id === notification.id)) {
          return current;
        }
        return [notification, ...current].slice(0, 50);
      });
      toast.info(notification.title, { description: notification.body });
    }

    function handleFrame(frame: string) {
      const message = parseSSEMessage(frame);
      if (!message || message.event === "ready") {
        return;
      }
      try {
        const parsed: unknown = JSON.parse(message.data);
        if (isNotification(parsed)) {
          upsertNotification(parsed);
        }
      } catch {
        // Stream reconnects handle transport failures; malformed frames are ignored.
      }
    }

    function scheduleReconnect() {
      if (!controller.signal.aborted) {
        reconnectTimer = window.setTimeout(connect, 3000);
      }
    }

    async function connect() {
      try {
        const response = await fetch(notificationsEventsURL(), {
          credentials: "include",
          headers: {
            Accept: "text/event-stream",
          },
          signal: controller.signal,
        });
        if (response.status === 401) {
          useAuthStore.getState().logout();
          return;
        }
        if (!response.ok || !response.body) {
          throw new Error("notification event stream unavailable");
        }

        const reader = response.body.getReader();
        let buffer = "";
        for (;;) {
          const { value, done } = await reader.read();
          if (done) {
            break;
          }
          buffer += decoder.decode(value, { stream: true });
          const frames = buffer.split(/\n\n/);
          buffer = frames.pop() || "";
          frames.forEach(handleFrame);
        }
        scheduleReconnect();
      } catch {
        scheduleReconnect();
      }
    }

    void connect();
    return () => {
      controller.abort();
      if (reconnectTimer) {
        window.clearTimeout(reconnectTimer);
      }
    };
  }, [isAuthenticated, queryClient]);

  return (
    <div className="relative">
      <IconButton label="Notifications" onClick={() => setOpen((current) => !current)}>
        <Bell size={18} />
        {unreadCount > 0 ? (
          <span className="absolute -right-1 -top-1 min-w-5 rounded-full bg-red-600 px-1 text-[11px] font-semibold text-white">
            {unreadCount > 9 ? "9+" : unreadCount}
          </span>
        ) : null}
      </IconButton>

      {open ? (
        <div className="absolute right-0 top-11 z-40 w-80 overflow-hidden rounded-3xl border border-zinc-200 bg-white shadow-2xl">
          <div className="flex items-center justify-between border-b border-zinc-200 px-4 py-3">
            <div className="font-semibold text-zinc-950">Notifications</div>
            <Button
              className="h-8 px-3 text-xs"
              onClick={() => markAllRead.mutate()}
              disabled={markAllRead.isPending || unreadCount === 0}
            >
              <CheckCheck size={14} />
              Read all
            </Button>
          </div>
          <div className="max-h-96 overflow-y-auto p-2">
            {notifications.isError ? (
              <div className="px-3 py-2 text-sm text-red-600">
                {errorMessage(notifications.error, "Could not load notifications.")}
              </div>
            ) : null}
            {(notifications.data || []).map((notification) => (
              <Link
                key={notification.id}
                to={notificationLink(notification)}
                className={[
                  "block rounded-2xl px-3 py-2 transition hover:bg-zinc-50",
                  notification.read_at ? "text-zinc-500" : "bg-zinc-100 text-zinc-950",
                ].join(" ")}
                onClick={() => {
                  setOpen(false);
                  if (!notification.read_at) {
                    markRead.mutate(notification.id);
                  }
                }}
              >
                <div className="text-sm font-medium">{notification.title}</div>
                <div className="mt-1 line-clamp-2 text-xs">{notification.body}</div>
                <div className="mt-2 text-[11px] text-zinc-400">{relativeDate(notification.created_at)}</div>
              </Link>
            ))}
            {notifications.data?.length === 0 ? (
              <div className="px-3 py-8 text-center text-sm text-zinc-500">No notifications yet.</div>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}
