import { create } from 'zustand';
import type { Event } from '../lib/api';

interface EventStore {
  selectedEvent: Event | null;
  setSelectedEvent: (event: Event) => void;
}

export const useEventStore = create<EventStore>((set) => ({
  selectedEvent: null,
  setSelectedEvent: (event) => set({ selectedEvent: event }),
}));