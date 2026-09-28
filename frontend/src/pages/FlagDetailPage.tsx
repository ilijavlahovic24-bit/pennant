import { useEffect, useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { flagsApi } from "../api/flags";
import type { Flag, FlagEnvironment } from "../api/flags";
import { useAuth } from "../store/AuthContext";

const ENVS = [
  { id: "dev", label: "Development" },
  { id: "staging", label: "Staging" },
  { id: "production", label: "Production" },
];

export function FlagDetailPage() {
  const { flagId } = useParams<{ flagId: string }>();
  const { orgId } = useAuth();
  const navigate = useNavigate();
  const [flag, setFlag] = useState<Flag | null>(null);
  const [envStates, setEnvStates] = useState<Record<string, FlagEnvironment>>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState<string | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!orgId || !flagId) return;
    flagsApi.get(orgId, flagId)
      .then((res) => setFlag(res.data))
      .catch(() => setError("Failed to load flag."))
      .finally(() => setLoading(false));
  }, [orgId, flagId]);

  const toggleEnv = async (envId: string, current: FlagEnvironment | undefined) => {
    if (!orgId || !flagId) return;
    setSaving(envId);
    try {
      const { data } = await flagsApi.patchEnv(orgId, flagId, envId, {
        enabled: !(current?.enabled ?? false),
      });
      setEnvStates((prev) => ({ ...prev, [envId]: data }));
    } catch {
      setError("Failed to update environment.");
    } finally {
      setSaving(null);
    }
  };

  const updateRollout = async (envId: string, percent: number) => {
    if (!orgId || !flagId) return;
    setSaving(envId);
    try {
      const { data } = await flagsApi.patchEnv(orgId, flagId, envId, {
        rollout_percent: percent,
      });
      setEnvStates((prev) => ({ ...prev, [envId]: data }));
    } catch {
      setError("Failed to update rollout.");
    } finally {
      setSaving(null);
    }
  };

  const archiveFlag = async () => {
    if (!orgId || !flagId || !confirm("Archive this flag?")) return;
    try {
      await flagsApi.archive(orgId, flagId);
      navigate("/flags");
    } catch {
      setError("Failed to archive flag.");
    }
  };

  if (loading) return <div className="p-8 text-sm text-gray-400">Loading...</div>;
  if (!flag) return <div className="p-8 text-sm text-red-500">{error || "Flag not found."}</div>;

  return (
    <div className="max-w-2xl mx-auto px-6 py-8">
      <button
        onClick={() => navigate("/flags")}
        className="text-sm text-gray-400 hover:text-gray-600 mb-4 block"
      >
        ← Back
      </button>

      <div className="flex items-start justify-between mb-6">
        <div>
          <h1 className="text-lg font-semibold">{flag.name}</h1>
          <p className="text-sm font-mono text-gray-400 mt-0.5">{flag.key}</p>
          {flag.description && (
            <p className="text-sm text-gray-500 mt-1">{flag.description}</p>
          )}
        </div>
        <button
          onClick={archiveFlag}
          className="text-sm text-red-400 hover:text-red-600 border border-red-200 rounded px-3 py-1"
        >
          Archive
        </button>
      </div>

      {error && <p className="text-red-500 text-sm mb-4">{error}</p>}

      <h2 className="text-sm font-medium text-gray-500 uppercase tracking-wide mb-3">
        Environments
      </h2>

      <div className="space-y-3">
        {ENVS.map(({ id, label }) => {
          const env = envStates[id];
          const enabled = env?.enabled ?? false;
          const rollout = env?.rollout_percent ?? 0;
          const isSaving = saving === id;

          return (
            <div key={id} className="border rounded-lg p-4">
              <div className="flex items-center justify-between mb-3">
                <span className="text-sm font-medium">{label}</span>
                <button
                  onClick={() => toggleEnv(id, env)}
                  disabled={isSaving}
                  className={`relative w-10 h-5 rounded-full transition-colors ${
                    enabled ? "bg-blue-600" : "bg-gray-200"
                  } disabled:opacity-50`}
                >
                  <span
                    className={`absolute top-0.5 left-0.5 w-4 h-4 bg-white rounded-full shadow transition-transform ${
                      enabled ? "translate-x-5" : ""
                    }`}
                  />
                </button>
              </div>
              {enabled && (
                <div>
                  <div className="flex items-center justify-between text-xs text-gray-500 mb-1">
                    <span>Rollout</span>
                    <span>{rollout}%</span>
                  </div>
                  <input
                    type="range"
                    min={0}
                    max={100}
                    value={rollout}
                    onChange={(e) => setEnvStates((prev) => ({
                      ...prev,
                      [id]: { ...env, rollout_percent: Number(e.target.value) } as FlagEnvironment,
                    }))}
                    onMouseUp={(e) => updateRollout(id, Number((e.target as HTMLInputElement).value))}
                    className="w-full"
                  />
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}