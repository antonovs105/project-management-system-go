import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import { SortableContext, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { GripVertical } from "lucide-react";
import { useMemo, useState } from "react";
import type { ReactNode } from "react";
import { ticketStatuses } from "../../lib/constants";
import type { ID, Ticket, TicketStatus } from "../../types";
import { StatusBadge } from "../../components/StatusBadge";
import { EmptyState, Panel } from "../../components/ui";

function columnTitle(status: TicketStatus): string {
  return ticketStatuses.find((item) => item.id === status)?.label || status;
}

function TicketCard({ ticket, onOpen }: { ticket: Ticket; onOpen: (ticketId: ID) => void }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: ticket.id,
    data: { ticket },
  });

  return (
    <button
      type="button"
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.45 : 1,
      }}
      className="focus-ring w-full rounded-2xl text-left"
      onClick={() => onOpen(ticket.id)}
    >
      <Panel className="group p-3 transition hover:-translate-y-0.5 hover:border-zinc-300 hover:shadow-md">
        <div className="flex items-start gap-2">
          <div className="min-w-0 flex-1">
            <div className="mb-2 flex flex-wrap gap-1.5">
              <StatusBadge value={ticket.type} kind="type" />
              <StatusBadge value={ticket.priority} kind="priority" />
            </div>
            <h3 className="line-clamp-2 text-sm font-semibold leading-5 text-zinc-950">{ticket.title}</h3>
            {ticket.description ? <p className="mt-2 line-clamp-2 text-xs text-zinc-500">{ticket.description}</p> : null}
          </div>
          <span
            className="flex h-7 w-7 shrink-0 touch-none items-center justify-center rounded-xl text-zinc-300 transition group-hover:text-zinc-500 hover:bg-zinc-100"
            {...attributes}
            {...listeners}
            aria-label="Drag ticket"
          >
            <GripVertical size={16} />
          </span>
        </div>
      </Panel>
    </button>
  );
}

function BoardColumn({
  status,
  tickets,
  onOpen,
}: {
  status: TicketStatus;
  tickets: Ticket[];
  onOpen: (ticketId: ID) => void;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: status });

  return (
    <section className="flex min-h-[520px] min-w-[290px] flex-1 flex-col rounded-3xl border border-zinc-200 bg-white/70 shadow-sm backdrop-blur">
      <div className="flex items-center justify-between border-b border-zinc-200 px-4 py-3">
        <div className="font-semibold text-zinc-900">{columnTitle(status)}</div>
        <span className="rounded-full border border-zinc-200 bg-zinc-50 px-2 py-0.5 text-xs font-semibold text-zinc-500">
          {tickets.length}
        </span>
      </div>
      <div
        ref={setNodeRef}
        className={[
          "flex-1 space-y-2 overflow-y-auto rounded-b-3xl p-2 transition",
          isOver ? "bg-zinc-100" : "",
        ].join(" ")}
      >
        <SortableContext items={tickets.map((ticket) => ticket.id)} strategy={verticalListSortingStrategy}>
          {tickets.map((ticket) => (
            <TicketCard key={ticket.id} ticket={ticket} onOpen={onOpen} />
          ))}
        </SortableContext>
        {tickets.length === 0 ? (
          <div className="flex h-24 items-center justify-center rounded-2xl border border-dashed border-zinc-300 text-xs text-zinc-400">
            Empty
          </div>
        ) : null}
      </div>
    </section>
  );
}

export function TicketBoard({
  tickets,
  onOpenTicket,
  onMoveTicket,
  emptyAction,
}: {
  tickets: Ticket[];
  onOpenTicket: (ticketId: ID) => void;
  onMoveTicket: (ticketId: ID, status: TicketStatus) => void;
  emptyAction?: ReactNode;
}) {
  const [activeTicket, setActiveTicket] = useState<Ticket | null>(null);
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }));

  const grouped = useMemo(() => {
    const columns = new Map<TicketStatus, Ticket[]>();
    ticketStatuses.forEach((status) => columns.set(status.id, []));
    tickets.forEach((ticket) => {
      columns.get(ticket.status)?.push(ticket);
    });
    return columns;
  }, [tickets]);

  function handleDragStart(event: DragStartEvent) {
    const ticket = tickets.find((item) => item.id === String(event.active.id));
    setActiveTicket(ticket || null);
  }

  function handleDragEnd(event: DragEndEvent) {
    const activeId = String(event.active.id);
    const overId = event.over?.id ? String(event.over.id) : "";
    setActiveTicket(null);

    const ticket = tickets.find((item) => item.id === activeId);
    if (!ticket || !overId) {
      return;
    }

    const targetStatus = ticketStatuses.find((status) => status.id === overId)?.id;
    if (targetStatus && targetStatus !== ticket.status) {
      onMoveTicket(ticket.id, targetStatus);
    }
  }

  if (tickets.length === 0) {
    return <EmptyState title="No tickets yet" body="Create a ticket to start shaping the board." action={emptyAction} />;
  }

  return (
    <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
      <div className="flex gap-4 overflow-x-auto pb-3">
        {ticketStatuses.map((status) => (
          <BoardColumn key={status.id} status={status.id} tickets={grouped.get(status.id) || []} onOpen={onOpenTicket} />
        ))}
      </div>
      <DragOverlay>
        {activeTicket ? (
          <div className="w-[290px]">
            <Panel className="p-3 shadow-2xl">
              <div className="mb-2 flex gap-1.5">
                <StatusBadge value={activeTicket.type} kind="type" />
                <StatusBadge value={activeTicket.priority} kind="priority" />
              </div>
              <h3 className="text-sm font-semibold text-slate-950">{activeTicket.title}</h3>
            </Panel>
          </div>
        ) : null}
      </DragOverlay>
    </DndContext>
  );
}
