import axios from "axios";
import type {
	AccountSession,
  AdminAuditAction,
  AdminAuditEvent,
  AdminAuditTargetType,
  AdminUser,
  Comment,
  DeliveryFailureKind,
  DeliveryState,
  DomainBlock,
  FederationDelivery,
  FederationDeliverySummary,
  FederationFollowState,
  FederationInboxActivity,
  FederationRemoteActor,
  FederationRemoteFollow,
  FollowRemoteActorResult,
  GitHubCommit,
  GitHubRepository,
  GitHubSyncResult,
  GraphData,
  ID,
  InstanceCapabilities,
  InstanceRole,
  Label,
  Notification,
  OAuthProvider,
  Project,
  ProjectCreationPolicy,
  ProjectDelivery,
  ProjectDeliverySummary,
  ProjectInvite,
  ProjectInviteInspection,
  ProjectMember,
  ProjectPermission,
  ProjectRole,
  ProjectRoleKey,
  PublicInstanceConfig,
  RemoteProject,
  RemoteActorInspection,
  RemoteProjectInvite,
  RemoteProjectInviteResult,
	RemoteTicketWriteResult,
	SecurityEvent,
  Ticket,
  TicketPriority,
  TicketStatus,
  TicketType,
} from "../types";
import { useAuthStore } from "../store/auth";

const apiBaseURL = import.meta.env.VITE_API_URL || (import.meta.env.PROD ? "" : "http://localhost:8080");

const http = axios.create({
  baseURL: apiBaseURL,
  timeout: 20_000,
  withCredentials: true,
});

http.interceptors.response.use(
  (response) => response,
  (error: unknown) => {
    if (axios.isAxiosError(error) && error.response?.status === 401) {
      useAuthStore.getState().logout();
    }
    return Promise.reject(error);
  },
);

const apiPrefix = "/api/v1";

export interface LoginPayload {
	email: string;
	password: string;
	mfa_code?: string;
}

export interface RegisterPayload {
  username: string;
  email: string;
  password: string;
}

export interface CreateProjectPayload {
  name: string;
  description: string;
}

export interface UpdateProjectPayload {
  name?: string;
  description?: string;
}

export interface CreateTicketPayload {
  title: string;
  description: string;
  priority: TicketPriority;
  type: TicketType;
  parent_id?: ID | null;
  assignee_id?: ID | null;
  due_date?: string;
  label_ids?: ID[];
}

export interface UpdateTicketPayload {
  title?: string;
  description?: string;
  status?: TicketStatus;
  priority?: TicketPriority;
  type?: TicketType;
  parent_id?: ID | null;
  assignee_id?: ID | null;
  due_date?: string;
  label_ids?: ID[];
  is_resolved?: boolean;
}

export interface MoveTicketPayload {
  status: TicketStatus;
  before_ticket_id?: ID | null;
  after_ticket_id?: ID | null;
}

export interface RemoteMoveTicketPayload {
  status: TicketStatus;
}

export interface ChangePasswordPayload {
  current_password: string;
  new_password: string;
}

export interface AddProjectMemberPayload {
  user_ref?: string;
  user_id?: ID;
  role_id?: ID;
  role?: ProjectRoleKey;
}

export interface CreateProjectRolePayload {
  name: string;
  description?: string;
  permissions: ProjectPermission[];
}

export interface LinkGitHubRepositoryPayload {
  owner: string;
  name: string;
}

export interface ListGitHubCommitsFilters {
  repository_id?: ID;
  q?: string;
  unlinked?: boolean;
  limit?: number;
}

export interface LinkGitHubCommitPayload {
  commit_id: ID;
}

export interface UpdateProjectRolePayload {
  name?: string;
  description?: string;
  permissions?: ProjectPermission[];
}

export interface UpdateProjectMemberRolePayload {
  role_id?: ID;
  role?: ProjectRoleKey;
}

export interface AddTicketLinkPayload {
  target_id: ID;
  link_type: string;
}

export interface TicketFilters {
  q?: string;
  assignee?: "me" | "unassigned";
  assignee_id?: ID;
  status?: TicketStatus;
  priority?: TicketPriority;
  type?: TicketType;
  limit?: number;
  offset?: number;
}

