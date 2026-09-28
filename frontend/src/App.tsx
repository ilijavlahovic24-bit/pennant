import { Routes, Route, Navigate } from "react-router-dom";
import { useAuth } from "./store/AuthContext";
import { ProtectedRoute } from "./components/ProtectedRoute";
import { Navbar } from "./components/Navbar";
import { LoginPage } from "./pages/LoginPage";
import { RegisterPage } from "./pages/RegisterPage";
import { FlagsPage } from "./pages/FlagsPage";
import { FlagDetailPage } from "./pages/FlagDetailPage";
import { AuditPage } from "./pages/AuditPage";

function Layout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <Navbar />
      {children}
    </>
  );
}

export function App() {
  const { user } = useAuth();

  return (
    <Routes>
      <Route path="/login" element={user ? <Navigate to="/flags" replace /> : <LoginPage />} />
      <Route path="/register" element={user ? <Navigate to="/flags" replace /> : <RegisterPage />} />
      <Route path="/flags" element={
        <ProtectedRoute>
          <Layout><FlagsPage /></Layout>
        </ProtectedRoute>
      } />
      <Route path="/flags/:flagId" element={
        <ProtectedRoute>
          <Layout><FlagDetailPage /></Layout>
        </ProtectedRoute>
      } />
      <Route path="/audit" element={
        <ProtectedRoute>
          <Layout><AuditPage /></Layout>
        </ProtectedRoute>
      } />
      <Route path="*" element={<Navigate to="/flags" replace />} />
    </Routes>
  );
}