import type { InstanceRole, SessionUser } from "../types";

interface JwtPayload {
  sub?: unknown;
  instance_role?: unknown;
  role?: unknown;
  exp?: unknown;
}

function decodeBase64Url(value: string): string {
  const normalized = value.replace(/-/g, "+").replace(/_/g, "/");
  const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, "=");
  return atob(padded);
}

function parseInstanceRole(value: unknown, legacyValue: unknown): InstanceRole {
  if (value === "owner" || value === "admin" || value === "user") {
    return value;
  }
  if (legacyValue === "admin") {
    return "admin";
  }
  return "user";
}

export function sessionFromToken(token: string, email?: string): SessionUser | null {
  const [, payload] = token.split(".");
  if (!payload) {
    return null;
  }

  try {
    const parsed = JSON.parse(decodeBase64Url(payload)) as JwtPayload;
    if (typeof parsed.sub !== "string") {
      return null;
    }
    const instanceRole = parseInstanceRole(parsed.instance_role, parsed.role);
    if (typeof parsed.exp === "number" && parsed.exp * 1000 < Date.now()) {
      return null;
    }
    return {
      userId: parsed.sub,
      instanceRole,
      email,
    };
  } catch {
    return null;
  }
}
