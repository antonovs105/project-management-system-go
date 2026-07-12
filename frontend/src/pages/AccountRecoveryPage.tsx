import { useMutation } from "@tanstack/react-query";
import { CheckCircle2, Mail, RotateCcw } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Button, ErrorState, Panel, TextField } from "../components/ui";
import { api, errorMessage } from "../lib/api";
import { fieldLimits } from "../lib/limits";
import { useI18n } from "../lib/i18n-context";

type RecoveryMode = "forgot" | "reset" | "verify";

export function AccountRecoveryPage({ mode }: { mode: RecoveryMode }) {
  const { t } = useI18n();
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token") || "";
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const action = useMutation({
    mutationFn: async () => {
      if (mode === "forgot") {
        await api.forgotPassword(email.trim());
      } else if (mode === "reset") {
        await api.resetPassword(token, password);
      } else {
        await api.verifyEmail(token);
      }
    },
    onError: (error) => setFormError(errorMessage(error, t("recovery.requestFailed"))),
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFormError(null);
    if (mode !== "forgot" && !token) {
      setFormError(t("recovery.missingToken"));
      return;
    }
    if (mode === "reset") {
      const bytes = new TextEncoder().encode(password).length;
      if (Array.from(password).length < fieldLimits.passwordMinLength || bytes > fieldLimits.passwordMaxBytes) {
        setFormError(t("recovery.passwordInvalid"));
        return;
      }
      if (password !== confirmPassword) {
        setFormError(t("recovery.passwordMismatch"));
        return;
      }
    }
    action.mutate();
  }

  const title = mode === "forgot" ? t("recovery.forgotTitle") : mode === "reset" ? t("recovery.resetTitle") : t("recovery.verifyTitle");
  const complete = action.isSuccess;

  return (
    <main className="flex min-h-screen items-center justify-center bg-zinc-100 px-4 py-10">
      <Panel className="w-full max-w-md p-6">
        <div className="mb-5 flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-zinc-950 text-white">
            {mode === "forgot" ? <Mail size={18} /> : mode === "reset" ? <RotateCcw size={18} /> : <CheckCircle2 size={18} />}
          </div>
          <div>
            <div className="text-sm font-semibold text-zinc-500">Progo</div>
            <h1 className="text-xl font-semibold text-zinc-950">{title}</h1>
          </div>
        </div>

        {complete ? (
          <div className="grid gap-4">
            <div className="rounded-2xl border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-900">
              {mode === "forgot"
                ? t("recovery.forgotComplete")
                : mode === "reset"
                  ? t("recovery.resetComplete")
                  : t("recovery.verifyComplete")}
            </div>
            <Link className="focus-ring inline-flex h-9 items-center justify-center rounded-full bg-zinc-950 px-4 text-sm font-medium text-white hover:bg-zinc-800" to="/login">{t("recovery.returnToSignIn")}</Link>
          </div>
        ) : (
          <form className="grid gap-4" onSubmit={submit}>
            {formError ? <ErrorState title={t("recovery.errorTitle")} body={formError} /> : null}
            {mode === "forgot" ? (
              <TextField label={t("auth.email")} type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} required />
            ) : null}
            {mode === "reset" ? (
              <>
                <TextField label={t("recovery.newPassword")} type="password" autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} required />
                <TextField label={t("recovery.confirmPassword")} type="password" autoComplete="new-password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} required />
              </>
            ) : null}
            {mode === "verify" ? <p className="text-sm text-zinc-600">{t("recovery.verifyPrompt")}</p> : null}
            <Button type="submit" tone="primary" disabled={action.isPending}>{action.isPending ? t("common.working") : t("recovery.continue")}</Button>
            <Link className="text-center text-sm text-zinc-600 hover:text-zinc-950" to="/login">{t("recovery.backToSignIn")}</Link>
          </form>
        )}
      </Panel>
    </main>
  );
}
