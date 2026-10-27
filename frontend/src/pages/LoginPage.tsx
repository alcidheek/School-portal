import { useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { login } from "../services/authService";
import Toast from "../components/Toast.tsx";
import "../App.css";

function LoginPage() {
      // Lets us move to another page after login.
  const navigate = useNavigate();
  const location = useLocation();
  const message = typeof location.state?.message === "string" ? location.state.message : "";
  const toast = location.state?.toast;
  const toastMessage = typeof toast?.message === "string" ? toast.message : "";
  const toastKind = toast?.kind;

  // Store what the user types into the form.
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  // Store login errors and loading state.
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  // Runs when the login form is submitted.
  async function handleLogin(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (loading) return;
    setError("");
    setLoading(true);

    try {
      const result = await login({
        email,
        password,
      });

      // Save authentication information for later requests.
      localStorage.setItem("token", result.token);
      localStorage.setItem("user", JSON.stringify(result.user));

      // Send each role to its own dashboard.
switch (result.user.role) {
  case "ADMIN":
    navigate("/admin/dashboard");
    break;

  case "TEACHER":
    navigate("/teacher/dashboard");
    break;

  case "STUDENT":
    navigate("/student/dashboard");
    break;
}


    } catch (err) {
      // Display the backend/login error.
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError("Login failed");
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="login-page">
      {toastMessage && (toastKind === "success" || toastKind === "warning" || toastKind === "error") &&
        <Toast key={location.key} message={toastMessage} kind={toastKind} />}
      {/* Moving educational background */}
      <div className="book-background" aria-hidden="true">
        <span>📖</span>
        <span>📘</span>
        <span>📖</span>
        <span>📚</span>
        <span>📘</span>
        <span>📖</span>
        <span>📚</span>
        <span>📘</span>
      </div>

      <section className="login-card">
        {message && <p role="status">{message}</p>}
        <header className="school-header">
          <h2>
            <span>Blue</span>Print
          </h2>
          <p>Group of School</p>
        </header>

        <div className="heading-divider"></div>

        <h1>Welcome Back</h1>

        <p className="subtitle">
          Sign in to access your school portal
        </p>

        {/* Submit now calls our login function. */}
        <form onSubmit={handleLogin}>
          <div className="form-group">
            <label htmlFor="email">Email</label>

            <input
              id="email"
              type="email"
              placeholder="Enter your email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              required
            />
          </div>

          <div className="form-group">
            <label htmlFor="password">Password</label>

            <input
              id="password"
              type="password"
              placeholder="Enter your password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required
            />
          </div>

          {/* Show login errors to the user. */}
          {error && <p className="login-error">{error}</p>}

          <div className="forgot-password">
            <Link to="/forgot-password">Forgot password?</Link>
          </div>

          <button type="submit" disabled={loading}>
            {loading ? "Signing In..." : "Sign In →"}
          </button>
        </form>

        <p className="login-footer">
          Secure access for students, teachers and administrators.
        </p>
      </section>
    </main>
  );
}

export default LoginPage;
