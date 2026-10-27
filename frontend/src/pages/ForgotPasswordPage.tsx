import { useRef, useState } from "react";
import { Link } from "react-router-dom";
import { forgotPassword } from "../services/authService.ts";
import "../App.css";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const submitting = useRef(false);
  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current) return;
    setError(""); setMessage("");
    if (!/^[^\s@]+@[^\s@]+$/.test(email.trim())) { setError("Enter a valid email address."); return; }
    submitting.current = true; setLoading(true);
    try {
      await forgotPassword(email);
      setMessage("If the account exists, a reset code has been emailed. Codes expire after 15 minutes. Please wait at least one minute before requesting another.");
    } catch (error) { setError(error instanceof Error ? error.message : "Unable to request a reset code."); }
    finally { submitting.current = false; setLoading(false); }
  }
  return <main className="login-page"><section className="login-card">
    <header className="school-header"><h2><span>Blue</span>Print</h2><p>Group of School</p></header>
    <div className="heading-divider" /><h1>Forgot Password</h1>
    <p className="subtitle">Enter your registered email to request a reset code.</p>
    <form onSubmit={submit} noValidate>
      <div className="form-group"><label htmlFor="reset-email">Email</label><input id="reset-email" type="email" autoComplete="email" maxLength={255} required value={email} onChange={e => setEmail(e.target.value)} disabled={loading} /></div>
      {error && <p className="login-error" role="alert">{error}</p>}
      {message && <p role="status">{message}</p>}
      <button type="submit" disabled={loading}>{loading ? "Sending…" : "Send Reset Code"}</button>
    </form>
    <p className="login-footer"><Link to="/reset-password">Already have a code? Reset password</Link></p>
    <p className="login-footer"><Link to="/">Back to login</Link></p>
  </section></main>;
}
