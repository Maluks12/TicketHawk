'use client';

import { useEffect, useState } from 'react';
import { api } from '../lib/api';
import { useToast } from '../lib/toast';

// ─────────────────────────────────────────────────────────────────────────────
// Capacity — inline editor for events.max_capacity.
//
// This is the number that decides whether an event can sell anything at all.
// ComputeAvailableTickets clamps tier availability to
// (max_capacity - tickets_sold), so once those are equal the event reports
// is_sold_out and the apps stop offering tickets — regardless of how much room
// the tiers have. Adding tiers to a sold-out event therefore appears to do
// nothing, because tiers subdivide this number rather than adding to it.
//
// Raising it is how an organiser reopens sales, which is why this is editable
// on a LIVE event and applies immediately. Lowering is allowed too, but never
// below what has already been sold — the server enforces that floor and this
// mirrors it so the error arrives before the request does.
// ─────────────────────────────────────────────────────────────────────────────

interface Props {
  eventId: string;
  status: string;
  value: number | null | undefined;
  /** Tickets already sold — the floor a new capacity cannot go under. */
  ticketsSold: number;
  onChanged: () => void;
}

const EDITABLE = new Set(['draft', 'published', 'published_private']);

export default function EventCapacityRow({ eventId, status, value, ticketsSold, onChanged }: Props) {
  const toast = useToast();

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value == null ? '' : String(value));
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setDraft(value == null ? '' : String(value));
    setEditing(false);
  }, [value]);

  const canEdit = EDITABLE.has(status);
  const atCapacity = value != null && ticketsSold >= value;

  const save = async () => {
    const raw = draft.trim();
    const parsed = raw === '' ? null : Number(raw);

    if (parsed !== null && (!Number.isInteger(parsed) || parsed < 1)) {
      toast('Enter a whole number of 1 or more, or leave it blank for unlimited', 'error');
      return;
    }
    if (parsed !== null && parsed < ticketsSold) {
      toast(`You've already sold ${ticketsSold} — capacity can't go below that`, 'error');
      return;
    }

    setBusy(true);
    try {
      await api.patch(`/organiser/events/${eventId}/capacity`, { max_capacity: parsed });
      toast('Capacity updated', 'success');
      setEditing(false);
      onChanged();
    } catch (e: any) {
      toast(e?.response?.data?.error ?? 'Could not update capacity', 'error');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div style={{
      display: 'flex', gap: 16, padding: '7px 0',
      borderBottom: '1px solid var(--surface-2)', fontSize: 13.5,
    }}>
      <div style={{ width: 180, flexShrink: 0, color: 'var(--text-2)' }}>Capacity</div>

      <div style={{ flex: 1, color: 'var(--text)' }}>
        {!editing ? (
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
            <span>{value ? value.toLocaleString() : 'Unlimited'}</span>
            {canEdit && (
              <button className="btn btn-sm" onClick={() => setEditing(true)} disabled={busy}>
                Edit
              </button>
            )}
            {atCapacity && (
              // Without this the organiser sees a full bar and no explanation of
              // why newly added tiers changed nothing.
              <span style={{ fontSize: 11.5, color: 'var(--danger)', fontWeight: 600 }}>
                Sold out — raise this to sell more
              </span>
            )}
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <input
                type="number"
                min={Math.max(1, ticketsSold)}
                step={1}
                value={draft}
                placeholder="Unlimited"
                autoFocus
                onChange={(e) => setDraft(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') save();
                  if (e.key === 'Escape') setEditing(false);
                }}
                style={{
                  width: 130, padding: '7px 9px', border: '1px solid var(--border)',
                  borderRadius: 6, fontSize: 13, fontFamily: 'inherit',
                }}
              />
              <button className="btn btn-primary btn-sm" onClick={save} disabled={busy}>
                {busy ? 'Saving…' : 'Save'}
              </button>
              <button className="btn btn-sm" onClick={() => setEditing(false)} disabled={busy}>
                Cancel
              </button>
            </div>
            <div style={{ fontSize: 11.5, color: 'var(--text-3)' }}>
              {ticketsSold > 0
                ? `Minimum ${ticketsSold} — you can't go below what's sold. `
                : 'Blank for unlimited. '}
              Tiers share this total rather than adding to it, so raise it before adding tiers to a full event.
            </div>
          </div>
        )}
      </div>
    </div>
  );
}