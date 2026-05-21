import { useMutation } from "@tanstack/react-query";
import { ArrowRight, CheckCircle2 } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { api, errorMessage } from "../lib/api";
import { sessionFromToken } from "../lib/jwt";
import { useAuthStore } from "../store/auth";
import { Button, ErrorState, Panel, TextField } from "../components/ui";

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
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const login = useMutation({
    mutationFn: api.login,
    onSuccess: ({ token }) => {
      const user = sessionFromToken(token, email.trim());
      if (!user) {
        setFormError("The server returned an invalid session token.");
        return;
      }
      setSession(token, user);
      navigate(destinationFromLocation(location.state), { replace: true });
    },
    onError: (error) => setFormError(errorMessage(error, "Unable to sign in.")),
  });

  const register = useMutation({
    mutationFn: api.register,
    onSuccess: () => navigate("/login", { replace: true, state: { registered: true } }),
    onError: (error) => setFormError(errorMessage(error, "Unable to create account.")),
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
          <div className="text-sm font-semibold uppercase tracking-wide text-zinc-500">TaskFlow</div>
          <h1 className="mt-5 max-w-2xl text-5xl font-semibold leading-tight text-zinc-950">
            Practical project coordination for local teams and federated work.
          </h1>
        </div>
        <div className="grid max-w-2xl gap-3 text-sm text-zinc-600">
          <div className="flex items-center gap-2">
            <CheckCircle2 size={18} className="text-zinc-950" />
            Project boards, ticket hierarchy, comments, and graph relationships.
          </div>
          <div className="flex items-center gap-2">
            <CheckCircle2 size={18} className="text-zinc-950" />
            Built against the current backend API, not the old prototype.
          </div>
        </div>
      </section>

      <section className="flex min-h-screen items-center justify-center px-4 py-10">
        <Panel className="w-full max-w-md p-6">
          <div className="mb-6">
            <div className="text-sm font-semibold text-zinc-500">TaskFlow</div>
            <h2 className="mt-2 text-2xl font-semibold text-zinc-950">
              {mode === "login" ? "Sign in" : "Create account"}
            </h2>
            <p className="mt-1 text-sm text-zinc-500">
              {mode === "login" ? "Open your workspace." : "Create a worker account."}
            </p>
          </div>

          {location.state && mode === "login" && destinationFromLocation(location.state) === "/projects" ? (
            <div className="mb-4 rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
              Account created. Sign in to continue.
            </div>
          ) : null}

          {formError ? <div className="mb-4"><ErrorState title="Request failed" body={formError} /></div> : null}

          <form className="grid gap-4" onSubmit={submit}>
            {mode === "register" ? (
              <TextField
                label="Username"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                autoComplete="username"
                required
                minLength={3}
              />
            ) : null}
            <TextField
              label="Email"
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              autoComplete="email"
              required
            />
            <TextField
              label="Password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete={mode === "login" ? "current-password" : "new-password"}
              required
              minLength={6}
            />
            <Button type="submit" tone="primary" className="w-full" disabled={pending}>
              {pending ? "Working..." : mode === "login" ? "Sign in" : "Create account"}
              <ArrowRight size={16} />
            </Button>
          </form>

          <div className="mt-5 text-center text-sm text-zinc-500">
            {mode === "login" ? (
              <>
                Need an account?{" "}
                <Link className="font-medium text-zinc-950 underline-offset-4 hover:underline" to="/register">
                  Register
                </Link>
              </>
            ) : (
              <>
                Already registered?{" "}
                <Link className="font-medium text-zinc-950 underline-offset-4 hover:underline" to="/login">
                  Sign in
                </Link>
              </>
            )}
          </div>
        </Panel>
      </section>
    </main>
  );
}
