'use client';
// ─────────────────────────────────────────────────────────────────────────────
// ImageCropper
//
// A modal that crops a picked file to a fixed aspect BEFORE it is uploaded, so
// what the organiser sees framed here is exactly the bytes the backend stores.
//
// Why crop client-side rather than framing at display time: the same stored
// image is rendered by the web apps AND by EventsApp, and each frames it
// differently. Cropping on upload means one asset that is already the right
// shape everywhere, with no per-surface rules to keep in sync — and the
// organiser, not a centre-crop, decides which part of the artwork survives.
// That cuts both ways, which is why the ratios below are chosen per slot and
// not applied blindly to every image.
//
// Hand-rolled rather than pulling in react-easy-crop: this is ~1 canvas call
// and some clamped arithmetic, and the dashboard has no other cropping
// dependency to justify.
//
// NOT implemented: rotation, and two-finger pinch zoom. The dashboard is a
// desktop tool — drag to pan, slider or wheel to zoom. Add pinch here if
// organisers start doing this on tablets.
// ─────────────────────────────────────────────────────────────────────────────
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Crop, Check, X, MagnifyingGlassPlus, MagnifyingGlassMinus,
  ArrowsOutCardinal, ArrowCounterClockwise, Warning,
} from '@phosphor-icons/react';

// ── Ratios, in one place ─────────────────────────────────────────────────────
//
// COVER — 16:9. Not a preference: every surface that renders a cover already
// frames it at 16:9 (the feed and category cards on web-desktop, the wizard's
// own preview, the organiser event card). Cropping to 16:9 therefore discards
// only pixels that were never being shown anyway.
export const CROP_COVER = 16 / 9;

// LINE-UP PHOTO — 1:1. These render as circles in all three consumer apps, and
// a circle is inscribed in a square. Anything else is centre-cropped to a
// square by the client anyway.
export const CROP_LINEUP = 1;

// BANNER — deliberately null, meaning "no cropper, upload as picked".
//
// Locking the banner to a wide ratio is safe on web and destructive in the app.
// EventsApp/app/event/[id].tsx sizes its hero to the ARTWORK's own aspect,
// clamped between HERO_MIN_H (34% of screen height) and HERO_MAX_H (58%), with
// HERO_FIT = 'cover'. On a 390x844 phone a 3:1 banner is 130px tall at full
// width but the frame floor is 287px, so 'cover' scales it 2.2x and crops ~55%
// of its width — the exact failure the comment above HERO_FIT describes.
//
// Set this to a number ONLY alongside a decision about what the app should
// render instead (a second stored derivative, or a stored crop rect the app
// ignores). Until then the banner uploads untouched and every surface keeps
// framing it the way it already does.
export const CROP_BANNER: number | null = null;

// Stage bounds. Width wins for wide ratios, height for tall ones, so a 1:1 crop
// does not push the footer off a laptop screen.
const STAGE_MAX_W = 520;
const STAGE_MAX_H = 360;

// How far in the zoom slider goes, as a multiple of the "just covers the frame"
// scale. 4x is enough to crop a face out of a wide group shot and not so much
// that the output silently becomes unusably small.
const MAX_ZOOM = 4;

// Below this the output is too small for a hero or a card and the organiser
// should be told, not left to find out on the live page.
const LOW_RES_W = 800;

const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), hi);

/** Where the image sits in the frame: scale plus a top-left offset in stage px. */
interface View { scale: number; x: number; y: number }

/** Swap the extension on a filename, keeping the stem. */
function renameTo(name: string, ext: string): string {
  const dot = name.lastIndexOf('.');
  const stem = dot > 0 ? name.slice(0, dot) : name;
  return `${stem}${ext}`;
}

