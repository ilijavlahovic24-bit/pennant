import { Link, useLocation } from "react-router-dom";
import { useAuth } from "../store/AuthContext";

export function Navbar() {
  const { user, logout } = useAuth();
  const loc = useLocation();

  const linkClass = (path: string) =>
    `text-sm px-3 py-1 rounded ${loc.pathname.startsWith(path)
      ? "bg-gray-100 text-gray-900 font-medium"
      : "text-gray-500 hover:text-gray-900"}`;

  return (
    <nav className="border-b px-6 py-3 flex items-center justify-between">
      <div className="flex items-center gap-6">
        <span className="font-semibold text-gray-900">🚩 Pennant</span>
        <Link to="/flags" className={linkClass("/flags")}>Flags</Link>
        <Link to="/audit" className={linkClass("/audit")}>Audit Log</Link>
      </div>
      <div className="flex items-center gap-4">
        <span className="text-sm text-gray-400">{user?.email}</span>
        <button
          onClick={logout}
          className="text-sm text-red-500 hover:text-red-600"
        >
          Logout
        </button>
      </div>
    </nav>
  );
}