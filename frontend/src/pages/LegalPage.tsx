import { ArrowLeft, CircleDot } from "lucide-react";
import { Link } from "react-router-dom";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { Panel } from "../components/ui";
import { useI18n } from "../lib/i18n-context";
import type { MessageKey } from "../lib/i18n-messages";

const sections: Array<[MessageKey, MessageKey]> = [
  ["legal.copyrightTitle", "legal.copyrightBody"],
  ["legal.useTitle", "legal.useBody"],
  ["legal.dataTitle", "legal.dataBody"],
  ["legal.disclaimerTitle", "legal.disclaimerBody"],
];

export function LegalPage() {
  const { t } = useI18n();

  return (
    <main className="min-h-screen bg-zinc-100 px-4 py-6 text-zinc-950">
      <div className="mx-auto grid max-w-3xl gap-4">
        <div className="flex items-center justify-between gap-3">
          <Link to="/projects" className="focus-ring inline-flex items-center gap-2 rounded-2xl text-base font-semibold text-zinc-950">
            <span className="flex h-8 w-8 items-center justify-center rounded-2xl bg-zinc-950 text-white">
              <CircleDot size={17} />
            </span>
            {t("app.name")}
          </Link>
          <LanguageSwitcher />
        </div>

        <Panel className="overflow-hidden">
          <div className="border-b border-zinc-100 p-5">
            <Link className="focus-ring mb-4 inline-flex items-center gap-2 rounded-full text-sm text-zinc-500 hover:text-zinc-950" to="/projects">
              <ArrowLeft size={16} />
              {t("layout.workspace")}
            </Link>
            <h1 className="text-3xl font-semibold text-zinc-950">{t("legal.title")}</h1>
            <p className="mt-2 text-sm text-zinc-500">{t("legal.subtitle")}</p>
          </div>

          <div className="grid gap-5 p-5">
            {sections.map(([title, body]) => (
              <section key={title} className="rounded-2xl border border-zinc-200 bg-zinc-50 p-4">
                <h2 className="text-base font-semibold text-zinc-950">{t(title)}</h2>
                <p className="mt-2 text-sm leading-6 text-zinc-600">{t(body)}</p>
              </section>
            ))}
          </div>

          <div className="border-t border-zinc-100 px-5 py-4 text-xs text-zinc-500">{t("app.copyright")}</div>
        </Panel>
      </div>
    </main>
  );
}
