'use client';
import { useEffect, useRef, useState } from 'react';
import { X, Warning, ImageSquare } from '@phosphor-icons/react';
import { useToast } from '../lib/toast';
import {
  getEventLineup, replaceEventLineup, uploadLineupImage,
  type LineupEntry, type LineupKind,
} from '../lib/eventContent';

/**
 * Line-up builder — performers and teams, with images, stages and set times.
 *
 * ── Why one builder for both ────────────────────────────────────────────────
 *
 * A performer and a team are the same shape: a name, a picture, an optional
 * link. They differ only in how the apps lay them out — teams render as a
 * fixture, performers as a list — which is a presentation decision and does
 * not need two editors behind it.
 *
 * ── The warning on a live event is not decoration ───────────────────────────
 *
 * Saving a line-up change on a published event notifies every ticket holder
 * and follower within about seven minutes. That is the point — someone who
 * paid to see a headliner should not find out by arriving — but it means a
 * save here is not a private act, and an organiser experimenting with the form
 * would otherwise send thousands of pushes without knowing.
 */

const BLANK = (kind: LineupKind): LineupEntry => ({
  kind, name: '', sort_order: 0,
});

export default function LineupBuilder({
  eventId,
  eventStart,
  isSport,
}: {
  eventId: string;
  /** ISO start time, used to anchor set times to the right DAY. */
  eventStart?: string;
  isSport?: boolean;
}) {
  const toast = useToast();
  const fileInput = useRef<HTMLInputElement | null>(null);
  const [uploadFor, setUploadFor] = useState<number | null>(null);

  const [entries, setEntries] = useState<LineupEntry[]>([]);
  const [isPublished, setIsPublished] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await getEventLineup(eventId);
        if (cancelled) return;
        setEntries(res.entries);
        setIsPublished(res.is_published);
      } catch {
        if (!cancelled) toast('Could not load line-up', 'error');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [eventId, toast]);

  function mutate(fn: (list: LineupEntry[]) => LineupEntry[]) {
    setEntries(prev => fn(prev).map((e, i) => ({ ...e, sort_order: i })));
    setDirty(true);
  }

  function update(i: number, patch: Partial<LineupEntry>) {
    mutate(list => list.map((e, j) => (j === i ? { ...e, ...patch } : e)));
  }

  function move(i: number, dir: -1 | 1) {
    const j = i + dir;
    if (j < 0 || j >= entries.length) return;
    mutate(list => {
      const next = [...list];
      [next[i], next[j]] = [next[j], next[i]];
      return next;
    });
  }

  /**
   * Set time is stored as a FULL timestamp, not a clock time.
   *
   * Most festivals cross midnight, so a bare 01:30 is ambiguous about which
   * day it belongs to — and getting that wrong puts the headliner 24 hours out
   * on the schedule. The date comes from the event's own start.
   */
  function setTimeFor(i: number, hhmm: string) {
    if (!hhmm) { update(i, { set_time: undefined }); return; }
    const base = eventStart ? new Date(eventStart) : new Date();
    const [h, m] = hhmm.split(':').map(Number);
    const d = new Date(base);
    d.setHours(h, m, 0, 0);
    // A set time earlier in the clock than the event start almost always means
    // the small hours of the NEXT day — a 01:00 closing set at a party that
    // starts at 21:00. Rolling it forward is right far more often than not,
    // and the server's ±12h window catches anything genuinely mistyped.
    if (eventStart && d.getTime() < base.getTime() - 6 * 3600_000) {
      d.setDate(d.getDate() + 1);
    }
    update(i, { set_time: d.toISOString() });
  }

  function clockValue(iso?: string) {
    if (!iso) return '';
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return '';
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  }

  async function pickImage(i: number) {
    setUploadFor(i);
    fileInput.current?.click();
  }

  async function onFile(f: File | null) {
    const i = uploadFor;
    setUploadFor(null);
    if (!f || i === null) return;
    try {
      const url = await uploadLineupImage(f);
      update(i, { image_url: url });
    } catch {
      toast('Image upload failed', 'error');
    }
  }

  async function save() {
    const cleaned = entries
      .map(e => ({ ...e, name: e.name.trim() }))
      .filter(e => e.name.length > 0);

    if (cleaned.length !== entries.length) {
      // Silently dropping blank rows is friendlier than an error — a half-typed
      // row someone abandoned is not a mistake worth blocking a save over.
      setEntries(cleaned.map((e, i) => ({ ...e, sort_order: i })));
    }

    setSaving(true);
    try {
      const res = await replaceEventLineup(
        eventId,
        cleaned.map(({ id: _id, ...rest }) => rest),
      );
      setEntries(res.entries);
      setDirty(false);
      toast(
        isPublished
          ? 'Line-up saved — ticket holders will be notified'
          : 'Line-up saved',
        'success',
      );
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } };
      toast(err.response?.data?.error ?? 'Could not save line-up', 'error');
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <div style={{ padding: 24 }}><div className="spinner" /></div>;

  return (
    <div>
      <input
        ref={fileInput}
        type="file"
        accept="image/jpeg,image/png,image/webp"
        style={{ display: 'none' }}
        onChange={e => { onFile(e.target.files?.[0] ?? null); e.target.value = ''; }}
      />

      {isPublished && (
        <div style={{
          display: 'flex', gap: 10, alignItems: 'flex-start',
          background: 'var(--accent-soft)', borderRadius: 'var(--r-lg)',
          padding: '12px 14px', marginBottom: 18,
        }}>
          <Warning size={17} weight="bold" style={{ color: 'var(--accent)', flexShrink: 0, marginTop: 1 }} />
          <div style={{ fontSize: 12.5, color: 'var(--text-2)', lineHeight: 1.6 }}>
            This event is live. Adding or removing an act notifies everyone
            holding a ticket and everyone following you, within a few minutes.
            Editing a stage, a set time or a photo does not.
          </div>
        </div>
      )}

      {entries.length === 0 && (
        <div style={{ textAlign: 'center', padding: '28px 20px', color: 'var(--text-3)', fontSize: 13 }}>
          No line-up yet.
        </div>
      )}

      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        {entries.map((e, i) => (
          <div key={e.id ?? i} style={{
            border: '1px solid var(--border)', borderRadius: 'var(--r-lg)',
            padding: 14, display: 'flex', gap: 14, alignItems: 'flex-start',
          }}>
            {/* Image */}
            <button
              type="button"
              onClick={() => pickImage(i)}
              title="Upload a photo"
              style={{
                width: 62, height: 62, borderRadius: 12, flexShrink: 0,
                border: '1px dashed var(--border)', background: 'var(--surface-2)',
                cursor: 'pointer', overflow: 'hidden', display: 'grid', placeItems: 'center',
                padding: 0,
              }}
            >
              {e.image_url
                // eslint-disable-next-line @next/next/no-img-element
                ? <img src={e.image_url} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
                : <ImageSquare size={20} color="var(--text-3)" />}
            </button>

            <div style={{ flex: 1, display: 'grid', gap: 10 }}>
              <div style={{ display: 'flex', gap: 10 }}>
                <input
                  style={{ flex: 2 }}
                  placeholder={e.kind === 'team' ? 'Team name' : 'Act or performer name'}
                  value={e.name}
                  onChange={ev => update(i, { name: ev.target.value })}
                />
                <select
                  style={{ flex: 1, maxWidth: 130 }}
                  value={e.kind}
                  onChange={ev => update(i, { kind: ev.target.value as LineupKind })}
                >
                  <option value="performer">Performer</option>
                  <option value="team">Team</option>
                </select>
              </div>

              <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
                <input
                  style={{ flex: 1, minWidth: 130 }}
                  placeholder={e.kind === 'team' ? 'Home / Away' : 'Headliner, Support…'}
                  value={e.subtitle ?? ''}
                  onChange={ev => update(i, { subtitle: ev.target.value })}
                />
                {e.kind !== 'team' && (
                  <>
                    <input
                      style={{ flex: 1, minWidth: 120 }}
                      placeholder="Stage"
                      value={e.stage ?? ''}
                      onChange={ev => update(i, { stage: ev.target.value })}
                    />
                    <input
                      type="time"
                      style={{ width: 120 }}
                      value={clockValue(e.set_time)}
                      onChange={ev => setTimeFor(i, ev.target.value)}
                    />
                  </>
                )}
              </div>

              <input
                placeholder="Link — their Spotify, Instagram or site (optional)"
                value={e.external_url ?? ''}
                onChange={ev => update(i, { external_url: ev.target.value })}
              />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <button type="button" className="btn btn-xs" onClick={() => move(i, -1)} disabled={i === 0}>
                ↑
              </button>
              <button type="button" className="btn btn-xs" onClick={() => move(i, 1)} disabled={i === entries.length - 1}>
                ↓
              </button>
              <button
                type="button"
                className="btn btn-xs btn-danger"
                onClick={() => mutate(list => list.filter((_, j) => j !== i))}
              >
                <X size={12} weight="bold" />
              </button>
            </div>
          </div>
        ))}
      </div>

      <div style={{ marginTop: 16, display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        <button type="button" className="btn btn-sm"
          onClick={() => mutate(l => [...l, BLANK(isSport ? 'team' : 'performer')])}>
          + Add {isSport ? 'team' : 'performer'}
        </button>
        <button type="button" className="btn btn-sm"
          onClick={() => mutate(l => [...l, BLANK(isSport ? 'performer' : 'team')])}>
          + Add {isSport ? 'performer' : 'team'}
        </button>
      </div>

      <div style={{ marginTop: 22, display: 'flex', gap: 10, alignItems: 'center' }}>
        <button type="button" className="btn btn-primary" disabled={!dirty || saving} onClick={save}>
          {saving ? 'Saving…' : 'Save line-up'}
        </button>
        {dirty && !saving && <span style={{ fontSize: 12, color: 'var(--text-3)' }}>Unsaved changes</span>}
      </div>
    </div>
  );
}