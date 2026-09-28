import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { flagsApi } from "../api/flags";
import type { Flag, CreateFlagRequest, FlagType } from "../api/flags";
import { useAuth } from "../store/AuthContext";

export function FlagsPage() {
  const { orgId } = useAuth();
  const navigate = useNavigate();
  const [flags, setFlags] = useState<Flag[]>([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [form, setForm] = useState<CreateFlagRequest>({
    key: "", name: "", description: "", flag_type: "boolean",
  });
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!orgId) return;
    flagsApi.list(orgId)
      .then((res) => setFlags(res.data))
      .catch(() => setError("Failed to load flags."))
      .finally(() => setLoading(false));
  }, [orgId]);

  const handleCreate = async () => {
    if (!orgId) return;
    setCreating(true);
    try {
      const { data } = await flagsApi.create(orgId, form);
      setFlags((prev) => [data, ...prev]);
      setShowModal(false);
      setForm({ key: "", name: "", description: "", flag_type: "boolean" });
    } catch (err: any) {
      setError(err?.response?.data?.error ?? "Failed to create flag.");
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="max-w-4xl mx-auto px-6 py-8">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-lg font-semibold">Feature Flags</h1>
        <button
          onClick={() => setShowModal(true)}
          className="bg-blue-600 text-white text-sm px-4 py-2 rounded hover:bg-blue-700"
        >
          + New Flag
        </button>
      </div>

      {error && <p className="text-red-500 text-sm mb-4">{error}</p>}

      {loading ? (
        <p className="text-sm text-gray-400">Loading...</p>
      ) : flags.length === 0 ? (
        <p className="text-sm text-gray-400">No flags yet.</p>
      ) : (
        <table className="w-full text-sm border-collapse">
          <thead>
            <tr className="border-b text-left text-gray-500">
              <th className="py-2 pr-4 font-medium">Key</th>
              <th className="py-2 pr-4 font-medium">Name</th>
              <th className="py-2 pr-4 font-medium">Type</th>
              <th className="py-2 font-medium">Status</th>
            </tr>
          </thead>
          <tbody>
            {flags.map((flag) => (
              <tr
                key={flag.id}
                onClick={() => navigate(`/flags/${flag.id}`)}
                className="border-b hover:bg-gray-50 cursor-pointer"
              >
                <td className="py-3 pr-4 font-mono text-gray-700">{flag.key}</td>
                <td className="py-3 pr-4 text-gray-800">{flag.name}</td>
                <td className="py-3 pr-4 text-gray-500">{flag.flag_type}</td>
                <td className="py-3">
                  {flag.archived ? (
                    <span className="text-xs text-gray-400 bg-gray-100 px-2 py-0.5 rounded">archived</span>
                  ) : (
                    <span className="text-xs text-green-600 bg-green-50 px-2 py-0.5 rounded">active</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {showModal && (
        <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 w-full max-w-md shadow-lg">
            <h2 className="font-semibold mb-4">New Flag</h2>
            <div className="space-y-3">
              <div>
                <label className="block text-sm font-medium mb-1">Key</label>
                <input
                  value={form.key}
                  onChange={(e) => setForm({ ...form, key: e.target.value })}
                  placeholder="new-checkout-flow"
                  className="w-full border rounded px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Name</label>
                <input
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="New Checkout Flow"
                  className="w-full border rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Description</label>
                <input
                  value={form.description}
                  onChange={(e) => setForm({ ...form, description: e.target.value })}
                  className="w-full border rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Type</label>
                <select
                  value={form.flag_type}
                  onChange={(e) => setForm({ ...form, flag_type: e.target.value as FlagType })}
                  className="w-full border rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                >
                  <option value="boolean">boolean</option>
                  <option value="string">string</option>
                  <option value="number">number</option>
                  <option value="json">json</option>
                </select>
              </div>
            </div>
            <div className="flex justify-end gap-2 mt-5">
              <button
                onClick={() => setShowModal(false)}
                className="text-sm px-4 py-2 border rounded hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                onClick={handleCreate}
                disabled={creating || !form.key || !form.name}
                className="text-sm px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                {creating ? "Creating..." : "Create"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}