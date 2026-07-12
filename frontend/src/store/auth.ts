import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { SessionUser } from "../types";

interface AuthState {
  user: SessionUser | null;
  isAuthenticated: boolean;
  setSession: (user: SessionUser) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      isAuthenticated: false,
      setSession: (user) => set({ user, isAuthenticated: true }),
      logout: () => set({ user: null, isAuthenticated: false }),
    }),
    {
      name: "pms.session",
      partialize: (state) => ({
        user: state.user,
        isAuthenticated: state.isAuthenticated,
      }),
    },
  ),
);
