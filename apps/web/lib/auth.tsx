"use client";

import { createContext, useContext, useEffect, useMemo, useState } from "react";
import { api, refreshSession, type Organization, type Session, type User } from "./api";

type AuthState = {
  ready: boolean;
  user: User | null;
  token: string;
  organizations: Organization[];
  setSession: (session: Session) => Promise<void>;
  refreshUser: () => Promise<void>;
  signOut: () => Promise<void>;
};

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [ready, setReady] = useState(false);
  const [user, setUser] = useState<User | null>(null);
  const [token, setToken] = useState("");
  const [organizations, setOrganizations] = useState<Organization[]>([]);

  async function loadOrganizations(accessToken: string) {
    const response = await api<{ data: Organization[] }>("/api/v1/me/organizations", { token: accessToken });
    setOrganizations(response.data);
  }

  async function setSession(session: Session) {
    setUser(session.user);
    setToken(session.access_token);
    await loadOrganizations(session.access_token);
  }

  async function refreshUser() {
    if (!token) return;
    const next = await api<User>("/api/v1/me", { token });
    setUser(next);
    await loadOrganizations(token);
  }

  async function signOut() {
    await api("/api/v1/auth/logout", { method: "POST", body: {}, token }).catch(() => undefined);
    setUser(null);
    setToken("");
    setOrganizations([]);
  }

  useEffect(() => {
    let cancelled = false;
    refreshSession()
      .then(async (session) => {
        if (cancelled || !session) return;
        setUser(session.user);
        setToken(session.access_token);
        const response = await api<{ data: Organization[] }>("/api/v1/me/organizations", { token: session.access_token });
        if (!cancelled) setOrganizations(response.data);
      })
      .finally(() => {
        if (!cancelled) setReady(true);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const value = useMemo(
    () => ({ ready, user, token, organizations, setSession, refreshUser, signOut }),
    [ready, user, token, organizations],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("AuthProvider is missing");
  return value;
}
