import { useMutation } from "@tanstack/react-query";
import axios from "axios";
import { ArrowLeft } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { ThemeToggle } from "../components/ThemeToggle";
import { Button, ErrorState, LoadingState, Panel, TextField } from "../components/ui";
import { api, errorMessage } from "../lib/api";
import { useI18n } from "../lib/i18n-context";
import { useAuthStore } from "../store/auth";

function oauthErrorMessage(errorCode: string, t: ReturnType<typeof useI18n>["t"]): string {
  switch (errorCode) {
    case "provider_denied":
      return t("oauth.providerDenied");
    case "provider_unavailable":
      return t("oauth.providerUnavailable");
    case "invalid_state":
      return t("oauth.invalidState");
    case "email_unverified":
      return t("oauth.emailUnverified");
    case "email_registered":
      return t("oauth.emailRegistered");
    case "registration_disabled":
      return t("oauth.registrationDisabled");
    case "provider_failed":
      return t("oauth.providerFailed");
    default:
      return t("oauth.exchangeFailed");
  }
}

export function OAuthCallbackPage() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const setSession = useAuthStore((state) => state.setSession);
  const { t } = useI18n();
  const handledCode = useRef<string | null>(null);
	const [exchangeError, setExchangeError] = useState<string | null>(null);
	const [mfaRequired, setMFARequired] = useState(false);
	const [mfaCode, setMFACode] = useState("");

  const code = searchParams.get("code")?.trim() ?? "";
  const errorCode = searchParams.get("error")?.trim() ?? "";
  const callbackError = errorCode ? oauthErrorMessage(errorCode, t) : !code ? t("oauth.missingCode") : null;
  const pageError = callbackError ?? exchangeError;

  const { mutate, isPending } = useMutation({
		mutationFn: ({ exchangeCode, factor }: { exchangeCode: string; factor?: string }) => api.exchangeOAuthCode(exchangeCode, factor),
    onSuccess: (session) => {
		setSession({ userId: session.user_id, instanceRole: session.instance_role, email: session.email, emailVerified: session.email_verified, mfaEnrollmentRequired: session.mfa_enrollment_required });
		navigate(session.mfa_enrollment_required ? "/account" : "/projects", { replace: true });
    },
		onError: (error) => {
			if (axios.isAxiosError<{ code?: string; error?: { code?: string } }>(error) && (error.response?.data?.code === "mfa_required" || error.response?.data?.error?.code === "mfa_required")) {
				setMFARequired(true);
				setExchangeError(null);
				return;
			}
			setExchangeError(errorMessage(error, t("oauth.exchangeFailed")));
		},
  });

  useEffect(() => {
    if (callbackError) {
      return;
    }
    if (handledCode.current === code) {
      return;
    }
    handledCode.current = code;
		mutate({ exchangeCode: code });
  }, [callbackError, code, mutate]);

  return (
    <main className="min-h-screen bg-zinc-100 px-4 py-10">
      <div className="mx-auto flex min-h-[calc(100vh-5rem)] w-full max-w-md flex-col justify-center">
        <div className="mb-4 flex justify-end gap-2">
          <ThemeToggle />
          <LanguageSwitcher />
        </div>
        <Panel className="p-6">
          <div className="mb-6">
            <div className="text-sm font-semibold text-zinc-500">{t("app.name")}</div>
            <h1 className="mt-2 text-2xl font-semibold text-zinc-950">{t("oauth.title")}</h1>
            <p className="mt-1 text-sm text-zinc-500">{pageError ? t("oauth.failed") : t("oauth.loading")}</p>
          </div>

			{pageError ? (
            <div className="grid gap-4">
              <ErrorState title={t("common.requestFailed")} body={pageError} />
              <Link
                className="focus-ring inline-flex h-10 items-center justify-center gap-2 rounded-full border border-zinc-200 bg-white px-4 text-sm font-medium text-zinc-900 shadow-sm transition hover:border-zinc-300 hover:bg-zinc-50"
                to="/login"
              >
                <ArrowLeft size={16} />
                {t("oauth.backToLogin")}
              </Link>
            </div>
			) : mfaRequired ? (
				<form className="grid gap-4" onSubmit={(event) => { event.preventDefault(); mutate({ exchangeCode: code, factor: mfaCode.trim() }); }}>
					<TextField label={t("auth.mfaCode")} value={mfaCode} onChange={(event) => setMFACode(event.target.value)} autoComplete="one-time-code" required />
					<Button type="submit" tone="primary" disabled={isPending || !mfaCode.trim()}>{t("auth.completeSignIn")}</Button>
				</form>
			) : (
            <LoadingState label={isPending ? t("oauth.loading") : t("common.working")} />
          )}
        </Panel>
      </div>
    </main>
  );
}
