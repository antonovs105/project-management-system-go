import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Github, Link2, MessageSquare, Save, Trash2, Unlink, X } from "lucide-react";
import { useMemo, useState } from "react";
import type { FormEvent } from "react";
import { createPortal } from "react-dom";
import { toast } from "sonner";
import { api, errorMessage, type UpdateTicketPayload } from "../../lib/api";
import { ticketLinkTypes, ticketPriorities, ticketStatuses, ticketTypes } from "../../lib/constants";
import { compactId, relativeDate } from "../../lib/format";
import { fieldLimits } from "../../lib/limits";
import { queryKeys } from "../../lib/queryKeys";
import type { GitHubCommit, ID, ProjectMember, Ticket, TicketPriority, TicketStatus, TicketType } from "../../types";
import { StatusBadge } from "../../components/StatusBadge";
import { Button, ErrorState, IconButton, LoadingState, SelectField, TextAreaField, TextField } from "../../components/ui";
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
  onClose,
}: {
  projectId: ID;
  ticket: Ticket;
  tickets: Ticket[];
  members: ProjectMember[];
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [title, setTitle] = useState(ticket.title);
  const [description, setDescription] = useState(ticket.description);
  const [status, setStatus] = useState<TicketStatus>(ticket.status);
  const [priority, setPriority] = useState<TicketPriority>(ticket.priority);
  const [type, setType] = useState<TicketType>(ticket.type);
  const [parentId, setParentId] = useState(ticket.parent_id || "");
  const [assigneeId, setAssigneeId] = useState(ticket.assignee_id || "");

  const parents = useMemo(() => parentCandidates(type, ticket.id, tickets), [ticket.id, tickets, type]);

  const updateTicket = useMutation({
    mutationFn: (payload: UpdateTicketPayload) => api.updateTicket(ticket.id, payload),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.ticket(ticket.id) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.tickets(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.graph(projectId) }),
      ]);
      toast.success("Ticket updated");
    },
  });

  const deleteTicket = useMutation({
    mutationFn: () => api.deleteTicket(ticket.id),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.tickets(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.graph(projectId) }),
      ]);
      toast.success("Ticket deleted");
      onClose();
    },
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
    });
  }

  return (
    <form className="grid gap-4" onSubmit={submitTicket}>
      {updateTicket.isError ? (
        <ErrorState title="Could not update ticket" body={errorMessage(updateTicket.error, "Ticket update failed.")} />
      ) : null}
      {deleteTicket.isError ? (
        <ErrorState title="Could not delete ticket" body={errorMessage(deleteTicket.error, "Ticket delete failed.")} />
      ) : null}
      <TextField
        label="Title"
        value={title}
        onChange={(event) => setTitle(event.target.value)}
        maxLength={fieldLimits.ticketTitleMaxLength}
        required
      />
      <div className="grid gap-4 sm:grid-cols-3">
        <SelectField label="Status" value={status} onChange={(event) => setStatus(event.target.value as TicketStatus)}>
          {ticketStatuses.map((item) => (
            <option key={item.id} value={item.id}>
              {item.label}
            </option>
          ))}
        </SelectField>
        <SelectField label="Priority" value={priority} onChange={(event) => setPriority(event.target.value as TicketPriority)}>
          {ticketPriorities.map((item) => (
            <option key={item.id} value={item.id}>
              {item.label}
            </option>
          ))}
        </SelectField>
        <SelectField label="Type" value={type} onChange={(event) => setType(event.target.value as TicketType)}>
          {ticketTypes.map((item) => (
            <option key={item.id} value={item.id}>
              {item.label}
            </option>
          ))}
        </SelectField>
      </div>
      {type !== "epic" ? (
        <SelectField label="Parent" value={parentId} onChange={(event) => setParentId(event.target.value)}>
          <option value="">None</option>
          {parents.map((candidate) => (
            <option key={candidate.id} value={candidate.id}>
              {candidate.title}
            </option>
          ))}
        </SelectField>
      ) : null}
      <MemberAssigneeSelect members={members} value={assigneeId} onChange={setAssigneeId} />
      <TextAreaField
        label="Description"
        value={description}
        onChange={(event) => setDescription(event.target.value)}
        maxLength={fieldLimits.ticketDescriptionMaxLength}
      />
      <div className="flex justify-between gap-2">
        <Button
          tone="danger"
          onClick={() => {
            if (window.confirm("Delete this ticket?")) {
              deleteTicket.mutate();
            }
          }}
          disabled={deleteTicket.isPending}
        >
          <Trash2 size={16} />
          Delete
        </Button>
        <Button type="submit" tone="primary" disabled={updateTicket.isPending || !title.trim()}>
          <Save size={16} />
          Save
        </Button>
      </div>
    </form>
  );
}

