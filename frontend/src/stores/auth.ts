import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { User } from "@/lib/types";

interface AuthState {
  token: string | null;
  user: User | null;
  expiresAt: string | null;
  hydrated: boolean;
  setAuth: (token: string, user: User, expiresAt: string) => void;
  setUser: (user: User) => void;
  logout: () => void;
  setHydrated: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      expiresAt: null,
      hydrated: false,
      setAuth: (token, user, expiresAt) => set({ token, user, expiresAt }),
      setUser: (user) => set({ user }),
      logout: () => set({ token: null, user: null, expiresAt: null }),
      setHydrated: () => set({ hydrated: true }),
    }),
    {
      name: "fm-auth",
      partialize: (s) => ({ token: s.token, user: s.user, expiresAt: s.expiresAt }),
      onRehydrateStorage: () => (state) => {
        state?.setHydrated();
      },
    },
  ),
);
