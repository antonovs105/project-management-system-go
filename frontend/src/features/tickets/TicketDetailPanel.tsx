import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, Github, Link2, MessageSquare, Paperclip, Save, Trash2, Unlink, X } from "lucide-react";
import { useMemo, useState } from "react";
import type { FormEvent } from "react";
import { createPortal } from "react-dom";
import { toast } from "sonner";
import { api, attachmentContentURL, errorMessage, type UpdateTicketPayload } from "../../lib/api";
import { ticketLinkTypes, ticketPriorities, ticketStatuses, ticketTypes } from "../../lib/constants";
import { compactId } from "../../lib/format";
import { useI18n } from "../../lib/i18n-context";
import { fieldLimits } from "../../lib/limits";
import { queryKeys } from "../../lib/queryKeys";
import type { GitHubCommit, ID, Label, ProjectMember, Ticket, TicketPriority, TicketStatus, TicketType } from "../../types";
import { StatusBadge } from "../../components/StatusBadge";
import { Button, ErrorState, IconButton, LoadingState, SelectField, TextAreaField, TextField } from "../../components/ui";
import { OffsetPaginationControls } from "../../components/OffsetPaginationControls";
import { MemberAssigneeSelect } from "./MemberAssigneeSelect";

function parentCandidates(type: TicketType, ticketId: ID, tickets: Ticket[]): Ticket[] {
  if (type === "task") {
    return tickets.filter((ticket) => ticket.type === "epic" && ticket.id !== ticketId);
  }
  if (type === "subtask") {
    return tickets.filter((ticket) => ticket.type === "task" && ticket.id !== ticketId);
  }
  return [];
}

