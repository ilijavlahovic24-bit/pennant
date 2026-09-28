import { apiClient } from "./client";

export type FlagType = "boolean" | "string" | "number" | "json";

export interface Flag {
  id: string;
  key: string;
  name: string;
  description: string;
  flag_type: FlagType;
  archived: boolean;
  created_at: string;
}

export interface FlagEnvironment {
  id: string;
  flag_id: string;
  environment_id: string;
  enabled: boolean;
  rollout_percent: number;
  value: unknown;
  expires_at: string | null;
}

export interface TargetingRule {
  id: string;
  priority: number;
  attribute: string;
  operator: string;
  value: unknown;
  serve_enabled: boolean;
  serve_percent: number | null;
}

export interface CreateFlagRequest {
  key: string;
  name: string;
  description?: string;
  flag_type: FlagType;
}

export interface PatchFlagEnvRequest {
  enabled?: boolean;
  rollout_percent?: number;
  value?: unknown;
  expires_at?: string | null;
}

export const flagsApi = {
  list: (orgId: string) =>
    apiClient.get<Flag[]>(`/v1/orgs/${orgId}/flags`),

  get: (orgId: string, flagId: string) =>
    apiClient.get<Flag>(`/v1/orgs/${orgId}/flags/${flagId}`),

  create: (orgId: string, data: CreateFlagRequest) =>
    apiClient.post<Flag>(`/v1/orgs/${orgId}/flags`, data),

  patch: (orgId: string, flagId: string, data: Partial<CreateFlagRequest>) =>
    apiClient.patch<Flag>(`/v1/orgs/${orgId}/flags/${flagId}`, data),

  archive: (orgId: string, flagId: string) =>
    apiClient.delete(`/v1/orgs/${orgId}/flags/${flagId}`),

  patchEnv: (orgId: string, flagId: string, envId: string, data: PatchFlagEnvRequest) =>
    apiClient.patch<FlagEnvironment>(
      `/v1/orgs/${orgId}/flags/${flagId}/environments/${envId}`,
      data
    ),

  getRules: (orgId: string, flagId: string, envId: string) =>
    apiClient.get<TargetingRule[]>(
      `/v1/orgs/${orgId}/flags/${flagId}/environments/${envId}/rules`
    ),

  putRules: (orgId: string, flagId: string, envId: string, rules: Omit<TargetingRule, "id">[]) =>
    apiClient.put(
      `/v1/orgs/${orgId}/flags/${flagId}/environments/${envId}/rules`,
      { rules }
    ),

  deleteRule: (orgId: string, flagId: string, envId: string, ruleId: string) =>
    apiClient.delete(
      `/v1/orgs/${orgId}/flags/${flagId}/environments/${envId}/rules/${ruleId}`
    ),
};