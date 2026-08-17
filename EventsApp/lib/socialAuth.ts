import { Platform } from 'react-native';
import * as AppleAuthentication from 'expo-apple-authentication';
import * as AuthSession from 'expo-auth-session';
import * as Google from 'expo-auth-session/providers/google';
import * as Crypto from 'expo-crypto';
import * as WebBrowser from 'expo-web-browser';
import Constants from 'expo-constants';

// Required by expo-auth-session so the in-app browser closes itself when the
// provider redirects back. Without it the browser sheet hangs open after a
// successful Google sign-in and the user has to dismiss it by hand.
WebBrowser.maybeCompleteAuthSession();

// ─── What this module does, and what it deliberately doesn't ─────────────────
//
// It runs the interactive part of the flow and produces an ID TOKEN. That's all.
// The token goes to POST /v1/auth/social, and the server decides whether it
// means "sign in", "link" or "create". No account state is decided on-device.
//
// There is no client secret anywhere in here, and there must never be. A secret
// shipped in an app binary is a published secret. The flows below are the
// public-client variants (PKCE for Google, native attestation for Apple),
// which is exactly why the server verifies a signed assertion rather than
// trusting anything the client asserts about who the user is.

export type SocialProvider = 'google' | 'apple';

export interface SocialCredential {
  provider: SocialProvider;
  idToken: string;
  nonce?: string;
  /**
   * Apple returns the display name exactly ONCE — in the authorization
   * response on first authorisation, never in the ID token and never again on
   * subsequent sign-ins. If it isn't captured and forwarded here, the account
   * has no name forever (short of the user typing it). This is the single most
   * common Sign in with Apple implementation bug.
   */
  firstName?: string;
  lastName?: string;
}

export class SocialAuthCancelled extends Error {
  constructor() {
    super('cancelled');
    this.name = 'SocialAuthCancelled';
  }
}

// ─── Client IDs ──────────────────────────────────────────────────────────────
// Read from app config (extra) so they can differ per build profile. These are
// public identifiers, not secrets — safe to ship, and they must match the
// GOOGLE_CLIENT_IDS the API accepts as audiences.
const extra = (Constants.expoConfig?.extra ?? {}) as Record<string, string | undefined>;

export const GOOGLE_IOS_CLIENT_ID     = extra.googleIosClientId;
export const GOOGLE_ANDROID_CLIENT_ID = extra.googleAndroidClientId;
export const GOOGLE_WEB_CLIENT_ID     = extra.googleWebClientId;

/**
 * Whether Sign in with Apple can be offered.
 *
 * iOS 13+ only. Android and older iOS get the Google button alone, which is
 * fine — App Store Guideline 4.8 governs iOS, and on iOS the button is always
 * present.
 */
export async function isAppleAvailable(): Promise<boolean> {
  if (Platform.OS !== 'ios') return false;
  try {
    return await AppleAuthentication.isAvailableAsync();
  } catch {
    return false;
  }
}

/**
 * Generates a nonce pair for the flows that need one.
 *
 * The RAW value goes to our server (it compares against the token's `nonce`
 * claim); the SHA-256 HASH goes to Apple. Apple hashes what you give it and
 * puts the hash in the token, so sending the raw value to both would never
 * match. Getting this backwards is the second most common Sign in with Apple
 * bug and it fails with an unhelpful "nonce mismatch".
 */
async function makeNonce(): Promise<{ raw: string; hashed: string }> {
  const bytes = await Crypto.getRandomBytesAsync(32);
  const raw = Array.from(bytes).map((b) => b.toString(16).padStart(2, '0')).join('');
  const hashed = await Crypto.digestStringAsync(Crypto.CryptoDigestAlgorithm.SHA256, raw);
  return { raw, hashed };
}

// ─── Apple ───────────────────────────────────────────────────────────────────

export async function signInWithApple(): Promise<SocialCredential> {
  const { raw, hashed } = await makeNonce();

  let credential: AppleAuthentication.AppleAuthenticationCredential;
  try {
    credential = await AppleAuthentication.signInAsync({
      requestedScopes: [
        AppleAuthentication.AppleAuthenticationScope.FULL_NAME,
        AppleAuthentication.AppleAuthenticationScope.EMAIL,
      ],
      nonce: hashed,
    });
  } catch (err: any) {
    if (err?.code === 'ERR_REQUEST_CANCELED' || err?.code === 'ERR_CANCELED') {
      throw new SocialAuthCancelled();
    }
    throw err;
  }

  if (!credential.identityToken) {
    throw new Error('Apple did not return an identity token');
  }

  return {
    provider: 'apple',
    idToken: credential.identityToken,
    nonce: raw,
    firstName: credential.fullName?.givenName ?? undefined,
    lastName: credential.fullName?.familyName ?? undefined,
  };
}

// ─── Google ──────────────────────────────────────────────────────────────────
//
// Uses the AuthSession hook rather than a bare function because Google's flow
// needs a redirect listener mounted in the React tree. `useGoogleSignIn` is
// consumed by the auth screen; the promise resolves when the browser returns.

export function useGoogleSignIn() {
  const [request, response, promptAsync] = Google.useIdTokenAuthRequest({
    iosClientId: GOOGLE_IOS_CLIENT_ID,
    androidClientId: GOOGLE_ANDROID_CLIENT_ID,
    webClientId: GOOGLE_WEB_CLIENT_ID,
    // Ask for an ID token directly — we never need an access token, because
    // we are not calling Google APIs on the user's behalf. Requesting less is
    // both a better consent screen and less to get wrong.
    scopes: ['openid', 'profile', 'email'],
  });

  const signIn = async (): Promise<SocialCredential> => {
    const result = await promptAsync();

    if (result.type === 'cancel' || result.type === 'dismiss') {
      throw new SocialAuthCancelled();
    }
    if (result.type !== 'success') {
      throw new Error('Google sign-in failed');
    }

    const idToken =
      (result.params as Record<string, string> | undefined)?.id_token ??
      result.authentication?.idToken;

    if (!idToken) {
      throw new Error('Google did not return an identity token');
    }

    // Google's implicit ID-token flow generates and checks its own nonce
    // internally via AuthSession, so there is nothing for us to forward. The
    // server treats an absent nonce as "not applicable" rather than a failure.
    return { provider: 'google', idToken };
  };

  return { signIn, ready: !!request, response };
}

/**
 * The redirect URI Google must have registered for this app.
 *
 * Exported so it can be logged during setup — a mismatch here produces
 * Google's opaque "redirect_uri_mismatch" error, and printing the exact
 * expected value is far faster than guessing at the console.
 */
export function googleRedirectUri(): string {
  return AuthSession.makeRedirectUri({ scheme: 'eventsapp' });
}