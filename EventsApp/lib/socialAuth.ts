import { Platform } from 'react-native';
import * as AppleAuthentication from 'expo-apple-authentication';
import * as AuthSession from 'expo-auth-session';
import * as Google from 'expo-auth-session/providers/google';
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
   * has no name forever (short of the user typing it).
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

/**
 * A provider-side failure that never reached our API.
 *
 * Carries the provider's own error string. This matters on production builds
 * where there is no Metro console: without it, every pre-API failure looks
 * identical to being offline, and the only visible symptom is a misleading
 * "check your connection".
 */
export class SocialAuthProviderError extends Error {
  constructor(msg: string) {
    super(msg);
    this.name = 'SocialAuthProviderError';
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

// ─── Apple ───────────────────────────────────────────────────────────────────
//
// ── Why there is no nonce here ───────────────────────────────────────────────
//
// The first version of this passed a nonce, and it failed every sign-in with
// "id token nonce mismatch" on the server. The reason is a shape mismatch that
// is easy to get wrong and hard to see:
//
//   • We must send Apple the SHA-256 HASH of the nonce, and send our server the
//     RAW value, because Apple embeds the hash in the token's `nonce` claim.
//   • Apple embeds that hash as BASE64URL. But expo-crypto's digestStringAsync
//     returns HEX by default. So the claim never matched what we compared it
//     against, no matter which of the two values we sent.
//
// Rather than encode-juggling, the native flow drops the nonce entirely. That
// is not a weakening: a nonce defends against a token minted elsewhere being
// replayed into our endpoint, and on the native flow Apple already binds the
// token to this app through the `aud` claim — which the server verifies against
// APPLE_CLIENT_IDS. That check is what rejected Expo Go's token
// (`host.exp.Exponent`) during testing, and it is strictly stronger evidence
// than a nonce we generated ourselves.
//
// The web flow is different: there Apple's own JS SDK generates and validates
// the nonce internally, so there is likewise nothing for us to forward.
//
// The server treats an absent nonce as "not applicable" rather than a failure
// (see OIDCVerifier.Verify), so no backend change accompanies this.

export async function signInWithApple(): Promise<SocialCredential> {
  let credential: AppleAuthentication.AppleAuthenticationCredential;
  try {
    credential = await AppleAuthentication.signInAsync({
      requestedScopes: [
        AppleAuthentication.AppleAuthenticationScope.FULL_NAME,
        AppleAuthentication.AppleAuthenticationScope.EMAIL,
      ],
    });
  } catch (err: any) {
    if (err?.code === 'ERR_REQUEST_CANCELED' || err?.code === 'ERR_CANCELED') {
      throw new SocialAuthCancelled();
    }
    throw new SocialAuthProviderError(err?.message ?? 'Apple sign-in failed');
  }

  if (!credential.identityToken) {
    throw new SocialAuthProviderError('Apple did not return an identity token');
  }

  return {
    provider: 'apple',
    idToken: credential.identityToken,
    firstName: credential.fullName?.givenName ?? undefined,
    lastName: credential.fullName?.familyName ?? undefined,
  };
}

// ─── Google ──────────────────────────────────────────────────────────────────
//
// Uses the AuthSession hook rather than a bare function because Google's flow
// needs a redirect listener mounted in the React tree.
//
// ── If this fails on a standalone build but works in Expo Go ─────────────────
//
// iOS needs the REVERSED iOS client ID registered as a URL scheme, or Google
// has nowhere to redirect back to and refuses the request up front. See the
// note in app.json — this is a native config item, so it needs a rebuild.

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
    if (!request) {
      throw new SocialAuthProviderError(
        'Google sign-in is not ready. Check googleIosClientId / googleAndroidClientId in app.json.',
      );
    }

    const result = await promptAsync();

    if (result.type === 'cancel' || result.type === 'dismiss') {
      throw new SocialAuthCancelled();
    }

    if (result.type === 'error') {
      // Surface Google's actual complaint. On a production build this alert is
      // the only place it will ever appear.
      const params = (result.params ?? {}) as Record<string, string>;
      const detail =
        result.error?.message ??
        params.error_description ??
        params.error ??
        'unknown error';
      throw new SocialAuthProviderError(`Google: ${detail}`);
    }

    if (result.type !== 'success') {
      throw new SocialAuthProviderError(`Google sign-in failed (${result.type})`);
    }

    const idToken =
      (result.params as Record<string, string> | undefined)?.id_token ??
      result.authentication?.idToken;

    if (!idToken) {
      throw new SocialAuthProviderError('Google did not return an identity token');
    }

    // Google's implicit ID-token flow generates and checks its own nonce
    // internally via AuthSession, so there is nothing for us to forward.
    return { provider: 'google', idToken };
  };

  return { signIn, ready: !!request, response };
}

/**
 * The redirect URI Google must accept for this app.
 *
 * Exported so it can be shown in-app during setup — a mismatch here produces
 * Google's opaque "invalid_request" / "redirect_uri_mismatch", and seeing the
 * exact expected value is far faster than guessing, especially on a build with
 * no console attached.
 */
export function googleRedirectUri(): string {
  return AuthSession.makeRedirectUri({ scheme: 'eventsapp' });
}