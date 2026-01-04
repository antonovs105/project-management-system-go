import { useDroppable } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { KanbanCard } from './KanbanCard';

interface Ticket {
    id: number;
    title: string;
    type: string;
    priority: string;
    status: string;
}

interface KanbanColumnProps {
    id: string;
    title: string;
    tickets: Ticket[];
    onTicketClick?: (ticketId: number) => void;
}

export function KanbanColumn({ id, title, tickets, onTicketClick }: KanbanColumnProps) {
    const { setNodeRef } = useDroppable({
        id: id,
    });

    return (
        <div className="flex flex-col h-full bg-slate-100/50 rounded-xl border border-slate-200 min-w-[280px] w-[300px]">
            <div className="p-3 border-b border-slate-200 flex justify-between items-center bg-white/50 rounded-t-xl backdrop-blur-sm">
                <h3 className="font-semibold text-slate-700 text-sm flex items-center gap-2">
                    {title}
                    <span className="bg-slate-200 text-slate-600 px-2 py-0.5 rounded-full text-xs font-bold">
                        {tickets.length}
                    </span>
                </h3>
            </div>

            <div ref={setNodeRef} className="flex-1 p-2 space-y-2 overflow-y-auto min-h-[100px]">
                <SortableContext items={tickets.map(t => t.id.toString())} strategy={verticalListSortingStrategy}>
                    {tickets.map((ticket) => (
                        <KanbanCard key={ticket.id} ticket={ticket} onClick={() => onTicketClick && onTicketClick(ticket.id)} />
                    ))}
                </SortableContext>
                {tickets.length === 0 && (
                    <div className="h-24 border-2 border-dashed border-slate-200 rounded-lg flex items-center justify-center text-slate-400 text-xs italic">
                        Drop here
                    </div>
                )}
            </div>
        </div>
    );
}
