// ── Event collections (backend migration 000096) ──────────────────────────────
//
// A group of the organiser's own events — the dates of a tour — shown on the
// TicketHawk home feed as ONE card instead of one card per date. Search,
// category pages and the map still list each date individually.
//
// Routes are /organiser/event-collections, NOT /organiser/collections: that
// path is merch collections, an unrelated feature.
import { api, authFetch } from './api';

export interface EventCollection {
  id: string;
  slug: string;
  title: string;
  description?: string | null;
  cover_image_url?: string | null;
  created_at: string;
  updated_at: string;
  /** Every member, whatever its status. */
  event_count: number;
  /** Members the public can see right now (published, public, not ended).
   *  The feed shows the collection as one card only when this is 2+. */
  live_event_count: number;
  /** Member ids, soonest first. */
  event_ids: string[];
}

/** The feed folds a collection into one card from this many live dates. */
export const MIN_LIVE_FOR_CARD = 2;

export async function listEventCollections(): Promise<EventCollection[]> {
  const { data } = await api.get<{ collections: EventCollection[] }>('/organiser/event-collections');
  return data.collections ?? [];
}

export async function createEventCollection(body: {
  title: string; description?: string; event_ids: string[];
}): Promise<EventCollection> {
  const { data } = await api.post<EventCollection>('/organiser/event-collections', body);
  return data;
}

export async function updateEventCollection(id: string, patch: {
  title?: string; description?: string;
}): Promise<EventCollection> {
  const { data } = await api.patch<EventCollection>(`/organiser/event-collections/${id}`, patch);
  return data;
}

export async function setEventCollectionEvents(id: string, eventIds: string[]): Promise<EventCollection> {
  const { data } = await api.put<EventCollection>(`/organiser/event-collections/${id}/events`, {
    event_ids: eventIds,
  });
  return data;
}

export async function deleteEventCollection(id: string): Promise<void> {
  await api.delete(`/organiser/event-collections/${id}`);
}

export async function removeEventCollectionCover(id: string): Promise<void> {
  await api.delete(`/organiser/event-collections/${id}/cover`);
}

/** Multipart upload through authFetch so the browser sets the boundary AND an
 *  expired access token is refreshed (see authFetch in api.ts). */
export async function uploadEventCollectionCover(id: string, file: File): Promise<string> {
  const fd = new FormData();
  fd.append('image', file);
  const res = await authFetch(`/organiser/event-collections/${id}/cover`, { method: 'POST', body: fd });
  const json = await res.json().catch(() => null) as { cover_image_url?: string; error?: string } | null;
  if (!res.ok) throw new Error(json?.error ?? `Upload failed (HTTP ${res.status})`);
  if (!json?.cover_image_url) throw new Error('Upload succeeded but no URL was returned');
  return json.cover_image_url;
}