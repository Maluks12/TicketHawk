import { create } from 'zustand';
import type { Event, Category } from '../lib/api';

interface EventStore {
  selectedEvent: Event | null;
  setSelectedEvent: (event: Event) => void;
  allEvents: Event[];
  setAllEvents: (events: Event[]) => void;
  dbCategories: Category[];
  setDbCategories: (cats: Category[]) => void;
}

export const useEventStore = create<EventStore>((set) => ({
  selectedEvent:    null,
  setSelectedEvent: (event) => set({ selectedEvent: event }),
  allEvents:        [],
  setAllEvents:     (events) => set({ allEvents: events }),
  dbCategories:     [],
  setDbCategories:  (cats) => set({ dbCategories: cats }),
}));