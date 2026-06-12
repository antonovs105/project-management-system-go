import { Languages } from "lucide-react";
import { useI18n } from "../lib/i18n-context";
import { supportedLocales } from "../lib/i18n-messages";
import type { Locale } from "../lib/i18n-messages";

const localeLabels: Record<Locale, string> = {
  en: "EN",
  uk: "UA",
};

export function LanguageSwitcher() {
  const { locale, setLocale, t } = useI18n();

  return (
    <div className="inline-flex items-center gap-1 rounded-full border border-zinc-200 bg-white p-1 shadow-sm" aria-label={t("language.label")}>
      <Languages size={15} className="ml-2 text-zinc-500" />
      {supportedLocales.map((item) => (
        <button
          key={item}
          type="button"
          title={item === "uk" ? t("language.ukrainian") : t("language.english")}
          className={`focus-ring rounded-full px-2.5 py-1 text-xs font-semibold transition ${
            locale === item ? "bg-zinc-950 text-white" : "text-zinc-500 hover:bg-zinc-100 hover:text-zinc-950"
          }`}
          onClick={() => setLocale(item)}
        >
          {localeLabels[item]}
        </button>
      ))}
    </div>
  );
}