function TicketLinksPanel({ projectId, ticketId, tickets }: { projectId: ID; ticketId: ID; tickets: Ticket[] }) {
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
        queryClient.invalidateQueries({ queryKey: queryKeys.tickets(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.graph(projectId) }),
      ]);
      setTargetTicketId("");
      toast.success("Ticket link created");
    },
  });

  const removeLink = useMutation({
    mutationFn: () => api.removeTicketLink(linkId.trim()),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.graph(projectId) });
      setLinkId("");
      toast.success("Ticket link removed");
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
        <h3 className="font-semibold text-slate-950">Links</h3>
      </div>
      <div className="grid min-w-0 gap-5">
        <form className="grid min-w-0 gap-3" onSubmit={submitLink}>
          {addLink.isError ? <ErrorState title="Could not link ticket" body={errorMessage(addLink.error, "Link request failed.")} /> : null}
          <SelectField label="Target" value={targetTicketId} onChange={(event) => setTargetTicketId(event.target.value)}>
            <option value="">Select ticket</option>
            {candidates.map((ticket) => (
              <option key={ticket.id} value={ticket.id}>
                {ticket.title}
              </option>
            ))}
          </SelectField>
          <SelectField
            label="Type"
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
            Add link
          </Button>
        </form>

        <form className="grid min-w-0 gap-3 border-t border-slate-200 pt-4" onSubmit={submitRemove}>
          {removeLink.isError ? (
            <ErrorState title="Could not remove link" body={errorMessage(removeLink.error, "Remove link request failed.")} />
          ) : null}
          <TextField label="Link ID" value={linkId} onChange={(event) => setLinkId(event.target.value)} />
          <Button className="w-full" type="submit" tone="danger" disabled={removeLink.isPending || !linkId.trim()}>
            <Unlink size={16} />
            Remove link
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
      toast.success("GitHub commit linked");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not link GitHub commit.")),
  });

  const unlinkCommit = useMutation({
    mutationFn: (targetCommitId: ID) => api.unlinkTicketGitHubCommit(ticketId, targetCommitId),
    onSuccess: async () => {
      await refreshGitHubLinks();
      toast.success("GitHub commit unlinked");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not unlink GitHub commit.")),
  });

  return (
    <section className="border-t border-slate-200 pt-5">
      <div className="mb-3 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div className="flex items-center gap-2">
          <Github size={18} className="text-slate-500" />
          <h3 className="font-semibold text-slate-950">GitHub Commits</h3>
        </div>
        <form
          className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]"
          onSubmit={(event) => {
            event.preventDefault();
            linkCommit.mutate();
          }}
        >
          <SelectField label="Imported commit" value={commitId} onChange={(event) => setCommitId(event.target.value)}>
            <option value="">Select commit</option>
            {candidates.map((commit) => (
              <option key={commit.id} value={commit.id}>
                {commit.short_sha} / {commitTitle(commit.message)}
              </option>
            ))}
          </SelectField>
          <Button className="self-end" type="submit" tone="primary" disabled={linkCommit.isPending || !commitId}>
            <Link2 size={16} />
            Link
          </Button>
        </form>
      </div>
      {projectCommits.isError ? (
        <ErrorState title="Could not load imported commits" body={errorMessage(projectCommits.error, "Project commit request failed.")} />
      ) : null}
      {commits.isLoading ? <LoadingState label="Loading GitHub commits" /> : null}
      {commits.isError ? (
        <ErrorState title="Could not load GitHub commits" body={errorMessage(commits.error, "GitHub commit request failed.")} />
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
                  {commit.repository_full_name || "GitHub"} / {commit.author_name || "Unknown author"} /{" "}
                  {commit.authored_at ? relativeDate(commit.authored_at) : "unknown date"}
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
                  Unlink
                </Button>
              </div>
            </div>
          </div>
        ))}
        {commits.data?.length === 0 ? <p className="text-sm text-slate-500">No linked GitHub commits yet.</p> : null}
      </div>
    </section>
  );
}

