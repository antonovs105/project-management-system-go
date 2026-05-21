import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { SessionUser } from "../types";

interface AuthState {
  token: string | null;
  user: SessionUser | null;
  isAuthenticated: boolean;
  setSession: (token: string, user: SessionUser) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      isAuthenticated: false,
      setSession: (token, user) => set({ token, user, isAuthenticated: true }),
      logout: () => set({ token: null, user: null, isAuthenticated: false }),
    }),
    {
      name: "pms.session",
      partialize: (state) => ({
        token: state.token,
        user: state.user,
        isAuthenticated: state.isAuthenticated,
      }),
    },
  ),
);
