import { ChevronLeft, ChevronRight } from "lucide-react";
import type { OffsetPage } from "../lib/api";
import { Button } from "./ui";
import { useI18n } from "../lib/i18n-context";

export function OffsetPaginationControls<T>({ page, onOffsetChange, disabled = false }: {
  page: OffsetPage<T>;
  onOffsetChange: (offset: number) => void;
  disabled?: boolean;
}) {
  const { t } = useI18n();
  const pageNumber = Math.floor(page.offset / page.limit) + 1;
  return (
    <nav className="flex items-center justify-between gap-3" aria-label={t("common.pagination")}>
      <Button onClick={() => onOffsetChange(Math.max(0, page.offset - page.limit))} disabled={disabled || page.offset === 0}>
        <ChevronLeft size={15} />{t("common.previous")}
      </Button>
      <span className="text-sm text-zinc-500">{t("common.pageShown", { page: pageNumber, count: page.items.length })}</span>
      <Button onClick={() => onOffsetChange(page.offset + page.limit)} disabled={disabled || !page.hasMore}>
        {t("common.next")}<ChevronRight size={15} />
      </Button>
    </nav>
  );
}
