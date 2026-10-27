import { authenticatedFetch, clearSession } from "./authSession.ts";
import { readApiResponse } from "./apiResponse.ts";
import type { LoginResponse, LoginUser } from "./authService.ts";

const baseURL = "http://localhost:8080/api/v1/me";

function isAdminUser(value: unknown): value is LoginUser {
  return !!value && typeof value === "object" && "id" in value && typeof value.id === "string" &&
    "email" in value && typeof value.email === "string" && "role" in value && value.role === "ADMIN";
}

export function storedAdmin(): LoginUser | null {
  try {
    const user: unknown = JSON.parse(localStorage.getItem("user") ?? "null");
    if (localStorage.getItem("token") && isAdminUser(user)) {
      return { id: user.id, email: user.email, role: "ADMIN" };
    }
  } catch { /* Invalid cached authentication is treated as signed out. */ }
  return null;
}

async function request<T>(path: string, data?: object): Promise<T> {
  const token = localStorage.getItem("token");
  if (!token) throw new Error("Please sign in again.");
  const response = await authenticatedFetch(baseURL + path, {
    method: data ? "PATCH" : "GET",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    ...(data ? { body: JSON.stringify(data) } : {}),
  });
  return readApiResponse<T>(response, "Unable to update your account. Please try again.");
}

export async function currentAccount(): Promise<LoginUser> {
  const user = await request<{ user_id: string; email: string; role: LoginUser["role"] }>("");
  if (!user || typeof user.user_id !== "string" || typeof user.email !== "string") throw new Error("Invalid account response. Please sign in again.");
  if (user.role !== "ADMIN") throw new Error("Administrator access is required.");
  return { id: user.user_id, email: user.email, role: user.role };
}

export async function changeEmail(newEmail: string, currentPassword: string): Promise<LoginResponse> {
  const result = await request<LoginResponse>("/email", { new_email: newEmail.trim().toLowerCase(), current_password: currentPassword });
  if (!result || typeof result.token !== "string" || !result.token || typeof result.message !== "string" || !isAdminUser(result.user)) {
    throw new Error("Invalid account response. Please sign in again.");
  }
  return result;
}

export async function changePassword(currentPassword: string, newPassword: string, confirmPassword: string): Promise<{ message: string }> {
  const result = await request<{ message: string }>("/password", { current_password: currentPassword, new_password: newPassword, confirm_password: confirmPassword });
  if (!result || typeof result.message !== "string") throw new Error("Invalid account response. Please sign in again.");
  return result;
}

export function saveAccountSession(result: LoginResponse) {
  localStorage.setItem("token", result.token);
  localStorage.setItem("user", JSON.stringify(result.user));
}

export function clearAccountSession() {
  clearSession();
}

export function strongPasswordError(value: string): string {
  return Array.from(value).length >= 12 && new TextEncoder().encode(value).length <= 72 &&
    /\p{Lu}/u.test(value) && /\p{Ll}/u.test(value) && /\p{Nd}/u.test(value) && /[\p{P}\p{S}]/u.test(value)
    ? "" : "Use at least 12 characters, uppercase, lowercase, a number and a symbol (maximum 72 bytes).";
}
