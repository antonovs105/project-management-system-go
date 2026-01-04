import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import * as z from 'zod';
import { useQueryClient, useQuery } from '@tanstack/react-query';
import api from '@/lib/axios';
import { toast } from 'sonner';
import { useParams } from 'react-router-dom';

import { Button } from '@/components/ui/button';
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "@/components/ui/dialog";
import {
    Form,
    FormControl,
    FormField,
    FormItem,
    FormLabel,
    FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

const formSchema = z.object({
    title: z.string().min(1, "Title is required"),
    description: z.string(),
    priority: z.enum(["low", "medium", "high"]),
    type: z.enum(["epic", "task", "subtask"]),
    parent_id: z.string().optional(), // We'll parse to number on submit
});

interface Ticket {
    id: number;
    title: string;
    type: string;
}

export function CreateTicketDialog({ children }: { children?: React.ReactNode }) {
    const { projectId } = useParams();
    const [open, setOpen] = useState(false);
    const [loading, setLoading] = useState(false);
    const queryClient = useQueryClient();

    const form = useForm<z.infer<typeof formSchema>>({
        resolver: zodResolver(formSchema),
        defaultValues: {
            title: '',
            description: '',
            priority: 'medium',
            type: 'task',
            parent_id: '',
        },
    });

    const type = form.watch("type");

    // Fetch potential parents if type is task or subtask
    const { data: potentialParents } = useQuery<Ticket[]>({
        queryKey: ['tickets', projectId],
        queryFn: async () => {
            if (!projectId) return [];
            const res = await api.get(`/api/projects/${projectId}/tickets`);
            return res.data;
        },
        enabled: !!projectId && open && (type === 'task' || type === 'subtask'),
    });

    const filteredParents = potentialParents?.filter(t => {
        if (type === 'task') return t.type === 'epic';
        if (type === 'subtask') return t.type === 'task';
        return false;
    }) || [];

    async function onSubmit(values: z.infer<typeof formSchema>) {
        if (!projectId) return;
        setLoading(true);
        try {
            const payload: any = {
                title: values.title,
                description: values.description,
                priority: values.priority,
                type: values.type,
            };
            if (values.parent_id && values.parent_id !== '0') {
                payload.parent_id = parseInt(values.parent_id);
            }

            await api.post(`/api/projects/${projectId}/tickets`, payload);
            toast.success('Ticket created');
            queryClient.invalidateQueries({ queryKey: ['tickets', projectId] });
            setOpen(false);
            form.reset();
        } catch (error: any) {
            const msg = error.response?.data?.error || 'Failed to create ticket';
            toast.error(msg);
        } finally {
            setLoading(false);
        }
    }

    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
                {children || <Button>Create Ticket</Button>}
            </DialogTrigger>
            <DialogContent className="sm:max-w-[500px]">
                <DialogHeader>
                    <DialogTitle>Create Ticket</DialogTitle>
                    <DialogDescription>
                        Add a new ticket to this project.
                    </DialogDescription>
                </DialogHeader>
                <Form {...form}>
                    <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
                        <FormField
                            control={form.control}
                            name="title"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Title</FormLabel>
                                    <FormControl>
                                        <Input placeholder="Ticket title" {...field} />
                                    </FormControl>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <div className="grid grid-cols-2 gap-4">
                            <FormField
                                control={form.control}
                                name="type"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Type</FormLabel>
                                        <Select onValueChange={field.onChange} defaultValue={field.value}>
                                            <FormControl>
                                                <SelectTrigger>
                                                    <SelectValue placeholder="Select type" />
                                                </SelectTrigger>
                                            </FormControl>
                                            <SelectContent>
                                                <SelectItem value="epic">Epic</SelectItem>
                                                <SelectItem value="task">Task</SelectItem>
                                                <SelectItem value="subtask">Subtask</SelectItem>
                                            </SelectContent>
                                        </Select>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <FormField
                                control={form.control}
                                name="priority"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Priority</FormLabel>
                                        <Select onValueChange={field.onChange} defaultValue={field.value}>
                                            <FormControl>
                                                <SelectTrigger>
                                                    <SelectValue placeholder="Select priority" />
                                                </SelectTrigger>
                                            </FormControl>
                                            <SelectContent>
                                                <SelectItem value="low">Low</SelectItem>
                                                <SelectItem value="medium">Medium</SelectItem>
                                                <SelectItem value="high">High</SelectItem>
                                            </SelectContent>
                                        </Select>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                        </div>

                        {(type === 'task' || type === 'subtask') && (
                            <FormField
                                control={form.control}
                                name="parent_id"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Parent Ticket</FormLabel>
                                        <Select onValueChange={field.onChange} value={field.value}>
                                            <FormControl>
                                                <SelectTrigger>
                                                    <SelectValue placeholder={`Select ${type === 'task' ? 'Epic' : 'Task'}`} />
                                                </SelectTrigger>
                                            </FormControl>
                                            <SelectContent>
                                                {filteredParents.map(p => (
                                                    <SelectItem key={p.id} value={p.id.toString()}>
                                                        {p.title}
                                                    </SelectItem>
                                                ))}
                                            </SelectContent>
                                        </Select>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                        )}

                        <FormField
                            control={form.control}
                            name="description"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Description</FormLabel>
                                    <FormControl>
                                        <Textarea placeholder="Ticket description..." {...field} />
                                    </FormControl>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <DialogFooter>
                            <Button type="submit" disabled={loading}>
                                {loading ? 'Creating...' : 'Create'}
                            </Button>
                        </DialogFooter>
                    </form>
                </Form>
            </DialogContent>
        </Dialog>
    );
}
