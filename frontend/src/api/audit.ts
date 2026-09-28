import { apiClient } from "./client";

export interface AuditLog {
  id: string;
  user_id: string;
  action: string;
  resource_type: string;
  resource_id: string;
  diff: { before: unknown; after: unknown } | null;
  created_at: string;
}

export const auditApi = {
  list: (orgId: string, params?: { resource_type?: string; from?: string; to?: string; page?: number }) =>
    apiClient.get<AuditLog[]>(`/v1/orgs/${orgId}/audit`, { params }),
};