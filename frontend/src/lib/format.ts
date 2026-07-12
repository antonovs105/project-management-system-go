import type { Locale } from "./i18n-messages";

export type PluralForms = Partial<Record<Intl.LDMLPluralRule, string>> & { other: string };

function activeLocale(locale?: Locale): Locale {
  if (locale) return locale;
  return typeof document !== "undefined" && document.documentElement.lang.toLowerCase().startsWith("uk") ? "uk" : "en";
}

export function relativeDate(value: string, locale?: Locale, now = Date.now()): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return activeLocale(locale) === "uk" ? "невідомо" : "unknown";
  const seconds = (timestamp - now) / 1000;
  const units: Array<[Intl.RelativeTimeFormatUnit, number]> = [
    ["year", 365 * 24 * 60 * 60], ["month", 30 * 24 * 60 * 60], ["week", 7 * 24 * 60 * 60],
    ["day", 24 * 60 * 60], ["hour", 60 * 60], ["minute", 60], ["second", 1],
  ];
  const [unit, divisor] = units.find(([, size]) => Math.abs(seconds) >= size) || units.at(-1)!;
  return new Intl.RelativeTimeFormat(activeLocale(locale), { numeric: "auto" }).format(Math.round(seconds / divisor), unit);
}

export function dateTime(value: string, locale?: Locale): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return activeLocale(locale) === "uk" ? "невідомо" : "unknown";
  return new Intl.DateTimeFormat(activeLocale(locale), { dateStyle: "medium", timeStyle: "short" }).format(timestamp);
}

export function number(value: number, locale?: Locale): string {
  return new Intl.NumberFormat(activeLocale(locale)).format(value);
}

export function plural(count: number, forms: PluralForms, locale?: Locale): string {
  const category = new Intl.PluralRules(activeLocale(locale)).select(count);
  return (forms[category] || forms.other).replaceAll("{count}", number(count, locale));
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