export default function ImageCropper({
  file,
  aspect,
  title,
  hint,
  maxOutputWidth = 1600,
  onCancel,
  onConfirm,
}: {
  /** The file the organiser just picked. Type and size are the caller's job. */
  file: File;
  /** Width / height. Required — a free crop is not offered on purpose. */
  aspect: number;
  title: string;
  hint?: string;
  /** Longest edge of the exported image. Never upscales past the source. */
  maxOutputWidth?: number;
  onCancel: () => void;
  /** Receives a NEW File. The caller uploads it exactly as it uploads any other. */
  onConfirm: (cropped: File) => void;
}) {
  // The object URL and the decoded element land together, from the load
  // callback, so nothing here sets state synchronously inside an effect and
  // there is no render pass where one exists without the other.
  const [loaded, setLoaded] = useState<{ url: string; img: HTMLImageElement } | null>(null);
  const [failed, setFailed] = useState(false);
  const [busy, setBusy] = useState(false);

  // null means "untouched" and falls back to `fit` below. That is what makes
  // Reset a one-liner and keeps the opening position out of an effect — an
  // effect that sets state on mount is the cascading-render pattern the
  // react-hooks rules flag, and here it was never needed.
  const [view, setView] = useState<View | null>(null);

  const stageRef = useRef<HTMLDivElement | null>(null);
  const dragRef = useRef<{ px: number; py: number; ox: number; oy: number } | null>(null);

  const img = loaded?.img ?? null;

  // ── Decode ────────────────────────────────────────────────────────────────
  // Via an <img> rather than createImageBitmap: browsers apply EXIF orientation
  // to <img> by default (image-orientation: from-image), and drawImage then
  // draws what was displayed. createImageBitmap defaults the other way, which
  // is how a portrait phone photo ends up cropped sideways.
  // The URL is created INSIDE the effect and revoked by its cleanup, so a
  // StrictMode double-invoke in development gets a fresh URL on the second run
  // rather than a revoked one.
  useEffect(() => {
    const url = URL.createObjectURL(file);
    const el = new Image();
    el.onload = () => setLoaded({ url, img: el });
    el.onerror = () => setFailed(true);
    el.src = url;
    return () => URL.revokeObjectURL(url);
  }, [file]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onCancel(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onCancel]);

  // ── Stage geometry ────────────────────────────────────────────────────────
  const stage = useMemo(() => {
    if (aspect >= STAGE_MAX_W / STAGE_MAX_H) {
      return { w: STAGE_MAX_W, h: Math.round(STAGE_MAX_W / aspect) };
    }
    return { w: Math.round(STAGE_MAX_H * aspect), h: STAGE_MAX_H };
  }, [aspect]);

  // The opening view: smallest scale that still fills the frame, centred. Also
  // the floor every other scale is clamped against, which is what makes gaps at
  // the edges impossible rather than merely discouraged. Derived, not stored —
  // so "reset" is setView(null) and the opening position needs no effect.
  const fit = useMemo<View | null>(() => {
    if (!img) return null;
    const scale = Math.max(stage.w / img.naturalWidth, stage.h / img.naturalHeight);
    return {
      scale,
      x: (stage.w - img.naturalWidth * scale) / 2,
      y: (stage.h - img.naturalHeight * scale) / 2,
    };
  }, [img, stage]);

  const cur = view ?? fit;

  const clampView = useCallback((v: View): View => {
    if (!img) return v;
    const w = img.naturalWidth * v.scale;
    const h = img.naturalHeight * v.scale;
    return {
      scale: v.scale,
      x: clamp(v.x, stage.w - w, 0),
      y: clamp(v.y, stage.h - h, 0),
    };
  }, [img, stage]);

  /** Zoom about the centre of the frame, so the subject stays put. */
  const zoomTo = useCallback((next: number) => {
    if (!fit || !cur) return;
    const s = clamp(next, fit.scale, fit.scale * MAX_ZOOM);
    setView(clampView({
      scale: s,
      x: stage.w / 2 - (stage.w / 2 - cur.x) * (s / cur.scale),
      y: stage.h / 2 - (stage.h / 2 - cur.y) * (s / cur.scale),
    }));
  }, [fit, cur, stage, clampView]);

  // Wheel zoom needs a NON-PASSIVE listener. React's onWheel is registered
  // passively at the root, so preventDefault there is ignored and the modal
  // scrolls behind the crop instead of zooming.
  useEffect(() => {
    const el = stageRef.current;
    if (!el || !cur) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      zoomTo(cur.scale * (e.deltaY < 0 ? 1.12 : 1 / 1.12));
    };
    el.addEventListener('wheel', onWheel, { passive: false });
    return () => el.removeEventListener('wheel', onWheel);
  }, [zoomTo, cur]);

  // ── Drag to pan ───────────────────────────────────────────────────────────
  function onPointerDown(e: React.PointerEvent) {
    if (!cur) return;
    (e.target as Element).setPointerCapture?.(e.pointerId);
    dragRef.current = { px: e.clientX, py: e.clientY, ox: cur.x, oy: cur.y };
  }
  function onPointerMove(e: React.PointerEvent) {
    const d = dragRef.current;
    if (!d || !cur) return;
    setView(clampView({
      scale: cur.scale,
      x: d.ox + (e.clientX - d.px),
      y: d.oy + (e.clientY - d.py),
    }));
  }
  function onPointerUp() { dragRef.current = null; }

  // ── Output ────────────────────────────────────────────────────────────────
  // Source rect in IMAGE pixels: the stage shows image coords from
  // (-offset / scale) across (stage size / scale).
  const srcRect = useMemo(() => {
    if (!img || !cur) return null;
    return {
      x: -cur.x / cur.scale,
      y: -cur.y / cur.scale,
      w: stage.w / cur.scale,
      h: stage.h / cur.scale,
    };
  }, [img, cur, stage]);

  // Never upscale. Zooming right in on a small photo gives a small file, and
  // the readout below says so rather than inventing pixels.
  const outW = srcRect ? Math.max(1, Math.min(maxOutputWidth, Math.round(srcRect.w))) : 0;
  const outH = Math.round(outW / aspect);
  const lowRes = outW > 0 && outW < LOW_RES_W;

  async function confirm() {
    if (!img || !srcRect) return;
    setBusy(true);
    try {
      const canvas = document.createElement('canvas');
      canvas.width = outW;
      canvas.height = outH;
      const ctx = canvas.getContext('2d');
      if (!ctx) throw new Error('no 2d context');
      ctx.imageSmoothingQuality = 'high';

      // PNG in, PNG out — a logo lock-up with a transparent background stays
      // transparent. Everything else (JPEG, and WebP, which can also carry
      // alpha) is flattened onto white first, because JPEG has no alpha and an
      // unfilled canvas exports those pixels black.
      const keepAlpha = file.type === 'image/png';
      if (!keepAlpha) {
        ctx.fillStyle = '#ffffff';
        ctx.fillRect(0, 0, outW, outH);
      }
      ctx.drawImage(img, srcRect.x, srcRect.y, srcRect.w, srcRect.h, 0, 0, outW, outH);

      const type = keepAlpha ? 'image/png' : 'image/jpeg';
      const blob: Blob | null = await new Promise(res =>
        canvas.toBlob(res, type, keepAlpha ? undefined : 0.92));
      if (!blob) throw new Error('encode failed');

      onConfirm(new File(
        [blob],
        renameTo(file.name, keepAlpha ? '.png' : '.jpg'),
        { type, lastModified: Date.now() },
      ));
    } catch {
      setFailed(true);
    } finally {
      setBusy(false);
    }
  }

  const minScale = fit?.scale ?? 1;

  const iconBtn: React.CSSProperties = {
    width: 30, height: 30, display: 'grid', placeItems: 'center',
    border: '1px solid var(--border)', borderRadius: 'var(--r)',
    background: 'var(--surface-alpha)', color: 'var(--text-2)',
    cursor: 'pointer', flexShrink: 0,
  };

  return (
    <div
      className="modal-overlay"
      onMouseDown={e => { if (e.target === e.currentTarget) onCancel(); }}
    >
      <div className="modal" role="dialog" aria-modal="true" aria-label={title}>
        <div className="modal-header">
          <div style={{ display: 'flex', alignItems: 'center', gap: 11 }}>
            <span style={{
              width: 32, height: 32, borderRadius: 10, flexShrink: 0,
              background: 'var(--accent-soft)', color: 'var(--accent)',
              display: 'grid', placeItems: 'center',
            }}>
              <Crop size={17} weight="bold" />
            </span>
            <div>
              <div className="modal-title">{title}</div>
              <div style={{ fontSize: 12.5, color: 'var(--text-3)', marginTop: 2 }}>
                {hint ?? 'Drag to reposition, then scroll or use the slider to zoom.'}
              </div>
            </div>
          </div>
          <button type="button" className="btn-icon" onClick={onCancel} aria-label="Cancel">
            <X size={15} />
          </button>
        </div>

        <div className="modal-body">
          {failed ? (
            <div className="alert alert-danger" style={{ marginBottom: 0 }}>
              <Warning size={16} weight="fill" />
              <div>That image could not be read. Try a different file, or a JPEG or PNG export of it.</div>
            </div>
          ) : (
            <>
              <div
                ref={stageRef}
                onPointerDown={onPointerDown}
                onPointerMove={onPointerMove}
                onPointerUp={onPointerUp}
                onPointerCancel={onPointerUp}
                style={{
                  width: stage.w, height: stage.h, margin: '0 auto',
                  position: 'relative', overflow: 'hidden',
                  borderRadius: 'var(--r-lg)',
                  border: '1px solid var(--border-2)',
                  background: 'var(--surface-2)',
                  display: loaded ? 'block' : 'grid',
                  placeItems: 'center',
                  cursor: loaded ? 'grab' : 'wait',
                  touchAction: 'none',
                  userSelect: 'none',
                }}
              >
                {loaded && img && cur ? (
                  <>
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={loaded.url}
                      alt=""
                      draggable={false}
                      style={{
                        position: 'absolute',
                        left: cur.x, top: cur.y,
                        width: img.naturalWidth * cur.scale,
                        height: img.naturalHeight * cur.scale,
                        maxWidth: 'none', display: 'block',
                        pointerEvents: 'none',
                      }}
                    />
                    {/* Rule-of-thirds guides. Faint and non-interactive — they
                        help line a face or a logo up without competing with
                        the art. */}
                    <div style={{ position: 'absolute', inset: 0, pointerEvents: 'none' }}>
                      {[1, 2].map(i => (
                        <div key={`h${i}`} style={{
                          position: 'absolute', left: 0, right: 0, top: `${(i * 100) / 3}%`,
                          height: 1, background: 'rgba(255,255,255,.28)',
                        }} />
                      ))}
                      {[1, 2].map(i => (
                        <div key={`v${i}`} style={{
                          position: 'absolute', top: 0, bottom: 0, left: `${(i * 100) / 3}%`,
                          width: 1, background: 'rgba(255,255,255,.28)',
                        }} />
                      ))}
                    </div>
                  </>
                ) : (
                  <div className="spinner" />
                )}
              </div>

              <div style={{
                display: 'flex', alignItems: 'center', gap: 10,
                marginTop: 16, maxWidth: stage.w, marginLeft: 'auto', marginRight: 'auto',
              }}>
                <button type="button" style={iconBtn} aria-label="Zoom out"
                  disabled={!cur} onClick={() => cur && zoomTo(cur.scale / 1.2)}>
                  <MagnifyingGlassMinus size={15} />
                </button>
                <input
                  type="range"
                  aria-label="Zoom"
                  disabled={!cur}
                  min={minScale}
                  max={minScale * MAX_ZOOM}
                  step={minScale / 100}
                  value={cur?.scale ?? minScale}
                  onChange={e => zoomTo(Number(e.target.value))}
                  style={{ flex: 1, accentColor: 'var(--accent)', cursor: 'pointer' }}
                />
                <button type="button" style={iconBtn} aria-label="Zoom in"
                  disabled={!cur} onClick={() => cur && zoomTo(cur.scale * 1.2)}>
                  <MagnifyingGlassPlus size={15} />
                </button>
                <button type="button" style={iconBtn} aria-label="Reset crop"
                  disabled={!cur} onClick={() => setView(null)}>
                  <ArrowCounterClockwise size={15} />
                </button>
              </div>

              <div style={{
                display: 'flex', alignItems: 'center', gap: 6, marginTop: 12,
                maxWidth: stage.w, marginLeft: 'auto', marginRight: 'auto',
                fontSize: 12, color: lowRes ? 'var(--warning-fg)' : 'var(--text-3)',
              }}>
                {lowRes ? <Warning size={13} weight="fill" /> : <ArrowsOutCardinal size={13} />}
                <span style={{ fontVariantNumeric: 'tabular-nums' }}>
                  {outW === 0
                    ? 'Reading image…'
                    : lowRes
                      ? `Exports at ${outW} x ${outH} — small for this slot. Zoom out for a sharper result.`
                      : `Exports at ${outW} x ${outH}`}
                </span>
              </div>
            </>
          )}
        </div>

        <div className="modal-footer">
          <button type="button" className="btn" onClick={onCancel}>Cancel</button>
          <button
            type="button"
            className="btn btn-primary"
            onClick={confirm}
            disabled={!img || failed || busy}
          >
            <Check size={15} weight="bold" />
            {busy ? 'Cropping…' : 'Use this crop'}
          </button>
        </div>
      </div>
    </div>
  );
}