import { useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { resetPassword } from "../services/authService.ts";
import { strongPasswordError } from "../services/accountSecurityService.ts";
import "../App.css";

export default function ResetPasswordPage() {
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const submitting = useRef(false);
  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current) return;
    setError("");
    if (!/^[^\s@]+@[^\s@]+$/.test(email.trim())) { setError("Enter a valid email address."); return; }
    if (!/^\d{6}$/.test(code.trim())) { setError("Enter the six-digit reset code."); return; }
    const policy = strongPasswordError(password);
    if (policy) { setError(policy); return; }
    if (password !== confirmation) { setError("New passwords do not match."); return; }
    submitting.current = true; setLoading(true);
    try {
      await resetPassword(email, code, password);
      setCode(""); setPassword(""); setConfirmation("");
      navigate("/", { replace: true, state: { message: "Password reset successfully. Please sign in with your new password." } });
    } catch (error) { setError(error instanceof Error ? error.message : "Unable to reset your password."); }
    finally { submitting.current = false; setLoading(false); }
  }
  return <main className="login-page"><section className="login-card">
    <header className="school-header"><h2><span>Blue</span>Print</h2><p>Group of School</p></header>
    <div className="heading-divider" /><h1>Reset Password</h1>
    <p className="subtitle">Use at least 12 characters, uppercase, lowercase, a number and a symbol (maximum 72 bytes).</p>
    <form onSubmit={submit} noValidate>
      <div className="form-group"><label htmlFor="reset-email">Email</label><input id="reset-email" type="email" autoComplete="email" maxLength={255} value={email} onChange={e => setEmail(e.target.value)} required disabled={loading} /></div>
      <div className="form-group"><label htmlFor="reset-code">Six-digit code</label><input id="reset-code" type="text" inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code} onChange={e => setCode(e.target.value)} required disabled={loading} /></div>
      <div className="form-group"><label htmlFor="reset-password">New password</label><input id="reset-password" type="password" autoComplete="new-password" value={password} onChange={e => setPassword(e.target.value)} required disabled={loading} /></div>
      <div className="form-group"><label htmlFor="reset-confirmation">Confirm new password</label><input id="reset-confirmation" type="password" autoComplete="new-password" value={confirmation} onChange={e => setConfirmation(e.target.value)} required disabled={loading} /></div>
      {error && <p className="login-error" role="alert">{error}</p>}
      <button type="submit" disabled={loading}>{loading ? "Resetting…" : "Reset Password"}</button>
    </form>
    <p className="login-footer"><Link to="/forgot-password">Request a new code</Link></p>
    <p className="login-footer"><Link to="/">Back to login</Link></p>
  </section></main>;
}
