import { createContext, useContext } from "react";
import type { Locale, MessageKey } from "./i18n-messages";
import type { PluralForms } from "./format";

export interface I18nContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: MessageKey, values?: Record<string, string | number>) => string;
  relativeDate: (value: string) => string;
  dateTime: (value: string) => string;
  number: (value: number) => string;
  plural: (count: number, forms: PluralForms) => string;
}

export const I18nContext = createContext<I18nContextValue | null>(null);

export function useI18n(): I18nContextValue {
  const value = useContext(I18nContext);
  if (!value) {
    throw new Error("useI18n must be used within I18nProvider");
  }
  return value;
}
