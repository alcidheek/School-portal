import { useRef, useState } from "react";
import type { ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import { logout } from "../services/authService.ts";
import type { ToastKind } from "./Toast.tsx";

export default function LogoutButton({ children = "Logout", className }: { children?: ReactNode; className?: string }) {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  const pending = useRef(false);
  async function click() {
    if (pending.current) return;
    pending.current = true; setLoading(true);
    let message = "Signed out successfully.";
    let kind: ToastKind = "success";
    try { await logout(); }
    catch { message = "Signed out on this device. Server logout could not be confirmed; the previous session may remain active."; kind = "warning"; }
    finally { navigate("/", { replace: true, state: { toast: { message, kind } } }); }
  }
  return <button type="button" title="Logout" className={className} onClick={click} disabled={loading}>{loading ? "Signing out…" : children}</button>;
}
