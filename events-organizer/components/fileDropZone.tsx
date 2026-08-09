'use client';
// ─────────────────────────────────────────────────────────────────────────────
// FileDropZone
//
// Replaces the bare <input type="file"> on the KYC page, which rendered as the
// browser's default "Choose File / No file chosen" — an OS-styled control that
// ignores the theme entirely and gives no feedback about what was picked.
//
// Accepts a drop or a click, validates type and size before the file goes
// anywhere, and shows the chosen file with a way to remove it. PDFs are
// supported alongside images because proof of address is usually a bank
// statement, not a photo.
// ─────────────────────────────────────────────────────────────────────────────
import { useState } from 'react';
import { UploadSimple, FilePdf, ImageSquare, X, Warning } from '@phosphor-icons/react';

export default function FileDropZone({
  label, hint, file, onPick, accept, maxMB = 5, required = false, disabled = false,
}: {
  label: string;
  hint?: string;
  file: File | null;
  onPick: (f: File | null) => void;
  /** MIME types accepted, e.g. ['image/jpeg','application/pdf']. */
  accept: string[];
  maxMB?: number;
  required?: boolean;
  disabled?: boolean;
}) {
  const [isOver, setIsOver] = useState(false);
  const [rejected, setRejected] = useState<string | null>(null);

  // Checked client-side purely for speed of feedback — the server validates
  // too. The point is that a 20 MB scan fails instantly with a reason rather
  // than after a long upload that ends in a generic error.
  function check(f: File): boolean {
    if (!accept.includes(f.type)) {
      setRejected(`That file type isn\u2019t accepted — use ${humanTypes(accept)}`);
      return false;
    }
    if (f.size > maxMB * 1024 * 1024) {
      setRejected(`That file is ${(f.size / 1024 / 1024).toFixed(1)} MB — the limit is ${maxMB} MB`);
      return false;
    }
    setRejected(null);
    return true;
  }

  function handle(f: File | undefined | null) {
    if (!f) return;
    if (check(f)) onPick(f);
  }

  const isPdf = file?.type === 'application/pdf';

  return (
    <div style={{ marginBottom: 14 }}>
      <label style={{
        display: 'block', fontSize: 11, fontWeight: 700,
        letterSpacing: '.06em', textTransform: 'uppercase',
        color: 'var(--text-3)', marginBottom: 7,
      }}>
        {label}
        {required && <span style={{ color: 'var(--danger)', marginLeft: 4 }}>*</span>}
      </label>

      {file ? (
        <div style={{
          display: 'flex', alignItems: 'center', gap: 12,
          padding: '12px 14px',
          background: 'var(--success-bg)',
          border: '1px solid var(--success-border)',
          borderRadius: 'var(--r-lg)',
        }}>
          <span style={{ color: 'var(--success-fg)', flexShrink: 0, display: 'flex' }}>
            {isPdf ? <FilePdf size={22} weight="fill" /> : <ImageSquare size={22} weight="fill" />}
          </span>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{
              fontSize: 13.5, fontWeight: 600, color: 'var(--text)',
              overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
            }}>
              {file.name}
            </div>
            <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 1 }}>
              {(file.size / 1024).toFixed(0)} KB
            </div>
          </div>
          <button
            type="button"
            onClick={() => { onPick(null); setRejected(null); }}
            className="btn-icon"
            aria-label={`Remove ${file.name}`}
            style={{ flexShrink: 0 }}
          >
            <X size={15} />
          </button>
        </div>
      ) : (
        <label
          onDragOver={e => { e.preventDefault(); if (!disabled) setIsOver(true); }}
          onDragEnter={e => { e.preventDefault(); if (!disabled) setIsOver(true); }}
          // relatedTarget is null when the pointer leaves the window entirely;
          // without the containment check the highlight sticks on.
          onDragLeave={e => { if (!e.currentTarget.contains(e.relatedTarget as Node)) setIsOver(false); }}
          onDrop={e => {
            e.preventDefault();
            setIsOver(false);
            if (!disabled) handle(e.dataTransfer.files?.[0]);
          }}
          style={{
            display: 'flex', alignItems: 'center', gap: 12,
            padding: '16px 16px',
            border: `1.5px dashed ${isOver ? 'var(--accent)' : 'var(--border-2)'}`,
            borderRadius: 'var(--r-lg)',
            background: isOver ? 'var(--accent-soft)' : 'var(--surface-alpha)',
            color: isOver ? 'var(--accent)' : 'var(--text-3)',
            cursor: disabled ? 'not-allowed' : 'pointer',
            opacity: disabled ? 0.6 : 1,
            transition: 'border-color .15s ease, background .15s ease, color .15s ease',
          }}
        >
          <input
            type="file"
            accept={accept.join(',')}
            disabled={disabled}
            style={{ display: 'none' }}
            onChange={e => { handle(e.target.files?.[0]); e.target.value = ''; }}
          />
          <UploadSimple size={22} weight={isOver ? 'bold' : 'regular'} style={{ flexShrink: 0 }} />
          <span style={{ fontSize: 13, fontWeight: 600 }}>
            {isOver ? 'Drop to attach' : 'Drag a file here, or click to browse'}
          </span>
        </label>
      )}

      {rejected ? (
        <div style={{
          display: 'flex', alignItems: 'center', gap: 5,
          fontSize: 12, color: 'var(--danger-fg)', marginTop: 6, fontWeight: 500,
        }}>
          <Warning size={13} weight="fill" />
          {rejected}
        </div>
      ) : hint ? (
        <div style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 6 }}>{hint}</div>
      ) : null}
    </div>
  );
}

function humanTypes(types: string[]): string {
  const names = types.map(t => {
    if (t === 'application/pdf') return 'PDF';
    if (t.startsWith('image/')) return t.replace('image/', '').toUpperCase();
    return t;
  });
  const uniq = Array.from(new Set(names));
  if (uniq.length === 1) return uniq[0];
  return `${uniq.slice(0, -1).join(', ')} or ${uniq[uniq.length - 1]}`;
}