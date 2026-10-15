// ── Event content: information block, line-up, preview ────────────────────────
import { api, authFetch } from './api';

// ── Information ──────────────────────────────────────────────────────────────

export type InfoValueType = 'text' | 'enum' | 'time';

export interface InfoCatalogueEntry {
  key: string;
  label: string;
  hint?: string;
  value_type: InfoValueType;
  enum_options?: string[];
  display_order: number;
  /**
   * When true, this field cannot be changed on a LIVE event without admin
   * review. Today only age_requirement.
   *
   * Served rather than hardcoded so the form explains WHY a field is locked
   * instead of just disabling it — and so admin turning the flag on or off
   * takes effect without a dashboard deploy.
   */
  review_required_when_published: boolean;
  is_active: boolean;
}

export interface InfoItem {
  key: string;
  label: string;
  value: string;
  value_type: InfoValueType;
  sort_order: number;
}

export async function getInfoCatalogue(): Promise<InfoCatalogueEntry[]> {
  const { data } = await api.get<{ catalogue: InfoCatalogueEntry[] }>(
    '/organiser/event-info/catalogue',
  );
  return data.catalogue ?? [];
}

export async function getEventInfo(eventId: string): Promise<{
  items: InfoItem[];
  is_published: boolean;
  status: string;
}> {
  const { data } = await api.get(`/organiser/events/${eventId}/info`);
  return {
    items: data.items ?? [],
    is_published: !!data.is_published,
    status: data.status ?? 'draft',
  };
}

/**
 * Replaces the whole block.
 *
 * A field with an EMPTY value is dropped server-side rather than rejected —
 * that is what makes an unchosen field have no row at all, which is in turn
 * what stops the event page rendering a heading with nothing under it. So the
 * form can send everything it holds and let the server decide.
 *
 * Throws with `code: 'review_required'` (HTTP 409) when the event is live and
 * a review-locked field was changed OR removed. That is not a validation
 * failure — the organiser is entitled to the change, just not through this
 * door — so the UI should point at the edit-request flow rather than showing a
 * red field.
 */
export async function replaceEventInfo(
  eventId: string,
  items: { key: string; value: string }[],
): Promise<{ items: InfoItem[] }> {
  const { data } = await api.put(`/organiser/events/${eventId}/info`, { items });
  return data;
}

// ── Line-up ──────────────────────────────────────────────────────────────────

export type LineupKind = 'performer' | 'team';

export interface LineupEntry {
  id?: string;
  kind: LineupKind;
  name: string;
  image_url?: string;
  subtitle?: string;
  stage?: string;
  /** Full RFC3339 timestamp, not a clock time — festivals cross midnight. */
  set_time?: string;
  external_url?: string;
  sort_order: number;
}

export async function getEventLineup(eventId: string): Promise<{
  entries: LineupEntry[];
  is_published: boolean;
  status: string;
}> {
  const { data } = await api.get(`/organiser/events/${eventId}/lineup`);
  return {
    entries: data.entries ?? [],
    is_published: !!data.is_published,
    status: data.status ?? 'draft',
  };
}

/**
 * Replaces the whole line-up. Applies immediately even on a live event.
 *
 * That is deliberate — an act drops out three days before a festival and the
 * replacement is announced the same afternoon, so a line-up waiting in an
 * admin queue would be wrong on exactly the days most people are looking at
 * it.
 *
 * It is not free, though: a change to a published event's line-up notifies
 * every ticket holder and follower within a few minutes. The form warns before
 * saving so nobody sends 4 000 pushes while experimenting.
 */
export async function replaceEventLineup(
  eventId: string,
  entries: Omit<LineupEntry, 'id'>[],
): Promise<{ entries: LineupEntry[] }> {
  const { data } = await api.put(`/organiser/events/${eventId}/lineup`, { entries });
  return data;
}

/**
 * Uploads a line-up image and returns its URL.
 *
 * Reuses the generic organiser upload rather than a dedicated endpoint: it
 * already enforces the MIME allow-list, the size ceiling and compression, and
 * now writes a card thumbnail alongside the full image.
 */
export async function uploadLineupImage(file: File): Promise<string> {
  // ── Raw fetch, not the axios instance ───────────────────────────────────
  //
  // Two things were wrong with the axios version and both produced the same
  // symptom — an upload that appeared to do nothing:
  //
  //  1. The response field is `image_url`, not `url`. Reading `data.url` gave
  //     undefined, so the image was uploaded successfully and then thrown away
  //     without an error anywhere.
  //
  //  2. Passing `Content-Type: undefined` to axios does not reliably remove
  //     the header — depending on the adapter it can send an empty
  //     Content-Type, and multipart without a boundary is rejected by the
  //     server. The browser must be left to generate the boundary itself.
  //
  // This mirrors uploadImage() in the create page, which has always worked.
  const fd = new FormData();
  fd.append('image', file);

  // authFetch = fetch + the interceptor's 401 → refresh → retry.
  const res = await authFetch('/organiser/upload', { method: 'POST', body: fd });
  const json = await res.json().catch(() => null) as { image_url?: string; error?: string } | null;

  if (!res.ok) throw new Error(json?.error ?? `Upload failed (HTTP ${res.status})`);
  if (!json?.image_url) throw new Error('Upload succeeded but no URL was returned');
  return json.image_url;
}