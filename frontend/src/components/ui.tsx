import type { FormEventHandler, InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from "react";
import { X } from "lucide-react";

type Tone = "primary" | "secondary" | "danger" | "ghost";

function cx(...classes: Array<string | false | null | undefined>): string {
  return classes.filter(Boolean).join(" ");
}

function buttonTone(tone: Tone): string {
  switch (tone) {
    case "primary":
      return "border-cyan-700 bg-cyan-700 text-white hover:bg-cyan-800";
    case "danger":
      return "border-red-600 bg-red-600 text-white hover:bg-red-700";
    case "ghost":
      return "border-transparent bg-transparent text-slate-600 hover:bg-slate-100 hover:text-slate-950";
    default:
      return "border-slate-300 bg-white text-slate-700 hover:border-slate-400 hover:bg-slate-50";
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
        "focus-ring inline-flex h-9 items-center justify-center gap-2 rounded-md border px-3 text-sm font-medium disabled:opacity-50",
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
        "focus-ring inline-flex h-9 w-9 items-center justify-center rounded-md border text-sm disabled:opacity-50",
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
    <label className={cx("grid gap-1.5 text-sm", className)}>
      <span className="font-medium text-slate-700">{label}</span>
      <input
        className="focus-ring h-10 rounded-md border border-slate-300 bg-white px-3 text-sm text-slate-950 placeholder:text-slate-400"
        {...props}
      />
      {hint ? <span className="text-xs text-slate-500">{hint}</span> : null}
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
    <label className={cx("grid gap-1.5 text-sm", className)}>
      <span className="font-medium text-slate-700">{label}</span>
      <textarea
        className="focus-ring min-h-24 resize-y rounded-md border border-slate-300 bg-white px-3 py-2 text-sm text-slate-950 placeholder:text-slate-400"
        {...props}
      />
      {hint ? <span className="text-xs text-slate-500">{hint}</span> : null}
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
    <label className={cx("grid gap-1.5 text-sm", className)}>
      <span className="font-medium text-slate-700">{label}</span>
      <select
        className="focus-ring h-10 rounded-md border border-slate-300 bg-white px-3 text-sm text-slate-950"
        {...props}
      >
        {children}
      </select>
    </label>
  );
}

export function Badge({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <span className={cx("inline-flex items-center rounded px-2 py-0.5 text-xs font-medium", className)}>
      {children}
    </span>
  );
}

export function Panel({ children, className }: { children: ReactNode; className?: string }) {
  return <section className={cx("rounded-lg border border-slate-200 bg-white", className)}>{children}</section>;
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
    <div className="flex min-h-56 flex-col items-center justify-center rounded-lg border border-dashed border-slate-300 bg-white px-6 py-10 text-center">
      {icon ? <div className="mb-3 text-slate-400">{icon}</div> : null}
      <h3 className="text-base font-semibold text-slate-900">{title}</h3>
      <p className="mt-1 max-w-md text-sm text-slate-500">{body}</p>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

export function LoadingState({ label }: { label: string }) {
  return (
    <div className="flex min-h-56 items-center justify-center text-sm text-slate-500">
      <span className="mr-2 h-2 w-2 animate-pulse rounded-full bg-cyan-600" />
      {label}
    </div>
  );
}

export function ErrorState({ title, body }: { title: string; body: string }) {
  return (
    <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3">
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
  if (!open) {
    return null;
  }

  const content = (
    <>
      <div className="flex items-center justify-between border-b border-slate-200 px-5 py-4">
        <h2 className="text-base font-semibold text-slate-950">{title}</h2>
        <IconButton label="Close" onClick={onClose}>
          <X size={18} />
        </IconButton>
      </div>
      <div className="px-5 py-4">{children}</div>
      {footer ? <div className="flex justify-end gap-2 border-t border-slate-200 px-5 py-4">{footer}</div> : null}
    </>
  );

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/40 p-4">
      {formId ? (
        <form
          id={formId}
          onSubmit={onSubmit}
          className="max-h-[90vh] w-full max-w-xl overflow-auto rounded-lg bg-white shadow-xl"
        >
          {content}
        </form>
      ) : (
        <div className="max-h-[90vh] w-full max-w-xl overflow-auto rounded-lg bg-white shadow-xl">{content}</div>
      )}
    </div>
  );
}
