// ─────────────────────────────────────────────────────────────────────────────
// Event staffing — API layer
//
// Two sides of the same feature:
//   * Management (owner / manager / admin): list, add, and revoke staff for an
//     event; create and revoke join codes.
//   * Staffer: redeem a join code, list the events they can scan. Staffers are
//     ordinary accounts — their scanning power comes from event_staff
//     membership on the backend, not from a global role.
// ─────────────────────────────────────────────────────────────────────────────
import { api } from './api';

// ── Types ─────────────────────────────────────────────────────────────────────

export interface StaffMember {
  user_id: string;
  first_name: string;
  last_name: string;
  email: string;
  role: 'scanner' | 'manager';
  status: 'active' | 'revoked';
  added_by: string; // email of the granter (empty when unknown)
  via_code: boolean;
  created_at: string;
  revoked_at?: string;
}

export interface StaffCode {
  id: string;
  code: string; // display form XXXX-XXXX
  grants_role: 'scanner' | 'manager';
  max_uses: number;
  use_count: number;
  expires_at: string;
  revoked_at?: string;
  created_at: string;
}

export interface ScannableEvent {
  id: string;
  title: string;
  start_time: string;
  end_time: string;
  venue_name?: string;
  city?: string;
  role: 'owner' | 'manager' | 'scanner';
}

// ── Management (owner / manager / admin) ─────────────────────────────────────

export const listEventStaff = (eventId: string) =>
  api.get<{ staff: StaffMember[] }>(`/v1/events/${eventId}/staff`);

export const addEventStaff = (eventId: string, email: string, role: 'scanner' | 'manager' = 'scanner') =>
  api.post<{ message: string; member: StaffMember }>(`/v1/events/${eventId}/staff`, { email, role });

export const revokeEventStaff = (eventId: string, userId: string) =>
  api.delete<{ message: string }>(`/v1/events/${eventId}/staff/${userId}`);

export const createStaffCode = (
  eventId: string,
  opts: { role?: 'scanner' | 'manager'; max_uses?: number } = {},
) => api.post<{ code: StaffCode }>(`/v1/events/${eventId}/staff/codes`, opts);

export const listStaffCodes = (eventId: string) =>
  api.get<{ codes: StaffCode[] }>(`/v1/events/${eventId}/staff/codes`);

export const revokeStaffCode = (eventId: string, codeId: string) =>
  api.delete<{ message: string }>(`/v1/events/${eventId}/staff/codes/${codeId}`);

// ── Staffer side ──────────────────────────────────────────────────────────────

export const redeemStaffCode = (code: string) =>
  api.post<{ message: string; event: ScannableEvent }>('/v1/staff/redeem', { code });

export const listMyScanEvents = () =>
  api.get<{ events: ScannableEvent[] }>('/v1/staff/events');