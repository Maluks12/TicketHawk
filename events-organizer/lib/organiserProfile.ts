// ── Organiser public page ─────────────────────────────────────────────────────
//
// The organiser-facing side of what the apps render at /organiser/<slug>.
//
// ⚠️  These endpoints are /organiser/public-profile, NOT /organiser/profile.
//
// /organiser/profile already exists and returns the ACCOUNT record — legal
// name, business email, payout state. The two are genuinely different things,
// which is why they are different paths rather than one endpoint with a mode
// flag. Registering both on the same path panics gin at startup.
import { api } from './api';

export type LinkPlatform =
  | 'instagram' | 'x' | 'facebook' | 'tiktok'
  | 'youtube' | 'linkedin' | 'whatsapp' | 'spotify' | 'website';

export interface OrganizerLink {
  platform: LinkPlatform;
  url: string;
  sort_order: number;
}

export interface PublicOrganizer {
  id: string;
  slug: string;
  business_name: string;
  description?: string;
  logo_url?: string;
  cover_image_url?: string;
  verified: boolean;
  member_since: number;
  events_hosted: number;
  follower_count: number;
  links?: OrganizerLink[];
}

/**
 * What the dashboard sees of its OWN page, including when it is not live.
 *
 * `is_live` is the server's answer, not something to re-derive here. It folds
 * together three conditions — kyc_level >= 2, the organiser's own toggle, and
 * account standing — and `blocked_reason` names the one the organiser can
 * actually act on. Recomputing that client-side would mean the dashboard and
 * the public route could disagree about whether a page is visible.
 */
export interface OwnerProfile extends PublicOrganizer {
  public_profile_enabled: boolean;
  is_live: boolean;
  blocked_reason?: 'kyc_level_required' | 'disabled_by_organiser' | 'account_suspended' | 'unavailable';
  public_url_path: string;
}

export async function getMyPublicProfile(): Promise<OwnerProfile> {
  const { data } = await api.get<OwnerProfile>('/organiser/public-profile');
  return data;
}

/**
 * Sparse patch — omit a key to leave it alone.
 *
 * Every field is optional on purpose: sending `description: ''` clears the
 * description, and that has to stay distinguishable from not sending the key
 * at all. A non-sparse update would blank whatever the form did not happen to
 * be holding.
 */
export async function updateMyPublicProfile(patch: {
  description?: string;
  cover_image_url?: string;
  public_profile_enabled?: boolean;
}): Promise<OwnerProfile> {
  const { data } = await api.patch<OwnerProfile>('/organiser/public-profile', patch);
  return data;
}

/**
 * Replaces the WHOLE link set.
 *
 * Not a per-link CRUD surface: the organiser edits them as one form and the
 * order is part of the content, so reconciling adds/edits/removes/reorders
 * across four endpoints would be more code on both sides for no benefit.
 * An empty array is valid and means "remove all my links".
 *
 * The server validates and NORMALISES: "@wildernessbeach" comes back as
 * "https://instagram.com/wildernessbeach", and a link whose host does not
 * match its platform is rejected with a message naming the platform. Show that
 * message rather than a generic one — it is the difference between an
 * organiser fixing their own typo and filing a bug.
 */
export async function replaceMyLinks(
  links: { platform: string; url: string }[],
): Promise<{ links: OrganizerLink[] }> {
  const { data } = await api.put<{ links: OrganizerLink[] }>(
    '/organiser/public-profile/links', { links },
  );
  return data;
}

export async function getLinkPlatforms(): Promise<LinkPlatform[]> {
  const { data } = await api.get<{ platforms: LinkPlatform[] }>(
    '/organiser/public-profile/link-platforms',
  );
  return data.platforms ?? [];
}

// ── Media ────────────────────────────────────────────────────────────────────
//
// These upload AND persist in one call — the response carries `saved: true`
// once the row is updated. If `saved` is false the bytes reached R2 but the
// profile row did not update, and the URL is returned anyway so the client can
// retry the PATCH without re-uploading the file.

async function uploadProfileImage(
  kind: 'cover' | 'logo',
  file: File,
): Promise<{ url: string; saved: boolean; error?: string }> {
  const form = new FormData();
  form.append('image', file);
  const { data } = await api.post<Record<string, unknown>>(
    `/organiser/public-profile/media/${kind}`,
    form,
    // Content-Type is deliberately NOT set. The browser must generate the
    // multipart boundary itself; setting it by hand produces a header with no
    // boundary and the server rejects the body.
    { headers: { 'Content-Type': undefined as unknown as string } },
  );
  return {
    url: (data.cover_image_url ?? data.logo_url ?? '') as string,
    saved: data.saved !== false,
    error: data.error as string | undefined,
  };
}

export const uploadCover = (f: File) => uploadProfileImage('cover', f);
export const uploadLogo  = (f: File) => uploadProfileImage('logo', f);

// ── Public read, for the preview ─────────────────────────────────────────────

/** Accepts a slug or a UUID. 404s when the page is not live. */
export async function getPublicOrganizer(slugOrId: string): Promise<PublicOrganizer> {
  const { data } = await api.get<PublicOrganizer>(`/organisers/${slugOrId}`);
  return data;
}