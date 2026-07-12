import { CheckCircle2, Clock3, Flame, ListChecks } from "lucide-react";
import type { ReactNode } from "react";
import type { TicketPriority, TicketStatus } from "../../types";
import { useI18n } from "../../lib/i18n-context";

type SummaryTicket = { status: TicketStatus; priority: TicketPriority };

function SummaryItem({ icon, label, value }: { icon: ReactNode; label: string; value: number }) {
  return (
    <div className="inline-flex min-w-32 items-center gap-2 rounded-full border border-zinc-200 bg-zinc-50 px-3 py-1.5 text-sm">
      <span className="text-zinc-400">{icon}</span>
      <span className="font-semibold text-zinc-950">{value}</span>
      <span className="text-zinc-500">{label}</span>
    </div>
  );
}

export function ProjectTicketSummary({ tickets }: { tickets: SummaryTicket[] }) {
  const { t } = useI18n();
  const stats = {
    total: tickets.length,
    active: tickets.filter((ticket) => ticket.status === "in_progress" || ticket.status === "review").length,
    urgent: tickets.filter((ticket) => ticket.priority === "urgent").length,
    done: tickets.filter((ticket) => ticket.status === "done").length,
  };

  return (
    <>
      <SummaryItem icon={<ListChecks size={15} />} label={t("summary.tickets")} value={stats.total} />
      <SummaryItem icon={<Clock3 size={15} />} label={t("summary.active")} value={stats.active} />
      <SummaryItem icon={<Flame size={15} />} label={t("summary.urgent")} value={stats.urgent} />
      <SummaryItem icon={<CheckCircle2 size={15} />} label={t("summary.done")} value={stats.done} />
    </>
  );
}
