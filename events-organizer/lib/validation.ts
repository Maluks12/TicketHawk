// ── Field validators ──────────────────────────────────────────────────────────
// Each returns an error string or null if valid.

export function validateEmail(email: string): string | null {
  if (!email.trim()) return 'Email is required';
  const re = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  if (!re.test(email)) return 'Enter a valid email address';
  return null;
}

export function validatePassword(password: string): string | null {
  if (!password) return 'Password is required';
  if (password.length < 8) return 'Password must be at least 8 characters';
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