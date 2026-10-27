import { useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";

import "./AddUserPage.css";

import { createUser } from "../../services/adminService";

function AddUserPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  // Dashboard buttons can preset STUDENT or TEACHER.
  const presetRole = searchParams.get("role");

  const [role, setRole] = useState<"STUDENT" | "TEACHER">(
    presetRole === "TEACHER" ? "TEACHER" : "STUDENT"
  );

  // Form values.
  const [fullName, setFullName] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [password, setPassword] = useState("");

  // Guardian details are required only for student accounts.
  const [guardianName, setGuardianName] = useState("");
  const [guardianPhone, setGuardianPhone] = useState("");

  // Submission states.
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const submitting = useRef(false);

  // Send the form to the registration endpoint.
  async function handleSubmit(
    event: React.FormEvent<HTMLFormElement>
  ) {
    event.preventDefault();
    if (submitting.current) return;
    submitting.current = true;

    setError("");
    setLoading(true);

    try {
      if (!fullName.trim() || fullName.trim().length > 255) throw new Error("Enter a full name of 1 to 255 characters.");
      if (!/^[^\s@]+@[^\s@]+$/.test(email.trim())) throw new Error("Enter a valid email.");
      const validPhone = (value: string) => /^\+?[\d ()-]+$/.test(value.trim()) && value.trim().length <= 20 &&
        value.replace(/\D/g, "").length >= 7 && value.replace(/\D/g, "").length <= 15;
      if (!validPhone(phone)) throw new Error("Enter a phone number with 7 to 15 digits (maximum 20 characters).");
      if (!password.trim() || password.length < 6 || new TextEncoder().encode(password).length > 72) throw new Error("Password must contain 6 to 72 bytes.");
      if (role === "STUDENT" && (!guardianName.trim() || guardianName.trim().length > 255 || !validPhone(guardianPhone))) {
        throw new Error("Enter a guardian name and a valid guardian phone number.");
      }
      await createUser({
        full_name: fullName.trim(),
        email: email.trim().toLowerCase(),
        phone: phone.trim(),
        password,
        role,
        ...(role === "STUDENT" && {
          guardian_name: guardianName.trim(),
          guardian_phone: guardianPhone.trim(),
        }),
      });

      // Return to the users page after successful creation.
      navigate("/admin/users");
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Failed to create user"
      );
    } finally {
      submitting.current = false;
      setPassword("");
      setLoading(false);
    }
  }

  return (


        <main className="add-user-content">
          {/* Page heading */}
          <div className="add-user-heading">
            <div>
              <h2>Add User</h2>
              <p>Create a new student or teacher account.</p>
            </div>

            <button
              className="back-users-button"
              onClick={() => navigate("/admin/users")}
            >
              ← Back to Users
            </button>
          </div>

          <section className="add-user-card">
            <form
              className="add-user-form"
              onSubmit={handleSubmit}
            >
              {/* User role */}
              <div className="form-group full-width">
                <label htmlFor="role">User Role</label>

                <select
                  id="role"
                  value={role}
                  onChange={(event) =>
                    setRole(
                      event.target.value as
                        | "STUDENT"
                        | "TEACHER"
                    )
                  }
                >
                  <option value="STUDENT">Student</option>
                  <option value="TEACHER">Teacher</option>
                </select>
              </div>

              {/* Full name */}
              <div className="form-group">
                <label htmlFor="fullName">Full Name</label>

                <input
                  id="fullName"
                  type="text"
                  placeholder="Enter full name"
                  value={fullName}
                  onChange={(event) =>
                    setFullName(event.target.value)
                  }
                  required
                />
              </div>

              {/* Email */}
              <div className="form-group">
                <label htmlFor="email">Email Address</label>

                <input
                  id="email"
                  type="email"
                  placeholder="Enter email address"
                  value={email}
                  onChange={(event) =>
                    setEmail(event.target.value)
                  }
                  required
                />
              </div>

              {/* Phone */}
              <div className="form-group">
                <label htmlFor="phone">Phone Number</label>

                <input
                  id="phone"
                  type="tel"
                  placeholder="Enter phone number"
                  value={phone}
                  onChange={(event) =>
                    setPhone(event.target.value)
                  }
                  required
                />
              </div>

              {/* Password */}
              <div className="form-group">
                <label htmlFor="password">Password</label>

                <input
                  id="password"
                  type="password"
                  placeholder="Enter password"
                  value={password}
                  onChange={(event) =>
                    setPassword(event.target.value)
                  }
                  required
                />
              </div>

              {/* Student-only guardian information */}
              {role === "STUDENT" && (
                <>
                  <div className="form-group">
                    <label htmlFor="guardianName">
                      Guardian Name
                    </label>

                    <input
                      id="guardianName"
                      type="text"
                      placeholder="Enter guardian name"
                      value={guardianName}
                      onChange={(event) =>
                        setGuardianName(event.target.value)
                      }
                    />
                  </div>

                  <div className="form-group">
                    <label htmlFor="guardianPhone">
                      Guardian Phone
                    </label>

                    <input
                      id="guardianPhone"
                      type="tel"
                      placeholder="Enter guardian phone number"
                      value={guardianPhone}
                      onChange={(event) =>
                        setGuardianPhone(event.target.value)
                      }
                    />
                  </div>
                </>
              )}

              {/* Backend error */}
              {error && (
                <div className="form-error full-width">
                  {error}
                </div>
              )}

              {/* Form actions */}
              <div className="form-actions full-width">
                <button
                  type="button"
                  className="cancel-user-button"
                  onClick={() => navigate("/admin/users")}
                  disabled={loading}
                >
                  Cancel
                </button>

                <button
                  type="submit"
                  className="create-user-button"
                  disabled={loading}
                >
                  {loading
                    ? "Creating..."
                    : `Create ${
                        role === "STUDENT"
                          ? "Student"
                          : "Teacher"
                      }`}
                </button>
              </div>
            </form>
          </section>
        </main>

  );
}

export default AddUserPage;
