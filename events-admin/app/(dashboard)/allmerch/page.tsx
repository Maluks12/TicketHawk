'use client';
import { useEffect, useState } from 'react';
import { api } from '../../../lib/api';
import { fmtMoney, Badge, toLocalInput } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';

interface MerchRow {
  id: string; name: string; description?: string; event_id?: string; event_title?: string;
  base_price: number; category?: string; collection_id?: string; is_active: boolean;
  fulfillment_mode: string; pickup_location?: string; delivery_fee?: number;
  sale_start?: string; sale_end?: string; display_order: number;
  variants?: { id: string; sku: string; stock_quantity: number; price_override?: number; is_active: boolean }[];
}

const EMPTY_EDIT = {
  name: '', description: '', base_price: '', category: '', display_order: '0',
  fulfillment_mode: 'delivery', pickup_location: '', delivery_fee: '0',
  sale_start: '', sale_end: '', is_active: true,
};
const EMPTY_VARIANT = { sku: '', attributes: '', stock_quantity: '0', price_override: '' };

export default function AllMerchPage() {
  const toast = useToast();
  const confirm = useConfirm();

  const [all, setAll] = useState<MerchRow[]>([]);
  const [filtered, setFiltered] = useState<MerchRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('');

  // Edit modal
  const [editModal, setEditModal] = useState(false);
  const [editingItem, setEditingItem] = useState<MerchRow | null>(null);
  const [eForm, setEForm] = useState({ ...EMPTY_EDIT });
  const [eSubmitting, setESubmitting] = useState(false);

  // Variant modal
  const [variantModal, setVariantModal] = useState(false);
  const [variantMerchId, setVariantMerchId] = useState<string | null>(null);
  const [vForm, setVForm] = useState({ ...EMPTY_VARIANT });
  const [vSubmitting, setVSubmitting] = useState(false);

  async function loadAll() {
    setLoading(true);
    try {
      const { data } = await api.get<{ data: MerchRow[] }>('/admin/merch');
      setAll(data.data || []); setFiltered(data.data || []);
    } catch { toast('Failed to load merchandise', 'error'); }
    finally { setLoading(false); }
  }

  useEffect(() => { loadAll(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const q = search.toLowerCase();
    let result = all;
    if (q) result = result.filter(m => m.name.toLowerCase().includes(q) || (m.event_title || '').toLowerCase().includes(q) || (m.category || '').toLowerCase().includes(q));
    if (statusFilter === 'active') result = result.filter(m => m.is_active);
    if (statusFilter === 'inactive') result = result.filter(m => !m.is_active);
    setFiltered(result);
  }, [search, statusFilter, all]);

  function setEF(patch: Partial<typeof EMPTY_EDIT>) { setEForm(p => ({ ...p, ...patch })); }

  function openEdit(m: MerchRow) {
    setEditingItem(m);
    setEForm({
      name: m.name || '', description: m.description || '',
      base_price: String(m.base_price || ''), category: m.category || '',
      display_order: String(m.display_order ?? 0),
      fulfillment_mode: m.fulfillment_mode || 'delivery',
      pickup_location: m.pickup_location || '',
      delivery_fee: String(m.delivery_fee ?? 0),
      sale_start: toLocalInput(m.sale_start), sale_end: toLocalInput(m.sale_end),
      is_active: m.is_active,
    });
    setEditModal(true);
  }

  async function submitEdit() {
    if (!editingItem) return;
    setESubmitting(true);
    try {
      const toISO = (v: string) => v ? new Date(v).toISOString() : null;
      const body: Record<string, unknown> = {
        name: eForm.name.trim(),
        base_price: parseFloat(eForm.base_price),
        display_order: parseInt(eForm.display_order) || 0,
        fulfillment_mode: eForm.fulfillment_mode,
        delivery_fee: parseFloat(eForm.delivery_fee) || 0,
        is_active: eForm.is_active,
      };
      if (eForm.description) body.description = eForm.description;
      if (eForm.category) body.category = eForm.category;
      if (eForm.pickup_location) body.pickup_location = eForm.pickup_location;
      body.sale_start = eForm.sale_start ? toISO(eForm.sale_start) : null;
      body.sale_end = eForm.sale_end ? toISO(eForm.sale_end) : null;

      await api.put(`/admin/merch/${editingItem.id}`, body);
      toast('Merchandise updated', 'success');
      setEditModal(false);
      loadAll();
    } catch (e: unknown) {
      toast((e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Update failed', 'error');
    } finally { setESubmitting(false); }
  }

  function toggleActive(id: string, activate: boolean) {
    confirm(
      activate ? 'Activate Merchandise' : 'Deactivate Merchandise',
      activate ? 'This will make the item visible in listings again.' : 'This will hide the item from listings. Existing orders are not affected.',
      async () => {
        await api.put(`/admin/merch/${id}`, { is_active: activate });
        toast(activate ? 'Item activated' : 'Item deactivated', 'success'); loadAll();
      }
    );
  }

  function openVariant(id: string) { setVariantMerchId(id); setVForm({ ...EMPTY_VARIANT }); setVariantModal(true); }

  async function submitVariant() {
    try { JSON.parse(vForm.attributes); } catch { toast('Attributes must be valid JSON, e.g. {"size":"L"}', 'error'); return; }
    setVSubmitting(true);
    try {
      const body: Record<string, unknown> = { sku: vForm.sku.trim(), attributes: vForm.attributes.trim(), stock_quantity: parseInt(vForm.stock_quantity) || 0 };
      if (vForm.price_override) body.price_override = parseFloat(vForm.price_override);
      await api.post(`/organiser/merch/${variantMerchId}/variants`, body);
      toast('Variant added', 'success'); setVariantModal(false); loadAll();
    } catch (e: unknown) {
      toast((e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Failed to add variant', 'error');
    } finally { setVSubmitting(false); }
  }

  const showPickup = eForm.fulfillment_mode !== 'delivery';

  return (
    <div style={{ padding: '1.5rem', maxWidth: '1400px', display: 'flex', flexDirection: 'column', flex: 1 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.1rem', fontWeight: 600 }}>All Merchandise</div>
          <div style={{ fontSize: '.8rem', color: 'var(--text-2)', marginTop: '.15rem' }}>All merch items across every event</div>
        </div>
        <button className="btn btn-sm" onClick={loadAll}>↺ Refresh</button>
      </div>

      <div className="table-wrap">
        <div className="table-toolbar">
          <input className="search-input" placeholder="Filter by name, event, category…" value={search} onChange={e => setSearch(e.target.value)} />
          <select className="filter-select" value={statusFilter} onChange={e => setStatusFilter(e.target.value)}>
            <option value="">All Items</option>
            <option value="active">Active only</option>
            <option value="inactive">Inactive only</option>
          </select>
          <div style={{ marginLeft: 'auto', fontFamily: 'var(--font-mono)', fontSize: '.72rem', color: 'var(--text-3)' }}>{filtered.length} items</div>
        </div>

        <div style={{ overflowX: 'auto', flex: 1 }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '.85rem', tableLayout: 'fixed' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                {[['Name','20%'],['Event','18%'],['Price','8%'],['Category','9%'],['Status','8%'],['Fulfillment','10%'],['Stock','8%'],['Actions','19%']].map(([h, w]) => (
                  <th key={h} style={{ padding: '.65rem 1rem', textAlign: 'left', fontFamily: 'var(--font-mono)', fontSize: '.65rem', textTransform: 'uppercase', letterSpacing: '.07em', color: 'var(--text-3)', fontWeight: 500, width: w }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={8} style={{ padding: '2rem', textAlign: 'center' }}><div className="spinner" style={{ margin: 'auto' }} /></td></tr>
              ) : filtered.length === 0 ? (
                <tr><td colSpan={8} style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-3)' }}>No merchandise found.</td></tr>
              ) : filtered.map(m => {
                const variants = m.variants ?? [];
                const totalStock = variants.reduce((s, v) => s + (v.stock_quantity || 0), 0);
                return (
                  <tr key={m.id} style={{ borderBottom: '1px solid rgba(45,45,61,.5)' }}>
                    <td style={{ padding: '.65rem 1rem', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={m.name}>{m.name}</td>
                    <td style={{ padding: '.65rem 1rem', color: 'var(--text-2)', fontSize: '.8rem', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{m.event_title || '—'}</td>
                    <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)' }}>{fmtMoney(m.base_price)}</td>
                    <td style={{ padding: '.65rem 1rem', color: 'var(--text-2)', fontSize: '.8rem', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{m.category || '—'}</td>
                    <td style={{ padding: '.65rem 1rem' }}><Badge status={m.is_active ? 'active' : 'inactive'} /></td>
                    <td style={{ padding: '.65rem 1rem' }}><Badge status={m.fulfillment_mode || 'delivery'} /></td>
                    <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)' }}>{variants.length > 0 ? `${totalStock} units` : '—'}</td>
                    <td style={{ padding: '.65rem 1rem', overflow: 'visible', whiteSpace: 'nowrap' }}>
                      <div style={{ display: 'flex', gap: '.3rem' }}>
                        <button className="btn btn-sm" onClick={() => openEdit(m)}>✎ Edit</button>
                        <button className="btn btn-sm" onClick={() => openVariant(m.id)}>+ Variant</button>
                        {m.is_active
                          ? <button className="btn btn-sm btn-danger" onClick={() => toggleActive(m.id, false)}>Deactivate</button>
                          : <button className="btn btn-sm btn-success" onClick={() => toggleActive(m.id, true)}>Activate</button>}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* Edit Modal */}
      {editModal && editingItem && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setEditModal(false); }}>
          <div className="modal">
            <div className="modal-header">
              <div>
                <div className="modal-title">Edit Merchandise</div>
                <div style={{ fontSize: '.74rem', color: 'var(--text-3)', marginTop: '.1rem', fontFamily: 'var(--font-mono)' }}>{editingItem.id.slice(0, 8)}… {editingItem.event_title ? `· ${editingItem.event_title}` : '· Standalone'}</div>
              </div>
              <button className="modal-close" onClick={() => setEditModal(false)}>✕</button>
            </div>
            <div className="modal-body">
              <div className="field"><label>Name *</label><input type="text" maxLength={255} value={eForm.name} onChange={e => setEF({ name: e.target.value })} /></div>
              <div className="field"><label>Description</label><textarea rows={2} value={eForm.description} onChange={e => setEF({ description: e.target.value })} placeholder="Optional description…" /></div>
              <div className="field-row">
                <div className="field"><label>Base Price *</label><input type="number" min="0.01" step="0.01" value={eForm.base_price} onChange={e => setEF({ base_price: e.target.value })} /></div>
                <div className="field">
                  <label>Category <small style={{ color: 'var(--text-3)', textTransform: 'none', fontWeight: 400 }}>(drives colour theme)</small></label>
                  <input type="text" value={eForm.category} onChange={e => setEF({ category: e.target.value })} placeholder="e.g. apparel, footwear" />
                </div>
              </div>
              <div className="field-row">
                <div className="field"><label>Display Order</label><input type="number" min="0" value={eForm.display_order} onChange={e => setEF({ display_order: e.target.value })} /></div>
                <div className="field"><label>Fulfillment Mode</label>
                  <select value={eForm.fulfillment_mode} onChange={e => setEF({ fulfillment_mode: e.target.value })}>
                    <option value="delivery">Delivery only</option>
                    <option value="pickup">Pickup only</option>
                    <option value="both">Both (buyer chooses)</option>
                  </select>
                </div>
              </div>
              {showPickup && (
                <div className="field-row">
                  <div className="field"><label>Pickup Location</label><input type="text" value={eForm.pickup_location} onChange={e => setEF({ pickup_location: e.target.value })} placeholder="e.g. Gate B, FNB Stadium" /></div>
                  <div className="field"><label>Delivery Fee (ZAR)</label><input type="number" min="0" step="0.01" value={eForm.delivery_fee} onChange={e => setEF({ delivery_fee: e.target.value })} /></div>
                </div>
              )}
              <div className="section-sep">Sale Window <small style={{ fontWeight: 300, textTransform: 'none', letterSpacing: 0, color: 'var(--text-3)' }}>(optional)</small></div>
              <div className="field-row">
                <div className="field"><label>Sale Start</label><input type="datetime-local" value={eForm.sale_start} onChange={e => setEF({ sale_start: e.target.value })} /></div>
                <div className="field"><label>Sale End</label><input type="datetime-local" value={eForm.sale_end} onChange={e => setEF({ sale_end: e.target.value })} /></div>
              </div>
              <div className="field" style={{ flexDirection: 'row', alignItems: 'center', gap: '.6rem' }}>
                <input type="checkbox" id="is_active_toggle" checked={eForm.is_active} onChange={e => setEF({ is_active: e.target.checked })} style={{ width: 'auto', accentColor: 'var(--accent)' }} />
                <label htmlFor="is_active_toggle" style={{ margin: 0, fontSize: '.85rem', color: 'var(--text-1)', textTransform: 'none', letterSpacing: 0, fontWeight: 400, cursor: 'pointer' }}>Item is active (visible in listings)</label>
              </div>

              {/* Variants summary — read-only in edit modal */}
              {editingItem.variants && editingItem.variants.length > 0 && (
                <>
                  <div className="section-sep">Variants <small style={{ fontWeight: 300, textTransform: 'none', letterSpacing: 0, color: 'var(--text-3)' }}>(read-only — use All Merchandise table to add more)</small></div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '.35rem' }}>
                    {editingItem.variants.map(v => (
                      <div key={v.id} style={{ display: 'flex', gap: '.75rem', alignItems: 'center', padding: '.4rem .6rem', background: 'var(--bg)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '.8rem' }}>
                        <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--accent)', flexShrink: 0 }}>{v.sku}</span>
                        <span style={{ fontFamily: 'var(--font-mono)', flexShrink: 0 }}>{v.stock_quantity}x</span>
                        {v.price_override != null && <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--accent)' }}>{fmtMoney(v.price_override)}</span>}
                        <span style={{ marginLeft: 'auto' }}><Badge status={v.is_active ? 'active' : 'inactive'} /></span>
                      </div>
                    ))}
                  </div>
                </>
              )}
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => setEditModal(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={submitEdit} disabled={eSubmitting}>{eSubmitting ? 'Saving…' : 'Save Changes'}</button>
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