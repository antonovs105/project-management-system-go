
import { useState, useMemo } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
    DndContext,
    DragOverlay,
    useSensors,
    useSensor,
    PointerSensor,
    type DragStartEvent,
    type DragEndEvent,
    defaultDropAnimationSideEffects,
    type DropAnimation,
} from '@dnd-kit/core';
import { createPortal } from 'react-dom';
import api from '@/lib/axios';
import { KanbanColumn } from '@/components/board/KanbanColumn';
import { KanbanCard } from '@/components/board/KanbanCard';
import { CreateTicketDialog } from '@/components/ticket/CreateTicketDialog';
import { Button } from '@/components/ui/button';
import { Network } from 'lucide-react';

interface Ticket {
    id: number;
    title: string;
    description: string;
    status: string;
    priority: string;
    type: string;
    parent_id?: number;
}

const COLUMNS = [
    { id: 'open', title: 'To Do' },
    { id: 'in_progress', title: 'In Progress' },
    { id: 'review', title: 'Review' },
    { id: 'done', title: 'Done' },
];

export default function BoardPage() {
    const { projectId } = useParams();
    const queryClient = useQueryClient();
    const [activeTicket, setActiveTicket] = useState<Ticket | null>(null);

    const { data: tickets = [], isLoading } = useQuery<Ticket[]>({
        queryKey: ['tickets', projectId],
        queryFn: async () => {
            const res = await api.get(`/api/projects/${projectId}/tickets`);
            return res.data || [];
        },
        enabled: !!projectId,
    });

    const updateTicketStatus = useMutation({
        mutationFn: async ({ id, status }: { id: number; status: string }) => {
            await api.patch(`/api/tickets/${id}`, { status });
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['tickets', projectId] });
        },
    });

    const sensors = useSensors(
        useSensor(PointerSensor, {
            activationConstraint: {
                distance: 5,
            },
        })
    );

    const columns = useMemo(() => {
        const cols: Record<string, Ticket[]> = {
            open: [],
            in_progress: [],
            review: [],
            done: [],
        };

        // Normalize status just in case
        (tickets || []).forEach(ticket => {
            const status = ticket.status || 'open';
            if (cols[status]) {
                cols[status].push(ticket);
            } else {
                // Fallback to open if status unknown
                cols['open'].push(ticket);
            }
        });
        return cols;
    }, [tickets]);

    function onDragStart(event: DragStartEvent) {
        if (event.active.data.current?.type === 'Ticket') {
            setActiveTicket(event.active.data.current.ticket);
        }
    }

    function onDragEnd(event: DragEndEvent) {
        const { active, over } = event;
        setActiveTicket(null);

        if (!over) return;

        const overId = over.id;

        // We dropped on a column?

        // Or did we drop on a card?
        if (active.data.current?.sortable?.containerId !== over.data.current?.sortable?.containerId) {
            // If sorting context changes, update status
            // Actually simpler to just check if `over` is a column ID
            // If over is a ticket, we need to find which column it belongs to.
            // But wait, the column droppable ID is the status.
            // The SortableContext items are ticket IDs.
            // dnd-kit is flexible. 

            // Let's rely on onDragOver to handle optimistic updates if we wanted smooth sorting between columns
            // But for simple Kanban status change:
            const activeTicketData = active.data.current?.ticket as Ticket;

            // Check if dropped on a container (Column)
            if (COLUMNS.some(c => c.id === overId)) {
                if (activeTicketData.status !== overId) {
                    updateTicketStatus.mutate({ id: activeTicketData.id, status: overId as string });
                }
                return;
            }

            // Dropped on another ticket?
            const overTicketData = over.data.current?.ticket as Ticket;
            if (overTicketData && activeTicketData.status !== overTicketData.status) {
                updateTicketStatus.mutate({ id: activeTicketData.id, status: overTicketData.status });
            }
        }
    }

    function onDragOver() {
        // Optional: Logic for reordering within column (not persisted on backend yet needed for UI flow)
    }

    const dropAnimation: DropAnimation = {
        sideEffects: defaultDropAnimationSideEffects({
            styles: {
                active: {
                    opacity: '0.5',
                },
            },
        }),
    };

    if (isLoading) return <div className="p-8">Loading board...</div>;

    return (
        <div className="h-full flex flex-col space-y-4">
            <div className="flex items-center justify-between px-2">
                <h2 className="text-2xl font-bold">Board</h2>
                <div className="flex items-center gap-2">
                    <Button variant="outline" asChild className="gap-2">
                        <Link to={`/projects/${projectId}/graph`}>
                            <Network size={16} /> Graph View
                        </Link>
                    </Button>
                    <CreateTicketDialog />
                </div>
            </div>

            <DndContext
                sensors={sensors}
                onDragStart={onDragStart}
                onDragEnd={onDragEnd}
                onDragOver={onDragOver}
            >
                <div className="flex-1 flex gap-4 overflow-x-auto pb-4 px-2">
                    {COLUMNS.map((col) => (
                        <KanbanColumn
                            key={col.id}
                            id={col.id}
                            title={col.title}
                            tickets={columns[col.id]}
                            onTicketClick={(id) => console.log('Edit ticket', id)}
                        />
                    ))}
                </div>

                {createPortal(
                    <DragOverlay dropAnimation={dropAnimation}>
                        {activeTicket && (
                            <div className="w-[300px]">
                                <KanbanCard ticket={activeTicket} />
                            </div>
                        )}
                    </DragOverlay>,
                    document.body
                )}
            </DndContext>
        </div>
    );
}
