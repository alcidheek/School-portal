// Read a response once. Legacy plaintext/HTML errors receive a safe fallback
// instead of leaking server diagnostics or causing a JSON parsing exception.
export async function readApiResponse<T>(
  response: Response,
  fallback: string,
): Promise<T> {
  let result: unknown;
  try {
    result = JSON.parse(await response.text());
  } catch {
    if (response.ok) throw new Error("Invalid response from server. Please try again.");
  }

  if (!response.ok) {
    if (response.status >= 500) {
      throw new Error("Server error. Please try again later.");
    }
    const message =
      typeof result === "object" && result !== null && "error" in result
        ? result.error
        : undefined;
    // Do not display diagnostics from an older backend returning raw SQL errors.
    const safeMessage = typeof message === "string" && message.trim() &&
      !/sql:|pq:|sqlstate|panic:|stack trace|password_hash/i.test(message);
    throw new Error(safeMessage ? message : fallback);
  }

  return result as T;
}
