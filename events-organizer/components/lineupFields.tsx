'use client';
import { useRef, useState } from 'react';
import { X, Warning, UploadSimple } from '@phosphor-icons/react';
import { useToast } from '../lib/toast';
import { TimePicker } from './dateTimePicker';
import ImageCropper, { CROP_LINEUP } from './imageCropper';
import { uploadLineupImage, type LineupEntry, type LineupKind } from '../lib/eventContent';

/**
 * Line-up fields — CONTROLLED. No state of its own beyond the file input.
 *
 * ── Why this is split out of LineupBuilder ──────────────────────────────────
 *
 * LineupBuilder loads by eventId and saves itself, which is right on the event
 * edit screen and useless in the CREATE wizard, where there is no event id yet.
 * So the editing UI lives here and each caller supplies its own persistence.
 *
 * ── On the styling ──────────────────────────────────────────────────────────
 *
 * Every input sits inside a `.field` wrapper, and that is not cosmetic:
 * globals.css only styles `.field input`, `.field select`, `.field textarea`.
 * A bare <input> gets no border, no background, no focus ring — which is
 * exactly why the first version of this looked like it belonged to a different
 * product.
 */

export const BLANK_LINEUP = (kind: LineupKind): LineupEntry => ({
  kind, name: '', sort_order: 0,
});