export type GraphFilters = Omit<TicketFilters, "offset">;

export interface AdminUsersFilters {
  role?: InstanceRole | "";
  q?: string;
  limit?: number;
  offset?: number;
}

export interface AdminAuditFilters {
  action?: AdminAuditAction | "";
  actor_user_id?: ID;
  target_type?: AdminAuditTargetType | "";
  limit?: number;
  offset?: number;
}

export interface DeliveryFilters {
  state?: DeliveryState | "";
  limit?: number;
}

export interface NotificationFilters {
  unread?: boolean;
  limit?: number;
  offset?: number;
}

export interface ProjectInviteFilters {
  status?: ProjectInvite["status"] | "";
  limit?: number;
  offset?: number;
}

export interface FederationDeliveryFilters extends DeliveryFilters {
  failure_kind?: DeliveryFailureKind | "";
}

export interface FederationFollowFilters {
  state?: FederationFollowState | "";
  limit?: number;
  offset?: number;
}

export interface RemoteFederationResourcePayload {
  resource: string;
}

export interface FederationInboxFilters {
  limit?: number;
  offset?: number;
}

export interface BlockDomainPayload {
  domain: string;
  reason?: string;
}

export interface RemoteActorFilters {
  fetch_error?: boolean | "";
  limit?: number;
}

export interface LoginResponse {
  user_id: ID;
  instance_role: InstanceRole;
	email?: string;
	email_verified: boolean;
	mfa_enrollment_required?: boolean;
}

interface ProfileResponse {
  user_id: ID;
}

interface OAuthProvidersResponse {
  providers?: OAuthProvider[];
}

interface InstanceConfigResponse {
  name?: string;
  version?: string;
  registration_enabled?: boolean;
  project_creation_policy?: ProjectCreationPolicy;
  oauth_providers?: OAuthProvider[];
}

interface InstanceCapabilitiesResponse extends InstanceConfigResponse {
  instance_role?: InstanceRole;
  can_create_projects?: boolean;
}

interface ErrorResponse {
  error?: string;
  message?: string;
}

function asArray<T>(data: T[] | null | undefined): T[] {
  return Array.isArray(data) ? data : [];
}

