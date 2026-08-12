const EMAIL_CONFIRMATION_CODES = new Set([
  "EMAIL_NOT_CONFIRMED",
  "EMAIL_NOT_VERIFIED",
  "EMAIL_UNCONFIRMED",
  "EMAIL_UNVERIFIED",
  "UNCONFIRMED_EMAIL",
  "UNVERIFIED_EMAIL",
  "USER_EMAIL_NOT_CONFIRMED",
  "USER_EMAIL_NOT_VERIFIED",
]);

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? value as Record<string, unknown>
    : null;
}

export function isEmailConfirmationRequired(error: unknown): boolean {
  const record = asRecord(error);
  if (!record) return false;

  // This is the status used by the current TwinBid backend contract.
  if (record.status === 403) return true;

  const code = typeof record.code === "string" ? record.code.toUpperCase() : "";
  if (EMAIL_CONFIRMATION_CODES.has(code)) return true;

  const message = typeof record.message === "string" ? record.message.toLowerCase() : "";
  if (!message) return false;

  return [
    /(?:email|e-mail|mail).{0,30}(?:not|isn't|is not|un).{0,15}(?:confirm|verif)/,
    /user.{0,20}(?:not|isn't|is not|un).{0,15}(?:confirm|verif)/,
    /(?:confirm|verif).{0,20}(?:email|e-mail|mail)/,
    /(?:почт|email).{0,30}не.{0,15}подтвержд/,
    /подтверд.{0,20}(?:почт|email)/,
    /(?:correo|e-mail).{0,30}no.{0,15}confirm/,
    /confirm.{0,20}(?:correo|e-mail)/,
  ].some((pattern) => pattern.test(message));
}
