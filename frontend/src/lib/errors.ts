const fallback = 'Something went wrong. The details were saved to LeGaJ’s log.';

/**
 * The user-facing text of a failed backend call. Wails rejects with the Go
 * error's message, which is already user-safe (internal/apperr).
 */
export function errorMessage(e: unknown): string {
  if (typeof e === 'string' && e.trim() !== '') return e;
  if (e instanceof Error && e.message.trim() !== '') return e.message;
  return fallback;
}
