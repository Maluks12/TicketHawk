import { create } from 'zustand';
import * as SecureStore from 'expo-secure-store';
import type { Event, Category } from '../lib/api';

export type SectionMeta = {
  title: string;
  sectionId?: string;   // API section id (trending, happening_soon, cat-*, etc.)
  filterKey?: string;   // editorial filter key (free, premium, music, etc.)
  events?: Event[];     // fallback events when no fetch strategy available
};

const RECENTLY_VIEWED_MAX = 10;

function recentlyViewedKey(userId: string) {
  return `recently_viewed_${userId}`;
}

interface EventStore {
  selectedEvent: Event | null;
  setSelectedEvent: (event: Event) => void;
  allEvents: Event[];
  setAllEvents: (events: Event[]) => void;
  dbCategories: Category[];
  setDbCategories: (cats: Category[]) => void;
  sectionMeta: SectionMeta | null;
  setSectionMeta: (meta: SectionMeta | null) => void;
  // Recently viewed -- logged-in users only, persisted per user ID.
  // Only IDs are written to SecureStore (stays well under the 2048-byte limit).
  // The full Event objects live in recentlyViewedCache (in-memory only) and are
  // rebuilt as the user opens events; on cold start the home screen falls back
  // to resolving ids against allEvents.
  recentlyViewedIds:   string[];
  recentlyViewedCache: Record<string, Event>;
  loadRecentlyViewed:  (userId: string) => Promise<void>;
  addRecentlyViewed:   (event: Event, userId: string) => Promise<void>;
  clearRecentlyViewed: () => void;
}

export const useEventStore = create<EventStore>((set, get) => ({
  selectedEvent:    null,
  setSelectedEvent: (event) => set({ selectedEvent: event }),
  allEvents:        [],
  setAllEvents:     (events) => set({ allEvents: events }),
  dbCategories:     [],
  setDbCategories:  (cats) => set({ dbCategories: cats }),
  sectionMeta:      null,
  setSectionMeta:   (meta) => set({ sectionMeta: meta }),

  // ── Recently Viewed ────────────────────────────────────────────────────────
  recentlyViewedIds:   [],
  recentlyViewedCache: {},

  loadRecentlyViewed: async (userId: string) => {
    try {
      const raw = await SecureStore.getItemAsync(recentlyViewedKey(userId));
      if (!raw) {
        set({ recentlyViewedIds: [], recentlyViewedCache: {} });
        return;
      }
      const parsed = JSON.parse(raw);
      // Handle old format { ids, cache } and new format (plain string[])
      const ids: string[] = Array.isArray(parsed)
        ? parsed
        : Array.isArray(parsed?.ids) ? parsed.ids : [];
      // Cache starts empty on load -- rebuilt as events are opened
      set({ recentlyViewedIds: ids, recentlyViewedCache: {} });
    } catch {
      set({ recentlyViewedIds: [], recentlyViewedCache: {} });
    }
  },

  addRecentlyViewed: async (event: Event, userId: string) => {
    const { recentlyViewedIds: prev, recentlyViewedCache: prevCache } = get();

    // Safety guard -- prev must be an array (handles corrupt/migrated state)
    const safeIds = Array.isArray(prev) ? prev : [];

    // Move to front, deduplicate, cap at max
    const ids = [event.id, ...safeIds.filter((id) => id !== event.id)].slice(
      0,
      RECENTLY_VIEWED_MAX,
    );

    // Keep in-memory cache up to date -- trim stale entries
    const idSet = new Set(ids);
    const cache: Record<string, Event> = { ...prevCache };
    for (const key of Object.keys(cache)) {
      if (!idSet.has(key)) delete cache[key];
    }
    cache[event.id] = event;

    set({ recentlyViewedIds: ids, recentlyViewedCache: cache });

    try {
      // Persist only the IDs -- well under the 2048-byte SecureStore limit
      await SecureStore.setItemAsync(recentlyViewedKey(userId), JSON.stringify(ids));
    } catch { /* non-critical -- in-memory state already updated */ }
  },

  clearRecentlyViewed: () => set({ recentlyViewedIds: [], recentlyViewedCache: {} }),
}));