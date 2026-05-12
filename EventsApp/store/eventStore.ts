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

// Stored shape -- ids (ordered, most-recent-first) + a cache of full event
// objects so the home screen can render them instantly without an API call.
interface StoredRecentlyViewed {
  ids:   string[];
  cache: Record<string, Event>;
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
  // The cache holds the full Event shape so no round-trip is needed on load.
  recentlyViewedIds:   string[];
  recentlyViewedCache: Record<string, Event>;
  loadRecentlyViewed:  (userId: string) => Promise<void>;
  addRecentlyViewed:   (event: Event,  userId: string) => Promise<void>;
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

  // Recently Viewed
  recentlyViewedIds:   [],
  recentlyViewedCache: {},

  loadRecentlyViewed: async (userId: string) => {
    try {
      const raw = await SecureStore.getItemAsync(recentlyViewedKey(userId));
      if (!raw) {
        set({ recentlyViewedIds: [], recentlyViewedCache: {} });
        return;
      }
      const stored: StoredRecentlyViewed = JSON.parse(raw);
      set({
        recentlyViewedIds:   stored.ids   ?? [],
        recentlyViewedCache: stored.cache ?? {},
      });
    } catch {
      set({ recentlyViewedIds: [], recentlyViewedCache: {} });
    }
  },

  addRecentlyViewed: async (event: Event, userId: string) => {
    const { recentlyViewedIds: prev, recentlyViewedCache: prevCache } = get();

    // Move to front, deduplicate, cap at max
    const ids = [event.id, ...prev.filter((id) => id !== event.id)].slice(
      0,
      RECENTLY_VIEWED_MAX,
    );

    // Update cache -- trim stale entries that are no longer in the ID list
    const idSet = new Set(ids);
    const cache: Record<string, Event> = { ...prevCache };
    for (const key of Object.keys(cache)) {
      if (!idSet.has(key)) delete cache[key];
    }
    cache[event.id] = event;

    set({ recentlyViewedIds: ids, recentlyViewedCache: cache });

    try {
      const stored: StoredRecentlyViewed = { ids, cache };
      await SecureStore.setItemAsync(recentlyViewedKey(userId), JSON.stringify(stored));
    } catch { /* non-critical -- in-memory state already updated */ }
  },

  clearRecentlyViewed: () => set({ recentlyViewedIds: [], recentlyViewedCache: {} }),
}));