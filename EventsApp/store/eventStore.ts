import { create } from 'zustand';
import type { Event, Category } from '../lib/api';

export type SectionMeta = {
  title: string;
  sectionId?: string;   // API section id (trending, happening_soon, cat-*, etc.)
  filterKey?: string;   // editorial filter key (free, premium, music, etc.)
  events?: Event[];     // fallback events when no fetch strategy available
};

interface EventStore {
  selectedEvent: Event | null;
  setSelectedEvent: (event: Event) => void;
  allEvents: Event[];
  setAllEvents: (events: Event[]) => void;
  dbCategories: Category[];
  setDbCategories: (cats: Category[]) => void;
  sectionMeta: SectionMeta | null;
  setSectionMeta: (meta: SectionMeta | null) => void;
}

export const useEventStore = create<EventStore>((set) => ({
  selectedEvent:    null,
  setSelectedEvent: (event) => set({ selectedEvent: event }),
  allEvents:        [],
  setAllEvents:     (events) => set({ allEvents: events }),
  dbCategories:     [],
  setDbCategories:  (cats) => set({ dbCategories: cats }),
  sectionMeta:      null,
  setSectionMeta:   (meta) => set({ sectionMeta: meta }),
}));