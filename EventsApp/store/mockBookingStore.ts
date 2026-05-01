import { create } from 'zustand';
import type { Event } from '../lib/api';

export interface MockBooking {
  id: string;
  booking_number: string;
  event_id: string;
  ticket_quantity: number;
  total_amount: number;
  currency: string;
  booking_status: 'confirmed';
  payment_status: 'completed';
  qr_code_url: null;
  created_at: string;
  confirmed_at: string;
  ticket_type_name: string;
  event: {
    title: string;
    start_time: string;
    venue_name: string;
    city: string;
    cover_image_url?: string | null;
  };
}

interface MockBookingState {
  bookings: MockBooking[];
  addBooking: (event: Event) => MockBooking;
  clearBookings: () => void;
}

function randomBookingNumber() {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';
  let out = 'BK-';
  for (let i = 0; i < 6; i++) out += chars[Math.floor(Math.random() * chars.length)];
  return out;
}

export const useMockBookingStore = create<MockBookingState>((set, get) => ({
  bookings: [],

  addBooking: (event: Event) => {
    const now = new Date().toISOString();
    const booking: MockBooking = {
      id:               Math.random().toString(36).slice(2),
      booking_number:   randomBookingNumber(),
      event_id:         event.id,
      ticket_quantity:  1,
      total_amount:     event.price ?? 0,
      currency:         event.currency ?? 'ZAR',
      booking_status:   'confirmed',
      payment_status:   'completed',
      qr_code_url:      null,
      created_at:       now,
      confirmed_at:     now,
      ticket_type_name: 'GENERAL ADMISSION',
      event: {
        title:           event.title,
        start_time:      event.start_time,
        venue_name:      event.venue_name,
        city:            event.city,
        cover_image_url: event.cover_image_url ?? null,
      },
    };
    set({ bookings: [booking, ...get().bookings] });
    return booking;
  },

  clearBookings: () => set({ bookings: [] }),
}));