export function errorMessage(error: unknown, fallback: string): string {
  if (axios.isAxiosError<ErrorResponse>(error)) {
    return error.response?.data?.error || error.response?.data?.message || fallback;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return fallback;
}

function isOAuthProvider(value: string): value is OAuthProvider {
  return value === "google" || value === "github";
}

function isProjectCreationPolicy(value: string): value is ProjectCreationPolicy {
  return value === "everyone" || value === "admins_only";
}

function isInstanceRole(value: string): value is InstanceRole {
  return value === "owner" || value === "admin" || value === "user";
}

function normalizeOAuthProviders(values: OAuthProvider[] | undefined): OAuthProvider[] {
  return asArray(values).filter(isOAuthProvider);
}

function normalizeProjectCreationPolicy(value: ProjectCreationPolicy | undefined): ProjectCreationPolicy {
  return value && isProjectCreationPolicy(value) ? value : "everyone";
}

function normalizeInstanceConfig(data: InstanceConfigResponse) {
  return {
    name: data.name || "Progo",
    version: data.version || "dev",
    registration_enabled: data.registration_enabled ?? true,
    project_creation_policy: normalizeProjectCreationPolicy(data.project_creation_policy),
    oauth_providers: normalizeOAuthProviders(data.oauth_providers),
  };
}

function apiURL(path: string): string {
  return `${apiBaseURL.replace(/\/$/, "")}${path}`;
}

export function oauthStartURL(provider: OAuthProvider): string {
  return apiURL(`/auth/${provider}/start`);
}

export function projectTicketEventsURL(projectId: ID): string {
  return apiURL(`${apiPrefix}/projects/${projectId}/tickets/events`);
}

export function notificationsEventsURL(): string {
  return apiURL(`${apiPrefix}/me/notifications/events`);
}

export const api = {
  async getPublicInstance(): Promise<PublicInstanceConfig> {
    const { data } = await http.get<InstanceConfigResponse>("/instance");
    return normalizeInstanceConfig(data);
  },

  async getInstanceCapabilities(): Promise<InstanceCapabilities> {
    const { data } = await http.get<InstanceCapabilitiesResponse>(`${apiPrefix}/instance`);
    const normalized = normalizeInstanceConfig(data);
    return {
      ...normalized,
      instance_role: data.instance_role && isInstanceRole(data.instance_role) ? data.instance_role : "user",
      can_create_projects: data.can_create_projects ?? (normalized.project_creation_policy === "everyone"),
    };
  },

  async login(payload: LoginPayload): Promise<LoginResponse> {
    const { data } = await http.post<LoginResponse>("/login", payload);
    return data;
  },

  async listOAuthProviders(): Promise<OAuthProvider[]> {
    const { data } = await http.get<OAuthProvidersResponse>("/auth/oauth/providers");
    return asArray(data.providers).filter(isOAuthProvider);
  },

	async exchangeOAuthCode(code: string, mfaCode?: string): Promise<LoginResponse> {
		const { data } = await http.post<LoginResponse>("/auth/oauth/exchange", { code, mfa_code: mfaCode || undefined });
    return data;
  },

	async register(payload: RegisterPayload): Promise<void> {
		await http.post("/register", payload);
	},

	async forgotPassword(email: string): Promise<void> {
		await http.post("/auth/password/forgot", { email });
	},

	async resetPassword(token: string, newPassword: string): Promise<void> {
		await http.post("/auth/password/reset", { token, new_password: newPassword });
	},

	async verifyEmail(token: string): Promise<void> {
		await http.post("/auth/email/verify", { token });
	},

	async requestEmailVerification(): Promise<void> {
		await http.post(`${apiPrefix}/me/email/verification`);
	},

	async listSessions(): Promise<AccountSession[]> {
		const { data } = await http.get<AccountSession[] | null>(`${apiPrefix}/me/sessions`);
		return asArray(data);
	},

	async revokeSession(sessionId: ID): Promise<void> {
		await http.delete(`${apiPrefix}/me/sessions/${sessionId}`);
	},

	async listSecurityEvents(): Promise<SecurityEvent[]> {
		const { data } = await http.get<SecurityEvent[] | null>(`${apiPrefix}/me/security-events`);
		return asArray(data);
	},

	async getMFAStatus(): Promise<{ enabled: boolean }> {
		const { data } = await http.get<{ enabled: boolean }>(`${apiPrefix}/me/mfa`);
		return data;
	},

	async beginMFA(): Promise<{ secret: string; uri: string }> {
		const { data } = await http.post<{ secret: string; uri: string }>(`${apiPrefix}/me/mfa/setup`);
		return data;
	},

	async confirmMFA(code: string): Promise<{ recovery_codes: string[] }> {
		const { data } = await http.post<{ recovery_codes: string[] }>(`${apiPrefix}/me/mfa/confirm`, { code });
		return data;
	},

	async disableMFA(code: string): Promise<void> {
		await http.delete(`${apiPrefix}/me/mfa`, { data: { code } });
	},

  async profile(): Promise<ProfileResponse> {
    const { data } = await http.get<ProfileResponse>(`${apiPrefix}/me`);
    return data;
  },

  async changePassword(payload: ChangePasswordPayload): Promise<void> {
    await http.patch(`${apiPrefix}/me/password`, payload);
  },

  async logout(): Promise<void> {
    await http.post(`${apiPrefix}/me/logout`);
  },

  async listMyProjectInvites(filters: ProjectInviteFilters = {}): Promise<ProjectInviteInspection[]> {
    const { data } = await http.get<ProjectInviteInspection[] | null>(`${apiPrefix}/me/invites`, {
      params: { limit: 100, offset: 0, ...filters },
    });
    return asArray(data);
  },

  async listAdminUsers(filters: AdminUsersFilters = {}): Promise<AdminUser[]> {
    const { data } = await http.get<AdminUser[] | null>(`${apiPrefix}/admin/users`, {
      params: { limit: 100, offset: 0, ...filters },
    });
    return asArray(data);
  },

  async updateAdminUserRole(userId: ID, instanceRole: InstanceRole): Promise<AdminUser> {
    const { data } = await http.patch<AdminUser>(`${apiPrefix}/admin/users/${userId}/role`, {
      instance_role: instanceRole,
    });
    return data;
  },

  async listAdminAuditEvents(filters: AdminAuditFilters = {}): Promise<AdminAuditEvent[]> {
    const { data } = await http.get<AdminAuditEvent[] | null>(`${apiPrefix}/admin/audit-events`, {
      params: { limit: 100, offset: 0, ...filters },
    });
    return asArray(data);
  },

  async listProjects(): Promise<Project[]> {
    const { data } = await http.get<Project[] | null>(`${apiPrefix}/projects`, {
      params: { limit: 100, offset: 0 },
    });
    return asArray(data);
  },

  async createProject(payload: CreateProjectPayload): Promise<Project> {
    const { data } = await http.post<Project>(`${apiPrefix}/projects`, payload);
    return data;
  },

  async getProject(projectId: ID): Promise<Project> {
    const { data } = await http.get<Project>(`${apiPrefix}/projects/${projectId}`);
    return data;
  },

  async updateProject(projectId: ID, payload: UpdateProjectPayload): Promise<void> {
    await http.patch(`${apiPrefix}/projects/${projectId}`, payload);
  },

  async deleteProject(projectId: ID): Promise<void> {
    await http.delete(`${apiPrefix}/projects/${projectId}`);
  },

  async listGitHubRepositories(projectId: ID): Promise<GitHubRepository[]> {
    const { data } = await http.get<GitHubRepository[] | null>(`${apiPrefix}/projects/${projectId}/github/repositories`);
    return asArray(data);
  },

  async linkGitHubRepository(projectId: ID, payload: LinkGitHubRepositoryPayload): Promise<GitHubRepository> {
    const { data } = await http.post<GitHubRepository>(`${apiPrefix}/projects/${projectId}/github/repositories`, payload);
    return data;
  },

  async deleteGitHubRepository(projectId: ID, repositoryId: ID): Promise<void> {
    await http.delete(`${apiPrefix}/projects/${projectId}/github/repositories/${repositoryId}`);
  },

  async syncGitHubRepository(projectId: ID, repositoryId: ID): Promise<GitHubSyncResult> {
    const { data } = await http.post<GitHubSyncResult>(`${apiPrefix}/projects/${projectId}/github/repositories/${repositoryId}/sync`);
    return data;
  },

  async listProjectGitHubCommits(projectId: ID, filters: ListGitHubCommitsFilters = {}): Promise<GitHubCommit[]> {
    const { data } = await http.get<GitHubCommit[] | null>(`${apiPrefix}/projects/${projectId}/github/commits`, {
      params: filters,
    });
    return asArray(data);
  },

  async inviteProjectMember(projectId: ID, payload: AddProjectMemberPayload): Promise<ProjectInvite> {
    const { data } = await http.post<ProjectInvite>(`${apiPrefix}/projects/${projectId}/members`, payload);
    return data;
  },

  async listProjectMembers(projectId: ID): Promise<ProjectMember[]> {
    const { data } = await http.get<ProjectMember[] | null>(`${apiPrefix}/projects/${projectId}/members`, {
      params: { limit: 200, offset: 0 },
    });
    return asArray(data);
  },

  async removeProjectMember(projectId: ID, userId: ID): Promise<void> {
    await http.delete(`${apiPrefix}/projects/${projectId}/members/${userId}`);
  },

  async updateProjectMemberRole(projectId: ID, userId: ID, payload: UpdateProjectMemberRolePayload): Promise<ProjectMember> {
    const { data } = await http.patch<ProjectMember>(`${apiPrefix}/projects/${projectId}/members/${userId}`, payload);
    return data;
  },

  async listProjectInvites(projectId: ID, filters: ProjectInviteFilters = {}): Promise<ProjectInviteInspection[]> {
    const { data } = await http.get<ProjectInviteInspection[] | null>(`${apiPrefix}/projects/${projectId}/invites`, {
      params: { limit: 100, offset: 0, ...filters },
    });
    return asArray(data);
  },

  async acceptInvite(inviteId: ID): Promise<void> {
    await http.post(`${apiPrefix}/invites/${inviteId}/accept`);
  },

  async rejectInvite(inviteId: ID): Promise<void> {
    await http.post(`${apiPrefix}/invites/${inviteId}/reject`);
  },

  async revokeInvite(inviteId: ID): Promise<void> {
    await http.post(`${apiPrefix}/invites/${inviteId}/revoke`);
  },

  async listRemoteProjectInvites(filters: ProjectInviteFilters = {}): Promise<RemoteProjectInvite[]> {
    const { data } = await http.get<RemoteProjectInvite[] | null>(`${apiPrefix}/me/remote-project-invites`, {
      params: { limit: 100, offset: 0, state: filters.status || undefined },
    });
    return asArray(data);
  },

  async acceptRemoteProjectInvite(inviteId: ID): Promise<RemoteProjectInviteResult> {
    const { data } = await http.post<RemoteProjectInviteResult>(`${apiPrefix}/me/remote-project-invites/${inviteId}/accept`);
    return data;
  },

  async rejectRemoteProjectInvite(inviteId: ID): Promise<RemoteProjectInviteResult> {
    const { data } = await http.post<RemoteProjectInviteResult>(`${apiPrefix}/me/remote-project-invites/${inviteId}/reject`);
    return data;
  },

  async listRemoteProjects(): Promise<RemoteProject[]> {
    const { data } = await http.get<RemoteProject[] | null>(`${apiPrefix}/remote-projects`, {
      params: { limit: 100, offset: 0 },
    });
    return asArray(data);
  },

  async getRemoteProject(projectId: ID): Promise<RemoteProject> {
    const { data } = await http.get<RemoteProject>(`${apiPrefix}/remote-projects/${projectId}`);
    return data;
  },

  async listRemoteProjectTickets(projectId: ID, filters: TicketFilters = {}): Promise<Ticket[]> {
    const { data } = await http.get<Ticket[] | null>(`${apiPrefix}/remote-projects/${projectId}/tickets`, {
      params: { limit: 500, offset: 0, ...filters },
    });
    return asArray(data);
  },

  async createRemoteTicket(projectId: ID, payload: CreateTicketPayload): Promise<RemoteTicketWriteResult> {
    const { data } = await http.post<RemoteTicketWriteResult>(`${apiPrefix}/remote-projects/${projectId}/tickets`, payload);
    return data;
  },

  async getRemoteTicket(projectId: ID, ticketId: ID): Promise<Ticket> {
    const { data } = await http.get<Ticket>(`${apiPrefix}/remote-projects/${projectId}/tickets/${ticketId}`);
    return data;
  },

  async updateRemoteTicket(projectId: ID, ticketId: ID, payload: UpdateTicketPayload): Promise<RemoteTicketWriteResult> {
    const { data } = await http.patch<RemoteTicketWriteResult>(`${apiPrefix}/remote-projects/${projectId}/tickets/${ticketId}`, payload);
    return data;
  },

  async moveRemoteTicket(projectId: ID, ticketId: ID, payload: RemoteMoveTicketPayload): Promise<RemoteTicketWriteResult> {
    const { data } = await http.post<RemoteTicketWriteResult>(`${apiPrefix}/remote-projects/${projectId}/tickets/${ticketId}/move`, payload);
    return data;
  },

  async deleteRemoteTicket(projectId: ID, ticketId: ID): Promise<RemoteTicketWriteResult> {
    const { data } = await http.delete<RemoteTicketWriteResult>(`${apiPrefix}/remote-projects/${projectId}/tickets/${ticketId}`);
    return data;
  },

  async listProjectRoles(projectId: ID): Promise<ProjectRole[]> {
    const { data } = await http.get<ProjectRole[] | null>(`${apiPrefix}/projects/${projectId}/roles`);
    return asArray(data);
  },

  async createProjectRole(projectId: ID, payload: CreateProjectRolePayload): Promise<ProjectRole> {
    const { data } = await http.post<ProjectRole>(`${apiPrefix}/projects/${projectId}/roles`, payload);
    return data;
  },

  async updateProjectRole(projectId: ID, roleId: ID, payload: UpdateProjectRolePayload): Promise<ProjectRole> {
    const { data } = await http.patch<ProjectRole>(`${apiPrefix}/projects/${projectId}/roles/${roleId}`, payload);
    return data;
  },

  async deleteProjectRole(projectId: ID, roleId: ID): Promise<void> {
    await http.delete(`${apiPrefix}/projects/${projectId}/roles/${roleId}`);
  },

  async listTickets(projectId: ID, filters: TicketFilters = {}): Promise<Ticket[]> {
    const { data } = await http.get<Ticket[] | null>(`${apiPrefix}/projects/${projectId}/tickets`, {
      params: { limit: 500, offset: 0, ...filters },
    });
    return asArray(data);
  },

  async listProjectLabels(projectId: ID): Promise<Label[]> {
    const { data } = await http.get<Label[] | null>(`${apiPrefix}/projects/${projectId}/labels`);
    return asArray(data);
  },

  async createProjectLabel(projectId: ID, payload: { name: string; color: string }): Promise<Label> {
    const { data } = await http.post<Label>(`${apiPrefix}/projects/${projectId}/labels`, payload);
    return data;
  },

  async deleteProjectLabel(projectId: ID, labelId: ID): Promise<void> {
    await http.delete(`${apiPrefix}/projects/${projectId}/labels/${labelId}`);
  },

  async createTicket(projectId: ID, payload: CreateTicketPayload): Promise<Ticket> {
    const { data } = await http.post<Ticket>(`${apiPrefix}/projects/${projectId}/tickets`, payload);
    return data;
  },

  async getTicket(ticketId: ID): Promise<Ticket> {
    const { data } = await http.get<Ticket>(`${apiPrefix}/tickets/${ticketId}`);
    return data;
  },

  async updateTicket(ticketId: ID, payload: UpdateTicketPayload): Promise<void> {
    await http.patch(`${apiPrefix}/tickets/${ticketId}`, payload);
  },

  async moveTicket(ticketId: ID, payload: MoveTicketPayload): Promise<Ticket> {
    const { data } = await http.post<Ticket>(`${apiPrefix}/tickets/${ticketId}/move`, payload);
    return data;
  },

  async deleteTicket(ticketId: ID): Promise<void> {
    await http.delete(`${apiPrefix}/tickets/${ticketId}`);
  },

  async listNotifications(filters: NotificationFilters = {}): Promise<Notification[]> {
    const { data } = await http.get<Notification[] | null>(`${apiPrefix}/me/notifications`, {
      params: { limit: 50, offset: 0, ...filters },
    });
    return asArray(data);
  },

  async markNotificationRead(notificationId: ID): Promise<Notification> {
    const { data } = await http.patch<Notification>(`${apiPrefix}/me/notifications/${notificationId}/read`);
    return data;
  },

  async markAllNotificationsRead(): Promise<void> {
    await http.post(`${apiPrefix}/me/notifications/read-all`);
  },

  async addTicketLink(ticketId: ID, payload: AddTicketLinkPayload): Promise<void> {
    await http.post(`${apiPrefix}/tickets/${ticketId}/links`, payload);
  },

  async removeTicketLink(linkId: ID): Promise<void> {
    await http.delete(`${apiPrefix}/links/${linkId}`);
  },

  async listTicketGitHubCommits(ticketId: ID): Promise<GitHubCommit[]> {
    const { data } = await http.get<GitHubCommit[] | null>(`${apiPrefix}/tickets/${ticketId}/github/commits`);
    return asArray(data);
  },

  async linkTicketGitHubCommit(ticketId: ID, payload: LinkGitHubCommitPayload): Promise<GitHubCommit> {
    const { data } = await http.post<GitHubCommit>(`${apiPrefix}/tickets/${ticketId}/github/commits`, payload);
    return data;
  },

  async unlinkTicketGitHubCommit(ticketId: ID, commitId: ID): Promise<void> {
    await http.delete(`${apiPrefix}/tickets/${ticketId}/github/commits/${commitId}`);
  },

  async listComments(ticketId: ID): Promise<Comment[]> {
    const { data } = await http.get<Comment[] | null>(`${apiPrefix}/tickets/${ticketId}/comments`, {
      params: { limit: 100, offset: 0 },
    });
    return asArray(data);
  },

  async createComment(ticketId: ID, content: string): Promise<Comment> {
    const { data } = await http.post<Comment>(`${apiPrefix}/tickets/${ticketId}/comments`, { content });
    return data;
  },

  async deleteComment(commentId: ID): Promise<void> {
    await http.delete(`${apiPrefix}/comments/${commentId}`);
  },

  async getProjectGraph(projectId: ID, filters: GraphFilters = {}): Promise<GraphData> {
    const { data } = await http.get<GraphData>(`${apiPrefix}/projects/${projectId}/graph`, {
      params: filters,
    });
    return data;
  },

  async listProjectDeliveries(projectId: ID, filters: DeliveryFilters = {}): Promise<ProjectDelivery[]> {
    const { data } = await http.get<ProjectDelivery[] | null>(`${apiPrefix}/projects/${projectId}/deliveries`, {
      params: { limit: 100, ...filters },
    });
    return asArray(data);
  },

  async getProjectDeliverySummary(projectId: ID): Promise<ProjectDeliverySummary> {
    const { data } = await http.get<ProjectDeliverySummary>(`${apiPrefix}/projects/${projectId}/deliveries/summary`);
    return data;
  },

  async retryProjectDelivery(projectId: ID, deliveryId: ID): Promise<ProjectDelivery> {
    const { data } = await http.post<ProjectDelivery>(`${apiPrefix}/projects/${projectId}/deliveries/${deliveryId}/retry`);
    return data;
  },

  async listPersonalFederationInbox(filters: FederationInboxFilters = {}): Promise<FederationInboxActivity[]> {
    const { data } = await http.get<FederationInboxActivity[] | null>(`${apiPrefix}/me/federation/inbox`, {
      params: { limit: 100, offset: 0, ...filters },
    });
    return asArray(data);
  },

  async listPersonalFederationFollows(filters: FederationFollowFilters = {}): Promise<FederationRemoteFollow[]> {
    const { data } = await http.get<FederationRemoteFollow[] | null>(`${apiPrefix}/me/federation/follows`, {
      params: { limit: 100, offset: 0, ...filters },
    });
    return asArray(data);
  },

  async discoverPersonalFederationActor(resource: string): Promise<FederationRemoteActor> {
    const payload: RemoteFederationResourcePayload = { resource };
    const { data } = await http.post<FederationRemoteActor>(`${apiPrefix}/me/federation/discover`, payload);
    return data;
  },

  async followPersonalFederationActor(resource: string): Promise<FollowRemoteActorResult> {
    const payload: RemoteFederationResourcePayload = { resource };
    const { data } = await http.post<FollowRemoteActorResult>(`${apiPrefix}/me/federation/follows`, payload);
    return data;
  },

  async listFederationDomainBlocks(): Promise<DomainBlock[]> {
    const { data } = await http.get<DomainBlock[] | null>(`${apiPrefix}/admin/federation/domain-blocks`);
    return asArray(data);
  },

  async blockFederationDomain(payload: BlockDomainPayload): Promise<DomainBlock> {
    const { data } = await http.post<DomainBlock>(`${apiPrefix}/admin/federation/domain-blocks`, payload);
    return data;
  },

  async unblockFederationDomain(domain: string): Promise<void> {
    await http.delete(`${apiPrefix}/admin/federation/domain-blocks/${encodeURIComponent(domain)}`);
  },

  async listFederationRemoteActors(filters: RemoteActorFilters = {}): Promise<RemoteActorInspection[]> {
    const { data } = await http.get<RemoteActorInspection[] | null>(`${apiPrefix}/admin/federation/remote-actors`, {
      params: { limit: 100, ...filters },
    });
    return asArray(data);
  },

  async listFederationDeliveries(filters: FederationDeliveryFilters = {}): Promise<FederationDelivery[]> {
    const { data } = await http.get<FederationDelivery[] | null>(`${apiPrefix}/admin/federation/deliveries`, {
      params: { limit: 100, ...filters },
    });
    return asArray(data);
  },

  async getFederationDeliverySummary(): Promise<FederationDeliverySummary> {
    const { data } = await http.get<FederationDeliverySummary>(`${apiPrefix}/admin/federation/deliveries/summary`);
    return data;
  },

  async retryFederationDelivery(deliveryId: ID): Promise<FederationDelivery> {
    const { data } = await http.post<FederationDelivery>(`${apiPrefix}/admin/federation/deliveries/${deliveryId}/retry`);
    return data;
  },
};
