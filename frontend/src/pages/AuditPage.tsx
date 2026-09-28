import { useEffect, useState } from "react";
import type{ AuditLog } from "../api/audit";
import { auditApi } from "../api/audit";
import { useAuth } from "../store/AuthContext";

export function AuditPage() {
  const { orgId } = useAuth();
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);

  useEffect(() => {
    if (!orgId) return;
    auditApi.list(orgId)
      .then((res: { data: AuditLog[] }) => setLogs(res.data))
      .catch(() => setError("Failed to load audit log."))
      .finally(() => setLoading(false));
  }, [orgId]);

  const formatDate = (iso: string) =>
    new Date(iso).toLocaleString("sr-RS", { dateStyle: "short", timeStyle: "short" });

  return (
    <div className="max-w-4xl mx-auto px-6 py-8">
      <h1 className="text-lg font-semibold mb-6">Audit Log</h1>

      {error && <p className="text-red-500 text-sm mb-4">{error}</p>}

      {loading ? (
        <p className="text-sm text-gray-400">Loading...</p>
      ) : logs.length === 0 ? (
        <p className="text-sm text-gray-400">No audit entries yet.</p>
      ) : (
        <div className="space-y-1">
          {logs.map((log) => (
            <div key={log.id} className="border rounded">
              <button
                onClick={() => setExpanded(expanded === log.id ? null : log.id)}
                className="w-full text-left px-4 py-3 flex items-center justify-between hover:bg-gray-50"
              >
                <div className="flex items-center gap-4">
                  <span className="text-xs font-mono bg-gray-100 text-gray-600 px-2 py-0.5 rounded">
                    {log.action}
                  </span>
                  <span className="text-sm text-gray-500">{log.resource_type}</span>
                </div>
                <span className="text-xs text-gray-400">{formatDate(log.created_at)}</span>
              </button>
              {expanded === log.id && log.diff && (
                <div className="border-t px-4 py-3 bg-gray-50 text-xs font-mono">
                  <div className="text-red-500 mb-1">
                    − {JSON.stringify(log.diff.before, null, 2)}
                  </div>
                  <div className="text-green-600">
                    + {JSON.stringify(log.diff.after, null, 2)}
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}