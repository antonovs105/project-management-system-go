import type * as API from "./generated/api-schema";

export type ID = string;

export type InstanceRole = API.InstanceRole;
export type OAuthProvider = API.PublicInstanceConfig["oauth_providers"][number];
export type ProjectCreationPolicy = API.PublicInstanceConfig["project_creation_policy"];
export type ProjectRoleKey = API.ProjectRoleKey;
export type ProjectPermission = API.ProjectPermission;
export type TicketStatus = API.TicketStatus;
export type TicketPriority = API.TicketPriority;
export type TicketType = API.TicketType;
export type DeliveryState = API.ProjectDelivery["state"];
export type DeliveryFailureKind = NonNullable<API.ProjectDelivery["last_failure_kind"]>;
export type FederationFollowState = API.FederationRemoteFollow["state"];
export type AdminAuditAction = API.AdminAuditAction;
export type AdminAuditTargetType = API.AdminAuditTargetType;

export interface SessionUser {
  userId: ID;
  instanceRole: InstanceRole;
  email?: string;
  emailVerified?: boolean;
  mfaEnrollmentRequired?: boolean;
}

export type AccountSession = API.AccountSession;
export type APIToken = API.ApiToken;
export type CreatedAPIToken = API.CreatedApiToken;
export type APITokenScope = API.ApiTokenScope;
export type SecurityEvent = API.SecurityEvent;
export type ProjectActivityEvent = API.ProjectActivityEvent;
export type ArchivedProject = API.ArchivedProject;
export type ArchivedTicket = API.ArchivedTicket;
export type TicketAttachment = API.TicketAttachment;
export type PublicInstanceConfig = API.PublicInstanceConfig;
export type InstanceCapabilities = API.InstanceCapabilities;
export type Project = API.Project;
export type ProjectInvite = API.ProjectInvite;
export type ProjectInviteInspection = API.ProjectInviteInspection;
export type RemoteProjectInvite = API.RemoteProjectInvite;
export type RemoteProjectInviteResult = API.RemoteProjectInviteResult;
export type RemoteProject = API.RemoteProject;
export type RemoteTicket = API.RemoteTicket;
export type RemoteTicketWriteResult = API.RemoteTicketWriteResult;
export type ProjectMember = API.ProjectMember;
export type ProjectRole = API.ProjectRole;
export type Ticket = API.Ticket;
export type BoardTicket = Pick<Ticket, "id" | "title" | "description" | "status" | "priority" | "type"> &
  Partial<Pick<Ticket, "assignee_id" | "due_date" | "label_ids">>;
export type Label = API.Label;
export type TicketEventType = API.TicketEvent["type"];
export type TicketEvent = API.TicketEvent;
export type NotificationType = API.NotificationType;
export type Notification = API.Notification;
export type NotificationPreference = API.NotificationPreference;
export type Comment = API.Comment;

export type GraphNode = API.GraphResponse["nodes"][number] & {
  x?: number;
  y?: number;
};

export type GraphLink = Omit<API.GraphResponse["links"][number], "source" | "target"> & {
  source: ID | GraphNode;
  target: ID | GraphNode;
};

export type GraphData = Omit<API.GraphResponse, "nodes" | "links"> & {
  nodes: GraphNode[];
  links: GraphLink[];
};

export type AdminUser = API.User;
export type AdminAuditEvent = API.AdminAuditEvent;
export type ProjectDelivery = API.ProjectDelivery;
export type ProjectDeliverySummary = API.DeliverySummary;
export type GitHubRepository = API.GitHubRepository;
export type GitHubCommit = API.GitHubCommit;
export type GitHubSyncResult = API.GitHubSyncResult;
export type ProjectWebhook = API.ProjectWebhook;
export type CreatedProjectWebhook = API.CreatedProjectWebhook;
export type ProjectWebhookDelivery = API.ProjectWebhookDelivery;
export type WebhookEvent = API.WebhookEvent;
export type ProjectBundle = API.ProjectBundle;
export type UserBundle = API.UserBundle;
export type ImportResult = API.ImportResult;
export type DomainBlock = API.FederationDomainBlock;
export type RemoteActorInspection = API.FederationRemoteActor;
export type FederationDelivery = API.FederationDelivery;
export type FederationDeliverySummary = API.FederationDeliverySummary;
export type FederationInboxActivity = API.FederationInboxActivity;
export type FederationRemoteFollow = API.FederationRemoteFollow;
export type FederationRemoteActor = API.FederationRemoteActor;
export type FederationFollowDelivery = Omit<API.FederationFollowDelivery, "state"> & { state: DeliveryState };
export type FollowRemoteActorResult = Omit<API.FollowRemoteActorResult, "delivery"> & {
  delivery?: FederationFollowDelivery;
};
