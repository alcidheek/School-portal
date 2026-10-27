import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import "./Toast.css";

export type ToastKind = "success" | "warning" | "error";

// Mount with a new key for each notification to start a fresh dismissal timer.
export default function Toast({ message, kind }: { message: string; kind: ToastKind }) {
  const [visible, setVisible] = useState(true);
  useEffect(() => {
    const timer = window.setTimeout(() => setVisible(false), 4000);
    return () => window.clearTimeout(timer);
  }, []);

  if (!visible) return null;
  return createPortal(
    <div className={`notification-toast notification-toast--${kind}`} role={kind === "success" ? "status" : "alert"} aria-atomic="true">
      <span className="notification-toast__label">{kind === "success" ? "Success" : kind === "warning" ? "Warning" : "Error"}</span>
      <p>{message}</p>
      <button type="button" className="notification-toast__close" aria-label="Dismiss notification" onClick={() => setVisible(false)}>×</button>
    </div>,
    document.body,
  );
}