function TicketEditor({
  projectId,
  ticket,
  tickets,
  members,
  labels,
  onClose,
}: {
  projectId: ID;
  ticket: Ticket;
  tickets: Ticket[];
  members: ProjectMember[];
  labels: Label[];
  onClose: () => void;
}) {
  const { t, relativeDate } = useI18n();
  const queryClient = useQueryClient();
  const [title, setTitle] = useState(ticket.title);
  const [description, setDescription] = useState(ticket.description);
  const [status, setStatus] = useState<TicketStatus>(ticket.status);
  const [priority, setPriority] = useState<TicketPriority>(ticket.priority);
  const [type, setType] = useState<TicketType>(ticket.type);
  const [parentId, setParentId] = useState(ticket.parent_id || "");
  const [assigneeId, setAssigneeId] = useState(ticket.assignee_id || "");
  const [dueDate, setDueDate] = useState(ticket.due_date?.slice(0, 10) || "");
  const [labelIds, setLabelIds] = useState<ID[]>(ticket.label_ids || []);

  const parents = useMemo(() => parentCandidates(type, ticket.id, tickets), [ticket.id, tickets, type]);

  const updateTicket = useMutation({
    mutationFn: (payload: UpdateTicketPayload) => api.updateTicket(ticket.id, payload, ticket.version),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.ticket(ticket.id) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.ticketsScope(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.graphScope(projectId) }),
      ]);
      toast.success(t("ticket.updated"));
    },
  });

  const archiveTicket = useMutation({
	mutationFn: () => api.archiveTicket(ticket.id, ticket.version),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.ticketsScope(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.graphScope(projectId) }),
      ]);
	  toast.success(t("ticket.archived"));
      onClose();
    },
  });

  const capabilities = useQuery({ queryKey: queryKeys.instanceCapabilities, queryFn: api.getInstanceCapabilities });
  const attachments = useQuery({
    queryKey: queryKeys.ticketAttachments(ticket.id),
    queryFn: () => api.listTicketAttachments(ticket.id),
    enabled: capabilities.data?.attachments_enabled === true,
  });
  const uploadAttachment = useMutation({
    mutationFn: (file: File) => api.uploadTicketAttachment(ticket.id, file),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.ticketAttachments(ticket.id) });
	  toast.success(t("ticket.attachmentUploaded"));
    },
    onError: (error) => toast.error(errorMessage(error, t("ticket.attachmentUploadFailed"))),
  });
  const deleteAttachment = useMutation({
    mutationFn: api.deleteTicketAttachment,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.ticketAttachments(ticket.id) });
    },
    onError: (error) => toast.error(errorMessage(error, t("ticket.attachmentDeleteFailed"))),
  });

  function submitTicket(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    updateTicket.mutate({
      title: title.trim(),
      description: description.trim(),
      status,
      priority,
      type,
      parent_id: type === "epic" ? null : parentId || null,
      assignee_id: assigneeId.trim() || null,
      due_date: dueDate,
      label_ids: labelIds,
    });
  }

  return (
    <form className="grid gap-4" onSubmit={submitTicket}>
      {updateTicket.isError ? (
        <ErrorState title={t("ticket.updateFailed")} body={errorMessage(updateTicket.error, t("ticket.updateRequestFailed"))} />
      ) : null}
      {archiveTicket.isError ? (
		<ErrorState title={t("ticket.archiveFailed")} body={errorMessage(archiveTicket.error, t("ticket.archiveRequestFailed"))} />
      ) : null}
      <TextField
        label={t("ticket.title")}
        value={title}
        onChange={(event) => setTitle(event.target.value)}
        maxLength={fieldLimits.ticketTitleMaxLength}
        required
      />
      <div className="grid gap-4 sm:grid-cols-3">
        <SelectField label={t("workspace.status")} value={status} onChange={(event) => setStatus(event.target.value as TicketStatus)}>
          {ticketStatuses.map((item) => (
            <option key={item.id} value={item.id}>
              {item.label}
            </option>
          ))}
        </SelectField>
        <SelectField label={t("ticket.priority")} value={priority} onChange={(event) => setPriority(event.target.value as TicketPriority)}>
          {ticketPriorities.map((item) => (
            <option key={item.id} value={item.id}>
              {item.label}
            </option>
          ))}
        </SelectField>
        <SelectField label={t("ticket.type")} value={type} onChange={(event) => setType(event.target.value as TicketType)}>
          {ticketTypes.map((item) => (
            <option key={item.id} value={item.id}>
              {item.label}
            </option>
          ))}
        </SelectField>
      </div>
      {type !== "epic" ? (
        <SelectField label={t("ticket.parent")} value={parentId} onChange={(event) => setParentId(event.target.value)}>
          <option value="">{t("ticket.none")}</option>
          {parents.map((candidate) => (
            <option key={candidate.id} value={candidate.id}>
              {candidate.title}
            </option>
          ))}
        </SelectField>
      ) : null}
      <MemberAssigneeSelect members={members} value={assigneeId} onChange={setAssigneeId} />
      <TextField label={t("ticket.dueDate")} type="date" value={dueDate} onChange={(event) => setDueDate(event.target.value)} />
      {labels.length > 0 ? (
        <fieldset className="grid gap-2">
          <legend className="text-sm font-medium text-zinc-700">{t("ticket.labels")}</legend>
          <div className="flex flex-wrap gap-2">
            {labels.map((label) => (
              <label key={label.id} className="flex cursor-pointer items-center gap-2 rounded-full border border-zinc-200 px-3 py-1.5 text-sm">
                <input
                  type="checkbox"
                  checked={labelIds.includes(label.id)}
                  onChange={(event) => setLabelIds((current) => event.target.checked ? [...current, label.id] : current.filter((id) => id !== label.id))}
                />
                <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: label.color }} />
                {label.name}
              </label>
            ))}
          </div>
        </fieldset>
      ) : null}
      <TextAreaField
        label={t("ticket.description")}
        value={description}
        onChange={(event) => setDescription(event.target.value)}
        maxLength={fieldLimits.ticketDescriptionMaxLength}
      />
      {capabilities.data?.attachments_enabled ? (
        <fieldset className="grid gap-2 rounded-xl border border-zinc-200 p-3">
          <legend className="px-1 text-sm font-medium text-zinc-700">{t("ticket.attachments")}</legend>
          <label className="focus-ring inline-flex w-fit cursor-pointer items-center gap-2 rounded-full border border-zinc-200 px-3 py-2 text-sm font-medium">
            <Paperclip size={15} />
            {uploadAttachment.isPending ? t("ticket.uploading") : t("ticket.addFile")}
            <input
              className="sr-only"
              type="file"
              accept="image/png,image/jpeg,image/gif,image/webp,application/pdf,application/json,application/zip,text/plain,text/csv"
              disabled={uploadAttachment.isPending}
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) uploadAttachment.mutate(file);
                event.currentTarget.value = "";
              }}
            />
          </label>
          {attachments.isLoading ? <LoadingState label={t("ticket.attachmentsLoading")} /> : null}
          {attachments.isError ? (
            <ErrorState
              title={t("ticket.attachmentsLoadFailed")}
              body={errorMessage(attachments.error, t("ticket.attachmentRequestFailed"))}
            />
          ) : null}
          {(attachments.data || []).map((attachment) => (
            <div key={attachment.id} className="flex items-center justify-between gap-2 rounded-lg bg-zinc-50 p-2 text-sm">
              <div className="min-w-0">
                <div className="truncate font-medium">{attachment.filename}</div>
                <div className="text-xs text-zinc-500">
                  {Math.ceil(attachment.size_bytes / 1024)} KiB / {relativeDate(attachment.created_at)}
                </div>
              </div>
              <div className="flex gap-1">
                <a
                  className="focus-ring rounded-full p-2 hover:bg-zinc-200"
                  href={attachmentContentURL(attachment.id)}
                  aria-label={t("ticket.downloadFile", { name: attachment.filename })}
                >
                  <Download size={15} />
                </a>
                <IconButton label={t("ticket.deleteFile", { name: attachment.filename })} onClick={() => deleteAttachment.mutate(attachment.id)}>
                  <Trash2 size={15} />
                </IconButton>
              </div>
            </div>
          ))}
        </fieldset>
      ) : null}
      <div className="flex justify-between gap-2">
        <Button
          tone="danger"
          onClick={() => {
			if (window.confirm(t("ticket.archiveConfirm"))) {
			  archiveTicket.mutate();
            }
          }}
		  disabled={archiveTicket.isPending}
        >
          <Trash2 size={16} />
		  {t("actions.archive")}
        </Button>
        <Button type="submit" tone="primary" disabled={updateTicket.isPending || !title.trim()}>
          <Save size={16} />
          {t("actions.save")}
        </Button>
      </div>
    </form>
  );
}

