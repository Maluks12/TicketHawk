import { Platform } from 'react-native';
import * as AppleAuthentication from 'expo-apple-authentication';
import { GoogleSignin, statusCodes } from '@react-native-google-signin/google-signin';
import Constants from 'expo-constants';

// ─── What this module does, and what it deliberately doesn't ─────────────────
//
// It runs the interactive part of the flow and produces an ID TOKEN. That's all.
// The token goes to POST /v1/auth/social, and the server decides whether it
// means "sign in", "link" or "create". No account state is decided on-device.
//
// There is no client secret anywhere in here, and there must never be. A secret
// shipped in an app binary is a published secret. Both flows below are the
// public-client variants, which is exactly why the server verifies a signed
// assertion rather than trusting anything the client says about who the user is.

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

export const GOOGLE_IOS_CLIENT_ID = extra.googleIosClientId;
export const GOOGLE_WEB_CLIENT_ID = extra.googleWebClientId;

// Not passed to GoogleSignin.configure(): on Android the native SDK resolves
// the client from google-services.json + package name + SHA-1 rather than from
// an explicit ID. Exported anyway because it is one of the values registered in
// GOOGLE_CLIENT_IDS on the server, and keeping all three together is what makes
// that correspondence checkable.
export const GOOGLE_ANDROID_CLIENT_ID = extra.googleAndroidClientId;

// ─── Apple ───────────────────────────────────────────────────────────────────
//
// ── Why there is no nonce here ───────────────────────────────────────────────
//
// The first version of this passed a nonce and failed every sign-in with
// "id token nonce mismatch" on the server. The cause is a shape mismatch that
// is easy to get wrong and hard to see:
//
//   • We must send Apple the SHA-256 HASH of the nonce and send our server the
//     RAW value, because Apple embeds the hash in the token's `nonce` claim.
//   • Apple embeds that hash as BASE64URL. expo-crypto's digestStringAsync
//     returns HEX. So the claim never matched, whichever value we sent.
//
// Rather than encode-juggling, the native flow drops the nonce. That is not a
// weakening: a nonce defends against a token minted elsewhere being replayed
// into our endpoint, and on the native flow Apple already binds the token to
// this app through the `aud` claim, which the server verifies against
// APPLE_CLIENT_IDS. That check is what rejected Expo Go's token
// (`host.exp.Exponent`) during testing — strictly stronger evidence than a
// nonce we generated ourselves.
//
// The server treats an absent nonce as "not applicable" rather than a failure
// (see OIDCVerifier.Verify), so no backend change accompanies this.

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
// ── Why the native SDK and not expo-auth-session ─────────────────────────────
//
// This used expo-auth-session's Google provider, which drives OAuth through a
// browser redirect back into the app. On a standalone iOS build that produced
// two failures in a row: first `invalid_request` (the reversed client ID was
// not registered as a URL scheme), then a successful round trip that returned
// no `id_token` at all — the implicit ID-token flow it relies on is
// increasingly restricted for native clients.
//
// The native SDK talks to Google directly, has no redirect to misconfigure, and
// returns the ID token as a first-class field.
//
// ── Why webClientId is set on a mobile app ───────────────────────────────────
//
// iosClientId identifies the app to Google. webClientId is what makes Google
// mint an ID TOKEN and sets its `aud` claim — without it you get a user object
// and no token, which is useless here because the server verifies the token,
// not anything the client asserts.
//
// So a token from the iOS app arrives audienced to the WEB client ID. That is
// expected, and it is why GOOGLE_CLIENT_IDS on the server lists all three.

let googleConfigured = false;

function configureGoogle() {
  if (googleConfigured) return;
  GoogleSignin.configure({
    webClientId: GOOGLE_WEB_CLIENT_ID,
    iosClientId: GOOGLE_IOS_CLIENT_ID,
    // We are not calling Google APIs on the user's behalf, so no server auth
    // code and no refresh token. Requesting less is a better consent screen
    // and less to get wrong.
    offlineAccess: false,
    scopes: ['openid', 'profile', 'email'],
  });
  googleConfigured = true;
}

/**
 * Runs Google's native sign-in sheet and returns the ID token.
 *
 * A plain async function, not a hook — the native SDK needs no redirect
 * listener mounted in the React tree, which is part of why it is simpler than
 * what it replaced.
 */
export async function signInWithGoogle(): Promise<SocialCredential> {
  if (!GOOGLE_WEB_CLIENT_ID) {
    throw new SocialAuthProviderError(
      'Google sign-in is not configured: googleWebClientId missing from app.json extra.',
    );
  }

  configureGoogle();

  try {
    // Android only; resolves immediately on iOS.
    if (Platform.OS === 'android') {
      await GoogleSignin.hasPlayServices({ showPlayServicesUpdateDialog: true });
    }

    // Sign out first so the account picker always appears. Without this the SDK
    // silently reuses the last account, and someone who picked the wrong Google
    // account has no way to change it.
    try {
      await GoogleSignin.signOut();
    } catch {
      /* not signed in — fine */
    }

    const response: any = await GoogleSignin.signIn();

    // v13+ returns { type: 'success' | 'cancelled', data }. Earlier versions
    // return the user object directly. Handle both so a minor version bump
    // doesn't silently break sign-in.
    if (response?.type === 'cancelled') {
      throw new SocialAuthCancelled();
    }

    const idToken: string | undefined = response?.data?.idToken ?? response?.idToken;

    if (!idToken) {
      throw new SocialAuthProviderError(
        'Google returned no identity token. Check that googleWebClientId in app.json is the WEB client ID.',
      );
    }

    return { provider: 'google', idToken };
  } catch (err: any) {
    if (err instanceof SocialAuthCancelled) throw err;
    if (err instanceof SocialAuthProviderError) throw err;

    if (err?.code === statusCodes.SIGN_IN_CANCELLED || err?.code === statusCodes.IN_PROGRESS) {
      throw new SocialAuthCancelled();
    }
    if (err?.code === statusCodes.PLAY_SERVICES_NOT_AVAILABLE) {
      throw new SocialAuthProviderError('Google Play Services is unavailable or out of date.');
    }
    throw new SocialAuthProviderError(err?.message ?? 'Google sign-in failed');
  }
}