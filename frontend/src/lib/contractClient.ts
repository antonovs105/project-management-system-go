import createClient from "openapi-fetch";
import type { paths } from "../generated/api-schema";

const contractBaseURL = import.meta.env.VITE_API_URL || (import.meta.env.PROD ? "" : "http://localhost:8080");

export const contractClient = createClient<paths>({
  baseUrl: contractBaseURL,
  credentials: "include",
});

type ContractResult<T> = {
  data?: T;
  error?: unknown;
  response: Response;
};

export function contractData<T>(result: ContractResult<T>): T {
  if (result.response.ok && result.data !== undefined) {
    return result.data;
  }
  throw contractError(result.error, result.response.status);
}

export function contractVoid(result: ContractResult<unknown>): void {
  if (result.response.ok) {
    return;
  }
  throw contractError(result.error, result.response.status);
}

function contractError(value: unknown, status: number): Error {
  if (value && typeof value === "object" && "error" in value) {
    const detail = (value as { error?: unknown }).error;
    if (detail && typeof detail === "object" && "message" in detail) {
      const message = (detail as { message?: unknown }).message;
      if (typeof message === "string" && message) {
        return new Error(message);
      }
    }
  }
  return new Error(`API request failed (${status})`);
}
