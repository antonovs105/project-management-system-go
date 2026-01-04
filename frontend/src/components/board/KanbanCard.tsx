import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { GripVertical } from 'lucide-react';

interface Ticket {
    id: number;
    title: string;
    type: string;
    priority: string;
}

interface KanbanCardProps {
    ticket: Ticket;
    onClick?: () => void;
}

export function KanbanCard({ ticket, onClick }: KanbanCardProps) {
    const {
        attributes,
        listeners,
        setNodeRef,
        transform,
        transition,
        isDragging,
    } = useSortable({
        id: ticket.id.toString(),
        data: {
            type: 'Ticket',
            ticket,
        },
    });

    const style = {
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.5 : 1,
    };

    const getPriorityColor = (p: string) => {
        switch (p) {
            case 'high': return 'bg-red-100 text-red-800';
            case 'medium': return 'bg-yellow-100 text-yellow-800';
            default: return 'bg-slate-100 text-slate-800';
        }
    };

    const getTypeIcon = (t: string) => {
        // Just simple indicators
        if (t === 'epic') return <span className="text-purple-600 font-bold text-xs uppercase">Epic</span>;
        if (t === 'subtask') return <span className="text-slate-500 font-bold text-xs uppercase">Sub</span>;
        return <span className="text-blue-600 font-bold text-xs uppercase">Task</span>;
    };

    return (
        <div ref={setNodeRef} style={style} className="touch-none" onClick={onClick}>
            <Card className="cursor-grab active:cursor-grabbing hover:shadow-md transition-shadow relative group">
                <CardContent className="p-3">
                    <div className="flex justify-between items-start gap-2">
                        <div className="flex-1">
                            <div className="flex items-center justify-between mb-2">
                                {getTypeIcon(ticket.type)}
                                <Badge variant="outline" className={`text-[10px] px-1 py-0 ${getPriorityColor(ticket.priority)} border-0`}>
                                    {ticket.priority}
                                </Badge>
                            </div>
                            <h4 className="text-sm font-medium leading-tight text-slate-800 line-clamp-2">
                                {ticket.title}
                            </h4>
                        </div>
                        <div {...attributes} {...listeners} className="text-slate-300 hover:text-slate-500 cursor-grab opacity-0 group-hover:opacity-100 transition-opacity">
                            <GripVertical size={16} />
                        </div>
                    </div>
                </CardContent>
            </Card>
        </div>
    );
}
