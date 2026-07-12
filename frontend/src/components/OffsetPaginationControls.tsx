import { ChevronLeft, ChevronRight } from "lucide-react";
import type { OffsetPage } from "../lib/api";
import { Button } from "./ui";

export function OffsetPaginationControls<T>({ page, onOffsetChange, disabled = false }: {
  page: OffsetPage<T>;
  onOffsetChange: (offset: number) => void;
  disabled?: boolean;
}) {
  const pageNumber = Math.floor(page.offset / page.limit) + 1;
  return (
    <nav className="flex items-center justify-between gap-3" aria-label="Pagination">
      <Button onClick={() => onOffsetChange(Math.max(0, page.offset - page.limit))} disabled={disabled || page.offset === 0}>
        <ChevronLeft size={15} />Previous
      </Button>
      <span className="text-sm text-zinc-500">Page {pageNumber} · {page.items.length} shown</span>
      <Button onClick={() => onOffsetChange(page.offset + page.limit)} disabled={disabled || !page.hasMore}>
        Next<ChevronRight size={15} />
      </Button>
    </nav>
  );
}
