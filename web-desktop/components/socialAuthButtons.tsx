'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, saveTokens, getMe } from '@/lib/api';
import { useAuthStore } from '@/store/authStore';

// ─── Web social sign-in ──────────────────────────────────────────────────────
//
// Both providers work the same way here: their JS SDK runs the interactive
// flow in a popup and hands back an ID token, which goes to POST
// /v1/auth/social. The server does the verifying. No secret is present on this
// page, and none may be added — anything in a client bundle is public.
//
// ── Why the SDKs are loaded lazily ───────────────────────────────────────────
//
// Google Identity Services and Apple's AppleID JS are third-party scripts on
// third-party domains. Putting them in the root layout means every page load —
// including the event pages that carry the SEO — blocks on two external
// requests that only the auth page needs. They are injected on mount here
// instead, so a script outage degrades the sign-in buttons rather than the
// whole site.

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

/** Random nonce, hex. The raw value goes to our API; Apple gets it too (it
 *  hashes internally on web, unlike the native SDK where WE must hash). */
function makeNonce(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return Array.from(bytes).map((b) => b.toString(16).padStart(2, '0')).join('');
}

interface Props {
  /**
   * Called after a successful sign-in. The login page passes its own
   * goAfterAuth, so social and password sign-in land in exactly the same
   * place — including honouring the ?next= that the booking deep link sets.
   */
  onAuthed?: () => void;
  dividerLabel?: string;
  isDark?: boolean;
}

export default function SocialAuthButtons({ onAuthed, dividerLabel = 'or continue with', isDark = true }: Props) {
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

  const handleError = useCallback((err: any) => {
    const status = err?.response?.status;
    const msg = err?.response?.data?.error;
    if (status === 403 && msg) setError(msg);
    else if (status === 501) setError('That sign-in method isn\u2019t available right now.');
    else if (!err?.response) setError('Check your connection and try again.');
    else setError(msg ?? 'Sign-in failed. Please try again.');
  }, []);

  // ── Google Identity Services ───────────────────────────────────────────────
  useEffect(() => {
    if (!GOOGLE_CLIENT_ID) return;
    let cancelled = false;

    loadScript('https://accounts.google.com/gsi/client', 'gsi-client')
      .then(() => {
        if (cancelled || !window.google?.accounts?.id) return;
        nonceRef.current = makeNonce();
        window.google.accounts.id.initialize({
          client_id: GOOGLE_CLIENT_ID,
          nonce: nonceRef.current,
          // We want an ID token, not a code — there is no server-side exchange.
          callback: async (resp: { credential?: string }) => {
            if (!resp?.credential) return;
            setBusy('google');
            setError(null);
            try {
              await exchange({ provider: 'google', id_token: resp.credential, nonce: nonceRef.current });
            } catch (err) {
              handleError(err);
            } finally {
              setBusy(null);
            }
          },
        });
        setGoogleReady(true);
      })
      .catch(() => setGoogleReady(false));

    return () => { cancelled = true; };
  }, [exchange, handleError]);

  // Renders Google's own button into the div below. Using their rendered button
  // rather than a custom one keeps us inside their branding terms and gets One
  // Tap behaviour for free.
  const googleButtonRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!googleReady || !googleButtonRef.current || !window.google?.accounts?.id) return;
    window.google.accounts.id.renderButton(googleButtonRef.current, {
      theme: isDark ? 'filled_black' : 'outline',
      size: 'large',
      width: 320,
      text: 'continue_with',
      shape: 'pill',
      logo_alignment: 'center',
    });
  }, [googleReady, isDark]);

  // ── Apple JS ───────────────────────────────────────────────────────────────
  useEffect(() => {
    if (!APPLE_CLIENT_ID) return;
    let cancelled = false;

    loadScript(
      'https://appleid.cdn-apple.com/appleauth/static/jsapi/appleid/1/en_US/appleid.auth.js',
      'appleid-auth',
    )
      .then(() => {
        if (cancelled || !window.AppleID?.auth) return;
        window.AppleID.auth.init({
          clientId: APPLE_CLIENT_ID, // Services ID, NOT the bundle ID
          scope: 'name email',
          redirectURI: `${window.location.origin}/auth/login`,
          usePopup: true,
        });
        setAppleReady(true);
      })
      .catch(() => setAppleReady(false));

    return () => { cancelled = true; };
  }, []);

  const signInWithApple = useCallback(async () => {
    if (!window.AppleID?.auth) return;
    setBusy('apple');
    setError(null);
    try {
      const res = await window.AppleID.auth.signIn();
      const idToken = res?.authorization?.id_token;
      if (!idToken) throw new Error('no id token');
      // Apple returns the name only on FIRST authorisation, and on web it
      // arrives in `res.user`, not in the token. Forward it or it is lost.
      await exchange({
        provider: 'apple',
        id_token: idToken,
        first_name: res?.user?.name?.firstName,
        last_name: res?.user?.name?.lastName,
      });
    } catch (err: any) {
      // popup_closed_by_user is a cancel, not a failure.
      if (err?.error === 'popup_closed_by_user' || err?.error === 'user_cancelled_authorize') {
        setBusy(null);
        return;
      }
      handleError(err);
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
            }}
          >
            <svg width="16" height="19" viewBox="0 0 17 20" fill="currentColor" aria-hidden="true">
              <path d="M14.1 10.6c0-2.3 1.9-3.4 2-3.5-1.1-1.6-2.8-1.8-3.4-1.8-1.4-.1-2.8.9-3.5.9s-1.8-.9-3-.8c-1.5 0-2.9.9-3.7 2.3-1.6 2.7-.4 6.8 1.1 9 .8 1.1 1.7 2.3 2.9 2.2 1.2 0 1.6-.7 3-.7s1.8.7 3 .7c1.3 0 2.1-1.1 2.8-2.2.9-1.2 1.3-2.5 1.3-2.5s-2.5-1-2.5-3.6zM11.9 3.8c.6-.8 1.1-1.9 1-3-1 0-2.1.7-2.8 1.5-.6.7-1.2 1.8-1 2.9 1.1.1 2.2-.6 2.8-1.4z" />
            </svg>
            {busy === 'apple' ? 'Signing in…' : 'Continue with Apple'}
          </button>
        )}

        {/* Google renders its own button into this node. */}
        <div ref={googleButtonRef} style={{ minHeight: googleReady ? 44 : 0 }} />
      </div>
    </div>
  );
}