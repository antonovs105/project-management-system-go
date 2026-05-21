import type { SessionUser, UserRole } from "../types";

interface JwtPayload {
  sub?: unknown;
  role?: unknown;
  exp?: unknown;
}

function decodeBase64Url(value: string): string {
  const normalized = value.replace(/-/g, "+").replace(/_/g, "/");
  const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, "=");
  return atob(padded);
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
    const role: UserRole = parsed.role === "admin" ? "admin" : "worker";
    if (typeof parsed.exp === "number" && parsed.exp * 1000 < Date.now()) {
      return null;
    }
    return {
      userId: parsed.sub,
      role,
      email,
    };
  } catch {
    return null;
  }
}
