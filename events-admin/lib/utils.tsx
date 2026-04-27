export function fmtMoney(v: number | null | undefined, cur = 'ZAR') {
  if (v == null) return '—';
  return new Intl.NumberFormat('en-ZA', { style: 'currency', currency: cur, minimumFractionDigits: 0, maximumFractionDigits: 0 }).format(v);
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
    admin: 'badge-admin', organizer: 'badge-organizer', organiser: 'badge-organizer',
    attendee: 'badge-attendee', user: 'badge-user', delivery: 'badge-delivery',
    pickup: 'badge-pickup', both: 'badge-both', oversold: 'badge-oversold',
  };
  return <span className={`badge ${map[status] || 'badge-draft'}`}>{status}</span>;
}