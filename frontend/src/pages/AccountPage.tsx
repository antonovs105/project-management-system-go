import { useMutation } from "@tanstack/react-query";
import { KeyRound, Shield } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { toast } from "sonner";
import { Button, ErrorState, Panel, TextField } from "../components/ui";
import { api, errorMessage } from "../lib/api";
import { compactId } from "../lib/format";
import { useAuthStore } from "../store/auth";

export function AccountPage() {
  const user = useAuthStore((state) => state.user);
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
      toast.success("Password changed");
    },
    onError: (error) => setFormError(errorMessage(error, "Could not change password.")),
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFormError(null);
    if (newPassword !== confirmPassword) {
      setFormError("New passwords do not match.");
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
            <h1 className="text-xl font-semibold text-zinc-950">Account</h1>
            <p className="text-sm text-zinc-500">{user?.email || "Signed in"}</p>
          </div>
        </div>
        <div className="grid gap-3 text-sm">
          <div className="rounded-xl border border-zinc-200 p-3">
            <div className="text-xs font-semibold uppercase tracking-wide text-zinc-400">User ID</div>
            <div className="mt-1 break-all text-zinc-950">{user?.userId || "unknown"}</div>
          </div>
          <div className="rounded-xl border border-zinc-200 p-3">
            <div className="text-xs font-semibold uppercase tracking-wide text-zinc-400">Instance Role</div>
            <div className="mt-1 text-zinc-950">{user?.instanceRole || "user"}</div>
          </div>
          {user?.userId ? (
            <div className="rounded-xl border border-zinc-200 p-3">
              <div className="text-xs font-semibold uppercase tracking-wide text-zinc-400">Short ID</div>
              <div className="mt-1 text-zinc-950">{compactId(user.userId)}</div>
            </div>
          ) : null}
        </div>
      </Panel>

      <Panel className="p-5">
        <div className="mb-4 flex items-center gap-2">
          <KeyRound size={18} className="text-zinc-500" />
          <h2 className="text-base font-semibold text-zinc-950">Password</h2>
        </div>
        {formError ? (
          <div className="mb-4">
            <ErrorState title="Password change failed" body={formError} />
          </div>
        ) : null}
        <form className="grid gap-4" onSubmit={submit}>
          <TextField
            label="Current password"
            type="password"
            value={currentPassword}
            onChange={(event) => setCurrentPassword(event.target.value)}
            autoComplete="current-password"
            required
          />
          <TextField
            label="New password"
            type="password"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
            autoComplete="new-password"
            minLength={6}
            required
          />
          <TextField
            label="Confirm new password"
            type="password"
            value={confirmPassword}
            onChange={(event) => setConfirmPassword(event.target.value)}
            autoComplete="new-password"
            minLength={6}
            required
          />
          <div className="flex justify-end">
            <Button type="submit" tone="primary" disabled={changePassword.isPending || !currentPassword || !newPassword}>
              Save password
            </Button>
          </div>
        </form>
      </Panel>
    </div>
  );
}
