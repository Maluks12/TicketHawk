import { create } from 'zustand';
import type { Event, Category } from '../lib/api';

export type SectionMeta = {
  title: string;
  sectionId?: string;
  filterKey?: string;
  events?: Event[];
};

const RECENTLY_VIEWED_MAX = 10;
const rvKey = (userId: string) => `recently_viewed_${userId}`;

interface EventStore {
  selectedEvent: Event | null;
  setSelectedEvent: (event: Event) => void;
  allEvents: Event[];
  setAllEvents: (events: Event[]) => void;
  dbCategories: Category[];
  setDbCategories: (cats: Category[]) => void;
  sectionMeta: SectionMeta | null;
  setSectionMeta: (meta: SectionMeta | null) => void;
  recentlyViewedIds: string[];
  recentlyViewedCache: Record<string, Event>;
  loadRecentlyViewed: (userId: string) => void;
  addRecentlyViewed: (event: Event, userId: string) => void;
  clearRecentlyViewed: () => void;
}

export const useEventStore = create<EventStore>((set, get) => ({
  selectedEvent: null,
  setSelectedEvent: (event) => set({ selectedEvent: event }),
  allEvents: [],
  setAllEvents: (events) => set({ allEvents: events }),
  dbCategories: [],
  setDbCategories: (cats) => set({ dbCategories: cats }),
  sectionMeta: null,
  setSectionMeta: (meta) => set({ sectionMeta: meta }),

  recentlyViewedIds: [],
  recentlyViewedCache: {},

  loadRecentlyViewed: (userId) => {
    if (typeof window === 'undefined') return;
    try {
      const raw = localStorage.getItem(rvKey(userId));
      if (!raw) { set({ recentlyViewedIds: [], recentlyViewedCache: {} }); return; }
      const ids: string[] = JSON.parse(raw);
      set({ recentlyViewedIds: ids });
    } catch {
      set({ recentlyViewedIds: [], recentlyViewedCache: {} });
    }
  },

  addRecentlyViewed: (event, userId) => {
    if (typeof window === 'undefined') return;
    const { recentlyViewedIds, recentlyViewedCache } = get();
    const filtered = recentlyViewedIds.filter((id) => id !== event.id);
    const updated = [event.id, ...filtered].slice(0, RECENTLY_VIEWED_MAX);
    const updatedCache = { ...recentlyViewedCache, [event.id]: event };
    set({ recentlyViewedIds: updated, recentlyViewedCache: updatedCache });
    localStorage.setItem(rvKey(userId), JSON.stringify(updated));
  },

  clearRecentlyViewed: () => {
    set({ recentlyViewedIds: [], recentlyViewedCache: {} });
  },
}));