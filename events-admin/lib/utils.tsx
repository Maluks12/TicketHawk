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

const STATUS_LABELS: Record<string, string> = {
  draft: 'Draft', published: 'Published', cancelled: 'Cancelled',
  completed: 'Completed', active: 'Active', inactive: 'Inactive',
  pending_review: 'Pending Review', rejected: 'Rejected', pending: 'Pending',
  admin: 'Admin', organizer: 'Organiser', organiser: 'Organiser',
  attendee: 'Attendee', user: 'User', delivery: 'Delivery',
  pickup: 'Pickup', oversold: 'Oversold',
};

export function Badge({ status }: { status: string }) {
  const cls = status.replace(/_/g, '_');
  const label = STATUS_LABELS[status] ?? status.charAt(0).toUpperCase() + status.slice(1).replace(/_/g, ' ');
  return <span className={`badge badge-${cls}`}>{label}</span>;
}