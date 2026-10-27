import { useEffect, useRef, useState } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import { changeEmail, changePassword, clearAccountSession, currentAccount, saveAccountSession, storedAdmin, strongPasswordError } from "../../services/accountSecurityService.ts";
import "./AccountSecurity.css";
import ProfilePictureSettings from "../../components/admin/ProfilePictureSettings.tsx";

function AccountSecurity() {
  const navigate = useNavigate();
  const [user, setUser] = useState(storedAdmin);
  const [ready, setReady] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [newEmail, setNewEmail] = useState("");
  const [emailPassword, setEmailPassword] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [emailMessage, setEmailMessage] = useState("");
  const [emailError, setEmailError] = useState("");
  const [passwordError, setPasswordError] = useState("");
  const [loading, setLoading] = useState<"email" | "password" | null>(null);
  const submitting = useRef(false);

  useEffect(() => {
    if (!storedAdmin()) return;
    let cancelled = false;
    currentAccount().then((account) => {
      if (!cancelled) {
        setUser(account);
        localStorage.setItem("user", JSON.stringify(account));
        setReady(true);
      }
    }).catch((error: unknown) => {
      if (!cancelled) setLoadError(error instanceof Error ? error.message : "Unable to load your account.");
    });
    return () => { cancelled = true; };
  }, []);

  if (!user) return <Navigate to="/" replace />;

  function clearPasswords() {
    setEmailPassword(""); setCurrentPassword(""); setNewPassword(""); setConfirmation("");
  }

  async function submitEmail(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current || !ready) return;
    submitting.current = true; setLoading("email"); setEmailMessage(""); setEmailError("");
    try {
      const email = newEmail.trim().toLowerCase();
      if (!/^[^\s@]+@[^\s@]+$/.test(email)) throw new Error("Enter a valid new email.");
      if (!emailPassword) throw new Error("Enter your current password.");
      const result = await changeEmail(email, emailPassword);
      saveAccountSession(result); setUser(result.user); setNewEmail(""); setEmailMessage(result.message);
    } catch (error) { setEmailError(error instanceof Error ? error.message : "Unable to change email."); }
    finally { clearPasswords(); submitting.current = false; setLoading(null); }
  }

  async function submitPassword(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current || !ready) return;
    submitting.current = true; setLoading("password"); setPasswordError(""); setEmailMessage("");
    try {
      if (!currentPassword) throw new Error("Enter your current password.");
      const policyError = strongPasswordError(newPassword);
      if (policyError) throw new Error(policyError);
      if (newPassword !== confirmation) throw new Error("New passwords do not match.");
      const result = await changePassword(currentPassword, newPassword, confirmation);
      clearAccountSession(); clearPasswords();
      navigate("/", { replace: true, state: { message: result.message } });
    } catch (error) { setPasswordError(error instanceof Error ? error.message : "Unable to change password."); }
    finally { clearPasswords(); submitting.current = false; setLoading(null); }
  }

  const disabled = !ready || loading !== null;
  return (
    <main className="account-security">
      <header><p>Settings</p><h1>Account Security</h1><p>Manage your administrator email and password.</p></header>
      {loadError && <p role="alert">{loadError} <a href="/">Sign in again</a></p>}
      {!ready && !loadError && <p role="status">Loading your account…</p>}
      <div className="security-forms">
        <ProfilePictureSettings />

        <section className="security-card">
          <h2>Change Email</h2>
          <p>Current email: <strong data-testid="current-email">{user.email}</strong></p>
          <form onSubmit={submitEmail} noValidate>
            <fieldset disabled={disabled}>
              <label htmlFor="new-email">New email</label>
              <input id="new-email" type="email" autoComplete="email" maxLength={255} required value={newEmail} onChange={(e) => setNewEmail(e.target.value)} />
              <label htmlFor="email-password">Current password</label>
              <input id="email-password" type="password" autoComplete="current-password" required value={emailPassword} onChange={(e) => setEmailPassword(e.target.value)} />
              <button type="submit">{loading === "email" ? "Updating…" : "Update Email"}</button>
            </fieldset>
            {emailError && <p role="alert">{emailError}</p>}
            {emailMessage && <p className="security-success" role="status">{emailMessage}</p>}
          </form>
        </section>
        <section className="security-card">
          <h2>Change Password</h2>
          <p>Use at least 12 characters, uppercase, lowercase, a number and a symbol. Maximum 72 bytes. You will sign in again after saving.</p>
          <form onSubmit={submitPassword} noValidate>
            <fieldset disabled={disabled}>
              <label htmlFor="current-password">Current password</label>
              <input id="current-password" type="password" autoComplete="current-password" required value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} />
              <label htmlFor="new-password">New password</label>
              <input id="new-password" type="password" autoComplete="new-password" required value={newPassword} onChange={(e) => setNewPassword(e.target.value)} />
              <label htmlFor="confirm-password">Confirm new password</label>
              <input id="confirm-password" type="password" autoComplete="new-password" required value={confirmation} onChange={(e) => setConfirmation(e.target.value)} />
              <button type="submit">{loading === "password" ? "Updating…" : "Update Password"}</button>
            </fieldset>
            {passwordError && <p role="alert">{passwordError}</p>}
          </form>
        </section>
      </div>
    </main>
  );
}
export default AccountSecurity;
