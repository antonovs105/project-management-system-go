import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { I18nContext } from "../lib/i18n-context";
import type { I18nContextValue } from "../lib/i18n-context";
import { messages, supportedLocales } from "../lib/i18n-messages";
import type { Locale } from "../lib/i18n-messages";
import { dateTime, number, plural, relativeDate } from "../lib/format";

function isLocale(value: string | null | undefined): value is Locale {
  return supportedLocales.includes(value as Locale);
}

function detectLocale(): Locale {
  const stored = window.localStorage.getItem("taskflow.locale");
  if (isLocale(stored)) {
    return stored;
  }
  const preferred = window.navigator.language.toLowerCase();
  return preferred.startsWith("uk") ? "uk" : "en";
}

function formatMessage(template: string, values: Record<string, string | number> = {}): string {
  return Object.entries(values).reduce((message, [key, value]) => message.replaceAll(`{${key}}`, String(value)), template);
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState<Locale>(detectLocale);

  useEffect(() => {
    window.localStorage.setItem("taskflow.locale", locale);
    document.documentElement.lang = locale === "uk" ? "uk" : "en";
  }, [locale]);

  const value = useMemo<I18nContextValue>(
    () => ({
      locale,
      setLocale,
      t: (key, values) => formatMessage(messages[locale][key] || messages.en[key], values),
      relativeDate: (value) => relativeDate(value, locale),
      dateTime: (value) => dateTime(value, locale),
      number: (value) => number(value, locale),
      plural: (count, forms) => plural(count, forms, locale),
    }),
    [locale],
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}