function TicketLinksPanel({ projectId, ticketId, tickets }: { projectId: ID; ticketId: ID; tickets: Ticket[] }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [targetTicketId, setTargetTicketId] = useState("");
  const [linkType, setLinkType] = useState<(typeof ticketLinkTypes)[number]["id"]>("relates_to");
  const [linkId, setLinkId] = useState("");
  const candidates = tickets.filter((ticket) => ticket.id !== ticketId);

  const addLink = useMutation({
    mutationFn: () => api.addTicketLink(ticketId, { target_id: targetTicketId, link_type: linkType }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.ticket(ticketId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.ticketsScope(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.graphScope(projectId) }),
      ]);
      setTargetTicketId("");
      toast.success(t("ticket.linkCreated"));
    },
  });

  const removeLink = useMutation({
    mutationFn: () => api.removeTicketLink(linkId.trim()),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.graphScope(projectId) });
      setLinkId("");
      toast.success(t("ticket.linkRemoved"));
    },
  });

  function submitLink(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    addLink.mutate();
  }

  function submitRemove(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    removeLink.mutate();
  }

  return (
    <section className="border-t border-slate-200 pt-5">
      <div className="mb-3 flex items-center gap-2">
        <Link2 size={18} className="text-slate-500" />
        <h3 className="font-semibold text-slate-950">{t("ticket.links")}</h3>
      </div>
      <div className="grid min-w-0 gap-5">
        <form className="grid min-w-0 gap-3" onSubmit={submitLink}>
          {addLink.isError ? <ErrorState title={t("ticket.linkFailed")} body={errorMessage(addLink.error, t("ticket.linkRequestFailed"))} /> : null}
          <SelectField label={t("ticket.linkTarget")} value={targetTicketId} onChange={(event) => setTargetTicketId(event.target.value)}>
            <option value="">{t("ticket.selectTicket")}</option>
            {candidates.map((ticket) => (
              <option key={ticket.id} value={ticket.id}>
                {ticket.title}
              </option>
            ))}
          </SelectField>
          <SelectField
            label={t("ticket.linkType")}
            value={linkType}
            onChange={(event) => setLinkType(event.target.value as (typeof ticketLinkTypes)[number]["id"])}
          >
            {ticketLinkTypes.map((item) => (
              <option key={item.id} value={item.id}>
                {item.label}
              </option>
            ))}
          </SelectField>
          <Button className="w-full" type="submit" tone="primary" disabled={addLink.isPending || !targetTicketId}>
            <Link2 size={16} />
            {t("ticket.addLink")}
          </Button>
        </form>

        <form className="grid min-w-0 gap-3 border-t border-slate-200 pt-4" onSubmit={submitRemove}>
          {removeLink.isError ? (
            <ErrorState title={t("ticket.removeLinkFailed")} body={errorMessage(removeLink.error, t("ticket.removeLinkRequestFailed"))} />
          ) : null}
          <TextField label={t("ticket.linkId")} value={linkId} onChange={(event) => setLinkId(event.target.value)} />
          <Button className="w-full" type="submit" tone="danger" disabled={removeLink.isPending || !linkId.trim()}>
            <Unlink size={16} />
            {t("ticket.removeLink")}
          </Button>
        </form>
      </div>
    </section>
  );
}

