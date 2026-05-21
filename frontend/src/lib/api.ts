import axios from "axios";
import type {
  Comment,
  GraphData,
  ID,
  Project,
  ProjectInvite,
  ProjectRole,
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

export interface AddProjectMemberPayload {
  user_id: ID;
  role: ProjectRole;
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
};
