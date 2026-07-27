// ─────────────────────────────────────────────────────────────────────────────
// Where to send someone after they log in or register.
//
// ── The bug this replaces ────────────────────────────────────────────────────
//
// The login page used to call router.back() on success. That is right in the
// common case — you were on an event page, hit "Log in", and want to land back
// on the event — but it is only ever "go to the previous history entry", and it
// has no idea what that entry is.
//
// After a password reset the previous entry is the reset page:
//
//     /reset-password?token=…  →  set password  →  push /auth/login
//     →  log in  →  router.back()  →  /reset-password
//
// So the user finished resetting their password, signed in, and was dropped
// straight back onto "Set your password" with a spent token.
//
// It fails in the other direction too. Open the reset link from an email in a
// fresh tab and there IS no previous entry, so router.back() does nothing and
// the user sits on the login page wondering whether it worked.
//
// ── The approach ─────────────────────────────────────────────────────────────
//
// Record the last page the user was on that is safe to return to, and go there.
// The recording is done centrally by components/returnPath.tsx, so the ten-odd
// call sites that push to /auth/login need no changes.
//
// Auth-flow routes are never recorded, which is what makes the loop impossible
// rather than merely unlikely: there is no code path that can store
// /reset-password as a destination.
// ─────────────────────────────────────────────────────────────────────────────

const KEY = 'th:returnPath';

/** Routes that must never be a post-login destination. */
const AUTH_ROUTES: RegExp[] = [
  /^\/auth(\/|$)/,          // /auth/login
  /^\/reset-password(\/|$)/, // password reset, incl. ?token=
  /^\/claim-ticket(\/|$)/,   // its own token flow; re-entering it post-login is wrong
];

/**
 * Validates AND normalises a candidate return path.
 *
 * Returns the canonical path to navigate to, or null if it is not safe.
 *
 * ── Why this parses instead of matching prefixes ─────────────────────────────
 *
 * The first version of this checked `startsWith('//')`, `startsWith('/\\')` and
 * so on. That is not sufficient, and the gap is not theoretical:
 *
 *     "/\t/evil.com"      -> starts with "/", is not "//", passes a prefix check
 *                          -> the URL parser STRIPS tab, LF and CR while parsing
 *                          -> becomes "//evil.com"
 *                          -> resolves to https://evil.com
 *
 * Whitespace and control characters are removed by the parser, so any check that
 * runs on the raw string is looking at something the browser will never see. The
 * only reliable oracle is the same parser the browser navigates with, so this
 * resolves the candidate against our own origin and requires the result to stay
 * there.
 *
 * It also returns the PARSED form rather than the caller's string, so whatever
 * reaches router.replace() is already canonical and cannot be re-interpreted a
 * second time by a different resolver.
 *
 * Why any of this matters: the destination can come from a ?next= parameter, so
 * it is attacker-influenceable. An open redirect on a login page is a standard
 * phishing primitive — the victim is sent a genuine ticket-hawk.com login link
 * and is bounced somewhere else immediately after authenticating.
 */
export function sanitizeReturnPath(path: string | null | undefined): string | null {
  if (!path || typeof path !== 'string') return null;

  // Cheap pre-filter. Anything not starting with "/" is either absolute
  // ("https://evil.com"), scheme-relative, or a non-navigational scheme
  // ("javascript:", "data:") — none of which is ever a valid return path.
  if (!path.startsWith('/')) return null;

  // Fixed fallback so the function is still callable during SSR, where the
  // origin only matters for relative-vs-absolute resolution.
  const origin =
    typeof window !== 'undefined' && window.location?.origin
      ? window.location.origin
      : 'https://ticket-hawk.com';

  // Resolve to a FIXPOINT, checking the origin on every pass.
  //
  // One pass is not enough, and this is not hypothetical either — it was found
  // by fuzzing the single-pass version:
  //
  //     "/..//evil.com"
  //       pass 1 -> origin https://ticket-hawk.com  (looks safe)
  //                 pathname "//evil.com"           (is not)
  //
  // The `..` is consumed by path normalisation and leaves a protocol-relative
  // path behind. Returning that would hand router.replace() a string which,
  // when IT resolves the value, points at evil.com. The value we emit gets
  // parsed again downstream, so it has to be safe under re-parsing, not merely
  // safe to have parsed.
  //
  // Looping until the output stops changing means the caller receives a value
  // that is a fixpoint of the parser: resolving it again cannot change it.
  let current = path;
  let url: URL;

  for (let pass = 0; pass < 5; pass++) {
    try {
      url = new URL(current, origin);
    } catch {
      return null;
    }

    if (url.origin !== origin) return null;

    const next = `${url.pathname}${url.search}${url.hash}`;
    if (next === current) break;
    current = next;
  }

  // Bail rather than trust a value still moving after 5 passes.
  try {
    const final = new URL(current, origin);
    if (final.origin !== origin) return null;
    if (`${final.pathname}${final.search}${final.hash}` !== current) return null;
    if (AUTH_ROUTES.some((re) => re.test(final.pathname))) return null;
  } catch {
    return null;
  }

  return current;
}

/** Boolean form of {@link sanitizeReturnPath}, for guard-style call sites. */
export function isSafeReturnPath(path: string | null | undefined): path is string {
  return sanitizeReturnPath(path) !== null;
}

/** Records a path as the place to return to. No-ops on unsafe paths. */
export function rememberReturnPath(path: string): void {
  if (typeof window === 'undefined') return;
  const safe = sanitizeReturnPath(path);
  if (!safe) return;
  try {
    window.sessionStorage.setItem(KEY, safe);
  } catch {
    // Safari private mode throws on sessionStorage writes. Losing the return
    // path is a minor inconvenience; throwing here would break login.
  }
}

/**
 * Resolves the destination after a successful login or registration.
 *
 * Order: an explicit ?next= (so a caller can be deliberate), then the last safe
 * page visited, then home. sessionStorage rather than localStorage — a return
 * path is meaningful for this tab and this session only.
 */
export function resolveReturnPath(nextParam?: string | null): string {
  // Note both branches return the SANITISED value, never the caller's string.
  const fromParam = sanitizeReturnPath(nextParam);
  if (fromParam) return fromParam;

  if (typeof window !== 'undefined') {
    try {
      const stored = sanitizeReturnPath(window.sessionStorage.getItem(KEY));
      if (stored) return stored;
    } catch {
      /* see above */
    }
  }
  return '/';
}

/** Called once the return path has been consumed, so it can't fire twice. */
export function clearReturnPath(): void {
  if (typeof window === 'undefined') return;
  try {
    window.sessionStorage.removeItem(KEY);
  } catch {
    /* see above */
  }
}