export default function LineupFields({
  entries,
  onChange,
  eventStart,
  isSport,
  notice,
}: {
  entries: LineupEntry[];
  onChange: (next: LineupEntry[]) => void;
  /** ISO start time. Anchors set times to the right DAY — see setTimeFor. */
  eventStart?: string;
  isSport?: boolean;
  notice?: React.ReactNode;
}) {
  const toast = useToast();
  const fileInput = useRef<HTMLInputElement | null>(null);
  const [uploadFor, setUploadFor] = useState<number | null>(null);
  const [uploading, setUploading] = useState<number | null>(null);
  // A picked file waiting to be cropped. Nothing is uploaded until the crop is
  // confirmed, so cancelling costs no bandwidth and leaves no orphan in R2.
  const [pendingCrop, setPendingCrop] = useState<{ index: number; file: File } | null>(null);

  function mutate(fn: (list: LineupEntry[]) => LineupEntry[]) {
    onChange(fn(entries).map((e, i) => ({ ...e, sort_order: i })));
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
   * Set time is a FULL timestamp, not a clock time.
   *
   * Most festivals cross midnight, so a bare 01:30 is ambiguous about which day
   * it belongs to — and getting that wrong puts the headliner 24 hours out.
   *
   * Without an event start (the wizard before a date is picked) the date falls
   * back to today; the wizard rewrites set times against the real start before
   * submitting, so nothing is persisted from that guess.
   */
  function setTimeFor(i: number, hhmm: string) {
    if (!hhmm) { update(i, { set_time: undefined }); return; }
    const base = eventStart ? new Date(eventStart) : new Date();
    const [h, m] = hhmm.split(':').map(Number);
    const d = new Date(base);
    d.setHours(h, m, 0, 0);
    // A set time earlier in the clock than the event start almost always means
    // the small hours of the NEXT day — a 01:00 closing set at a 21:00 party.
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

  /** Picked, not yet uploaded — hand it to the cropper first. */
  function onFile(f: File | null) {
    const i = uploadFor;
    setUploadFor(null);
    if (!f || i === null) return;
    setPendingCrop({ index: i, file: f });
  }

  async function uploadCropped(i: number, f: File) {
    setPendingCrop(null);
    setUploading(i);
    try {
      const url = await uploadLineupImage(f);
      update(i, { image_url: url });
      toast('Image uploaded', 'success');
    } catch (e: unknown) {
      toast((e instanceof Error ? e.message : null) ?? 'Image upload failed', 'error');
    } finally {
      setUploading(null);
    }
  }

  return (
    <div>
      <input
        ref={fileInput}
        type="file"
        accept="image/jpeg,image/png,image/webp"
        style={{ display: 'none' }}
        onChange={e => { onFile(e.target.files?.[0] ?? null); e.target.value = ''; }}
      />

      {/* Square, because every consumer surface renders these as CIRCLES and a
          circle is inscribed in a square. Without a crop step the client
          centre-crops for you, which is how a performer ends up framed by the
          middle of their own photo rather than by their face. */}
      {pendingCrop && (
        <ImageCropper
          file={pendingCrop.file}
          aspect={CROP_LINEUP}
          maxOutputWidth={900}
          title="Crop photo"
          hint="Shown as a circle on the event page — keep the face centred."
          onCancel={() => setPendingCrop(null)}
          onConfirm={f => uploadCropped(pendingCrop.index, f)}
        />
      )}

      {notice}

      {entries.length === 0 && (
        <div className="empty-state" style={{ padding: '26px 20px' }}>
          <div className="empty-desc">
            No line-up yet. Add the acts or teams people are coming to see.
          </div>
        </div>
      )}

      <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
        {entries.map((e, i) => (
          <div
            key={e.id ?? i}
            className="sub-section"
            style={{
              border: '1px solid var(--border)',
              borderRadius: 'var(--r-lg)',
              background: 'var(--surface-alpha)',
              padding: 16,
            }}
          >
            <div style={{ display: 'flex', gap: 16, alignItems: 'flex-start' }}>
              {/* Photo */}
              <div style={{ flexShrink: 0 }}>
                <button
                  type="button"
                  onClick={() => { setUploadFor(i); fileInput.current?.click(); }}
                  disabled={uploading === i}
                  title="Upload a photo"
                  style={{
                    width: 76, height: 76, borderRadius: 'var(--r-lg)',
                    border: '1px dashed var(--border-2)',
                    background: 'var(--surface-2)',
                    cursor: uploading === i ? 'wait' : 'pointer',
                    overflow: 'hidden', display: 'grid', placeItems: 'center',
                    padding: 0,
                  }}
                >
                  {uploading === i
                    ? <div className="spinner" />
                    : e.image_url
                      // eslint-disable-next-line @next/next/no-img-element
                      ? <img src={e.image_url} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
                      : <UploadSimple size={20} color="var(--text-3)" />}
                </button>
                {e.image_url && uploading !== i && (
                  <button
                    type="button"
                    className="btn btn-xs"
                    style={{ marginTop: 6, width: 76 }}
                    onClick={() => update(i, { image_url: undefined })}
                  >
                    Remove
                  </button>
                )}
              </div>

              {/* Fields */}
              <div style={{ flex: 1, minWidth: 0 }}>
                <div className="field-row" style={{ gridTemplateColumns: '2fr 1fr' }}>
                  <div className="field" style={{ marginBottom: 14 }}>
                    <label>{e.kind === 'team' ? 'Team name' : 'Name'}</label>
                    <input
                      placeholder={e.kind === 'team' ? 'e.g. Kaizer Chiefs' : 'e.g. Drae Sativ'}
                      value={e.name}
                      onChange={ev => update(i, { name: ev.target.value })}
                    />
                  </div>
                  <div className="field" style={{ marginBottom: 14 }}>
                    <label>Type</label>
                    <select value={e.kind} onChange={ev => update(i, { kind: ev.target.value as LineupKind })}>
                      <option value="performer">Performer</option>
                      <option value="team">Team</option>
                    </select>
                  </div>
                </div>

                <div
                  className="field-row"
                  style={{ gridTemplateColumns: e.kind === 'team' ? '1fr' : '1fr 1fr 130px' }}
                >
                  <div className="field" style={{ marginBottom: 14 }}>
                    <label>{e.kind === 'team' ? 'Home or away' : 'Billing'}</label>
                    <input
                      placeholder={e.kind === 'team' ? 'Home' : 'Headliner, Support…'}
                      value={e.subtitle ?? ''}
                      onChange={ev => update(i, { subtitle: ev.target.value })}
                    />
                  </div>
                  {e.kind !== 'team' && (
                    <>
                      <div className="field" style={{ marginBottom: 14 }}>
                        <label>Stage</label>
                        <input
                          placeholder="Main stage"
                          value={e.stage ?? ''}
                          onChange={ev => update(i, { stage: ev.target.value })}
                        />
                      </div>
                      <div className="field" style={{ marginBottom: 14 }}>
                        <label>Set time</label>
                        {/* Same picker as the event's start and end time. Still
                            emits 'HH:mm', so setTimeFor's day-anchoring maths
                            below is untouched. */}
                        <TimePicker
                          value={clockValue(e.set_time)}
                          onChange={v => setTimeFor(i, v)}
                        />
                      </div>
                    </>
                  )}
                </div>

                <div className="field" style={{ marginBottom: 0 }}>
                  <label>Link <span style={{ textTransform: 'none', letterSpacing: 0, fontWeight: 400 }}>(optional)</span></label>
                  <input
                    placeholder="Their Spotify, Instagram or website"
                    value={e.external_url ?? ''}
                    onChange={ev => update(i, { external_url: ev.target.value })}
                  />
                </div>
              </div>

              {/* Reorder / remove */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6, flexShrink: 0 }}>
                <button type="button" className="btn btn-xs" onClick={() => move(i, -1)} disabled={i === 0}>↑</button>
                <button type="button" className="btn btn-xs" onClick={() => move(i, 1)} disabled={i === entries.length - 1}>↓</button>
                <button
                  type="button"
                  className="btn btn-xs btn-danger"
                  onClick={() => mutate(list => list.filter((_, j) => j !== i))}
                  title="Remove"
                >
                  <X size={12} weight="bold" />
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>

      <div style={{ marginTop: 16, display: 'flex', gap: 10, flexWrap: 'wrap' }}>
        <button type="button" className="btn btn-sm"
          onClick={() => mutate(l => [...l, BLANK_LINEUP(isSport ? 'team' : 'performer')])}>
          + Add {isSport ? 'team' : 'performer'}
        </button>
        <button type="button" className="btn btn-sm"
          onClick={() => mutate(l => [...l, BLANK_LINEUP(isSport ? 'performer' : 'team')])}>
          + Add {isSport ? 'performer' : 'team'}
        </button>
      </div>
    </div>
  );
}

/** Shared warning, so both callers word it identically. */
export function LineupLiveNotice() {
  // This used to state that adding or removing an act notifies ticket holders
  // and followers within a few minutes. It does not. ReplaceEventLineupHandler
  // writes the line-up, busts the event cache and returns — there is no
  // notification call in that path, and its own comment says the notification
  // is not built yet.
  //
  // The old wording was worse than saying nothing: an organiser who dropped a
  // headliner would reasonably believe ticket holders had been told, and skip
  // announcing it themselves. Until the fan-out exists, the banner has to say
  // plainly that nobody is told.
  //
  // When it IS built, the distinction the old copy drew was the right one —
  // adds and removes are worth a notification, a stage or set-time or photo
  // edit is not — so restore that wording then rather than inventing new rules.
  return (
    <div className="alert alert-warning" style={{ marginBottom: 18 }}>
      <Warning size={16} weight="fill" />
      <div>
        This event is live, and line-up changes are published straight away —
        but nobody is notified. If you add or remove an act, tell your ticket
        holders and followers yourself.
      </div>
    </div>
  );
}