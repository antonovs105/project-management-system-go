import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { Button, ErrorState, LoadingState, Panel, TextField } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";
import { useI18n } from "../../lib/i18n-context";
import type { ID } from "../../types";

export function ProjectLabelsPanel({ projectId }: { projectId: ID }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [color, setColor] = useState("#3B82F6");
  const labels = useQuery({ queryKey: queryKeys.projectLabels(projectId), queryFn: () => api.listProjectLabels(projectId) });
  const createLabel = useMutation({
    mutationFn: () => api.createProjectLabel(projectId, { name: name.trim(), color }),
    onSuccess: async () => {
      setName("");
      await queryClient.invalidateQueries({ queryKey: queryKeys.projectLabels(projectId) });
      toast.success(t("project.labelCreated"));
    },
  });
  const deleteLabel = useMutation({
    mutationFn: (labelId: ID) => api.deleteProjectLabel(projectId, labelId),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.projectLabels(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.ticketsScope(projectId) }),
      ]);
      toast.success(t("project.labelDeleted"));
    },
  });

  return (
    <Panel className="p-4">
      <h3 className="text-base font-semibold text-zinc-950">{t("project.labelsTitle")}</h3>
      <p className="mt-1 text-sm text-zinc-500">{t("project.labelsBody")}</p>
      <form
        className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-end"
        onSubmit={(event) => {
          event.preventDefault();
          if (name.trim()) createLabel.mutate();
        }}
      >
        <TextField label={t("project.labelName")} value={name} maxLength={50} onChange={(event) => setName(event.target.value)} required />
        <TextField label={t("project.labelColor")} type="color" value={color} onChange={(event) => setColor(event.target.value)} />
        <Button type="submit" disabled={!name.trim() || createLabel.isPending}>
          <Plus size={16} /> {t("project.labelAdd")}
        </Button>
      </form>
      {labels.isLoading ? <LoadingState label={t("project.labelsLoading")} /> : null}
      {labels.isError ? <ErrorState title={t("project.labelsLoadFailed")} body={errorMessage(labels.error, t("project.labelsRequestFailed"))} /> : null}
      {createLabel.isError ? <ErrorState title={t("project.labelCreateFailed")} body={errorMessage(createLabel.error, t("project.labelCreationFailed"))} /> : null}
      <div className="mt-4 flex flex-wrap gap-2">
        {(labels.data || []).map((label) => (
          <div key={label.id} className="flex items-center gap-2 rounded-full border border-zinc-200 px-3 py-1.5 text-sm">
            <span className="h-3 w-3 rounded-full" style={{ backgroundColor: label.color }} />
            <span>{label.name}</span>
            <button type="button" aria-label={t("project.labelDelete", { name: label.name })} onClick={() => deleteLabel.mutate(label.id)}>
              <Trash2 size={14} />
            </button>
          </div>
        ))}
      </div>
    </Panel>
  );
}
