import { useMutation } from "@tanstack/react-query";
import { ArrowRight, CheckCircle2 } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { Button, ErrorState, Panel, TextField } from "../components/ui";
import { api, errorMessage } from "../lib/api";
import { useI18n } from "../lib/i18n-context";
import { sessionFromToken } from "../lib/jwt";
import { useAuthStore } from "../store/auth";

function destinationFromLocation(state: unknown): string {
  if (
    typeof state === "object" &&
    state !== null &&
    "from" in state &&
    typeof state.from === "object" &&
    state.from !== null &&
    "pathname" in state.from &&
    typeof state.from.pathname === "string"
  ) {
    return state.from.pathname;
  }
  return "/projects";
}

export function AuthPage({ mode }: { mode: "login" | "register" }) {
  const navigate = useNavigate();
  const location = useLocation();
  const setSession = useAuthStore((state) => state.setSession);
  const { t } = useI18n();
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const login = useMutation({
    mutationFn: api.login,
    onSuccess: ({ token }) => {
      const user = sessionFromToken(token, email.trim());
      if (!user) {
        setFormError(t("auth.invalidToken"));
        return;
      }
      setSession(token, user);
      navigate(destinationFromLocation(location.state), { replace: true });
    },
    onError: (error) => setFormError(errorMessage(error, t("auth.loginFailed"))),
  });

  const register = useMutation({
    mutationFn: api.register,
    onSuccess: () => navigate("/login", { replace: true, state: { registered: true } }),
    onError: (error) => setFormError(errorMessage(error, t("auth.registerFailed"))),
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFormError(null);
    if (mode === "register") {
      register.mutate({ username: username.trim(), email: email.trim(), password });
      return;
    }
    login.mutate({ email: email.trim(), password });
  }

  const pending = login.isPending || register.isPending;

  return (
    <main className="grid min-h-screen bg-zinc-100 lg:grid-cols-[1fr_460px]">
      <section className="hidden min-h-screen flex-col justify-between border-r border-zinc-200 bg-white p-10 lg:flex">
        <div>
          <div className="text-sm font-semibold uppercase tracking-wide text-zinc-500">{t("app.name")}</div>
          <h1 className="mt-5 max-w-2xl text-5xl font-semibold leading-tight text-zinc-950">
            {t("auth.heroTitle")}
          </h1>
        </div>
        <div className="grid max-w-2xl gap-3 text-sm text-zinc-600">
          <div className="flex items-center gap-2">
            <CheckCircle2 size={18} className="text-zinc-950" />
            {t("auth.heroBoards")}
          </div>
          <div className="flex items-center gap-2">
            <CheckCircle2 size={18} className="text-zinc-950" />
            {t("auth.heroApi")}
          </div>
        </div>
      </section>

      <section className="flex min-h-screen flex-col items-center justify-center px-4 py-10">
        <div className="mb-4 flex w-full max-w-md justify-end">
          <LanguageSwitcher />
        </div>
        <Panel className="w-full max-w-md p-6">
          <div className="mb-6">
            <div className="text-sm font-semibold text-zinc-500">{t("app.name")}</div>
            <h2 className="mt-2 text-2xl font-semibold text-zinc-950">
              {mode === "login" ? t("auth.loginTitle") : t("auth.registerTitle")}
            </h2>
            <p className="mt-1 text-sm text-zinc-500">
              {mode === "login" ? t("auth.loginSubtitle") : t("auth.registerSubtitle")}
            </p>
          </div>

          {location.state && mode === "login" && destinationFromLocation(location.state) === "/projects" ? (
            <div className="mb-4 rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
              {t("auth.accountCreated")}
            </div>
          ) : null}

          {formError ? (
            <div className="mb-4">
              <ErrorState title={t("common.requestFailed")} body={formError} />
            </div>
          ) : null}

          <form className="grid gap-4" onSubmit={submit}>
            {mode === "register" ? (
              <TextField
                label={t("auth.username")}
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                autoComplete="username"
                required
                minLength={3}
              />
            ) : null}
            <TextField
              label={t("auth.email")}
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              autoComplete="email"
              required
            />
            <TextField
              label={t("auth.password")}
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete={mode === "login" ? "current-password" : "new-password"}
              required
              minLength={6}
            />
            <Button type="submit" tone="primary" className="w-full" disabled={pending}>
              {pending ? t("common.working") : mode === "login" ? t("actions.signIn") : t("actions.createAccount")}
              <ArrowRight size={16} />
            </Button>
          </form>

          <div className="mt-5 text-center text-sm text-zinc-500">
            {mode === "login" ? (
              <>
                {t("auth.needAccount")}{" "}
                <Link className="font-medium text-zinc-950 underline-offset-4 hover:underline" to="/register">
                  {t("actions.register")}
                </Link>
              </>
            ) : (
              <>
                {t("auth.alreadyRegistered")}{" "}
                <Link className="font-medium text-zinc-950 underline-offset-4 hover:underline" to="/login">
                  {t("actions.signIn")}
                </Link>
              </>
            )}
          </div>
        </Panel>
        <div className="mt-4 flex w-full max-w-md flex-wrap items-center justify-between gap-2 px-1 text-xs text-zinc-500">
          <Link className="hover:text-zinc-950 hover:underline" to="/terms">
            {t("legal.link")}
          </Link>
          <span>{t("app.copyright")}</span>
        </div>
      </section>
    </main>
  );
}
