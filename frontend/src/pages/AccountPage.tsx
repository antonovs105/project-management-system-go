import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { History, KeyRound, MonitorSmartphone, Shield, Trash2 } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { toast } from "sonner";
import { Button, ErrorState, Panel, TextField } from "../components/ui";
import { api, errorMessage } from "../lib/api";
import { compactId, relativeDate } from "../lib/format";
import { useI18n } from "../lib/i18n-context";
import { fieldLimits } from "../lib/limits";
import { queryKeys } from "../lib/queryKeys";
import { useAuthStore } from "../store/auth";

export function AccountPage() {
	const queryClient = useQueryClient();
  const user = useAuthStore((state) => state.user);
  const { t } = useI18n();
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
	const [formError, setFormError] = useState<string | null>(null);
	const [mfaCode, setMFACode] = useState("");
	const [mfaSetup, setMFASetup] = useState<{ secret: string; uri: string } | null>(null);
	const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
	const sessions = useQuery({ queryKey: queryKeys.accountSessions, queryFn: api.listSessions });
	const securityEvents = useQuery({ queryKey: queryKeys.securityEvents, queryFn: api.listSecurityEvents });
	const mfaStatus = useQuery({ queryKey: queryKeys.mfaStatus, queryFn: api.getMFAStatus });
	const beginMFA = useMutation({
		mutationFn: api.beginMFA,
		onSuccess: (setup) => { setMFASetup(setup); setRecoveryCodes([]); },
		onError: (error) => toast.error(errorMessage(error, "Could not start MFA setup.")),
	});
	const confirmMFA = useMutation({
		mutationFn: api.confirmMFA,
		onSuccess: (result) => { setRecoveryCodes(result.recovery_codes); setMFASetup(null); setMFACode(""); void queryClient.invalidateQueries({ queryKey: queryKeys.mfaStatus }); },
		onError: (error) => toast.error(errorMessage(error, "Could not confirm MFA.")),
	});
	const disableMFA = useMutation({
		mutationFn: api.disableMFA,
		onSuccess: () => { setMFACode(""); void queryClient.invalidateQueries({ queryKey: queryKeys.mfaStatus }); },
		onError: (error) => toast.error(errorMessage(error, "Could not disable MFA.")),
	});
	const revokeSession = useMutation({
		mutationFn: api.revokeSession,
		onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.accountSessions }),
		onError: (error) => toast.error(errorMessage(error, "Could not revoke that session.")),
	});
	const resendVerification = useMutation({
		mutationFn: api.requestEmailVerification,
		onSuccess: () => toast.success("A new verification email has been queued."),
		onError: (error) => toast.error(errorMessage(error, "Could not queue verification email.")),
	});

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
    const currentPasswordLength = Array.from(currentPassword).length;
    const currentPasswordBytes = new TextEncoder().encode(currentPassword).length;
    const passwordLength = Array.from(newPassword).length;
    const passwordBytes = new TextEncoder().encode(newPassword).length;
    if (currentPasswordLength < fieldLimits.passwordMinLength) {
      setFormError(t("validation.passwordTooShort", { min: fieldLimits.passwordMinLength }));
      return;
    }
    if (currentPasswordBytes > fieldLimits.passwordMaxBytes) {
      setFormError(t("validation.passwordTooLong", { max: fieldLimits.passwordMaxBytes }));
      return;
    }
    if (passwordLength < fieldLimits.passwordMinLength) {
      setFormError(t("validation.passwordTooShort", { min: fieldLimits.passwordMinLength }));
      return;
    }
    if (passwordBytes > fieldLimits.passwordMaxBytes) {
      setFormError(t("validation.passwordTooLong", { max: fieldLimits.passwordMaxBytes }));
      return;
    }
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
            minLength={fieldLimits.passwordMinLength}
            maxLength={fieldLimits.passwordMaxLength}
            hint={t("validation.passwordHint", { min: fieldLimits.passwordMinLength })}
            required
          />
          <TextField
            label={t("account.newPassword")}
            type="password"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
            autoComplete="new-password"
            minLength={fieldLimits.passwordMinLength}
            maxLength={fieldLimits.passwordMaxLength}
            hint={t("validation.passwordHint", { min: fieldLimits.passwordMinLength })}
            required
          />
          <TextField
            label={t("account.confirmPassword")}
            type="password"
            value={confirmPassword}
            onChange={(event) => setConfirmPassword(event.target.value)}
            autoComplete="new-password"
            minLength={fieldLimits.passwordMinLength}
            maxLength={fieldLimits.passwordMaxLength}
            required
          />
          <div className="flex justify-end">
            <Button type="submit" tone="primary" disabled={changePassword.isPending || !currentPassword || !newPassword}>
              {t("account.savePassword")}
            </Button>
          </div>
        </form>
      </Panel>

		{user?.emailVerified === false ? (
			<Panel className="p-5 xl:col-span-2">
				<div className="flex flex-wrap items-center justify-between gap-3">
					<div><h2 className="font-semibold text-zinc-950">Verify your email</h2><p className="text-sm text-zinc-500">Local accounts must verify email ownership before signing in again.</p></div>
					<Button onClick={() => resendVerification.mutate()} disabled={resendVerification.isPending}>Resend verification</Button>
				</div>
			</Panel>
		) : null}

		<Panel className="p-5 xl:col-span-2">
			<div className="mb-4 flex items-center gap-2"><Shield size={18} className="text-zinc-500" /><h2 className="font-semibold text-zinc-950">Multi-factor authentication</h2></div>
			<p className="mb-4 text-sm text-zinc-500">TOTP is required before an account can be promoted to admin or owner. Recovery codes are single-use.</p>
			{mfaSetup ? <div className="mb-4 grid gap-2 rounded-xl border border-zinc-200 bg-zinc-50 p-3 text-sm"><div><span className="font-medium">Secret:</span> <code className="break-all">{mfaSetup.secret}</code></div><div className="break-all text-xs text-zinc-500">{mfaSetup.uri}</div></div> : null}
			{recoveryCodes.length > 0 ? <div className="mb-4 rounded-xl border border-amber-200 bg-amber-50 p-3"><div className="text-sm font-semibold text-amber-950">Save these recovery codes now</div><div className="mt-2 grid grid-cols-2 gap-1 font-mono text-sm">{recoveryCodes.map((code) => <span key={code}>{code}</span>)}</div></div> : null}
			<div className="flex flex-wrap items-end gap-3">
				{!mfaStatus.data?.enabled && !mfaSetup ? <Button onClick={() => beginMFA.mutate()} disabled={beginMFA.isPending}>Start setup</Button> : null}
				{mfaSetup || mfaStatus.data?.enabled ? <div className="min-w-64 flex-1"><TextField label={mfaStatus.data?.enabled ? "Authenticator or recovery code" : "Six-digit authenticator code"} value={mfaCode} onChange={(event) => setMFACode(event.target.value)} autoComplete="one-time-code" /></div> : null}
				{mfaSetup ? <Button tone="primary" onClick={() => confirmMFA.mutate(mfaCode)} disabled={!mfaCode || confirmMFA.isPending}>Enable MFA</Button> : null}
				{mfaStatus.data?.enabled ? <Button tone="danger" onClick={() => disableMFA.mutate(mfaCode)} disabled={!mfaCode || disableMFA.isPending}>Disable MFA</Button> : null}
			</div>
		</Panel>

		<Panel className="p-5 xl:col-span-2">
			<div className="mb-4 flex items-center gap-2"><MonitorSmartphone size={18} className="text-zinc-500" /><h2 className="font-semibold text-zinc-950">Sessions and devices</h2></div>
			{sessions.isError ? <ErrorState title="Could not load sessions" body={errorMessage(sessions.error, "Session request failed.")} /> : null}
			<div className="grid gap-2">
				{(sessions.data || []).map((session) => (
					<div key={session.id} className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-zinc-200 p-3">
						<div className="min-w-0"><div className="truncate text-sm font-medium text-zinc-950">{session.user_agent || "Unknown client"}{session.current ? " (current)" : ""}</div><div className="text-xs text-zinc-500">{session.ip_address || "Unknown IP"} · Last seen {relativeDate(session.last_seen_at)}</div></div>
						<Button tone="danger" onClick={() => revokeSession.mutate(session.id)} disabled={revokeSession.isPending}><Trash2 size={15} />Revoke</Button>
					</div>
				))}
			</div>
		</Panel>

		<Panel className="p-5 xl:col-span-2">
			<div className="mb-4 flex items-center gap-2"><History size={18} className="text-zinc-500" /><h2 className="font-semibold text-zinc-950">Security activity</h2></div>
			{securityEvents.isError ? <ErrorState title="Could not load security events" body={errorMessage(securityEvents.error, "Security event request failed.")} /> : null}
			<div className="grid gap-2">
				{(securityEvents.data || []).map((event) => <div key={event.id} className="rounded-xl border border-zinc-200 p-3 text-sm"><div className="font-medium text-zinc-950">{event.event_type.replaceAll(".", " ")}</div><div className="text-xs text-zinc-500">{event.ip_address || "Unknown IP"} · {relativeDate(event.created_at)}</div></div>)}
			</div>
		</Panel>
    </div>
  );
}
