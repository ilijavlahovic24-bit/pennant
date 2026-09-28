import { createContext, useContext, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { authApi } from "../api/auth";
import type { User } from "../api/auth";

interface AuthContextValue {
  user: User | null;
  orgId: string | null;
  loading: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [orgId, setOrgId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const token = localStorage.getItem("access_token");
    if (!token) { setLoading(false); return; }

    authApi.me()
      .then((res) => {
        setUser(res.data);
        setOrgId(localStorage.getItem("org_id"));
      })
      .catch(() => {
        localStorage.clear();
      })
      .finally(() => setLoading(false));
  }, []);

  const persist = (data: { access_token: string; refresh_token: string; user: User; org_id?: string }) => {
    localStorage.setItem("access_token", data.access_token);
    localStorage.setItem("refresh_token", data.refresh_token);
    if (data.org_id) {
      localStorage.setItem("org_id", data.org_id);
      setOrgId(data.org_id);
    }
    setUser(data.user);
  };

  const login = async (email: string, password: string) => {
    const { data } = await authApi.login(email, password);
    persist(data as any);
  };

  const register = async (email: string, password: string) => {
    const { data } = await authApi.register(email, password);
    persist(data as any);
  };

  const logout = async () => {
    const rt = localStorage.getItem("refresh_token") ?? "";
    await authApi.logout(rt).catch(() => {});
    localStorage.clear();
    setUser(null);
    setOrgId(null);
  };

  return (
    <AuthContext.Provider value={{ user, orgId, loading, login, register, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider");
  return ctx;
}