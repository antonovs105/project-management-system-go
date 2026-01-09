
import { useState, useMemo, useRef } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
    DndContext,
    DragOverlay,
    useSensors,
    useSensor,
    PointerSensor,
    type DragStartEvent,
    type DragOverEvent,
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
    const initialStatusRef = useRef<string | null>(null);

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
        onMutate: async (newTicket) => {
            // Cancel any outgoing refetches (so they don't overwrite our optimistic update)
            await queryClient.cancelQueries({ queryKey: ['tickets', projectId] });

            // Snapshot the previous value
            const previousTickets = queryClient.getQueryData<Ticket[]>(['tickets', projectId]);

            // Optimistically update to the new value
            if (previousTickets) {
                queryClient.setQueryData<Ticket[]>(['tickets', projectId], (old) =>
                    old?.map((t) => (t.id === newTicket.id ? { ...t, status: newTicket.status } : t))
                );
            }

            return { previousTickets };
        },
        onError: (_err, _newTicket, context) => {
            // If the mutation fails, use the context returned from onMutate to roll back
            if (context?.previousTickets) {
                queryClient.setQueryData(['tickets', projectId], context.previousTickets);
            }
        },
        onSettled: () => {
            // Always refetch after error or success to make sure the server-side state is synced
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
            const ticket = event.active.data.current.ticket as Ticket;
            setActiveTicket(ticket);
            initialStatusRef.current = ticket.status;
        }
    }

    function onDragOver(event: DragOverEvent) {
        const { active, over } = event;
        if (!over) return;

        const activeId = active.id;
        const overId = over.id;

        if (activeId === overId) return;

        const isActiveATask = active.data.current?.type === 'Ticket';
        if (!isActiveATask) return;

        // Find the target column ID
        let overColumnId = overId as string;
        const isOverATask = over.data.current?.type === 'Ticket';
        if (isOverATask) {
            overColumnId = over.data.current?.ticket.status;
        }

        // Get the current status from the cache to avoid redundant updates
        const currentTickets = queryClient.getQueryData<Ticket[]>(['tickets', projectId]) || [];
        const ticketInCache = currentTickets.find(t => t.id.toString() === activeId);

        if (ticketInCache && ticketInCache.status !== overColumnId) {
            // Live update the cache
            queryClient.setQueryData<Ticket[]>(['tickets', projectId], (old = []) => {
                return old.map(t => t.id === ticketInCache.id ? { ...t, status: overColumnId } : t);
            });
        }
    }

    function onDragEnd(event: DragEndEvent) {
        const { active, over } = event;
        setActiveTicket(null);

        if (!over) {
            initialStatusRef.current = null;
            return;
        }

        const activeTicketData = active.data.current?.ticket as Ticket;
        if (!activeTicketData) {
            initialStatusRef.current = null;
            return;
        }

        // Find the final status after drag end
        let finalStatus = over.id as string;
        if (over.data.current?.type === 'Ticket') {
            finalStatus = over.data.current.ticket.status;
        }

        // Trigger mutation only if the status actually changed from the START of the drag
        if (initialStatusRef.current && finalStatus !== initialStatusRef.current) {
            updateTicketStatus.mutate({ id: activeTicketData.id, status: finalStatus });
        }

        initialStatusRef.current = null;
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
