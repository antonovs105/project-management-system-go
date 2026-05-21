import type { ID } from "../types";

export const queryKeys = {
  profile: ["profile"] as const,
  projects: ["projects"] as const,
  project: (projectId: ID) => ["project", projectId] as const,
  projectRoles: (projectId: ID) => ["projectRoles", projectId] as const,
  projectDeliveries: (projectId: ID, state?: string) => ["projectDeliveries", projectId, state || "all"] as const,
  projectDeliverySummary: (projectId: ID) => ["projectDeliverySummary", projectId] as const,
  tickets: (projectId: ID) => ["tickets", projectId] as const,
  ticket: (ticketId: ID) => ["ticket", ticketId] as const,
  comments: (ticketId: ID) => ["comments", ticketId] as const,
  graph: (projectId: ID) => ["graph", projectId] as const,
  adminUsers: (role?: string, q?: string) => ["adminUsers", role || "all", q || ""] as const,
  adminAuditEvents: (action?: string, targetType?: string, actorUserId?: string) =>
    ["adminAuditEvents", action || "all", targetType || "all", actorUserId || ""] as const,
  federationDomainBlocks: ["federationDomainBlocks"] as const,
  federationRemoteActors: (fetchError?: string) => ["federationRemoteActors", fetchError || "all"] as const,
  federationDeliveries: (state?: string, failureKind?: string) =>
    ["federationDeliveries", state || "all", failureKind || "all"] as const,
  federationDeliverySummary: ["federationDeliverySummary"] as const,
};
