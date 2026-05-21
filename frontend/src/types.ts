export type ID = string;

export type InstanceRole = "owner" | "admin" | "user";
export type ProjectRoleKey = string;
export type ProjectPermission =
  | "project.read"
  | "project.update"
  | "project.delete"
  | "members.invite"
  | "members.remove"
  | "roles.manage"
  | "tickets.create"
  | "tickets.update"
  | "tickets.delete"
  | "comments.create"
  | "comments.moderate"
  | "federation.delivery.retry";
export type TicketStatus = "open" | "in_progress" | "review" | "done";
export type TicketPriority = "low" | "medium" | "high" | "urgent";
export type TicketType = "epic" | "task" | "subtask";

export interface SessionUser {
  userId: ID;
  instanceRole: InstanceRole;
  email?: string;
}

export interface Project {
  id: ID;
  ap_id: string;
  name: string;
  description: string;
  owner_id: ID;
  handle: string;
  created_at: string;
  updated_at: string;
}

export interface ProjectInvite {
  id: ID;
  ap_id: string;
  project_id: ID;
  inviter_actor_id: ID;
  invitee_actor_id: ID;
  role_id: ID;
  role: ProjectRoleKey;
  status: "pending" | "accepted" | "rejected" | "revoked";
  created_at: string;
  updated_at: string;
}

export interface ProjectRole {
  id: ID;
  project_id: ID;
  key: ProjectRoleKey;
  name: string;
  description: string;
  is_system: boolean;
  position: number;
  permissions: ProjectPermission[];
  created_at: string;
  updated_at: string;
}

export interface Ticket {
  id: ID;
  ap_id: string;
  title: string;
  description: string;
  status: TicketStatus;
  priority: TicketPriority;
  type: TicketType;
  parent_id: ID | null;
  project_id: ID;
  reporter_id: ID;
  assignee_id: ID | null;
  is_resolved: boolean;
  created_at: string;
  updated_at: string;
}

export interface Comment {
  id: ID;
  ap_id: string;
  ticket_id: ID;
  author_id: ID;
  content: string;
  created_at: string;
  updated_at: string;
}

export interface GraphNode {
  id: ID;
  label: string;
  type: TicketType;
  status: TicketStatus;
  priority: TicketPriority;
  group: string;
  x?: number;
  y?: number;
}

export interface GraphLink {
  source: ID | GraphNode;
  target: ID | GraphNode;
  type: string;
}

export interface GraphData {
  nodes: GraphNode[];
  links: GraphLink[];
}

export interface AdminUser {
  id: ID;
  ap_id: string;
  username: string;
  email: string;
  instance_role: InstanceRole;
  handle: string;
  name: string;
  created_at: string;
  updated_at: string;
}
