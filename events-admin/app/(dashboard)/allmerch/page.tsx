'use client';
import { useEffect, useState } from 'react';
import { api } from '../../../lib/api';
import { fmtMoney, Badge } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';

interface MerchRow { id: string; name: string; event_id?: string; event_title?: string; base_price: number; is_active: boolean; fulfillment_mode: string; category?: string; variants: { stock_quantity: number; is_active: boolean }[]; }

const EMPTY_VARIANT = { sku: '', attributes: '', stock_quantity: '0', price_override: '' };

export default function AllMerchPage() {
  const toast = useToast();
  const confirm = useConfirm();
  const [all, setAll] = useState<MerchRow[]>([]);
  const [filtered, setFiltered] = useState<MerchRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
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

  function toggleActive(id: string, activate: boolean) {
    confirm(activate ? 'Activate Merchandise' : 'Deactivate Merchandise',
      activate ? 'This will make the item visible in listings again.' : 'This will hide the item from listings. Existing orders are not affected.',
      async () => {
        await api.put(`/admin/merch/${id}`, { is_active: activate });
        toast(activate ? 'Item activated' : 'Item deactivated', 'success'); loadAll();
      });
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
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Failed to add variant';
      toast(msg, 'error');
    } finally { setVSubmitting(false); }
  }

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
          <input className="search-input" placeholder="Filter merchandise…" value={search} onChange={e => setSearch(e.target.value)} />
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
                {[['Name','22%'],['Event','22%'],['Price','9%'],['Status','9%'],['Fulfillment','10%'],['Variants','9%'],['Stock','10%'],['Actions','9%']].map(([h, w]) => (
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
                const totalStock = m.variants.reduce((s, v) => s + (v.stock_quantity || 0), 0);
                return (
                  <tr key={m.id} style={{ borderBottom: '1px solid rgba(45,45,61,.5)' }}>
                    <td style={{ padding: '.65rem 1rem', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={m.name}>{m.name}</td>
                    <td style={{ padding: '.65rem 1rem', color: 'var(--text-2)', fontSize: '.8rem', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{m.event_title || '—'}</td>
                    <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)' }}>{fmtMoney(m.base_price)}</td>
                    <td style={{ padding: '.65rem 1rem' }}><Badge status={m.is_active ? 'active' : 'inactive'} /></td>
                    <td style={{ padding: '.65rem 1rem' }}><Badge status={m.fulfillment_mode || 'delivery'} /></td>
                    <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)', textAlign: 'center' }}>{m.variants.length}</td>
                    <td style={{ padding: '.65rem 1rem', fontFamily: 'var(--font-mono)' }}>{m.variants.length > 0 ? `${totalStock} units` : '—'}</td>
                    <td style={{ padding: '.65rem 1rem', overflow: 'visible', whiteSpace: 'nowrap' }}>
                      <div style={{ display: 'flex', gap: '.3rem' }}>
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