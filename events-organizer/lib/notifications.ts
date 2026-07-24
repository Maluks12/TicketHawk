// ── Notifications client ──────────────────────────────────────────────────────
// The organiser dashboard doesn't have a full notification centre; it only
// needs to surface the handful of account-level events an organiser must not
// miss. Right now that's exactly one: 'kyc_approved', emitted once by
// PromoteOrganizerAfterKYC when full verification passes (auto registry match,
// admin approval, or admin registry re-run). Rendered as a banner on Overview
// and dismissed by marking the notification read, so it doesn't come back on
// the next login.
import { api } from './api';

export interface Notification {
  id: string;
  user_id: string;
  type: string;
  title: string;
  body: string;
  data?: Record<string, unknown> | null;
  is_read: boolean;
  created_at: string;
}

/** GET /notifications — newest first. Never throws; the dashboard is not
 *  allowed to fail to render because a banner couldn't load. */
export async function getNotifications(limit = 20): Promise<Notification[]> {
  try {
    const { data } = await api.get('/notifications', { params: { limit } });
    return data.data ?? [];
  } catch {
    return [];
  }
}

/** PATCH /notifications/:id/read */
export async function markRead(id: string): Promise<void> {
  try {
    await api.patch(`/notifications/${id}/read`);
  } catch {
    // Best effort — the banner is dismissed locally either way.
  }
}

export const NOTIF_KYC_APPROVED = 'kyc_approved';