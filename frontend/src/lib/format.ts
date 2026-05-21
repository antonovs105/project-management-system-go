import { formatDistanceToNowStrict, parseISO } from "date-fns";

export function relativeDate(value: string): string {
  try {
    return `${formatDistanceToNowStrict(parseISO(value))} ago`;
  } catch {
    return "unknown";
  }
}

export function initials(value: string | undefined): string {
  if (!value) {
    return "U";
  }
  return value
    .split(/[\s@._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase())
    .join("");
}

export function compactId(value: string): string {
  if (value.length <= 8) {
    return value;
  }
  return value.slice(0, 8);
}
