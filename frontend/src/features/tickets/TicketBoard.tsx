import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import {
  arrayMove,
  sortableKeyboardCoordinates,
  SortableContext,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { CalendarClock, GripVertical, UserRound } from "lucide-react";
import { useMemo, useState } from "react";
import type { ReactNode } from "react";
import { StatusBadge } from "../../components/StatusBadge";
import { EmptyState, Panel } from "../../components/ui";
import { ticketStatuses } from "../../lib/constants";
import type { ID, Label, ProjectMember, Ticket, TicketStatus } from "../../types";
import { projectMemberLabel } from "./memberLabels";

function columnTitle(status: TicketStatus): string {
  return ticketStatuses.find((item) => item.id === status)?.label || status;
}

function TicketCard({
  ticket,
  members,
  labels,
  onOpen,
  draggable,
}: {
  ticket: Ticket;
  members: ProjectMember[];
  labels: Label[];
  onOpen: (ticketId: ID) => void;
  draggable: boolean;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: ticket.id,
    data: { ticket },
    disabled: !draggable,
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
      className={[
        "focus-ring w-full touch-none rounded-2xl text-left",
        draggable ? "cursor-grab active:cursor-grabbing" : "cursor-pointer",
      ].join(" ")}
      onClick={() => onOpen(ticket.id)}
      {...(draggable ? attributes : {})}
      {...(draggable ? listeners : {})}
    >
      <Panel className="group p-3 transition hover:-translate-y-0.5 hover:border-zinc-300 hover:shadow-md">
        <div className="flex items-start gap-2">
          <div className="min-w-0 flex-1">
            <div className="mb-2 flex flex-wrap gap-1.5">
              <StatusBadge value={ticket.type} kind="type" />
              <StatusBadge value={ticket.priority} kind="priority" />
            </div>
            {ticket.label_ids?.length ? (
              <div className="mb-2 flex flex-wrap gap-1">
                {labels.filter((label) => ticket.label_ids.includes(label.id)).map((label) => (
                  <span key={label.id} className="rounded-full px-2 py-0.5 text-[11px] font-medium text-white" style={{ backgroundColor: label.color }}>
                    {label.name}
                  </span>
                ))}
              </div>
            ) : null}
            <h3 className="line-clamp-2 text-sm font-semibold leading-5 text-zinc-950">{ticket.title}</h3>
            {ticket.description ? <p className="mt-2 line-clamp-2 text-xs text-zinc-500">{ticket.description}</p> : null}
            <div className="mt-3 flex items-center gap-1.5 text-xs text-zinc-500">
              <UserRound size={13} />
              <span className="truncate">{projectMemberLabel(members, ticket.assignee_id)}</span>
            </div>
            {ticket.due_date ? (
              <div className="mt-1 flex items-center gap-1.5 text-xs text-zinc-500">
                <CalendarClock size={13} />
                <span>Due {new Date(ticket.due_date).toLocaleDateString()}</span>
              </div>
            ) : null}
          </div>
          <span
            className="pointer-events-none flex h-7 w-7 shrink-0 items-center justify-center rounded-xl text-zinc-300 transition group-hover:text-zinc-500"
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
  members,
  labels,
  onOpen,
  draggable,
}: {
  status: TicketStatus;
  tickets: Ticket[];
  members: ProjectMember[];
  labels: Label[];
  onOpen: (ticketId: ID) => void;
  draggable: boolean;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: status });

  return (
    <section className="flex min-h-[520px] min-w-[290px] flex-1 flex-col rounded-3xl border border-zinc-200 bg-white/70 shadow-sm backdrop-blur dark:border-zinc-800 dark:bg-zinc-900/70">
      <div className="flex items-center justify-between border-b border-zinc-200 px-4 py-3 dark:border-zinc-800">
        <div className="font-semibold text-zinc-900 dark:text-zinc-100">{columnTitle(status)}</div>
        <span className="rounded-full border border-zinc-200 bg-zinc-50 px-2 py-0.5 text-xs font-semibold text-zinc-500 dark:border-zinc-800 dark:bg-zinc-950 dark:text-zinc-400">
          {tickets.length}
        </span>
      </div>
      <div
        ref={setNodeRef}
        className={[
          "flex-1 space-y-2 overflow-y-auto rounded-b-3xl p-2 transition",
          isOver ? "bg-zinc-100 dark:bg-zinc-800/50" : "",
        ].join(" ")}
      >
        <SortableContext items={tickets.map((ticket) => ticket.id)} strategy={verticalListSortingStrategy}>
          {tickets.map((ticket) => (
            <TicketCard key={ticket.id} ticket={ticket} members={members} labels={labels} onOpen={onOpen} draggable={draggable} />
          ))}
        </SortableContext>
        {tickets.length === 0 ? (
          <div className="flex h-24 items-center justify-center rounded-2xl border border-dashed border-zinc-300 text-xs text-zinc-400 dark:border-zinc-700 dark:text-zinc-500">
            Empty
          </div>
        ) : null}
      </div>
    </section>
  );
}

