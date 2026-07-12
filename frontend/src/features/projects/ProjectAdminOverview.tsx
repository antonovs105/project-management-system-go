import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, BarChart3, CheckCircle2, Clock3, Download, FileJson, Flame, ListChecks, Shield, Upload } from "lucide-react";
import { useRef, type ReactNode } from "react";
import { toast } from "sonner";
import { Button, ErrorState, Panel } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { ticketPriorities, ticketStatuses } from "../../lib/constants";
import { queryKeys } from "../../lib/queryKeys";
import { downloadJSON } from "../../lib/download";
import type { Project, ProjectBundle, ProjectDeliverySummary, ProjectRole, Ticket } from "../../types";
import { downloadText, safeFilePart, ticketsToCSV } from "./projectSettingsExports";

function percent(count: number, total: number): number {
  return total === 0 ? 0 : Math.round((count / total) * 100);
}

export function MetricPill({ icon, label, value }: { icon: ReactNode; label: string; value: number | string }) {
  return (
    <div className="rounded-2xl border border-zinc-200 bg-zinc-50 px-3 py-3">
      <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-zinc-400">{icon}{label}</div>
      <div className="mt-2 text-2xl font-semibold text-zinc-950">{value}</div>
    </div>
  );
}

function DistributionList({ title, rows, total }: { title: string; rows: Array<{ id: string; label: string; count: number }>; total: number }) {
  return (
    <div className="rounded-2xl border border-zinc-200 p-4">
      <h3 className="text-sm font-semibold text-zinc-950">{title}</h3>
      <div className="mt-4 grid gap-3">
        {rows.map((row) => (
          <div key={row.id} className="grid gap-1.5">
            <div className="flex items-center justify-between gap-3 text-sm">
              <span className="text-zinc-600">{row.label}</span><span className="font-medium text-zinc-950">{row.count}</span>
            </div>
            <div className="h-2 overflow-hidden rounded-full bg-zinc-100">
              <div className="h-full rounded-full bg-zinc-950" style={{ width: `${percent(row.count, total)}%` }} />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

export function ProjectAdminOverview({ project, tickets }: { project: Project; tickets: Ticket[] }) {
	const queryClient = useQueryClient();
	const importInput = useRef<HTMLInputElement>(null);
  const roles = useQuery({ queryKey: queryKeys.projectRoles(project.id), queryFn: () => api.listProjectRoles(project.id) });
  const deliverySummary = useQuery({ queryKey: queryKeys.projectDeliverySummary(project.id), queryFn: () => api.getProjectDeliverySummary(project.id) });
  const statusRows = ticketStatuses.map((status) => ({ ...status, count: tickets.filter((ticket) => ticket.status === status.id).length }));
  const priorityRows = ticketPriorities.map((priority) => ({ ...priority, count: tickets.filter((ticket) => ticket.priority === priority.id).length }));
  const activeTickets = tickets.filter((ticket) => ticket.status === "in_progress" || ticket.status === "review").length;
  const urgentTickets = tickets.filter((ticket) => ticket.priority === "urgent").length;
  const doneTickets = tickets.filter((ticket) => ticket.status === "done").length;
  const customRoles = roles.data?.filter((role) => !role.is_system).length || 0;
  const delivery = deliverySummary.data;
	const portableExport = useMutation({
		mutationFn: () => api.exportProject(project.id),
		onSuccess: (bundle) => downloadJSON(`${safeFilePart(project.name)}.progo.json`, bundle),
	});
	const ticketImport = useMutation({
		mutationFn: (bundle: ProjectBundle) => api.importProjectTickets(project.id, bundle),
		onSuccess: async (result) => {
			await queryClient.invalidateQueries({ queryKey: queryKeys.ticketsScope(project.id) });
			toast.success(`Imported ${result.tickets_imported} tickets and ${result.comments_imported} comments.`);
		},
		onError: (error) => toast.error(errorMessage(error, "Ticket import failed.")),
	});

	async function selectTicketImport(file: File | undefined) {
		if (!file) return;
		if (file.size > 10 * 1024 * 1024) {
			toast.error("Project bundles must be 10 MiB or smaller.");
			return;
		}
		try {
			ticketImport.mutate(JSON.parse(await file.text()) as ProjectBundle);
		} catch {
			toast.error("Select a valid Progo project JSON bundle.");
		} finally {
			if (importInput.current) importInput.current.value = "";
		}
	}

  function exportJSON() {
    const report: {
      project: Project;
      generated_at: string;
      ticket_summary: Record<string, number>;
      status_distribution: typeof statusRows;
      priority_distribution: typeof priorityRows;
      roles: ProjectRole[];
      delivery_summary: ProjectDeliverySummary | null;
      tickets: Ticket[];
    } = {
      project,
      generated_at: new Date().toISOString(),
      ticket_summary: { total: tickets.length, active: activeTickets, urgent: urgentTickets, done: doneTickets, unresolved: tickets.length - doneTickets },
      status_distribution: statusRows,
      priority_distribution: priorityRows,
      roles: roles.data || [],
      delivery_summary: delivery || null,
      tickets,
    };
    downloadText(`${safeFilePart(project.name)}-report.json`, "application/json", `${JSON.stringify(report, null, 2)}\n`);
  }

  return (
    <Panel className="overflow-hidden">
      <div className="flex flex-col gap-3 px-4 py-4 md:flex-row md:items-start md:justify-between">
        <div><h2 className="flex items-center gap-2 text-base font-semibold text-zinc-950"><BarChart3 size={17} />Administration Overview</h2><p className="mt-1 text-sm text-zinc-500">Operational snapshot and exports for project administrators.</p></div>
        <div className="flex flex-wrap gap-2">
		  <input ref={importInput} className="sr-only" type="file" accept="application/json,.json" onChange={(event) => void selectTicketImport(event.target.files?.[0])} />
          <Button onClick={() => downloadText(`${safeFilePart(project.name)}-tickets.csv`, "text/csv", `${ticketsToCSV(tickets)}\n`)} disabled={tickets.length === 0}><Download size={16} />CSV</Button>
          <Button onClick={exportJSON}><FileJson size={16} />JSON</Button>
		  <Button onClick={() => portableExport.mutate()} disabled={portableExport.isPending}><Download size={16} />Portable</Button>
		  <Button onClick={() => importInput.current?.click()} disabled={ticketImport.isPending}><Upload size={16} />Import tickets</Button>
        </div>
      </div>
      <div className="grid gap-3 border-t border-zinc-100 p-4 sm:grid-cols-2 xl:grid-cols-5">
        <MetricPill icon={<ListChecks size={14} />} label="Tickets" value={tickets.length} />
        <MetricPill icon={<Clock3 size={14} />} label="Active" value={activeTickets} />
        <MetricPill icon={<Flame size={14} />} label="Urgent" value={urgentTickets} />
        <MetricPill icon={<CheckCircle2 size={14} />} label="Done" value={doneTickets} />
        <MetricPill icon={<Shield size={14} />} label="Custom Roles" value={roles.isLoading ? "..." : customRoles} />
      </div>
      <div className="grid gap-4 border-t border-zinc-100 p-4 xl:grid-cols-[1fr_1fr_0.8fr]">
        <DistributionList title="Ticket Status" rows={statusRows} total={tickets.length} />
        <DistributionList title="Priority Mix" rows={priorityRows} total={tickets.length} />
        <div className="rounded-2xl border border-zinc-200 p-4">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-zinc-950"><Activity size={15} />Delivery Health</h3>
          {deliverySummary.isLoading ? <div className="mt-4 text-sm text-zinc-500">Loading delivery summary</div> : null}
          {deliverySummary.isError ? <div className="mt-4"><ErrorState title="Could not load delivery summary" body={errorMessage(deliverySummary.error, "Delivery summary failed.")} /></div> : null}
          {delivery ? (
            <div className="mt-4 grid grid-cols-2 gap-2 text-sm">
              {[["Total", delivery.total], ["Failed", delivery.failed + delivery.dead], ["Retryable", delivery.retryable], ["Can Retry", delivery.can_retry ? "yes" : "no"]].map(([label, value]) => (
                <div key={label} className="rounded-xl bg-zinc-50 p-3"><div className="text-xs text-zinc-400">{label}</div><div className="mt-1 font-semibold text-zinc-950">{value}</div></div>
              ))}
            </div>
          ) : null}
        </div>
      </div>
      {roles.isError ? <div className="border-t border-zinc-100 p-4"><ErrorState title="Could not load role summary" body={errorMessage(roles.error, "Role summary failed.")} /></div> : null}
    </Panel>
  );
}
