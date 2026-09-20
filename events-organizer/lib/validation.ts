// ── Field validators ──────────────────────────────────────────────────────────
// Each returns an error string or null if valid.

export function validateEmail(email: string): string | null {
  if (!email.trim()) return 'Email is required';
  const re = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  if (!re.test(email)) return 'Enter a valid email address';
  return null;
}

// ── Password policy ───────────────────────────────────────────────────────────
// Mirrors the backend's `strong_password` binding tag verbatim
// (events-db/internal/utils/security.go → ValidatePassword): at least 8 runes,
// one uppercase, one digit, one special character, and at most 128 runes.
//
// This used to check length only, which meant "password" passed the form and
// then came back as a 400 from /auth/register that the user could do nothing
// with. A client check that is more lenient than the server is worse than no
// check at all — it turns a fixable form error into a dead end.

export const PASSWORD_MAX = 128;

export interface PasswordCheck {
  label: string;
  ok: boolean;
}

/** Counts code points, matching Go's len([]rune(pw)) rather than UTF-16 units. */
function runeLength(value: string): number {
  return [...value].length;
}

/**
 * The live requirement list rendered under the password field at signup.
 * validatePassword() below is built from the same array, so the ticks the user
 * sees can never drift from what the form actually rejects.
 *
 * "Special" is anything that is not a letter, a digit or whitespace. The server
 * tests unicode.IsPunct || unicode.IsSymbol, which is the same set in practice
 * and — the part that matters — does not count a plain space.
 */
export function passwordChecks(password: string): PasswordCheck[] {
  return [
    { label: 'At least 8 characters',              ok: runeLength(password) >= 8 },
    { label: 'One capital letter (A\u2013Z)',      ok: /[A-Z]/.test(password) },
    { label: 'One number (0\u20139)',              ok: /[0-9]/.test(password) },
    { label: 'One special character (! ? @ # $)',  ok: /[^A-Za-z0-9\s]/.test(password) },
  ];
}

export function validatePassword(password: string): string | null {
  if (!password) return 'Password is required';
  if (runeLength(password) > PASSWORD_MAX) {
    return `Password must not exceed ${PASSWORD_MAX} characters`;
  }
  if (passwordChecks(password).some(c => !c.ok)) {
    return 'Password does not meet all of the requirements below';
  }
  return null;
}

export function validatePasswordMatch(password: string, confirm: string): string | null {
  if (!confirm) return 'Please confirm your password';
  if (password !== confirm) return 'Passwords do not match';
  return null;
}

export function validateRequired(value: string, label: string): string | null {
  if (!value.trim()) return `${label} is required`;
  return null;
}

export function validateMinLength(value: string, min: number, label: string): string | null {
  if (value.trim().length < min) return `${label} must be at least ${min} characters`;
  return null;
}

export function validatePhone(phone: string): string | null {
  if (!phone.trim()) return 'Phone number is required';
  const digits = phone.replace(/\D/g, '');
  if (digits.length < 9) return 'Enter a valid phone number';
  return null;
}

// ── Form-level helpers ────────────────────────────────────────────────────────

// Run a map of { fieldName: errorString | null } and return the first error,
// or null if all pass. Use this to keep submit handlers clean.
export function firstError(checks: Record<string, string | null>): string | null {
  for (const err of Object.values(checks)) {
    if (err) return err;
  }
  return null;
}