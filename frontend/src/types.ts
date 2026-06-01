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
export type DeliveryState = "pending" | "processing" | "delivered" | "failed" | "dead";
export type DeliveryFailureKind = "http" | "network" | "signing" | "safety" | "unknown";
export type FederationFollowState = "pending" | "accepted" | "rejected";
export type AdminAuditAction =
  | "user.instance_role_updated"
  | "federation.domain_blocked"
  | "federation.domain_unblocked"
  | "federation.delivery_retried";
export type AdminAuditTargetType = "user" | "federation_domain" | "federation_delivery";

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
  summary: string;
  created_at: string;
  updated_at: string;
}

export interface AdminAuditEvent {
  id: ID;
  actor_user_id?: ID;
  action: AdminAuditAction;
  target_type: AdminAuditTargetType;
  target_id: string;
  metadata: Record<string, unknown>;
  created_at: string;
}

export interface ProjectDelivery {
  id: ID;
  activity_ap_id: string;
  activity_type: string;
  object_ap_id?: string;
  target_ap_id?: string;
  target_inbox_url: string;
  state: DeliveryState;
  attempts: number;
  max_attempts: number;
  next_attempt_at?: string;
  last_error?: string;
  last_attempt_at?: string;
  last_failure_kind?: DeliveryFailureKind;
  last_status_code?: number;
  delivered_at?: string;
  can_retry: boolean;
  created_at: string;
  updated_at: string;
}

export interface ProjectDeliverySummary {
  total: number;
  pending: number;
  processing: number;
  delivered: number;
  failed: number;
  dead: number;
  retryable: number;
  can_retry: boolean;
}

export interface DomainBlock {
  id: ID;
  domain: string;
  reason: string;
  created_by?: ID;
  created_at: string;
  updated_at: string;
}

export interface RemoteActorInspection {
  id: ID;
  ap_id: string;
  type: string;
  preferred_username: string;
  handle: string;
  name: string;
  summary: string;
  inbox_url: string;
  outbox_url: string;
  followers_url?: string;
  following_url?: string;
  last_fetched_at?: string;
  fetch_error?: string;
  fetch_error_at?: string;
  created_at: string;
  updated_at: string;
}

export interface FederationDelivery extends ProjectDelivery {
  actor_ap_id: string;
  project_id?: ID;
  project_ap_id?: string;
}

export interface FederationDeliverySummary extends ProjectDeliverySummary {
  due_retry: number;
  http_failures: number;
  network_failures: number;
  signing_failures: number;
  safety_failures: number;
  unknown_failures: number;
  oldest_pending_at?: string;
  oldest_dead_at?: string;
}

export interface FederationInboxActivity {
  id: ID;
  activity_ap_id: string;
  activity_type: string;
  actor_id: ID;
  actor_ap_id: string;
  actor_type: string;
  actor_handle: string;
  actor_name: string;
  object_ap_id?: string;
  object_type?: string;
  object_name?: string;
  object_content?: string;
  target_ap_id?: string;
  target_actor_id?: ID;
  target_type?: string;
  target_handle?: string;
  target_name?: string;
  received_at: string;
  created_at: string;
}

export interface FederationRemoteFollow {
  actor_id: ID;
  actor_ap_id: string;
  actor_type: string;
  preferred_username: string;
  handle: string;
  name: string;
  summary: string;
  inbox_url: string;
  outbox_url: string;
  followers_url?: string;
  following_url?: string;
  state: FederationFollowState;
  created_at: string;
  updated_at: string;
}

export interface FederationRemoteActor {
  id: ID;
  ap_id: string;
  type: string;
  preferred_username: string;
  handle: string;
  name: string;
  summary: string;
  inbox_url: string;
  outbox_url: string;
  followers_url?: string;
  following_url?: string;
  last_fetched_at?: string;
  created_at: string;
  updated_at: string;
}

export interface FederationFollowDelivery {
  id: ID;
  activity_ap_id: string;
  target_inbox_url: string;
  state: DeliveryState;
  created_at: string;
  updated_at: string;
}

export interface FollowRemoteActorResult {
  follow: FederationRemoteFollow;
  delivery?: FederationFollowDelivery;
  created: boolean;
}
