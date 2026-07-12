import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Clipboard, KeyRound, Trash2 } from "lucide-react";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Button, ErrorState, Panel, TextField } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { useI18n } from "../../lib/i18n-context";
import { queryKeys } from "../../lib/queryKeys";
import { useAuthStore } from "../../store/auth";
import type { APITokenScope } from "../../types";

export function APITokenPanel() {
  const { t, relativeDate } = useI18n();
  const queryClient = useQueryClient();
  const role = useAuthStore((state) => state.user?.instanceRole);
  const [name, setName] = useState("");
  const [expiresAt, setExpiresAt] = useState("");
  const [scopes, setScopes] = useState<APITokenScope[]>(["projects:read"]);
  const [createdSecret, setCreatedSecret] = useState("");
  const tokens = useQuery({ queryKey: queryKeys.apiTokens, queryFn: api.listAPITokens });
  const createToken = useMutation({
    mutationFn: api.createAPIToken,
    onSuccess: async (created) => {
      setCreatedSecret(created.token);
      setName("");
      setExpiresAt("");
      setScopes(["projects:read"]);
      await queryClient.invalidateQueries({ queryKey: queryKeys.apiTokens });
    },
    onError: (error) => toast.error(errorMessage(error, t("account.apiTokensCreateFailed"))),
  });
  const revokeToken = useMutation({
    mutationFn: api.revokeAPIToken,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.apiTokens }),
    onError: (error) => toast.error(errorMessage(error, t("account.apiTokensRevokeFailed"))),
  });

  const standardScopes: Array<{ scope: APITokenScope; label: string }> = [
    { scope: "projects:read", label: t("account.scopeProjectsRead") },
    { scope: "projects:write", label: t("account.scopeProjectsWrite") },
    { scope: "account:read", label: t("account.scopeAccountRead") },
    { scope: "account:write", label: t("account.scopeAccountWrite") },
    { scope: "tokens:manage", label: t("account.scopeTokensManage") },
  ];
  const availableScopes = role === "owner" || role === "admin" ? [...standardScopes, { scope: "admin" as const, label: t("account.scopeAdmin") }] : standardScopes;

  function toggleScope(scope: APITokenScope, enabled: boolean) {
    setScopes((current) => enabled ? [...new Set([...current, scope])] : current.filter((value) => value !== scope));
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    createToken.mutate({
      name: name.trim(),
      scopes,
      ...(expiresAt ? { expires_at: new Date(`${expiresAt}T23:59:59Z`).toISOString() } : {}),
    });
  }

  return (
    <Panel className="p-5 xl:col-span-2">
      <div className="mb-4 flex items-start gap-3">
        <KeyRound className="mt-0.5 text-zinc-500" size={18} />
        <div><h2 className="font-semibold text-zinc-950">{t("account.apiTokensTitle")}</h2><p className="text-sm text-zinc-500">{t("account.apiTokensBody")}</p></div>
      </div>
      {createdSecret ? (
        <div className="mb-4 rounded-xl border border-amber-300 bg-amber-50 p-3" role="status">
          <div className="text-sm font-semibold text-amber-950">{t("account.apiTokensCopyNow")}</div>
          <code className="mt-2 block break-all rounded-lg bg-white p-2 text-sm text-zinc-950">{createdSecret}</code>
          <div className="mt-2 flex gap-2"><Button onClick={() => void navigator.clipboard.writeText(createdSecret)}><Clipboard size={15} />{t("actions.copy")}</Button><Button onClick={() => setCreatedSecret("")}>{t("account.apiTokensSaved")}</Button></div>
        </div>
      ) : null}
      <form className="grid gap-4" onSubmit={submit}>
        <div className="grid gap-4 md:grid-cols-2">
          <TextField label={t("account.apiTokensName")} value={name} onChange={(event) => setName(event.target.value)} maxLength={80} required />
          <TextField label={t("account.apiTokensExpiryHint")} type="date" value={expiresAt} onChange={(event) => setExpiresAt(event.target.value)} />
        </div>
        <fieldset><legend className="mb-2 text-sm font-medium text-zinc-700">{t("account.apiTokensScopes")}</legend><div className="grid gap-2 sm:grid-cols-2">{availableScopes.map((item) => <label key={item.scope} className="flex items-center gap-2 rounded-xl border border-zinc-200 px-3 py-2 text-sm"><input type="checkbox" checked={scopes.includes(item.scope)} onChange={(event) => toggleScope(item.scope, event.target.checked)} />{item.label}</label>)}</div></fieldset>
        <div className="flex justify-end"><Button type="submit" tone="primary" disabled={!name.trim() || scopes.length === 0 || createToken.isPending}>{t("account.apiTokensCreate")}</Button></div>
      </form>
      {tokens.isError ? <div className="mt-4"><ErrorState title={t("account.apiTokensLoadFailed")} body={errorMessage(tokens.error, t("account.apiTokensRequestFailed"))} /></div> : null}
      <div className="mt-5 grid gap-2">{(tokens.data || []).map((token) => <div key={token.id} className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-zinc-200 p-3"><div><div className="font-medium text-zinc-950">{token.name} <code className="text-xs text-zinc-500">{token.prefix}…</code></div><div className="mt-1 text-xs text-zinc-500">{token.scopes.join(", ")} · {t("account.apiTokensCreated", { date: relativeDate(token.created_at) })}{token.last_used_at ? ` · ${t("account.apiTokensUsed", { date: relativeDate(token.last_used_at) })}` : ""}{token.expires_at ? ` · ${t("account.apiTokensExpires", { date: relativeDate(token.expires_at) })}` : ""}{token.revoked_at ? ` · ${t("account.apiTokensRevoked")}` : ""}</div></div>{!token.revoked_at ? <Button tone="danger" onClick={() => revokeToken.mutate(token.id)} disabled={revokeToken.isPending}><Trash2 size={15} />{t("account.sessionRevoke")}</Button> : null}</div>)}</div>
    </Panel>
  );
}
