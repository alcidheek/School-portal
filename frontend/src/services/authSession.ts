export function clearSession() {
  localStorage.removeItem("token");
  localStorage.removeItem("user");
}

// Public login/reset requests do not use this wrapper. Credential-confirmation
// failures are not expired sessions and must leave Account Security usable.
export async function authenticatedFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const response = await fetch(input, init);
  if (response.status === 401 || response.status === 403) {
    let message = "";
    try {
      const body: unknown = await response.clone().json();
      if (body && typeof body === "object" && "error" in body && typeof body.error === "string") message = body.error;
    } catch { /* A legacy non-JSON 401 still means authentication failed. */ }
    if ((response.status === 401 && message !== "Current password is incorrect") ||
        (response.status === 403 && message.startsWith("Account is inactive"))) {
      clearSession();
      window.dispatchEvent(new window.Event("school-session-invalid"));
    }
  }
  return response;
}
