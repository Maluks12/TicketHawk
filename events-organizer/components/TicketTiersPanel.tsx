'use client';

import { useCallback, useEffect, useState } from 'react';
import { api } from '../lib/api';
import { useToast } from '../lib/toast';
import { useConfirm } from '../lib/confirm';

// ─────────────────────────────────────────────────────────────────────────────
// Ticket tier management.
//
// The backend has had a complete tier CRUD surface for some time —
//
//     POST   /organiser/events/:id/ticket-types
//     PUT    /organiser/events/:id/ticket-types
//     PATCH  /organiser/ticket-types/:typeId
//     DELETE /organiser/ticket-types/:typeId
//
// — and the dashboard called none of it. Tiers could only be set once, in the
// event creation wizard, and were frozen from then on. An organiser who typo'd a
// price, wanted to add an Early Bird, needed to raise capacity on a tier that was
// selling, or wanted to pull a tier off sale had no route to any of it except
// asking support to run SQL.
//
// The old read-only table also showed Sold and Capacity but not RESERVED, which
// hid the most confusing situation an organiser hits: a tier that looks like it
// has stock but refuses to sell, because unpaid checkouts are holding the
// remainder. That column is why "available" is shown as its own number here
// rather than left as arithmetic for the reader.
//
// ── Deliberately NOT offering ────────────────────────────────────────────────
//
// PUT (replace-all) is not wired up. It swaps every tier atomically, which is
// right for the creation wizard and wrong here — a mis-click would drop tiers
// that already have bookings attached. Individual PATCH/DELETE is slower and far
// harder to do irreversible damage with.
// ─────────────────────────────────────────────────────────────────────────────

export interface TicketTier {
  id: string;
  name: string;
  description: string | null;
  price: number;
  capacity: number | null;
  tickets_sold: number;
  reserved_count: number;
  available_tickets?: number;
  is_sold_out?: boolean;
  sort_order: number;
  is_visible: boolean;
}

interface Props {
  eventId: string;
  tiers: TicketTier[];
  currency?: string;
  /** Re-fetch the event so the parent's copy matches what the server now holds. */
  onChanged: () => void;
}

const money = (v: number, cur = 'ZAR') =>
  v > 0 ? new Intl.NumberFormat('en-ZA', { style: 'currency', currency: cur, maximumFractionDigits: 0 }).format(v) : 'Free';

/** Mirrors models.TicketType.ComputeAvailable() for tiers fetched before it ran. */
function availabilityOf(t: TicketTier): { available: number | null; soldOut: boolean } {
  if (typeof t.available_tickets === 'number') {
    const av = t.available_tickets < 0 ? null : t.available_tickets;
    return { available: av, soldOut: t.is_sold_out ?? (av !== null && av <= 0) };
  }
  if (t.capacity == null) return { available: null, soldOut: false };
  const av = Math.max(0, t.capacity - t.tickets_sold - t.reserved_count);
  return { available: av, soldOut: av <= 0 };
}

const inputStyle: React.CSSProperties = {
  width: '100%', padding: '7px 9px', border: '1px solid var(--border)',
  borderRadius: 6, fontSize: 13, fontFamily: 'inherit',
};

