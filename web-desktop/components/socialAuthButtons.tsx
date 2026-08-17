'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, saveTokens, getMe } from '@/lib/api';
import { useAuthStore } from '@/store/authStore';

// ─── Web social sign-in ──────────────────────────────────────────────────────
//
// Both providers work the same way: their JS SDK runs the interactive flow in a
// popup and hands back an ID token, which goes to POST /v1/auth/social. The
// server does the verifying. No secret is present on this page, and none may be
// added — anything in a client bundle is public.
//
// ── Why the SDKs are loaded lazily ───────────────────────────────────────────
//
// Google Identity Services and Apple's AppleID JS are third-party scripts on
// third-party domains. Putting them in the root layout means every page load —
// including the event pages carrying the SEO — blocks on two external requests
// that only this page needs. They are injected on mount here instead, so a
// script outage degrades the sign-in buttons rather than the whole site.

declare global {
  interface Window {
    google?: any;
    AppleID?: any;
  }
}

const GOOGLE_CLIENT_ID = process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID ?? '';
const APPLE_CLIENT_ID  = process.env.NEXT_PUBLIC_APPLE_SERVICES_ID ?? '';

function loadScript(src: string, id: string): Promise<void> {
  return new Promise((resolve, reject) => {
    if (typeof document === 'undefined') return reject(new Error('no document'));
    const existing = document.getElementById(id) as HTMLScriptElement | null;
    if (existing) {
      if (existing.dataset.loaded === 'true') return resolve();
      existing.addEventListener('load', () => resolve());
      existing.addEventListener('error', () => reject(new Error(`failed to load ${src}`)));
      return;
    }
    const el = document.createElement('script');
    el.id = id;
    el.src = src;
    el.async = true;
    el.defer = true;
    el.onload = () => { el.dataset.loaded = 'true'; resolve(); };
    el.onerror = () => reject(new Error(`failed to load ${src}`));
    document.head.appendChild(el);
  });
}

/** Random nonce, hex. Google's GIS hashes it internally and compares. */
function makeNonce(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return Array.from(bytes).map((b) => b.toString(16).padStart(2, '0')).join('');
}

interface Props {
  /**
   * Called after a successful sign-in. The login page passes its own
   * goAfterAuth, so social and password sign-in land in exactly the same place
   * — including honouring the ?next= that the booking deep link sets.
   */
  onAuthed?: () => void;
  dividerLabel?: string;
  isDark?: boolean;
}

