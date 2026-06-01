import axios from "axios";
import type {
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
  GraphData,
  ID,
  InstanceRole,
  Project,
  ProjectDelivery,
  ProjectDeliverySummary,
  ProjectInvite,
  ProjectPermission,
  ProjectRole,
  ProjectRoleKey,
  RemoteActorInspection,
  Ticket,
  TicketPriority,
  TicketStatus,
  TicketType,
} from "../types";
import { useAuthStore } from "../store/auth";

const http = axios.create({
  baseURL: import.meta.env.VITE_API_URL || "http://localhost:8080",
  timeout: 20_000,
});

http.interceptors.request.use((config) => {
  const token = useAuthStore.getState().token;
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
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
}

export interface UpdateTicketPayload {
  title?: string;
  description?: string;
  status?: TicketStatus;
  priority?: TicketPriority;
  type?: TicketType;
  parent_id?: ID | null;
  assignee_id?: ID | null;
}

export interface ChangePasswordPayload {
  current_password: string;
  new_password: string;
}

export interface AddProjectMemberPayload {
  user_id: ID;
  role_id?: ID;
  role?: ProjectRoleKey;
}

export interface CreateProjectRolePayload {
  name: string;
  description?: string;
  permissions: ProjectPermission[];
}

export interface UpdateProjectRolePayload {
  name?: string;
  description?: string;
  permissions?: ProjectPermission[];
}

export interface AddTicketLinkPayload {
  target_id: ID;
  link_type: string;
}

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

interface LoginResponse {
  token: string;
}

interface ProfileResponse {
  user_id: ID;
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

export const api = {
  async login(payload: LoginPayload): Promise<LoginResponse> {
    const { data } = await http.post<LoginResponse>("/login", payload);
    return data;
  },

  async register(payload: RegisterPayload): Promise<void> {
    await http.post("/register", payload);
  },

  async profile(): Promise<ProfileResponse> {
    const { data } = await http.get<ProfileResponse>(`${apiPrefix}/me`);
    return data;
  },

  async changePassword(payload: ChangePasswordPayload): Promise<void> {
    await http.patch(`${apiPrefix}/me/password`, payload);
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

  async inviteProjectMember(projectId: ID, payload: AddProjectMemberPayload): Promise<ProjectInvite> {
    const { data } = await http.post<ProjectInvite>(`${apiPrefix}/projects/${projectId}/members`, payload);
    return data;
  },

  async removeProjectMember(projectId: ID, userId: ID): Promise<void> {
    await http.delete(`${apiPrefix}/projects/${projectId}/members/${userId}`);
  },

  async acceptInvite(inviteId: ID): Promise<ProjectInvite> {
    const { data } = await http.post<ProjectInvite>(`${apiPrefix}/invites/${inviteId}/accept`);
    return data;
  },

  async rejectInvite(inviteId: ID): Promise<ProjectInvite> {
    const { data } = await http.post<ProjectInvite>(`${apiPrefix}/invites/${inviteId}/reject`);
    return data;
  },

  async revokeInvite(inviteId: ID): Promise<ProjectInvite> {
    const { data } = await http.post<ProjectInvite>(`${apiPrefix}/invites/${inviteId}/revoke`);
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

  async listTickets(projectId: ID): Promise<Ticket[]> {
    const { data } = await http.get<Ticket[] | null>(`${apiPrefix}/projects/${projectId}/tickets`, {
      params: { limit: 500, offset: 0 },
    });
    return asArray(data);
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

  async deleteTicket(ticketId: ID): Promise<void> {
    await http.delete(`${apiPrefix}/tickets/${ticketId}`);
  },

  async addTicketLink(ticketId: ID, payload: AddTicketLinkPayload): Promise<void> {
    await http.post(`${apiPrefix}/tickets/${ticketId}/links`, payload);
  },

  async removeTicketLink(linkId: ID): Promise<void> {
    await http.delete(`${apiPrefix}/links/${linkId}`);
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

  async getProjectGraph(projectId: ID): Promise<GraphData> {
    const { data } = await http.get<GraphData>(`${apiPrefix}/projects/${projectId}/graph`);
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
