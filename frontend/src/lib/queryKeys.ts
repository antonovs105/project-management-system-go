import type { ID } from "../types";

export const queryKeys = {
  profile: ["profile"] as const,
  projects: ["projects"] as const,
  project: (projectId: ID) => ["project", projectId] as const,
  tickets: (projectId: ID) => ["tickets", projectId] as const,
  ticket: (ticketId: ID) => ["ticket", ticketId] as const,
  comments: (ticketId: ID) => ["comments", ticketId] as const,
  graph: (projectId: ID) => ["graph", projectId] as const,
};