function commitTitle(message: string): string {
  return message.split(/\r?\n/)[0] || "Commit";
}

function TicketGitHubCommitsPanel({ projectId, ticketId }: { projectId: ID; ticketId: ID }) {
  const { t, relativeDate } = useI18n();
  const queryClient = useQueryClient();
  const [commitId, setCommitId] = useState("");

  const commits = useQuery({
    queryKey: queryKeys.ticketGitHubCommits(ticketId),
    queryFn: () => api.listTicketGitHubCommits(ticketId),
  });

  const projectCommits = useQuery({
    queryKey: queryKeys.projectGitHubCommits(projectId, "ticket-picker"),
    queryFn: () => api.listProjectGitHubCommits(projectId, { limit: 100 }),
  });

  const linkedCommitIds = useMemo(() => new Set(commits.data?.map((commit) => commit.id) || []), [commits.data]);
  const candidates = (projectCommits.data || []).filter((commit) => !linkedCommitIds.has(commit.id));

  async function refreshGitHubLinks() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.ticketGitHubCommits(ticketId) }),
      queryClient.invalidateQueries({ queryKey: queryKeys.projectGitHubCommitsScope(projectId) }),
      queryClient.invalidateQueries({ queryKey: queryKeys.githubRepositories(projectId) }),
    ]);
  }

  const linkCommit = useMutation({
    mutationFn: () => api.linkTicketGitHubCommit(ticketId, { commit_id: commitId }),
    onSuccess: async () => {
      await refreshGitHubLinks();
      setCommitId("");
      toast.success(t("ticket.commitLinked"));
    },
    onError: (error) => toast.error(errorMessage(error, t("ticket.commitLinkFailed"))),
  });

  const unlinkCommit = useMutation({
    mutationFn: (targetCommitId: ID) => api.unlinkTicketGitHubCommit(ticketId, targetCommitId),
    onSuccess: async () => {
      await refreshGitHubLinks();
      toast.success(t("ticket.commitUnlinked"));
    },
    onError: (error) => toast.error(errorMessage(error, t("ticket.commitUnlinkFailed"))),
  });

  return (
    <section className="border-t border-slate-200 pt-5">
      <div className="mb-3 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div className="flex items-center gap-2">
          <Github size={18} className="text-slate-500" />
          <h3 className="font-semibold text-slate-950">{t("ticket.githubCommits")}</h3>
        </div>
        <form
          className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]"
          onSubmit={(event) => {
            event.preventDefault();
            linkCommit.mutate();
          }}
        >
          <SelectField label={t("ticket.importedCommit")} value={commitId} onChange={(event) => setCommitId(event.target.value)}>
            <option value="">{t("ticket.selectCommit")}</option>
            {candidates.map((commit) => (
              <option key={commit.id} value={commit.id}>
                {commit.short_sha} / {commitTitle(commit.message)}
              </option>
            ))}
          </SelectField>
          <Button className="self-end" type="submit" tone="primary" disabled={linkCommit.isPending || !commitId}>
            <Link2 size={16} />
            {t("github.link")}
          </Button>
        </form>
      </div>
      {projectCommits.isError ? (
        <ErrorState title={t("ticket.importedCommitsLoadFailed")} body={errorMessage(projectCommits.error, t("ticket.projectCommitRequestFailed"))} />
      ) : null}
      {commits.isLoading ? <LoadingState label={t("github.loadingCommits")} /> : null}
      {commits.isError ? (
        <ErrorState title={t("ticket.githubCommitsLoadFailed")} body={errorMessage(commits.error, t("ticket.githubCommitRequestFailed"))} />
      ) : null}
      <div className="space-y-2">
        {commits.data?.map((commit: GitHubCommit) => (
          <div
            key={commit.id}
            className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 transition hover:border-slate-300 hover:bg-white"
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <a
                  className="truncate text-sm font-medium text-slate-950 underline-offset-4 hover:underline"
                  href={commit.html_url}
                  target="_blank"
                  rel="noreferrer"
                >
                  {commitTitle(commit.message)}
                </a>
                <div className="mt-1 text-xs text-slate-500">
                  {commit.repository_full_name || "GitHub"} / {commit.author_name || t("github.unknownAuthor")} /{" "}
                  {commit.authored_at ? relativeDate(commit.authored_at) : t("github.unknownDate")}
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <code className="rounded-md bg-white px-1.5 py-0.5 text-xs text-slate-600">{commit.short_sha}</code>
                <Button
                  tone="danger"
                  className="h-8 px-2 text-xs"
                  disabled={unlinkCommit.isPending && unlinkCommit.variables === commit.id}
                  onClick={() => unlinkCommit.mutate(commit.id)}
                >
                  <Unlink size={14} />
                  {t("ticket.unlink")}
                </Button>
              </div>
            </div>
          </div>
        ))}
        {commits.data?.length === 0 ? <p className="text-sm text-slate-500">{t("ticket.noLinkedCommits")}</p> : null}
      </div>
    </section>
  );
}

