import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, History, KeyRound, MonitorSmartphone, Shield, Trash2 } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { toast } from "sonner";
import { Button, ErrorState, Panel, TextField } from "../components/ui";
import { NotificationPreferencesPanel } from "../features/account/NotificationPreferencesPanel";
import { APITokenPanel } from "../features/account/APITokenPanel";
import { PrivilegedAccountOnboarding } from "../features/account/PrivilegedAccountOnboarding";
import { api, errorMessage } from "../lib/api";
import { compactId } from "../lib/format";
import { downloadJSON } from "../lib/download";
import { useI18n } from "../lib/i18n-context";
import { fieldLimits } from "../lib/limits";
import { queryKeys } from "../lib/queryKeys";
import { useAuthStore } from "../store/auth";

export function AccountPage() {
	const queryClient = useQueryClient();
  const user = useAuthStore((state) => state.user);
  const setSession = useAuthStore((state) => state.setSession);
  const { t, relativeDate } = useI18n();
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
		onError: (error) => toast.error(errorMessage(error, t("account.mfaStartFailed"))),
	});
	const confirmMFA = useMutation({
		mutationFn: api.confirmMFA,
		onSuccess: (result) => {
			setRecoveryCodes(result.recovery_codes);
			setMFASetup(null);
			setMFACode("");
			if (user?.mfaEnrollmentRequired) {
				setSession({ ...user, mfaEnrollmentRequired: false });
			}
			void queryClient.invalidateQueries({ queryKey: queryKeys.mfaStatus });
		},
		onError: (error) => toast.error(errorMessage(error, t("account.mfaConfirmFailed"))),
	});
	const disableMFA = useMutation({
		mutationFn: api.disableMFA,
		onSuccess: () => { setMFACode(""); void queryClient.invalidateQueries({ queryKey: queryKeys.mfaStatus }); },
		onError: (error) => toast.error(errorMessage(error, t("account.mfaDisableFailed"))),
	});
	const revokeSession = useMutation({
		mutationFn: api.revokeSession,
		onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.accountSessions }),
		onError: (error) => toast.error(errorMessage(error, t("account.sessionRevokeFailed"))),
	});
	const resendVerification = useMutation({
		mutationFn: api.requestEmailVerification,
		onSuccess: () => toast.success(t("account.verificationQueued")),
		onError: (error) => toast.error(errorMessage(error, t("account.verificationQueueFailed"))),
	});
	const exportAccount = useMutation({
		mutationFn: api.exportCurrentUser,
		onSuccess: (bundle) => downloadJSON("progo-account-export.json", bundle),
		onError: (error) => toast.error(errorMessage(error, t("account.exportFailed"))),
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
      <PrivilegedAccountOnboarding instanceRole={user?.instanceRole} enrollmentRequired={user?.mfaEnrollmentRequired} />
      <Panel className="p-5">
        <div className="mb-4 flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-zinc-950 text-white">
            <Shield size={18} />
          </div>
          <div>
            <h1 className="text-xl font-semibold text-zinc-950">{t("account.title")}</h1>
            <p className="text-sm text-zinc-500">{user?.email || t("common.signedIn")}</p>
          </div>
		  <Button className="ml-auto" onClick={() => exportAccount.mutate()} disabled={exportAccount.isPending}><Download size={16} />{t("account.exportData")}</Button>
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
					<div><h2 className="font-semibold text-zinc-950">{t("account.verifyEmail")}</h2><p className="text-sm text-zinc-500">{t("account.verifyEmailBody")}</p></div>
					<Button onClick={() => resendVerification.mutate()} disabled={resendVerification.isPending}>{t("account.resendVerification")}</Button>
				</div>
			</Panel>
		) : null}

		<Panel className="p-5 xl:col-span-2">
			<div className="mb-4 flex items-center gap-2"><Shield size={18} className="text-zinc-500" /><h2 className="font-semibold text-zinc-950">{t("account.mfaTitle")}</h2></div>
			<p className="mb-4 text-sm text-zinc-500">{t("account.mfaBody")}</p>
			{mfaSetup ? <div className="mb-4 grid gap-2 rounded-xl border border-zinc-200 bg-zinc-50 p-3 text-sm"><div><span className="font-medium">{t("account.mfaSecret")}</span> <code className="break-all">{mfaSetup.secret}</code></div><div className="break-all text-xs text-zinc-500">{mfaSetup.uri}</div></div> : null}
			{recoveryCodes.length > 0 ? <div className="mb-4 rounded-xl border border-amber-200 bg-amber-50 p-3"><div className="text-sm font-semibold text-amber-950">{t("account.mfaSaveCodes")}</div><div className="mt-2 grid grid-cols-2 gap-1 font-mono text-sm">{recoveryCodes.map((code) => <span key={code}>{code}</span>)}</div></div> : null}
			<div className="flex flex-wrap items-end gap-3">
				{!mfaStatus.data?.enabled && !mfaSetup ? <Button onClick={() => beginMFA.mutate()} disabled={beginMFA.isPending}>{t("account.mfaStart")}</Button> : null}
				{mfaSetup || mfaStatus.data?.enabled ? <div className="min-w-64 flex-1"><TextField label={mfaStatus.data?.enabled ? t("account.mfaRecoveryCode") : t("account.mfaAuthenticatorCode")} value={mfaCode} onChange={(event) => setMFACode(event.target.value)} autoComplete="one-time-code" /></div> : null}
				{mfaSetup ? <Button tone="primary" onClick={() => confirmMFA.mutate(mfaCode)} disabled={!mfaCode || confirmMFA.isPending}>{t("account.mfaEnable")}</Button> : null}
				{mfaStatus.data?.enabled ? <Button tone="danger" onClick={() => disableMFA.mutate(mfaCode)} disabled={!mfaCode || disableMFA.isPending}>{t("account.mfaDisable")}</Button> : null}
			</div>
		</Panel>

    <NotificationPreferencesPanel />
	<APITokenPanel />

		<Panel className="p-5 xl:col-span-2">
			<div className="mb-4 flex items-center gap-2"><MonitorSmartphone size={18} className="text-zinc-500" /><h2 className="font-semibold text-zinc-950">{t("account.sessionsTitle")}</h2></div>
			{sessions.isError ? <ErrorState title={t("account.sessionsLoadFailed")} body={errorMessage(sessions.error, t("account.sessionsRequestFailed"))} /> : null}
			<div className="grid gap-2">
				{(sessions.data || []).map((session) => (
					<div key={session.id} className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-zinc-200 p-3">
\t\t\t\t\t\t<div className="min-w-0"><div className="truncate text-sm font-medium text-zinc-950">{session.user_agent || t("account.sessionUnknownClient")}{session.current ? ` (${t("account.sessionCurrent")})` : ""}</div><div className="text-xs text-zinc-500">{session.ip_address || t("account.sessionUnknownIP")} · {t("account.sessionLastSeen", { date: relativeDate(session.last_seen_at) })}</div></div>
						<Button tone="danger" onClick={() => revokeSession.mutate(session.id)} disabled={revokeSession.isPending}><Trash2 size={15} />{t("account.sessionRevoke")}</Button>
					</div>
				))}
			</div>
		</Panel>

		<Panel className="p-5 xl:col-span-2">
			<div className="mb-4 flex items-center gap-2"><History size={18} className="text-zinc-500" /><h2 className="font-semibold text-zinc-950">{t("account.securityTitle")}</h2></div>
			{securityEvents.isError ? <ErrorState title={t("account.securityLoadFailed")} body={errorMessage(securityEvents.error, t("account.securityRequestFailed"))} /> : null}
			<div className="grid gap-2">
\t\t\t\t{(securityEvents.data || []).map((event) => <div key={event.id} className="rounded-xl border border-zinc-200 p-3 text-sm"><div className="font-medium text-zinc-950">{event.event_type.replaceAll(".", " ")}</div><div className="text-xs text-zinc-500">{event.ip_address || t("account.sessionUnknownIP")} · {relativeDate(event.created_at)}</div></div>)}
			</div>
		</Panel>
    </div>
  );
}
