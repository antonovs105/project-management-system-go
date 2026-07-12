import type { ComponentPropsWithoutRef, FormEventHandler, InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from "react";
import { useEffect, useId, useRef } from "react";
import { X } from "lucide-react";
import { createPortal } from "react-dom";

type Tone = "primary" | "secondary" | "danger" | "ghost";

function cx(...classes: Array<string | false | null | undefined>): string {
  return classes.filter(Boolean).join(" ");
}

function buttonTone(tone: Tone): string {
  switch (tone) {
    case "primary":
      return "border-zinc-950 bg-zinc-950 text-white shadow-sm hover:bg-zinc-800";
    case "danger":
      return "border-red-600 bg-red-600 text-white shadow-sm hover:bg-red-700";
    case "ghost":
      return "border-transparent bg-transparent text-zinc-500 hover:bg-zinc-100 hover:text-zinc-950";
    default:
      return "border-zinc-200 bg-white text-zinc-800 shadow-sm hover:border-zinc-300 hover:bg-zinc-50";
  }
}

export function Button({
  children,
  tone = "secondary",
  className,
  type = "button",
  ...props
}: {
  children: ReactNode;
  tone?: Tone;
  className?: string;
  type?: "button" | "submit" | "reset";
} & Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, "type">) {
  return (
    <button
      type={type}
      className={cx(
        "focus-ring inline-flex h-9 items-center justify-center gap-2 rounded-full border px-4 text-sm font-medium transition disabled:opacity-50",
        buttonTone(tone),
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}

export function IconButton({
  label,
  children,
  className,
  tone = "ghost",
  ...props
}: {
  label: string;
  children: ReactNode;
  className?: string;
  tone?: Tone;
} & React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className={cx(
        "focus-ring inline-flex h-9 w-9 items-center justify-center rounded-full border text-sm transition disabled:opacity-50",
        buttonTone(tone),
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}

export function TextField({
  label,
  hint,
  className,
  ...props
}: {
  label: string;
  hint?: string;
} & InputHTMLAttributes<HTMLInputElement>) {
  return (
    <label className={cx("grid min-w-0 gap-1.5 text-sm", className)}>
      <span className="font-medium text-zinc-800">{label}</span>
      <input
        className="focus-ring h-10 w-full min-w-0 rounded-xl border border-zinc-200 bg-white px-3 text-sm text-zinc-950 shadow-sm placeholder:text-zinc-400"
        {...props}
      />
      {hint ? <span className="text-xs text-zinc-500">{hint}</span> : null}
    </label>
  );
}

export function TextAreaField({
  label,
  hint,
  className,
  ...props
}: {
  label: string;
  hint?: string;
} & TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <label className={cx("grid min-w-0 gap-1.5 text-sm", className)}>
      <span className="font-medium text-zinc-800">{label}</span>
      <textarea
        className="focus-ring min-h-24 w-full min-w-0 resize-y rounded-xl border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm placeholder:text-zinc-400"
        {...props}
      />
      {hint ? <span className="text-xs text-zinc-500">{hint}</span> : null}
    </label>
  );
}

export function SelectField({
  label,
  children,
  className,
  ...props
}: {
  label: string;
  children: ReactNode;
} & SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <label className={cx("grid min-w-0 gap-1.5 text-sm", className)}>
      <span className="font-medium text-zinc-800">{label}</span>
      <select
        className="focus-ring h-10 w-full min-w-0 rounded-xl border border-zinc-200 bg-white px-3 text-sm text-zinc-950 shadow-sm"
        {...props}
      >
        {children}
      </select>
    </label>
  );
}

export function Badge({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <span className={cx("inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium", className)}>
      {children}
    </span>
  );
}

export function Panel({ children, className, ...props }: ComponentPropsWithoutRef<"section">) {
	return <section className={cx("rounded-2xl border border-zinc-200 bg-white shadow-sm", className)} {...props}>{children}</section>;
}

export function EmptyState({
  icon,
  title,
  body,
  action,
}: {
  icon?: ReactNode;
  title: string;
  body: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-3xl border border-dashed border-zinc-300 bg-white px-6 py-10 text-center shadow-sm">
      {icon ? <div className="mb-3 rounded-2xl border border-zinc-200 bg-zinc-50 p-3 text-zinc-500">{icon}</div> : null}
      <h3 className="text-base font-semibold text-zinc-950">{title}</h3>
      <p className="mt-1 max-w-md text-sm text-zinc-500">{body}</p>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

export function LoadingState({ label }: { label: string }) {
  return (
    <div className="flex min-h-56 items-center justify-center text-sm text-zinc-500">
      <span className="mr-2 h-2 w-2 animate-pulse rounded-full bg-zinc-950" />
      {label}
    </div>
  );
}

export function ErrorState({ title, body }: { title: string; body: string }) {
  return (
    <div className="rounded-2xl border border-red-200 bg-red-50 px-4 py-3">
      <h3 className="text-sm font-semibold text-red-900">{title}</h3>
      <p className="mt-1 text-sm text-red-700">{body}</p>
    </div>
  );
}

export function Modal({
  open,
  title,
  children,
  onClose,
  footer,
  formId,
  onSubmit,
}: {
  open: boolean;
  title: string;
  children: ReactNode;
  onClose: () => void;
  footer?: ReactNode;
  formId?: string;
  onSubmit?: FormEventHandler<HTMLFormElement>;
}) {
	const titleId = useId();
	const formDialogRef = useRef<HTMLFormElement>(null);
	const divDialogRef = useRef<HTMLDivElement>(null);
	const onCloseRef = useRef(onClose);
	useEffect(() => {
		onCloseRef.current = onClose;
	}, [onClose]);

	useEffect(() => {
		if (!open) {
			return;
		}
		const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
		const dialog = formDialogRef.current || divDialogRef.current;
		const focusable = dialog?.querySelector<HTMLElement>(
			'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
		);
		(focusable || dialog)?.focus();

		function handleKeyDown(event: KeyboardEvent) {
			if (event.key === "Escape") {
				event.preventDefault();
				onCloseRef.current();
				return;
			}
			if (event.key !== "Tab" || !dialog) {
				return;
			}
			const items = Array.from(dialog.querySelectorAll<HTMLElement>(
				'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
			));
			if (items.length === 0) {
				event.preventDefault();
				dialog.focus();
				return;
			}
			const first = items[0];
			const last = items[items.length - 1];
			if (event.shiftKey && document.activeElement === first) {
				event.preventDefault();
				last.focus();
			} else if (!event.shiftKey && document.activeElement === last) {
				event.preventDefault();
				first.focus();
			}
		}

		document.addEventListener("keydown", handleKeyDown);
		return () => {
			document.removeEventListener("keydown", handleKeyDown);
			previousFocus?.focus();
		};
	}, [open]);

  if (!open) {
    return null;
  }

  const content = (
    <>
      <div className="flex items-center justify-between border-b border-zinc-200 px-5 py-4">
		<h2 id={titleId} className="text-base font-semibold text-zinc-950">{title}</h2>
        <IconButton label="Close" onClick={onClose}>
          <X size={18} />
        </IconButton>
      </div>
      <div className="px-5 py-4">{children}</div>
      {footer ? <div className="flex justify-end gap-2 border-t border-zinc-200 px-5 py-4">{footer}</div> : null}
    </>
  );

  return createPortal(
    <div className="fixed left-0 top-0 z-50 flex h-dvh w-screen items-center justify-center overflow-y-auto bg-zinc-950/60 p-4 backdrop-blur-sm">
      {formId ? (
        <form
		  ref={formDialogRef}
          id={formId}
		  role="dialog"
		  aria-modal="true"
		  aria-labelledby={titleId}
		  tabIndex={-1}
          onSubmit={onSubmit}
          className="max-h-[90vh] w-full max-w-xl overflow-auto rounded-3xl bg-white shadow-2xl"
        >
          {content}
        </form>
      ) : (
		<div ref={divDialogRef} role="dialog" aria-modal="true" aria-labelledby={titleId} tabIndex={-1} className="max-h-[90vh] w-full max-w-xl overflow-auto rounded-3xl bg-white shadow-2xl">{content}</div>
      )}
    </div>,
    document.body,
  );
}
