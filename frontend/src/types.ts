export type ID = string;

export type InstanceRole = "owner" | "admin" | "user";
export type OAuthProvider = "google" | "github";
export type ProjectCreationPolicy = "everyone" | "admins_only";
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
	emailVerified?: boolean;
	mfaEnrollmentRequired?: boolean;
}

export interface AccountSession {
	id: ID;
	user_agent: string;
	ip_address: string;
	created_at: string;
	last_seen_at: string;
	expires_at: string;
	revoked_at?: string;
	current: boolean;
}

export interface SecurityEvent {
	id: ID;
	event_type: string;
	ip_address: string;
	user_agent: string;
	metadata: Record<string, unknown>;
	created_at: string;
}

export interface ProjectActivityEvent {
	id: ID;
	project_id: ID;
	actor_id: ID | null;
	actor_handle: string | null;
	entity_type: "project" | "ticket";
	entity_id: ID;
	action: "created" | "updated" | "archived" | "restored";
	before_state: Record<string, unknown> | null;
	after_state: Record<string, unknown> | null;
	created_at: string;
}

export interface ArchivedProject {
	id: ID;
	name: string;
	description: string;
	version: number;
	archived_at: string;
}

export interface ArchivedTicket {
	id: ID;
	project_id: ID;
	title: string;
	version: number;
	archived_at: string;
}

export interface TicketAttachment {
	id: ID;
	ticket_id: ID;
	uploader_id: ID;
	filename: string;
	content_type: string;
	size_bytes: number;
	sha256: string;
	created_at: string;
}

export interface PublicInstanceConfig {
  name: string;
  version: string;
  registration_enabled: boolean;
  project_creation_policy: ProjectCreationPolicy;
  oauth_providers: OAuthProvider[];
  attachments_enabled: boolean;
}

export interface InstanceCapabilities extends PublicInstanceConfig {
  instance_role: InstanceRole;
  can_create_projects: boolean;
}

export interface Project {
  id: ID;
  ap_id: string;
  name: string;
  description: string;
  owner_id: ID;
  version: number;
  archived_at: string | null;
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

export interface ProjectInviteInspection extends ProjectInvite {
  project_name: string;
  project_handle: string;
  role_name: string;
  inviter_username: string;
  inviter_email: string;
  inviter_handle: string;
  inviter_name: string;
  invitee_username: string;
  invitee_email: string;
  invitee_handle: string;
  invitee_name: string;
}

export interface RemoteProjectInvite {
  id: ID;
  invite_ap_id: string;
  activity_id: ID;
  project_ap_id: string;
  project_name: string;
  inviter_actor_id: ID;
  inviter_ap_id: string;
  inviter_handle: string;
  inviter_name: string;
  invitee_actor_id: ID;
  role: string;
  role_permissions: string[];
  target_inbox_url: string;
  status: "pending" | "accepted" | "rejected";
  created_at: string;
  updated_at: string;
  resolved_at?: string;
}

export interface RemoteProjectInviteResult {
  invite: RemoteProjectInvite;
  delivery?: {
    id: ID;
    activity_ap_id: string;
    target_inbox_url: string;
    state: DeliveryState;
    created_at: string;
    updated_at: string;
  };
}

export interface RemoteProject {
  id: ID;
  project_ap_id: string;
  project_name: string;
  role: string;
  role_permissions: string[];
  target_inbox_url: string;
  inviter_actor_id: ID;
  inviter_ap_id: string;
  inviter_handle: string;
  inviter_name: string;
  remote_actor_id?: ID;
  remote_handle?: string;
  created_at: string;
  updated_at: string;
  resolved_at?: string;
}

export interface RemoteTicketWriteResult {
  ticket?: Ticket;
  delivery?: {
    id: ID;
    activity_ap_id: string;
    target_inbox_url: string;
    state: DeliveryState;
    created_at: string;
    updated_at: string;
  };
}

export interface ProjectMember {
  user_id: ID;
  project_id: ID;
  role_id: ID;
  role: ProjectRoleKey;
  role_name: string;
  username: string;
  email: string;
  handle: string;
  name: string;
  is_remote: boolean;
  created_at: string;
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
  rank: string;
  parent_id: ID | null;
  project_id: ID;
  reporter_id: ID;
  assignee_id: ID | null;
  is_resolved: boolean;
  due_date: string | null;
  label_ids: ID[];
  version: number;
  archived_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface Label {
  id: ID;
  project_id: ID;
  name: string;
  color: string;
  created_at: string;
}

export type TicketEventType = "ticket.created" | "ticket.updated" | "ticket.deleted" | "ticket.linked" | "ticket.unlinked";

export interface TicketEvent {
  id: ID;
  type: TicketEventType;
  project_id: ID;
  ticket_id?: ID;
  occurred_at: string;
}

export type NotificationType =
  | "ticket.assigned"
  | "ticket.status_changed"
  | "ticket.due_soon"
  | "ticket.overdue"
  | "comment.created"
  | "comment.mentioned"
  | "project.invited"
  | "project.role_changed"
  | "federation.delivery_failed"
  | "security.event";

export interface Notification {
  id: ID;
  user_id: ID;
  actor_id?: ID;
  project_id?: ID;
  ticket_id?: ID;
  type: NotificationType;
  title: string;
  body: string;
  read_at?: string;
  created_at: string;
}

export interface NotificationPreference {
  type: NotificationType;
  in_app_enabled: boolean;
  email_enabled: boolean;
  updated_at?: string;
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
  limit: number;
  truncated: boolean;
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

export interface GitHubRepository {
  id: ID;
  project_id: ID;
  owner: string;
  name: string;
  full_name: string;
  html_url: string;
  default_branch: string;
  last_synced_at?: string | null;
  last_sync_error: string;
  last_webhook_at?: string | null;
  commit_count: number;
  linked_commit_count: number;
  manual_link_count: number;
  created_by?: ID | null;
  created_at: string;
  updated_at: string;
}

export interface GitHubCommit {
  id: ID;
  repository_id: ID;
  repository_full_name: string;
  repository_html_url: string;
  sha: string;
  short_sha: string;
  message: string;
  author_name: string;
  author_email: string;
  authored_at?: string | null;
  html_url: string;
  ticket_ids: ID[];
  link_source: "" | "message" | "manual";
  created_at: string;
  updated_at: string;
}

export interface GitHubSyncResult {
  repository: GitHubRepository;
  imported: number;
  linked: number;
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
