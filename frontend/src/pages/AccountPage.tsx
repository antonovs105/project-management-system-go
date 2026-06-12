import { useMutation } from "@tanstack/react-query";
import { KeyRound, Shield } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { toast } from "sonner";
import { Button, ErrorState, Panel, TextField } from "../components/ui";
import { api, errorMessage } from "../lib/api";
import { compactId } from "../lib/format";
import { useI18n } from "../lib/i18n-context";
import { useAuthStore } from "../store/auth";

export function AccountPage() {
  const user = useAuthStore((state) => state.user);
  const { t } = useI18n();
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const changePassword = useMutation({
    mutationFn: () => api.changePassword({ current_password: currentPassword, new_password: newPassword }),
    onSuccess: () => {
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      toast.success(t("account.passwordChanged"));
    },
    onError: (error) => setFormError(errorMessage(error, t("account.passwordRequestFailed"))),
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFormError(null);
    if (newPassword !== confirmPassword) {
      setFormError(t("account.passwordMismatch"));
      return;
    }
    changePassword.mutate();
  }

  return (
    <div className="grid gap-5 xl:grid-cols-[0.9fr_1.1fr]">
      <Panel className="p-5">
        <div className="mb-4 flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-zinc-950 text-white">
            <Shield size={18} />
          </div>
          <div>
            <h1 className="text-xl font-semibold text-zinc-950">{t("account.title")}</h1>
            <p className="text-sm text-zinc-500">{user?.email || t("common.signedIn")}</p>
          </div>
        </div>
        <div className="grid gap-3 text-sm">
          <div className="rounded-xl border border-zinc-200 p-3">
            <div className="text-xs font-semibold uppercase tracking-wide text-zinc-400">{t("account.userId")}</div>
            <div className="mt-1 break-all text-zinc-950">{user?.userId || t("common.unknown")}</div>
          </div>
          <div className="rounded-xl border border-zinc-200 p-3">
            <div className="text-xs font-semibold uppercase tracking-wide text-zinc-400">{t("account.instanceRole")}</div>
            <div className="mt-1 text-zinc-950">{user?.instanceRole || t("common.user")}</div>
          </div>
          {user?.userId ? (
            <div className="rounded-xl border border-zinc-200 p-3">
              <div className="text-xs font-semibold uppercase tracking-wide text-zinc-400">{t("account.shortId")}</div>
              <div className="mt-1 text-zinc-950">{compactId(user.userId)}</div>
            </div>
          ) : null}
        </div>
      </Panel>

      <Panel className="p-5">
        <div className="mb-4 flex items-center gap-2">
          <KeyRound size={18} className="text-zinc-500" />
          <h2 className="text-base font-semibold text-zinc-950">{t("account.password")}</h2>
        </div>
        {formError ? (
          <div className="mb-4">
            <ErrorState title={t("account.passwordFailed")} body={formError} />
          </div>
        ) : null}
        <form className="grid gap-4" onSubmit={submit}>
          <TextField
            label={t("account.currentPassword")}
            type="password"
            value={currentPassword}
            onChange={(event) => setCurrentPassword(event.target.value)}
            autoComplete="current-password"
            required
          />
          <TextField
            label={t("account.newPassword")}
            type="password"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
            autoComplete="new-password"
            minLength={6}
            required
          />
          <TextField
            label={t("account.confirmPassword")}
            type="password"
            value={confirmPassword}
            onChange={(event) => setConfirmPassword(event.target.value)}
            autoComplete="new-password"
            minLength={6}
            required
          />
          <div className="flex justify-end">
            <Button type="submit" tone="primary" disabled={changePassword.isPending || !currentPassword || !newPassword}>
              {t("account.savePassword")}
            </Button>
          </div>
        </form>
      </Panel>
    </div>
  );
}
