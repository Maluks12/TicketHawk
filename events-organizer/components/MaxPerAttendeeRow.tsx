'use client';

import { useEffect, useState } from 'react';
import { api } from '../lib/api';
import { useToast } from '../lib/toast';

// ─────────────────────────────────────────────────────────────────────────────
// Max per attendee — inline editor for events.max_tickets_per_user.
//
// This row used to be a plain <Row>, so the only way to change the number was
// to reopen the create wizard as an editor, which on a live event files a
// review request for the whole form rather than the one field that changed.
//
// Deliberately its own component rather than a prop on <Row>: Row is a
// hookless presentational helper at module scope in the detail page, and this
// needs state. Its markup is copied so the two rows stay pixel-identical —
// if Row's styling changes, this changes with it.
//
// The 1–10 bound is the CHECK on the column (migration 000043), not a UI
// preference. Blank means no limit, which is NULL, not 0.
// ─────────────────────────────────────────────────────────────────────────────

interface Props {
  eventId: string;
  /** Event status — decides whether the change applies now or goes to review. */
  status: string;
  value: number | null | undefined;
  /** Re-fetch the event so the parent's copy matches what the server now holds. */
  onChanged: () => void;
}

const MIN = 1;
const MAX = 10;

const LIVE = new Set(['published', 'published_private']);
const EDITABLE = new Set(['draft', 'published', 'published_private']);

export default function MaxPerAttendeeRow({ eventId, status, value, onChanged }: Props) {
  const toast = useToast();

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value == null ? '' : String(value));
  const [busy, setBusy] = useState(false);

  // Close the editor whenever the parent hands down a fresh value, so a
  // successful save cannot leave a stale input open over the updated number.
  useEffect(() => {
    setDraft(value == null ? '' : String(value));
    setEditing(false);
  }, [value]);

  const live = LIVE.has(status);
  const canEdit = EDITABLE.has(status);

  const save = async () => {
    const raw = draft.trim();
    const parsed = raw === '' ? null : Number(raw);

    if (parsed !== null && (!Number.isInteger(parsed) || parsed < MIN || parsed > MAX)) {
      toast(`Enter a whole number between ${MIN} and ${MAX}, or leave it blank for no limit`, 'error');
      return;
    }

    setBusy(true);
    try {
      const res = await api.patch(`/organiser/events/${eventId}/ticket-limit`, {
        max_tickets_per_user: parsed,
      });
      // 202 means the event is live and the change is now an admin's to
      // approve — saying "updated" here would be a lie the organiser only
      // finds out about when the public page still shows the old number.
      if (res.status === 202) {
        toast(res.data?.message ?? 'Sent for review', 'success');
      } else {
        toast('Max per attendee updated', 'success');
      }
      setEditing(false);
      onChanged();
    } catch (e: any) {
      toast(e?.response?.data?.error ?? 'Could not update the limit', 'error');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div style={{
      display: 'flex', gap: 16, padding: '7px 0',
      borderBottom: '1px solid var(--surface-2)', fontSize: 13.5,
    }}>
      <div style={{ width: 180, flexShrink: 0, color: 'var(--text-2)' }}>Max per attendee</div>

      <div style={{ flex: 1, color: 'var(--text)' }}>
        {!editing ? (
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <span>{value ? String(value) : 'No limit'}</span>
            {canEdit && (
              <button
                className="btn btn-sm"
                onClick={() => setEditing(true)}
                disabled={busy}
              >
                Edit
              </button>
            )}
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <input
                type="number"
                min={MIN}
                max={MAX}
                step={1}
                value={draft}
                placeholder="No limit"
                autoFocus
                onChange={(e) => setDraft(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') save();
                  if (e.key === 'Escape') setEditing(false);
                }}
                style={{
                  width: 110, padding: '7px 9px', border: '1px solid var(--border)',
                  borderRadius: 6, fontSize: 13, fontFamily: 'inherit',
                }}
              />
              <button className="btn btn-primary btn-sm" onClick={save} disabled={busy}>
                {busy ? 'Saving…' : live ? 'Send for review' : 'Save'}
              </button>
              <button className="btn btn-sm" onClick={() => setEditing(false)} disabled={busy}>
                Cancel
              </button>
            </div>
            <div style={{ fontSize: 11.5, color: 'var(--text-3)' }}>
              {MIN}–{MAX} tickets per checkout, or blank for no limit.
              {live && ' This event is live, so the change goes to an admin for review — it stays on sale meanwhile.'}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}