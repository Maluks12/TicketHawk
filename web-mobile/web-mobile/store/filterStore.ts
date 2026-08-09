import { create } from 'zustand';

interface FilterDisplayStore {
  locationLabel:    string;
  dateLabelText:    string;
  filterOpen:       boolean;
  setLocationLabel: (label: string) => void;
  setDateLabelText: (label: string) => void;
  setFilterOpen:    (open: boolean) => void;
}

export const useFilterStore = create<FilterDisplayStore>((set) => ({
  locationLabel:    'Anywhere',
  dateLabelText:    'All dates',
  filterOpen:       false,
  setLocationLabel: (locationLabel) => set({ locationLabel }),
  setDateLabelText: (dateLabelText) => set({ dateLabelText }),
  setFilterOpen:    (filterOpen)    => set({ filterOpen }),
}));