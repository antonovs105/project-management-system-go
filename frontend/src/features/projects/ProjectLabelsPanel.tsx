import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { Button, ErrorState, LoadingState, Panel, TextField } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";
import type { ID } from "../../types";

export function ProjectLabelsPanel({ projectId }: { projectId: ID }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [color, setColor] = useState("#3B82F6");
  const labels = useQuery({ queryKey: queryKeys.projectLabels(projectId), queryFn: () => api.listProjectLabels(projectId) });
  const createLabel = useMutation({
    mutationFn: () => api.createProjectLabel(projectId, { name: name.trim(), color }),
    onSuccess: async () => {
      setName("");
      await queryClient.invalidateQueries({ queryKey: queryKeys.projectLabels(projectId) });
      toast.success("Label created");
    },
  });
  const deleteLabel = useMutation({
    mutationFn: (labelId: ID) => api.deleteProjectLabel(projectId, labelId),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.projectLabels(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.ticketsScope(projectId) }),
      ]);
      toast.success("Label deleted");
    },
  });

  return (
    <Panel className="p-4">
      <h3 className="text-base font-semibold text-zinc-950">Ticket labels</h3>
      <p className="mt-1 text-sm text-zinc-500">Create project tags that can be assigned from ticket forms.</p>
      <form
        className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-end"
        onSubmit={(event) => {
          event.preventDefault();
          if (name.trim()) createLabel.mutate();
        }}
      >
        <TextField label="Label name" value={name} maxLength={50} onChange={(event) => setName(event.target.value)} required />
        <TextField label="Color" type="color" value={color} onChange={(event) => setColor(event.target.value)} />
        <Button type="submit" disabled={!name.trim() || createLabel.isPending}>
          <Plus size={16} /> Add label
        </Button>
      </form>
      {labels.isLoading ? <LoadingState label="Loading labels" /> : null}
      {labels.isError ? <ErrorState title="Could not load labels" body={errorMessage(labels.error, "Label request failed.")} /> : null}
      {createLabel.isError ? <ErrorState title="Could not create label" body={errorMessage(createLabel.error, "Label creation failed.")} /> : null}
      <div className="mt-4 flex flex-wrap gap-2">
        {(labels.data || []).map((label) => (
          <div key={label.id} className="flex items-center gap-2 rounded-full border border-zinc-200 px-3 py-1.5 text-sm">
            <span className="h-3 w-3 rounded-full" style={{ backgroundColor: label.color }} />
            <span>{label.name}</span>
            <button type="button" aria-label={`Delete ${label.name}`} onClick={() => deleteLabel.mutate(label.id)}>
              <Trash2 size={14} />
            </button>
          </div>
        ))}
      </div>
    </Panel>
  );
}