export default function TicketTiersPanel({ eventId, tiers, currency = 'ZAR', onChanged }: Props) {
  const toast = useToast();
  const confirm = useConfirm();

  const [editingId, setEditingId] = useState<string | null>(null);
  const [draft, setDraft] = useState<Partial<TicketTier>>({});
  const [adding, setAdding] = useState(false);
  const [newTier, setNewTier] = useState({ name: '', price: '', capacity: '', description: '' });
  const [busy, setBusy] = useState(false);

  // Close the editor whenever the parent hands us a fresh list, so a successful
  // save cannot leave a stale draft open over updated numbers.
  useEffect(() => { setEditingId(null); }, [tiers]);

  const beginEdit = (t: TicketTier) => {
    setEditingId(t.id);
    setDraft({ name: t.name, price: t.price, capacity: t.capacity, description: t.description, is_visible: t.is_visible });
  };

  const saveEdit = useCallback(async (t: TicketTier) => {
    if (!draft.name || !String(draft.name).trim()) { toast('Name is required', 'error'); return; }
    const price = Number(draft.price);
    if (Number.isNaN(price) || price < 0) { toast('Price must be 0 or more', 'error'); return; }

    // Capacity below what is already committed would put the tier into a state
    // the booking transaction treats as oversold. The DB has a constraint that
    // would reject it anyway; catching it here gives a sentence instead of a 500.
    const committed = t.tickets_sold + t.reserved_count;
    const cap = draft.capacity === null || draft.capacity === undefined || String(draft.capacity) === ''
      ? null
      : Number(draft.capacity);
    if (cap !== null && (Number.isNaN(cap) || cap < committed)) {
      toast(`Capacity cannot be below ${committed} — that many are already sold or held`, 'error');
      return;
    }

    setBusy(true);
    try {
      await api.patch(`/organiser/ticket-types/${t.id}`, {
        name: String(draft.name).trim(),
        description: draft.description ? String(draft.description).trim() : null,
        price,
        capacity: cap,
        is_visible: draft.is_visible ?? true,
      });
      toast('Tier updated', 'success');
      onChanged();
    } catch (e: any) {
      toast(e?.response?.data?.error ?? 'Could not update tier', 'error');
    } finally {
      setBusy(false);
    }
  }, [draft, onChanged, toast]);

  const addTier = useCallback(async () => {
    if (!newTier.name.trim()) { toast('Name is required', 'error'); return; }
    const price = Number(newTier.price || 0);
    if (Number.isNaN(price) || price < 0) { toast('Price must be 0 or more', 'error'); return; }
    const cap = newTier.capacity.trim() === '' ? null : Number(newTier.capacity);
    if (cap !== null && (Number.isNaN(cap) || cap < 1)) { toast('Capacity must be at least 1, or blank for unlimited', 'error'); return; }

    setBusy(true);
    try {
      await api.post(`/organiser/events/${eventId}/ticket-types`, {
        types: [{
          name: newTier.name.trim(),
          description: newTier.description.trim() || null,
          price,
          capacity: cap,
          sort_order: tiers.length,
          is_visible: true,
        }],
      });
      toast('Tier added', 'success');
      setNewTier({ name: '', price: '', capacity: '', description: '' });
      setAdding(false);
      onChanged();
    } catch (e: any) {
      toast(e?.response?.data?.error ?? 'Could not add tier', 'error');
    } finally {
      setBusy(false);
    }
  }, [newTier, eventId, tiers.length, onChanged, toast]);

  const removeTier = (t: TicketTier) => {
    // A tier with sales cannot simply vanish — the bookings reference it and the
    // buyers hold tickets for it. Hiding takes it off sale without touching what
    // has already been sold, which is what the organiser almost always means.
    if (t.tickets_sold > 0) {
      toast(`${t.name} has ${t.tickets_sold} sold — hide it instead of deleting`, 'error');
      return;
    }
    confirm('Delete tier', `Delete "${t.name}"? This cannot be undone.`, async () => {
      try {
        await api.delete(`/organiser/ticket-types/${t.id}`);
        toast('Tier deleted', 'success');
        onChanged();
      } catch (e: any) {
        toast(e?.response?.data?.error ?? 'Could not delete tier', 'error');
      }
    });
  };

  const toggleVisible = useCallback(async (t: TicketTier) => {
    setBusy(true);
    try {
      await api.patch(`/organiser/ticket-types/${t.id}`, { is_visible: !t.is_visible });
      toast(t.is_visible ? `${t.name} hidden` : `${t.name} on sale`, 'success');
      onChanged();
    } catch (e: any) {
      toast(e?.response?.data?.error ?? 'Could not change visibility', 'error');
    } finally {
      setBusy(false);
    }
  }, [onChanged, toast]);

  const sorted = [...tiers].sort((a, b) => a.sort_order - b.sort_order);

  return (
    <div style={{ marginTop: 14 }}>
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8,
      }}>
        <div style={{ fontSize: 12, fontWeight: 700, color: 'var(--text-2)', textTransform: 'uppercase', letterSpacing: '.04em' }}>
          Ticket tiers
        </div>
        {!adding && (
          <button className="btn btn-sm" onClick={() => setAdding(true)} disabled={busy}>
            + Add tier
          </button>
        )}
      </div>

      {sorted.length === 0 && !adding && (
        <div style={{ fontSize: 12.5, color: 'var(--text-3)' }}>
          Flat pricing — no separate tiers. Add one to sell at more than one price.
        </div>
      )}

      {sorted.length > 0 && (
        <table className="meta-table" style={{ width: '100%' }}>
          <thead>
            <tr>
              <th>Tier</th><th>Price</th><th>Sold</th><th>Held</th>
              <th>Available</th><th>Status</th><th style={{ width: 150 }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((t) => {
              const { available, soldOut } = availabilityOf(t);
              const editing = editingId === t.id;

              if (editing) {
                return (
                  <tr key={t.id}>
                    <td colSpan={7} style={{ background: 'var(--surface-2)', padding: 12 }}>
                      <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr 1fr', gap: 10, marginBottom: 10 }}>
                        <div>
                          <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--text-2)' }}>Name</label>
                          <input style={inputStyle} value={String(draft.name ?? '')}
                            onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
                        </div>
                        <div>
                          <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--text-2)' }}>Price</label>
                          <input style={inputStyle} type="number" min={0} value={String(draft.price ?? '')}
                            onChange={(e) => setDraft({ ...draft, price: Number(e.target.value) })} />
                        </div>
                        <div>
                          <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--text-2)' }}>
                            Capacity <span style={{ fontWeight: 400 }}>(blank = unlimited)</span>
                          </label>
                          <input style={inputStyle} type="number" min={t.tickets_sold + t.reserved_count}
                            value={draft.capacity == null ? '' : String(draft.capacity)}
                            onChange={(e) => setDraft({ ...draft, capacity: e.target.value === '' ? null : Number(e.target.value) })} />
                        </div>
                      </div>
                      <div style={{ marginBottom: 10 }}>
                        <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--text-2)' }}>Description</label>
                        <input style={inputStyle} value={String(draft.description ?? '')}
                          placeholder="Shown to buyers under the tier name"
                          onChange={(e) => setDraft({ ...draft, description: e.target.value })} />
                      </div>
                      <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                        <button className="btn btn-primary btn-sm" onClick={() => saveEdit(t)} disabled={busy}>
                          {busy ? 'Saving…' : 'Save'}
                        </button>
                        <button className="btn btn-sm" onClick={() => setEditingId(null)} disabled={busy}>Cancel</button>
                        {t.tickets_sold + t.reserved_count > 0 && (
                          <span style={{ fontSize: 11.5, color: 'var(--text-3)' }}>
                            Capacity cannot go below {t.tickets_sold + t.reserved_count} (already sold or held)
                          </span>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              }

              return (
                <tr key={t.id} style={{ opacity: t.is_visible ? 1 : 0.55 }}>
                  <td>
                    <div style={{ fontWeight: 600 }}>{t.name}</div>
                    {t.description && (
                      <div style={{ fontSize: 12, color: 'var(--text-3)' }}>{t.description}</div>
                    )}
                  </td>
                  <td>{money(t.price, currency)}</td>
                  <td>{t.tickets_sold}</td>
                  <td>
                    {/* Reserved was invisible before. A tier that looks like it has
                        stock but will not sell is nearly always unpaid checkouts
                        holding the remainder, and without this column the
                        organiser has no way to see that. */}
                    {t.reserved_count > 0
                      ? <span title="Held by checkouts in progress">{t.reserved_count}</span>
                      : '—'}
                  </td>
                  <td>{available === null ? '∞' : available}</td>
                  <td>
                    {soldOut ? (
                      <span style={{ color: 'var(--danger)', fontWeight: 700, fontSize: 12 }}>Sold out</span>
                    ) : !t.is_visible ? (
                      <span style={{ color: 'var(--text-3)', fontWeight: 600, fontSize: 12 }}>Hidden</span>
                    ) : (
                      <span style={{ color: 'var(--success-fg)', fontWeight: 600, fontSize: 12 }}>On sale</span>
                    )}
                  </td>
                  <td>
                    <div style={{ display: 'flex', gap: 6 }}>
                      <button className="btn btn-sm" onClick={() => beginEdit(t)} disabled={busy}>Edit</button>
                      <button className="btn btn-sm" onClick={() => toggleVisible(t)} disabled={busy}>
                        {t.is_visible ? 'Hide' : 'Show'}
                      </button>
                      <button className="btn btn-sm btn-danger" onClick={() => removeTier(t)} disabled={busy}>
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      {adding && (
        <div style={{ marginTop: 12, padding: 12, background: 'var(--surface-2)', borderRadius: 8, border: '1px solid var(--border)' }}>
          <div style={{ fontSize: 12, fontWeight: 700, color: 'var(--text-2)', marginBottom: 10 }}>New tier</div>
          <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr 1fr', gap: 10, marginBottom: 10 }}>
            <div>
              <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--text-2)' }}>Name</label>
              <input style={inputStyle} value={newTier.name} placeholder="e.g. Early Bird"
                onChange={(e) => setNewTier({ ...newTier, name: e.target.value })} />
            </div>
            <div>
              <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--text-2)' }}>Price</label>
              <input style={inputStyle} type="number" min={0} value={newTier.price} placeholder="0"
                onChange={(e) => setNewTier({ ...newTier, price: e.target.value })} />
            </div>
            <div>
              <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--text-2)' }}>
                Capacity <span style={{ fontWeight: 400 }}>(blank = unlimited)</span>
              </label>
              <input style={inputStyle} type="number" min={1} value={newTier.capacity}
                onChange={(e) => setNewTier({ ...newTier, capacity: e.target.value })} />
            </div>
          </div>
          <div style={{ marginBottom: 10 }}>
            <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--text-2)' }}>Description</label>
            <input style={inputStyle} value={newTier.description}
              placeholder="Shown to buyers under the tier name"
              onChange={(e) => setNewTier({ ...newTier, description: e.target.value })} />
          </div>
          <div style={{ display: 'flex', gap: 8 }}>
            <button className="btn btn-primary btn-sm" onClick={addTier} disabled={busy}>
              {busy ? 'Adding…' : 'Add tier'}
            </button>
            <button className="btn btn-sm" onClick={() => setAdding(false)} disabled={busy}>Cancel</button>
          </div>
        </div>
      )}
    </div>
  );
}