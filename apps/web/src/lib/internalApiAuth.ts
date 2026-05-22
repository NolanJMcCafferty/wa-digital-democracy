import "server-only";

export const INTERNAL_API_TOKEN_ENV = "WADD_INTERNAL_API_TOKEN";

export function internalAPIHeaders(): Record<string, string> {
  const token = process.env[INTERNAL_API_TOKEN_ENV];
  if (!token) {
    throw new Error(`${INTERNAL_API_TOKEN_ENV} is required for WA DD API requests`);
  }
  return {
    Authorization: `Bearer ${token}`,
  };
}
