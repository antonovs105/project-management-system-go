import type {
  AdminAuditAction,
  AdminAuditTargetType,
  DeliveryFailureKind,
  DeliveryState,
  InstanceRole,
  ProjectPermission,
  TicketPriority,
  TicketStatus,
  TicketType,
} from "../types";

export const ticketStatuses: Array<{ id: TicketStatus; label: string }> = [
  { id: "open", label: "Open" },
  { id: "in_progress", label: "In Progress" },
  { id: "review", label: "Review" },
  { id: "done", label: "Done" },
];

export const ticketPriorities: Array<{ id: TicketPriority; label: string }> = [
  { id: "low", label: "Low" },
  { id: "medium", label: "Medium" },
  { id: "high", label: "High" },
  { id: "urgent", label: "Urgent" },
];

export const ticketTypes: Array<{ id: TicketType; label: string }> = [
  { id: "epic", label: "Epic" },
  { id: "task", label: "Task" },
  { id: "subtask", label: "Subtask" },
];

export const instanceRoles: Array<{ id: InstanceRole; label: string }> = [
  { id: "owner", label: "Owner" },
  { id: "admin", label: "Admin" },
  { id: "user", label: "User" },
];

export const projectPermissionGroups: Array<{
  group: string;
  permissions: Array<{ id: ProjectPermission; label: string }>;
}> = [
  {
    group: "Project",
    permissions: [
      { id: "project.read", label: "Read project" },
      { id: "project.update", label: "Update project" },
      { id: "project.delete", label: "Delete project" },
    ],
  },
  {
    group: "Members",
    permissions: [
      { id: "members.invite", label: "Invite members" },
      { id: "members.remove", label: "Remove members" },
      { id: "roles.manage", label: "Manage roles" },
    ],
  },
  {
    group: "Tickets",
    permissions: [
      { id: "tickets.create", label: "Create tickets" },
      { id: "tickets.update", label: "Update tickets" },
      { id: "tickets.delete", label: "Delete tickets" },
    ],
  },
  {
    group: "Comments",
    permissions: [
      { id: "comments.create", label: "Create comments" },
      { id: "comments.moderate", label: "Moderate comments" },
    ],
  },
  {
    group: "Federation",
    permissions: [{ id: "federation.delivery.retry", label: "Retry deliveries" }],
  },
];

export const deliveryStates: Array<{ id: DeliveryState; label: string }> = [
  { id: "pending", label: "Pending" },
  { id: "processing", label: "Processing" },
  { id: "delivered", label: "Delivered" },
  { id: "failed", label: "Failed" },
  { id: "dead", label: "Dead" },
];

export const deliveryFailureKinds: Array<{ id: DeliveryFailureKind; label: string }> = [
  { id: "http", label: "HTTP" },
  { id: "network", label: "Network" },
  { id: "signing", label: "Signing" },
  { id: "safety", label: "Safety" },
  { id: "unknown", label: "Unknown" },
];

export const adminAuditActions: Array<{ id: AdminAuditAction; label: string }> = [
  { id: "user.instance_role_updated", label: "Role updated" },
  { id: "federation.domain_blocked", label: "Domain blocked" },
  { id: "federation.domain_unblocked", label: "Domain unblocked" },
  { id: "federation.delivery_retried", label: "Delivery retried" },
];

export const adminAuditTargetTypes: Array<{ id: AdminAuditTargetType; label: string }> = [
  { id: "user", label: "User" },
  { id: "federation_domain", label: "Federation domain" },
  { id: "federation_delivery", label: "Federation delivery" },
];

export const ticketLinkTypes = [
  { id: "blocks", label: "Blocks" },
  { id: "relates_to", label: "Relates to" },
  { id: "duplicates", label: "Duplicates" },
  { id: "depends_on", label: "Depends on" },
] as const;