export default function SocialAuthButtons({
  onAuthed,
  dividerLabel = 'or continue with',
  isDark = true,
}: Props) {
  const router = useRouter();
  const applyUser = useAuthStore((s) => s.applyUser);

  const [googleReady, setGoogleReady] = useState(false);
  const [appleReady, setAppleReady] = useState(false);
  const [busy, setBusy] = useState<null | 'google' | 'apple'>(null);
  const [error, setError] = useState<string | null>(null);
  const nonceRef = useRef<string>('');

  // ── Exchange an ID token for a session ─────────────────────────────────────
  const exchange = useCallback(async (body: {
    provider: 'google' | 'apple';
    id_token: string;
    nonce?: string;
    first_name?: string;
    last_name?: string;
  }) => {
    const { data } = await api.post('/v1/auth/social', body);
    saveTokens(data.access_token, data.refresh_token);
    const { data: user } = await getMe();
    applyUser(user);
    if (onAuthed) onAuthed();
    else router.back();
  }, [applyUser, onAuthed, router]);

  // ── Error surfacing ────────────────────────────────────────────────────────
  //
  // Every failure is logged to the console as well as shown. The first version
  // only set state, which meant a provider-side config error produced a
  // misleading "check your connection" on screen and NOTHING in devtools — the
  // one place a developer would look. An unexplained failure with an empty
  // console is worse than a noisy one.
  //
  // Provider SDK rejections don't look like axios errors: Apple rejects with
  // { error: 'popup_closed_by_user' }, Google with { type, ... }. Neither has a
  // `.response`, so collapsing "no .response" into a network message hid them
  // both.
  const handleError = useCallback((where: string, err: any) => {
    // eslint-disable-next-line no-console
    console.error(`[social-auth] ${where} failed`, err);

    const status = err?.response?.status;
    const serverMsg = err?.response?.data?.error;

    if (status === 403 && serverMsg) {
      setError(serverMsg);                       // deleted account
    } else if (status === 501) {
      setError('That sign-in method isn\u2019t available right now.');
    } else if (status) {
      setError(serverMsg ?? 'Sign-in failed. Please try again.');
    } else if (err?.error) {
      // Provider SDK rejection — surface the provider's own code.
      setError(`${where}: ${err.error}`);
    } else if (err?.message) {
      setError(err.message);
    } else {
      setError('Sign-in failed. Please try again.');
    }
  }, []);

  // ── Google Identity Services ───────────────────────────────────────────────
  useEffect(() => {
    if (!GOOGLE_CLIENT_ID) {
      // eslint-disable-next-line no-console
      console.error('[social-auth] NEXT_PUBLIC_GOOGLE_CLIENT_ID is not set. ' +
        'NEXT_PUBLIC_* is inlined at BUILD time — setting it after `next build` has no effect.');
      return;
    }
    let cancelled = false;

    loadScript('https://accounts.google.com/gsi/client', 'gsi-client')
      .then(() => {
        if (cancelled || !window.google?.accounts?.id) return;
        nonceRef.current = makeNonce();
        window.google.accounts.id.initialize({
          client_id: GOOGLE_CLIENT_ID,
          nonce: nonceRef.current,
          callback: async (resp: { credential?: string }) => {
            if (!resp?.credential) {
              handleError('Google', new Error('Google returned no credential'));
              return;
            }
            setBusy('google');
            setError(null);
            try {
              await exchange({
                provider: 'google',
                id_token: resp.credential,
                nonce: nonceRef.current,
              });
            } catch (err) {
              handleError('Google', err);
            } finally {
              setBusy(null);
            }
          },
        });
        setGoogleReady(true);
      })
      .catch((err) => {
        handleError('Google script', err);
        setGoogleReady(false);
      });

    return () => { cancelled = true; };
  }, [exchange, handleError]);

  // Google renders its own button into this node. Using their rendered button
  // rather than a custom one keeps us inside their branding terms — which also
  // means it CANNOT be freely restyled to match the Apple button exactly.
  // `shape: 'pill'` and a matching max width get it as close as their terms
  // allow; the wrapper below constrains it so both buttons share a width and
  // centre line instead of the Google one sitting at its own fixed 320px.
  const googleButtonRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!googleReady || !googleButtonRef.current || !window.google?.accounts?.id) return;
    window.google.accounts.id.renderButton(googleButtonRef.current, {
      theme: isDark ? 'filled_black' : 'outline',
      size: 'large',
      // Google caps this at 400 and ignores percentages — a number is required.
      width: 320,
      text: 'continue_with',
      shape: 'pill',
      logo_alignment: 'center',
    });
  }, [googleReady, isDark]);

  // ── Apple JS ───────────────────────────────────────────────────────────────
  //
  // Deliberately NOT hidden on non-Apple devices. Sign in with Apple on the web
  // is a popup to appleid.apple.com and works in any browser, and an account
  // created through Apple on an iPhone has password IS NULL — so hiding this
  // button on a Windows laptop would leave that user with no way into their
  // account at all: email/password cannot work, and "forgot password" on an
  // account that never had one is a dead end. The button stays.
  useEffect(() => {
    if (!APPLE_CLIENT_ID) {
      // eslint-disable-next-line no-console
      console.error('[social-auth] NEXT_PUBLIC_APPLE_SERVICES_ID is not set.');
      return;
    }
    let cancelled = false;

    loadScript(
      'https://appleid.cdn-apple.com/appleauth/static/jsapi/appleid/1/en_US/appleid.auth.js',
      'appleid-auth',
    )
      .then(() => {
        if (cancelled || !window.AppleID?.auth) return;
        window.AppleID.auth.init({
          clientId: APPLE_CLIENT_ID,           // Services ID, NOT the bundle ID
          scope: 'name email',
          // Must EXACTLY match a Return URL registered on the Services ID. If
          // the user is on www. and only the apex is registered (or vice
          // versa), Apple rejects with invalid_client before showing anything.
          redirectURI: `${window.location.origin}/auth/login`,
          usePopup: true,
        });
        setAppleReady(true);
      })
      .catch((err) => {
        handleError('Apple script', err);
        setAppleReady(false);
      });

    return () => { cancelled = true; };
  }, [handleError]);

  const signInWithApple = useCallback(async () => {
    if (!window.AppleID?.auth) {
      handleError('Apple', new Error('Apple sign-in is still loading. Try again in a moment.'));
      return;
    }
    setBusy('apple');
    setError(null);
    try {
      const res = await window.AppleID.auth.signIn();
      const idToken = res?.authorization?.id_token;
      if (!idToken) throw new Error('Apple returned no identity token');
      // Apple returns the name only on FIRST authorisation, and on web it
      // arrives in res.user rather than in the token. Forward it or it is lost
      // for good.
      await exchange({
        provider: 'apple',
        id_token: idToken,
        first_name: res?.user?.name?.firstName,
        last_name: res?.user?.name?.lastName,
      });
    } catch (err: any) {
      // A cancel is a normal outcome, not an error.
      if (err?.error === 'popup_closed_by_user' || err?.error === 'user_cancelled_authorize') {
        setBusy(null);
        return;
      }
      handleError('Apple', err);
    } finally {
      setBusy(null);
    }
  }, [exchange, handleError]);

  if (!GOOGLE_CLIENT_ID && !APPLE_CLIENT_ID) return null;

  const subtle = isDark ? 'rgba(255,255,255,0.4)' : 'rgba(0,0,0,0.4)';
  const line   = isDark ? 'rgba(255,255,255,0.07)' : 'rgba(0,0,0,0.07)';

  return (
    <div style={{ width: '100%' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, margin: '20px 0' }}>
        <div style={{ flex: 1, height: 1, background: line }} />
        <span style={{ color: subtle, fontSize: 13 }}>{dividerLabel}</span>
        <div style={{ flex: 1, height: 1, background: line }} />
      </div>

      {error && (
        <div role="alert" style={{ color: '#ff6b6b', fontSize: 13, marginBottom: 12, textAlign: 'center' }}>
          {error}
        </div>
      )}

      {/* Both buttons share one column with a single max width, so they line up
          regardless of Google rendering its own artwork at a fixed pixel size. */}
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 10 }}>
        {appleReady && (
          <button
            type="button"
            onClick={signInWithApple}
            disabled={!!busy}
            style={{
              width: '100%', maxWidth: 320, height: 44, borderRadius: 22, border: 'none',
              background: isDark ? '#ffffff' : '#000000',
              color: isDark ? '#000000' : '#ffffff',
              fontSize: 15, fontWeight: 600, cursor: busy ? 'default' : 'pointer',
              display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8,
              opacity: busy ? 0.6 : 1,
              boxSizing: 'border-box',
            }}
          >
            <svg width="16" height="19" viewBox="0 0 17 20" fill="currentColor" aria-hidden="true">
              <path d="M14.1 10.6c0-2.3 1.9-3.4 2-3.5-1.1-1.6-2.8-1.8-3.4-1.8-1.4-.1-2.8.9-3.5.9s-1.8-.9-3-.8c-1.5 0-2.9.9-3.7 2.3-1.6 2.7-.4 6.8 1.1 9 .8 1.1 1.7 2.3 2.9 2.2 1.2 0 1.6-.7 3-.7s1.8.7 3 .7c1.3 0 2.1-1.1 2.8-2.2.9-1.2 1.3-2.5 1.3-2.5s-2.5-1-2.5-3.6zM11.9 3.8c.6-.8 1.1-1.9 1-3-1 0-2.1.7-2.8 1.5-.6.7-1.2 1.8-1 2.9 1.1.1 2.2-.6 2.8-1.4z" />
            </svg>
            {busy === 'apple' ? 'Signing in…' : 'Continue with Apple'}
          </button>
        )}

        {/* Google renders into this node. Constrained to the same max width as
            the Apple button above so the two align. */}
        <div
          ref={googleButtonRef}
          style={{
            width: '100%',
            maxWidth: 320,
            minHeight: googleReady ? 44 : 0,
            display: 'flex',
            justifyContent: 'center',
          }}
        />
      </div>
    </div>
  );
}