function neighbors(items: Ticket[], ticketId: ID) {
  const index = items.findIndex((item) => item.id === ticketId);
  return {
    beforeTicketId: index >= 0 ? items[index + 1]?.id || null : null,
    afterTicketId: index >= 0 ? items[index - 1]?.id || null : null,
  };
}

export function TicketBoard({
  tickets,
  members,
  labels = [],
  onOpenTicket,
  onMoveTicket,
  emptyAction,
  readOnly = false,
  showColumnsWhenEmpty = false,
  emptyTitle = "No tickets yet",
  emptyBody = "Create a ticket to start shaping the board.",
}: {
  tickets: Ticket[];
  members: ProjectMember[];
  labels?: Label[];
  onOpenTicket: (ticketId: ID) => void;
  onMoveTicket: (ticketId: ID, status: TicketStatus, beforeTicketId: ID | null, afterTicketId: ID | null) => void;
  emptyAction?: ReactNode;
  readOnly?: boolean;
  showColumnsWhenEmpty?: boolean;
  emptyTitle?: string;
  emptyBody?: string;
}) {
  const [activeTicket, setActiveTicket] = useState<Ticket | null>(null);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const grouped = useMemo(() => {
    const columns = new Map<TicketStatus, Ticket[]>();
    ticketStatuses.forEach((status) => columns.set(status.id, []));
    tickets.forEach((ticket) => {
      columns.get(ticket.status)?.push(ticket);
    });
    return columns;
  }, [tickets]);

  function statusFromOverId(overId: string): TicketStatus | null {
    const status = ticketStatuses.find((item) => item.id === overId)?.id;
    if (status) {
      return status;
    }
    return tickets.find((ticket) => ticket.id === overId)?.status || null;
  }

  function handleDragStart(event: DragStartEvent) {
    if (readOnly) {
      return;
    }
    const ticket = tickets.find((item) => item.id === String(event.active.id));
    setActiveTicket(ticket || null);
  }

  function handleDragEnd(event: DragEndEvent) {
    if (readOnly) {
      setActiveTicket(null);
      return;
    }
    const activeId = String(event.active.id);
    const overId = event.over?.id ? String(event.over.id) : "";
    const ticket = tickets.find((item) => item.id === activeId);
    if (!ticket || !overId) {
      setActiveTicket(null);
      return;
    }

    const targetStatus = statusFromOverId(overId);
    if (!targetStatus) {
      setActiveTicket(null);
      return;
    }

    const targetOriginal = grouped.get(targetStatus) || [];
    let nextTarget: Ticket[];
    if (targetStatus === ticket.status) {
      const oldIndex = targetOriginal.findIndex((item) => item.id === activeId);
      const newIndex = targetOriginal.findIndex((item) => item.id === overId);
      if (oldIndex < 0) {
        setActiveTicket(null);
        return;
      }
      nextTarget =
        newIndex >= 0 ? arrayMove(targetOriginal, oldIndex, newIndex) : [...targetOriginal.filter((item) => item.id !== activeId), ticket];
    } else {
      const withoutActive = targetOriginal.filter((item) => item.id !== activeId);
      const overIndex = withoutActive.findIndex((item) => item.id === overId);
      const insertIndex = overIndex >= 0 ? overIndex : withoutActive.length;
      nextTarget = [
        ...withoutActive.slice(0, insertIndex),
        { ...ticket, status: targetStatus },
        ...withoutActive.slice(insertIndex),
      ];
    }

    const previousNeighbors = neighbors(grouped.get(ticket.status) || [], ticket.id);
    const nextNeighbors = neighbors(nextTarget, ticket.id);
    if (
      targetStatus !== ticket.status ||
      previousNeighbors.beforeTicketId !== nextNeighbors.beforeTicketId ||
      previousNeighbors.afterTicketId !== nextNeighbors.afterTicketId
    ) {
      onMoveTicket(ticket.id, targetStatus, nextNeighbors.beforeTicketId, nextNeighbors.afterTicketId);
    }
    setActiveTicket(null);
  }

  if (tickets.length === 0 && !showColumnsWhenEmpty) {
    return <EmptyState title={emptyTitle} body={emptyBody} action={emptyAction} />;
  }

  return (
    <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
      <div className="flex gap-4 overflow-x-auto pb-3">
        {ticketStatuses.map((status) => (
          <BoardColumn
            key={status.id}
            status={status.id}
            tickets={grouped.get(status.id) || []}
            members={members}
            labels={labels}
            onOpen={onOpenTicket}
            draggable={!readOnly}
          />
        ))}
      </div>
      <DragOverlay dropAnimation={null}>
        {activeTicket ? (
          <div className="w-[290px]">
            <Panel className="p-3 shadow-2xl">
              <div className="mb-2 flex gap-1.5">
                <StatusBadge value={activeTicket.type} kind="type" />
                <StatusBadge value={activeTicket.priority} kind="priority" />
              </div>
              <h3 className="text-sm font-semibold text-zinc-950 dark:text-zinc-50">{activeTicket.title}</h3>
            </Panel>
          </div>
        ) : null}
      </DragOverlay>
    </DndContext>
  );
}