export function TicketDetailPanel({
  projectId,
  ticketId,
  tickets,
  members,
  labels,
  onClose,
}: {
  projectId: ID;
  ticketId: ID;
  tickets: Ticket[];
  members: ProjectMember[];
  labels: Label[];
  onClose: () => void;
}) {
  const { t, relativeDate } = useI18n();
  const queryClient = useQueryClient();
  const [comment, setComment] = useState("");
  const [commentOffset, setCommentOffset] = useState(0);

  const ticket = useQuery({
    queryKey: queryKeys.ticket(ticketId),
    queryFn: () => api.getTicket(ticketId),
  });

  const comments = useQuery({
    queryKey: queryKeys.comments(ticketId, commentOffset),
    queryFn: () => api.listCommentsPage(ticketId, { limit: 25, offset: commentOffset }),
  });

  const createComment = useMutation({
    mutationFn: (content: string) => api.createComment(ticketId, content),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.comments(ticketId) });
      setComment("");
    },
  });

  const removeComment = useMutation({
    mutationFn: api.deleteComment,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.comments(ticketId) });
    },
  });

  function submitComment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmed = comment.trim();
    if (trimmed) {
      createComment.mutate(trimmed);
    }
  }

  return createPortal(
    <div className="fixed left-0 top-0 z-40 flex h-dvh w-screen justify-end overflow-hidden bg-slate-950/40">
      <button type="button" aria-label={t("ticket.closePanel")} className="hidden flex-1 md:block" onClick={onClose} />
      <aside className="flex h-dvh w-full max-w-2xl flex-col overflow-hidden bg-white shadow-xl">
        <header className="flex items-start justify-between border-b border-slate-200 px-5 py-4">
          <div className="min-w-0">
            <div className="mb-2 flex flex-wrap gap-1.5">
              {ticket.data ? (
                <>
                  <StatusBadge value={ticket.data.type} kind="type" />
                  <StatusBadge value={ticket.data.status} kind="status" />
                  <StatusBadge value={ticket.data.priority} kind="priority" />
                </>
              ) : null}
            </div>
            <h2 className="truncate text-lg font-semibold text-slate-950">
              {ticket.data?.title || `${t("workspace.ticket")} ${compactId(ticketId)}`}
            </h2>
          </div>
          <IconButton label={t("ticket.close")} onClick={onClose}>
            <X size={18} />
          </IconButton>
        </header>

        <div className="flex-1 overflow-y-auto px-5 py-5">
          {ticket.isLoading ? <LoadingState label={t("ticket.loading")} /> : null}
          {ticket.isError ? (
            <ErrorState title={t("ticket.loadFailed")} body={errorMessage(ticket.error, t("ticket.requestFailed"))} />
          ) : null}

          {ticket.data ? (
            <div className="grid gap-6">
              <TicketEditor
                key={`${ticket.data.id}:${ticket.data.updated_at}`}
                projectId={projectId}
                ticket={ticket.data}
                tickets={tickets}
                members={members}
                labels={labels}
                onClose={onClose}
              />

              <TicketLinksPanel projectId={projectId} ticketId={ticketId} tickets={tickets} />

              <TicketGitHubCommitsPanel projectId={projectId} ticketId={ticketId} />

              <section className="border-t border-slate-200 pt-5">
                <div className="mb-3 flex items-center gap-2">
                  <MessageSquare size={18} className="text-slate-500" />
                  <h3 className="font-semibold text-slate-950">{t("ticket.comments")}</h3>
                </div>
                <form className="mb-4 grid gap-2" onSubmit={submitComment}>
                  <TextAreaField
                    label={t("ticket.addComment")}
                    value={comment}
                    onChange={(event) => setComment(event.target.value)}
                    placeholder={t("ticket.commentPlaceholder")}
                  />
                  <div className="flex justify-end">
                    <Button type="submit" tone="primary" disabled={createComment.isPending || !comment.trim()}>
                      {t("ticket.commentAction")}
                    </Button>
                  </div>
                </form>

                {comments.isLoading ? <LoadingState label={t("ticket.commentsLoading")} /> : null}
                {comments.isError ? (
                  <ErrorState title={t("ticket.commentsLoadFailed")} body={errorMessage(comments.error, t("ticket.commentRequestFailed"))} />
                ) : null}
                <div className="space-y-2">
                  {comments.data?.items.map((item) => (
                    <div key={item.id} className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-2">
                      <div className="mb-1 flex items-center justify-between gap-3 text-xs text-slate-500">
                        <span>{compactId(item.author_id)}</span>
                        <div className="flex items-center gap-2">
                          <span>{relativeDate(item.created_at)}</span>
                          <IconButton
                            label={t("ticket.deleteComment")}
                            className="h-7 w-7"
                            onClick={() => removeComment.mutate(item.id)}
                            disabled={removeComment.isPending}
                          >
                            <Trash2 size={14} />
                          </IconButton>
                        </div>
                      </div>
                      <p className="whitespace-pre-wrap text-sm text-slate-800">{item.content}</p>
                    </div>
                  ))}
                  {comments.data?.items.length === 0 ? <p className="text-sm text-slate-500">{t("ticket.noComments")}</p> : null}
                </div>
                {comments.data ? (
                  <div className="mt-3">
                    <OffsetPaginationControls page={comments.data} onOffsetChange={setCommentOffset} disabled={comments.isFetching} />
                  </div>
                ) : null}
              </section>
            </div>
          ) : null}
        </div>
      </aside>
    </div>,
    document.body,
  );
}
