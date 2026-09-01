export function fmtMoney(v: number | null | undefined, cur = 'ZAR') {
  if (v == null) return '—';
  // maximumFractionDigits was 0, which ROUNDED rather than truncated: a R2.50
  // refund displayed as "R 3" and a R4.50 commission as "R 5". On a page whose
  // whole job is showing an organiser what they are owed, silently moving
  // money by up to 50c per figure is not a rounding detail.
  //
  // The digit count is decided per value rather than by a fixed min/max pair.
  // A plain minimum of 0 lets Intl drop a TRAILING zero, so R28.20 came out as
  // "R 28,2" — which reads as a broken number, not an amount. Whole rands get
  // no decimals (R90 stays "R 90"), anything with cents gets both (R28,20).
  //
  // Rounded to cents before the test so float noise — 0.1 + 0.2 landing at
  // 0.30000000000000004 — cannot make a whole amount look fractional.
  const rounded = Math.round(v * 100) / 100;
  const hasCents = Math.round(Math.abs(rounded) * 100) % 100 !== 0;
  return new Intl.NumberFormat('en-ZA', {
    style: 'currency',
    currency: cur,
    minimumFractionDigits: hasCents ? 2 : 0,
    maximumFractionDigits: 2,
  }).format(rounded);
}

export function fmtDate(d: string | null | undefined) {
  if (!d) return '—';
  return new Date(d).toLocaleDateString('en-ZA', { day: '2-digit', month: 'short', year: 'numeric' });
}

export function fmtDateTime(d: string | null | undefined) {
  if (!d) return '—';
  return new Date(d).toLocaleDateString('en-ZA', { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });
}

export function toLocalInput(dt: string | null | undefined) {
  if (!dt) return '';
  const d = new Date(dt);
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
  return d.toISOString().slice(0, 16);
}

export function formatAttrs(raw: string | null | undefined) {
  try { const o = JSON.parse(raw || '{}'); return Object.entries(o).map(([k, v]) => `${k}: ${v}`).join(', '); } catch { return raw || ''; }
}

export function Badge({ status }: { status: string }) {
  const map: Record<string, string> = {
    draft: 'badge-draft', published: 'badge-published', cancelled: 'badge-cancelled',
    completed: 'badge-completed', active: 'badge-active', inactive: 'badge-inactive',
    pending: 'badge-pending', confirmed: 'badge-confirmed', refunded: 'badge-refunded',
    delivery: 'badge-delivery', pickup: 'badge-pickup', both: 'badge-both',
    oversold: 'badge-oversold', paused: 'badge-paused', ended: 'badge-ended',
  };
  return <span className={`badge ${map[status] || 'badge-draft'}`}>{status}</span>;
}