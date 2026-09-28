import { apiClient } from "./client";

export interface User {
  id: string;
  email: string;
  created_at: string;
}

export interface AuthResponse {
  access_token: string;
  refresh_token: string;
  user: User;
}

export const authApi = {
  register: (email: string, password: string) =>
    apiClient.post<AuthResponse>("/auth/register", { email, password }),

  login: (email: string, password: string) =>
    apiClient.post<AuthResponse>("/auth/login", { email, password }),

  refresh: (refreshToken: string) =>
    apiClient.post<AuthResponse>("/auth/refresh", { refresh_token: refreshToken }),

  logout: (refreshToken: string) =>
    apiClient.post("/auth/logout", { refresh_token: refreshToken }),

  me: () => apiClient.get<User>("/v1/auth/me"),
};