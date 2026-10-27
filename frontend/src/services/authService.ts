import { readApiResponse } from "./apiResponse.ts";
import { clearSession } from "./authSession.ts";

// Data sent to the backend.
export type LoginRequest = {
  email: string;
  password: string;
};

// User information returned after login.
export type LoginUser = {
  id: string;
  email: string;
  role: "ADMIN" | "TEACHER" | "STUDENT";
};

// Successful login response from the backend.
export type LoginResponse = {
  message: string;
  token: string;
  user: LoginUser;
};

// Send login details to the Go backend.
export async function login(
  data: LoginRequest
): Promise<LoginResponse> {
  const response = await fetch("http://localhost:8080/auth/login", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(data),
  });

  return readApiResponse<LoginResponse>(
    response,
    response.status === 401 ? "Invalid email or password" : "Login failed. Please try again.",
  );
}

async function resetRequest(path: string, data: object): Promise<{ message: string }> {
  let response: Response;
  try {
    response = await fetch(`http://localhost:8080/auth/${path}`, {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(data),
    });
  } catch { throw new Error("Unable to connect. Please check your connection and try again."); }
  const result = await readApiResponse<{ message: string }>(response, "Unable to process password reset. Please try again.");
  if (!result || typeof result.message !== "string") throw new Error("Invalid response from server. Please try again.");
  return result;
}

export function forgotPassword(email: string) {
  return resetRequest("forgot-password", { email: email.trim().toLowerCase() });
}

export async function resetPassword(email: string, code: string, newPassword: string) {
  const result = await resetRequest("reset-password", { email: email.trim().toLowerCase(), code: code.trim(), new_password: newPassword });
  clearSession();
  return result;
}

export async function logout(): Promise<void> {
  const token = localStorage.getItem("token");
  try {
    if (token) {
      const response = await fetch("http://localhost:8080/auth/logout", {
        method: "POST", headers: { Authorization: `Bearer ${token}` },
        signal: AbortSignal.timeout(10000),
      });
      if (!response.ok && response.status !== 401 && response.status !== 403) {
        await readApiResponse(response, "Unable to sign out on the server.");
      }
    }
  } finally { clearSession(); }
}