export function TicketDetailPanel({
  projectId,
  ticketId,
  tickets,
  members,
  onClose,
}: {
  projectId: ID;
  ticketId: ID;
  tickets: Ticket[];
  members: ProjectMember[];
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [comment, setComment] = useState("");

  const ticket = useQuery({
    queryKey: queryKeys.ticket(ticketId),
    queryFn: () => api.getTicket(ticketId),
  });

  const comments = useQuery({
    queryKey: queryKeys.comments(ticketId),
    queryFn: () => api.listComments(ticketId),
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
      <button type="button" aria-label="Close ticket panel" className="hidden flex-1 md:block" onClick={onClose} />
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
              {ticket.data?.title || `Ticket ${compactId(ticketId)}`}
            </h2>
          </div>
          <IconButton label="Close ticket" onClick={onClose}>
            <X size={18} />
          </IconButton>
        </header>

        <div className="flex-1 overflow-y-auto px-5 py-5">
          {ticket.isLoading ? <LoadingState label="Loading ticket" /> : null}
          {ticket.isError ? (
            <ErrorState title="Could not load ticket" body={errorMessage(ticket.error, "Ticket request failed.")} />
          ) : null}

          {ticket.data ? (
            <div className="grid gap-6">
              <TicketEditor
                key={`${ticket.data.id}:${ticket.data.updated_at}`}
                projectId={projectId}
                ticket={ticket.data}
                tickets={tickets}
                members={members}
                onClose={onClose}
              />

              <TicketLinksPanel projectId={projectId} ticketId={ticketId} tickets={tickets} />

              <TicketGitHubCommitsPanel projectId={projectId} ticketId={ticketId} />

              <section className="border-t border-slate-200 pt-5">
                <div className="mb-3 flex items-center gap-2">
                  <MessageSquare size={18} className="text-slate-500" />
                  <h3 className="font-semibold text-slate-950">Comments</h3>
                </div>
                <form className="mb-4 grid gap-2" onSubmit={submitComment}>
                  <TextAreaField
                    label="Add comment"
                    value={comment}
                    onChange={(event) => setComment(event.target.value)}
                    placeholder="Write an update"
                  />
                  <div className="flex justify-end">
                    <Button type="submit" tone="primary" disabled={createComment.isPending || !comment.trim()}>
                      Comment
                    </Button>
                  </div>
                </form>

                {comments.isLoading ? <LoadingState label="Loading comments" /> : null}
                {comments.isError ? (
                  <ErrorState title="Could not load comments" body={errorMessage(comments.error, "Comment request failed.")} />
                ) : null}
                <div className="space-y-2">
                  {comments.data?.map((item) => (
                    <div key={item.id} className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-2">
                      <div className="mb-1 flex items-center justify-between gap-3 text-xs text-slate-500">
                        <span>{compactId(item.author_id)}</span>
                        <div className="flex items-center gap-2">
                          <span>{relativeDate(item.created_at)}</span>
                          <IconButton
                            label="Delete comment"
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
                  {comments.data?.length === 0 ? <p className="text-sm text-slate-500">No comments yet.</p> : null}
                </div>
              </section>
            </div>
          ) : null}
        </div>
      </aside>
    </div>,
    document.body,
  );
}
