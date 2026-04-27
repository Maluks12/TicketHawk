'use client';
import { useEffect, useState } from 'react';
import { api } from '../../../lib/api';
import { fmtMoney, fmtDate, toLocalInput, Badge, formatAttrs } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';

interface EventOption { id: string; title: string; status: string; }
interface MerchItem { id: string; name: string; description?: string; base_price: number; category?: string; is_active: boolean; fulfillment_mode: string; pickup_location?: string; delivery_fee?: number; sale_start?: string; sale_end?: string; images?: string; display_order: number; variants: Variant[]; }
interface Variant { id: string; sku: string; attributes: string; stock_quantity: number; price_override?: number; is_active: boolean; }

const EMPTY_MERCH = { name: '', description: '', base_price: '', category: '', display_order: '0', fulfillment_mode: 'delivery', pickup_location: '', delivery_fee: '0', sale_start: '', sale_end: '', images: '' };
const EMPTY_VARIANT = { sku: '', attributes: '', stock_quantity: '0', price_override: '' };

export default function MerchPage() {
  const toast = useToast();
  const confirm = useConfirm();
  const [events, setEvents] = useState<EventOption[]>([]);
  const [eventId, setEventId] = useState('');
  const [items, setItems] = useState<MerchItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [merchModal, setMerchModal] = useState(false);
  const [editingMerchId, setEditingMerchId] = useState<string | null>(null);
  const [mForm, setMForm] = useState({ ...EMPTY_MERCH });
  const [mSubmitting, setMSubmitting] = useState(false);
  const [variantModal, setVariantModal] = useState(false);
  const [variantMerchId, setVariantMerchId] = useState<string | null>(null);
  const [vForm, setVForm] = useState({ ...EMPTY_VARIANT });
  const [vSubmitting, setVSubmitting] = useState(false);

  useEffect(() => {
    api.get<{ data: EventOption[] }>('/admin/events?limit=100')
      .then(r => setEvents(r.data.data || [])).catch(() => {});
  }, []);

  async function loadMerch() {
    if (!eventId) return;
    setLoading(true);
    try {
      const { data } = await api.get<{ data: MerchItem[] }>(`/events/${eventId}/merch`);
      setItems(data.data || []);
    } catch { toast('Failed to load merch', 'error'); }
    finally { setLoading(false); }
  }

  useEffect(() => { loadMerch(); }, [eventId]); // eslint-disable-line react-hooks/exhaustive-deps

  function setMF(patch: Partial<typeof EMPTY_MERCH>) { setMForm(p => ({ ...p, ...patch })); }

  function openCreateMerch() { setEditingMerchId(null); setMForm({ ...EMPTY_MERCH }); setMerchModal(true); }

  async function openEditMerch(id: string) {
    setEditingMerchId(id); setMerchModal(true);
    try {
      const { data: m } = await api.get<MerchItem>(`/merch/${id}`);
      setMForm({
        name: m.name || '', description: m.description || '', base_price: String(m.base_price || ''),
        category: m.category || '', display_order: String(m.display_order || 0),
        fulfillment_mode: m.fulfillment_mode || 'delivery', pickup_location: m.pickup_location || '',
        delivery_fee: String(m.delivery_fee || 0), sale_start: toLocalInput(m.sale_start),
        sale_end: toLocalInput(m.sale_end), images: m.images || '',
      });
    } catch { toast('Failed to load merch item', 'error'); }
  }

  async function submitMerch() {
    setMSubmitting(true);
    try {
      const toISO = (v: string) => v ? new Date(v).toISOString() : null;
      const body: Record<string, unknown> = {
        name: mForm.name.trim(), base_price: parseFloat(mForm.base_price),
        display_order: parseInt(mForm.display_order) || 0,
        fulfillment_mode: mForm.fulfillment_mode,
        delivery_fee: parseFloat(mForm.delivery_fee) || 0,
      };
      if (mForm.description) body.description = mForm.description;
      if (mForm.category) body.category = mForm.category;
      if (mForm.pickup_location) body.pickup_location = mForm.pickup_location;
      if (mForm.sale_start) body.sale_start = toISO(mForm.sale_start);
      if (mForm.sale_end) body.sale_end = toISO(mForm.sale_end);
      if (mForm.images) {
        const urls = mForm.images.split(',').map(u => u.trim()).filter(Boolean);
        body.images = JSON.stringify(urls);
      }
      if (editingMerchId) { await api.put(`/admin/merch/${editingMerchId}`, body); toast('Merch updated', 'success'); }
      else if (eventId) { await api.post(`/organiser/events/${eventId}/merch`, body); toast('Merch created', 'success'); }
      else { await api.post('/admin/merch', body); toast('Standalone merch created', 'success'); }
      setMerchModal(false); loadMerch();
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Save failed';
      toast(msg, 'error');
    } finally { setMSubmitting(false); }
  }

  function deactivate(id: string) {
    confirm('Deactivate Merchandise', 'This will hide the item from listings. Existing orders are not affected.', async () => {
      await api.delete(`/organiser/merch/${id}`);
      toast('Item deactivated', 'success'); loadMerch();
    });
  }

  function openVariant(id: string) { setVariantMerchId(id); setVForm({ ...EMPTY_VARIANT }); setVariantModal(true); }

  async function submitVariant() {
    let attrs = vForm.attributes.trim();
    try { JSON.parse(attrs); } catch { toast('Attributes must be valid JSON, e.g. {"size":"L"}', 'error'); return; }
    setVSubmitting(true);
    try {
      const body: Record<string, unknown> = { sku: vForm.sku.trim(), attributes: attrs, stock_quantity: parseInt(vForm.stock_quantity) || 0 };
      if (vForm.price_override) body.price_override = parseFloat(vForm.price_override);
      await api.post(`/organiser/merch/${variantMerchId}/variants`, body);
      toast('Variant added', 'success'); setVariantModal(false); loadMerch();
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Failed to add variant';
      toast(msg, 'error');
    } finally { setVSubmitting(false); }
  }

  const showPickup = mForm.fulfillment_mode !== 'delivery';

  return (
    <div style={{ padding: '1.5rem', maxWidth: '1400px' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.1rem', fontWeight: 600 }}>Merchandise</div>
          <div style={{ fontSize: '.8rem', color: 'var(--text-2)', marginTop: '.15rem' }}>Manage merch items and collections per event</div>
        </div>
      </div>

      <div className="table-toolbar" style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-lg)', marginBottom: '1rem', flexWrap: 'wrap' }}>
        <label style={{ fontFamily: 'var(--font-mono)', fontSize: '.72rem', textTransform: 'uppercase', letterSpacing: '.07em', color: 'var(--text-3)' }}>
          Event <small style={{ textTransform: 'none', letterSpacing: 0, color: 'var(--text-3)' }}>(optional)</small>
        </label>
        <select className="filter-select" value={eventId} onChange={e => setEventId(e.target.value)} style={{ minWidth: '200px' }}>
          <option value="">— Select an event —</option>
          {events.map(e => <option key={e.id} value={e.id}>{e.title} ({e.status})</option>)}
        </select>
        <button className="btn btn-primary btn-sm" onClick={openCreateMerch}>+ Add Merch Item</button>
      </div>

      {!eventId ? (
        <div style={{ textAlign: 'center', padding: '3rem', color: 'var(--text-3)', fontSize: '.9rem' }}>Select an event above to see its merch, or click "+ Add Merch Item" to create a standalone item.</div>
      ) : loading ? (
        <div style={{ textAlign: 'center', padding: '2rem' }}><div className="spinner" style={{ margin: 'auto' }} /></div>
      ) : items.length === 0 ? (
        <div style={{ textAlign: 'center', padding: '3rem', color: 'var(--text-3)', fontSize: '.9rem' }}>No merch items yet. Click "+ Add Merch Item" to create one.</div>
      ) : (
        <div className="merch-grid">
          {items.map(m => (
            <div key={m.id} className="merch-card">
              <div style={{ padding: '1rem', flex: 1 }}>
                <div style={{ fontWeight: 500, marginBottom: '.2rem', fontSize: '.9rem', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={m.name}>{m.name}</div>
                <div style={{ fontFamily: 'var(--font-mono)', color: 'var(--accent)', fontSize: '.9rem', marginBottom: '.5rem' }}>{fmtMoney(m.base_price)}</div>
                <div style={{ display: 'flex', gap: '.4rem', flexWrap: 'wrap', marginBottom: '.5rem' }}>
                  <Badge status={m.is_active ? 'active' : 'inactive'} />
                  <Badge status={m.fulfillment_mode} />
                  {m.category && <span className="badge badge-draft">{m.category}</span>}
                </div>
                {m.description && <div style={{ fontSize: '.78rem', color: 'var(--text-2)', marginBottom: '.3rem', overflow: 'hidden', display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical' as const }}>{m.description}</div>}
                <div style={{ fontSize: '.75rem', color: 'var(--text-3)' }}>Sale: {m.sale_start ? `${fmtDate(m.sale_start)} → ${fmtDate(m.sale_end)}` : 'Always live'}</div>
                {m.variants.length > 0 && (
                  <div style={{ marginTop: '.6rem', display: 'flex', flexDirection: 'column', gap: '.4rem' }}>
                    {m.variants.map(v => (
                      <div key={v.id} className="variant-item">
                        <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--accent)', flexShrink: 0 }}>{v.sku}</span>
                        <span style={{ color: 'var(--text-2)', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis' }}>{formatAttrs(v.attributes)}</span>
                        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '.78rem', flexShrink: 0 }}>{v.stock_quantity}x</span>
                        {v.price_override != null && <span style={{ fontFamily: 'var(--font-mono)', fontSize: '.72rem', color: 'var(--accent)' }}>{fmtMoney(v.price_override)}</span>}
                      </div>
                    ))}
                  </div>
                )}
              </div>
              <div style={{ padding: '.65rem 1rem', borderTop: '1px solid var(--border)', display: 'flex', gap: '.4rem', alignItems: 'center' }}>
                <button className="btn btn-sm" onClick={() => openEditMerch(m.id)}>✎ Edit</button>
                <button className="btn btn-sm" onClick={() => openVariant(m.id)}>+ Variant</button>
                <button className="btn btn-sm btn-danger" onClick={() => deactivate(m.id)}>Deactivate</button>
                <span style={{ fontSize: '.75rem', color: 'var(--text-2)', marginLeft: 'auto' }}>{m.variants.length} variants</span>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Merch Modal */}
      {merchModal && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setMerchModal(false); }}>
          <div className="modal">
            <div className="modal-header">
              <div className="modal-title">{editingMerchId ? 'Edit Merchandise Item' : 'Add Merchandise Item'}</div>
              <button className="modal-close" onClick={() => setMerchModal(false)}>✕</button>
            </div>
            <div className="modal-body">
              <div className="field"><label>Name *</label><input type="text" maxLength={255} value={mForm.name} onChange={e => setMF({ name: e.target.value })} placeholder="e.g. Festival T-Shirt" /></div>
              <div className="field"><label>Description</label><textarea rows={2} value={mForm.description} onChange={e => setMF({ description: e.target.value })} placeholder="Optional description…" /></div>
              <div className="field-row">
                <div className="field"><label>Base Price *</label><input type="number" min="0.01" step="0.01" value={mForm.base_price} onChange={e => setMF({ base_price: e.target.value })} placeholder="0.00" /></div>
                <div className="field"><label>Collection</label><input type="text" value={mForm.category} onChange={e => setMF({ category: e.target.value })} placeholder="e.g. Shoes, Apparel" /></div>
              </div>
              <div className="field-row">
                <div className="field"><label>Display Order</label><input type="number" min="0" value={mForm.display_order} onChange={e => setMF({ display_order: e.target.value })} /></div>
                <div className="field"><label>Fulfillment Mode</label>
                  <select value={mForm.fulfillment_mode} onChange={e => setMF({ fulfillment_mode: e.target.value })}>
                    <option value="delivery">Delivery only</option>
                    <option value="pickup">Pickup only</option>
                    <option value="both">Both (buyer chooses)</option>
                  </select>
                </div>
              </div>
              {showPickup && (
                <div className="field-row">
                  <div className="field"><label>Pickup Location</label><input type="text" value={mForm.pickup_location} onChange={e => setMF({ pickup_location: e.target.value })} placeholder="e.g. Gate B, FNB Stadium" /></div>
                  <div className="field"><label>Delivery Fee (ZAR)</label><input type="number" min="0" step="0.01" value={mForm.delivery_fee} onChange={e => setMF({ delivery_fee: e.target.value })} /></div>
                </div>
              )}
              <div className="section-sep">Sale Window <small style={{ fontWeight: 300, textTransform: 'none', letterSpacing: 0, color: 'var(--text-3)' }}>(optional)</small></div>
              <div className="field-row">
                <div className="field"><label>Sale Start</label><input type="datetime-local" value={mForm.sale_start} onChange={e => setMF({ sale_start: e.target.value })} /></div>
                <div className="field"><label>Sale End</label><input type="datetime-local" value={mForm.sale_end} onChange={e => setMF({ sale_end: e.target.value })} /></div>
              </div>
              <div className="field"><label>Images <small style={{ color: 'var(--text-3)' }}>(URL or comma-separated URLs)</small></label><input type="text" value={mForm.images} onChange={e => setMF({ images: e.target.value })} placeholder="https://example.com/img.jpg" /></div>
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => setMerchModal(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={submitMerch} disabled={mSubmitting}>{mSubmitting ? 'Saving…' : editingMerchId ? 'Save Changes' : 'Add Item'}</button>
            </div>
          </div>
        </div>
      )}

      {/* Variant Modal */}
      {variantModal && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setVariantModal(false); }}>
          <div className="modal modal-sm">
            <div className="modal-header">
              <div className="modal-title">Add Variant</div>
              <button className="modal-close" onClick={() => setVariantModal(false)}>✕</button>
            </div>
            <div className="modal-body">
              <div className="alert alert-info" style={{ marginBottom: '.75rem', fontSize: '.78rem' }}>Attributes are key-value pairs, e.g. <code style={{ fontFamily: 'var(--font-mono)' }}>{`{"size":"L","colour":"black"}`}</code></div>
              <div className="field"><label>SKU *</label><input type="text" maxLength={100} value={vForm.sku} onChange={e => setVForm(p => ({ ...p, sku: e.target.value }))} placeholder="e.g. TSH-L-BLK" /></div>
              <div className="field"><label>Attributes (JSON) *</label><input type="text" value={vForm.attributes} onChange={e => setVForm(p => ({ ...p, attributes: e.target.value }))} placeholder={`{"size":"L","colour":"Black"}`} /></div>
              <div className="field-row">
                <div className="field"><label>Stock Quantity</label><input type="number" min="0" value={vForm.stock_quantity} onChange={e => setVForm(p => ({ ...p, stock_quantity: e.target.value }))} /></div>
                <div className="field"><label>Price Override <small style={{ color: 'var(--text-3)' }}>(optional)</small></label><input type="number" min="0.01" step="0.01" value={vForm.price_override} onChange={e => setVForm(p => ({ ...p, price_override: e.target.value }))} placeholder="Leave blank to use base" /></div>
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => setVariantModal(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={submitVariant} disabled={vSubmitting}>{vSubmitting ? 'Saving…' : 'Add Variant'}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}