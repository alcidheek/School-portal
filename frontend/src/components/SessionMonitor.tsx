import { useEffect } from "react";
import { useNavigate } from "react-router-dom";

export default function SessionMonitor() {
  const navigate = useNavigate();
  useEffect(() => {
    const invalid = () => navigate("/", { replace: true, state: { message: "Your session ended. Please sign in again." } });
    window.addEventListener("school-session-invalid", invalid);
    return () => window.removeEventListener("school-session-invalid", invalid);
  }, [navigate]);
  return null;
}
