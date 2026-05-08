'use client';
import { useEffect, useState, useMemo } from 'react';
import { api } from '../../../lib/api';
import { fmtDate, fmtMoney } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';

interface Collection {
  id: string; name: string; description?: string; display_order: number;
  is_active: boolean; organizer_id?: string; created_at: string; updated_at: string;
}
interface MerchItem {
  id: string; name: string; base_price: number; category?: string;
  is_active: boolean; event_title?: string; collection_id?: string;
}

const EMPTY_FORM = { name: '', description: '', display_order: '0' };

export default function CollectionsPage() {
  const toast = useToast();
  const confirm = useConfirm();

  const [collections, setCollections] = useState<Collection[]>([]);
  const [loading, setLoading] = useState(true);
  const [allMerch, setAllMerch] = useState<MerchItem[]>([]);
  const [allMerchLoaded, setAllMerchLoaded] = useState(false);

  const [formModal, setFormModal] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState({ ...EMPTY_FORM });
  const [submitting, setSubmitting] = useState(false);

  const [manageModal, setManageModal] = useState(false);
  const [manageCollection, setManageCollection] = useState<Collection | null>(null);
  const [currentItems, setCurrentItems] = useState<MerchItem[]>([]);
  const [manageLoading, setManageLoading] = useState(false);
  const [addSearch, setAddSearch] = useState('');
  const [saving, setSaving] = useState<string | null>(null);

  const [filter, setFilter] = useState<'all' | 'active' | 'inactive'>('all');
  const [search, setSearch] = useState('');

  useEffect(() => { loadCollections(); }, []);

  async function loadCollections() {
    setLoading(true);
    try {
      const { data } = await api.get<{ data: Collection[] }>('/admin/collections');
      setCollections(data.data || []);
    } catch { toast('Failed to load collections', 'error'); }
    finally { setLoading(false); }
  }

  async function ensureAllMerch() {
    if (allMerchLoaded) return;
    try {
      const { data } = await api.get<{ data: MerchItem[] }>('/admin/merch');
      setAllMerch(data.data || []);
      setAllMerchLoaded(true);
    } catch { toast('Failed to load merchandise list', 'error'); }
  }

  async function openManage(c: Collection) {
    setManageCollection(c); setCurrentItems([]); setAddSearch('');
    setManageModal(true); setManageLoading(true);
    await ensureAllMerch();
    try {
      const { data } = await api.get<{ items: MerchItem[] }>(`/collections/${c.id}`);
      setCurrentItems(data.items || []);
    } catch { toast('Failed to load collection items', 'error'); }
    finally { setManageLoading(false); }
  }

  const currentIds = useMemo(() => new Set(currentItems.map(i => i.id)), [currentItems]);
  const availableToAdd = useMemo(() => {
    const q = addSearch.toLowerCase();
    return allMerch.filter(m =>
      m.is_active && !currentIds.has(m.id) &&
      (m.name.toLowerCase().includes(q) || (m.event_title || '').toLowerCase().includes(q))
    );
  }, [allMerch, currentIds, addSearch]);

  async function addToCollection(item: MerchItem) {
    if (!manageCollection) return;
    setSaving(item.id);
    try {
      await api.put(`/admin/merch/${item.id}/collection`, { collection_id: manageCollection.id });
      setCurrentItems(prev => [...prev, item]);
      toast(`"${item.name}" added`, 'success');
    } catch { toast('Failed to add item', 'error'); }
    finally { setSaving(null); }
  }

  async function removeFromCollection(item: MerchItem) {
    setSaving(item.id);
    try {
      await api.put(`/admin/merch/${item.id}/collection`, { collection_id: null });
      setCurrentItems(prev => prev.filter(i => i.id !== item.id));
      toast(`"${item.name}" removed`, 'success');
    } catch { toast('Failed to remove item', 'error'); }
    finally { setSaving(null); }
  }

  function setF(patch: Partial<typeof EMPTY_FORM>) { setForm(p => ({ ...p, ...patch })); }
  function openCreate() { setEditingId(null); setForm({ ...EMPTY_FORM }); setFormModal(true); }
  function openEdit(c: Collection) {
    setEditingId(c.id);
    setForm({ name: c.name, description: c.description || '', display_order: String(c.display_order) });
    setFormModal(true);
  }

  async function submitForm() {
    if (!form.name.trim()) { toast('Name is required', 'error'); return; }
    setSubmitting(true);
    try {
      const body = { name: form.name.trim(), description: form.description.trim() || undefined, display_order: parseInt(form.display_order) || 0 };
      if (editingId) { await api.put(`/admin/collections/${editingId}`, body); toast('Collection updated', 'success'); }
      else { await api.post('/admin/collections', body); toast('Collection created', 'success'); }
      setFormModal(false); loadCollections();
    } catch (e: unknown) {
      toast((e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Save failed', 'error');
    } finally { setSubmitting(false); }
  }

  function deactivate(c: Collection) {
    confirm('Deactivate Collection', `"${c.name}" will be hidden and all its merch items will be un-assigned (not deleted).`, async () => {
      try { await api.delete(`/admin/collections/${c.id}`); toast('Collection deactivated', 'success'); loadCollections(); }
      catch { toast('Failed to deactivate', 'error'); }
    });
  }

  const filtered = collections.filter(c => {
    if (filter === 'active' && !c.is_active) return false;
    if (filter === 'inactive' && c.is_active) return false;
    if (search && !c.name.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });
  const activeCount = collections.filter(c => c.is_active).length;

  return (
    <div style={{ padding: '1.5rem', maxWidth: '1200px' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
        <div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.1rem', fontWeight: 600 }}>Collections</div>
          <div style={{ fontSize: '.8rem', color: 'var(--text-2)', marginTop: '.15rem' }}>Curated groups of merchandise</div>
        </div>
        <button className="btn btn-primary btn-sm" onClick={openCreate}>+ New Collection</button>
      </div>

      <div style={{ display: 'flex', gap: '1rem', marginBottom: '1rem', flexWrap: 'wrap' }}>
        {[{ label: 'Total', value: collections.length, accent: false }, { label: 'Active', value: activeCount, accent: true }, { label: 'Inactive', value: collections.length - activeCount, accent: false }].map(s => (
          <div key={s.label} style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-lg)', padding: '.6rem 1rem', minWidth: '90px' }}>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.3rem', fontWeight: 600, color: s.accent ? 'var(--accent)' : 'var(--text-1)' }}>{s.value}</div>
            <div style={{ fontSize: '.72rem', color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '.06em' }}>{s.label}</div>
          </div>
        ))}
      </div>

      <div className="table-toolbar" style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-lg)', marginBottom: '1rem', flexWrap: 'wrap' }}>
        <input className="filter-select" type="text" placeholder="Search collections…" value={search} onChange={e => setSearch(e.target.value)} style={{ minWidth: '180px' }} />
        {(['all', 'active', 'inactive'] as const).map(f => (
          <button key={f} className={`btn btn-sm${filter === f ? ' btn-primary' : ''}`} onClick={() => setFilter(f)}>
            {f.charAt(0).toUpperCase() + f.slice(1)}
          </button>
        ))}
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: '2rem' }}><div className="spinner" style={{ margin: 'auto' }} /></div>
      ) : filtered.length === 0 ? (
        <div style={{ textAlign: 'center', padding: '3rem', color: 'var(--text-3)', fontSize: '.9rem' }}>
          {collections.length === 0 ? 'No collections yet. Click "+ New Collection" to create one.' : 'No collections match your filter.'}
        </div>
      ) : (
        <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-lg)', overflow: 'hidden' }}>
          <table className="table" style={{ width: '100%' }}>
            <thead>
              <tr>
                <th>Name</th><th>Description</th><th style={{ textAlign: 'center' }}>Order</th>
                <th>Owner</th><th style={{ textAlign: 'center' }}>Status</th><th>Created</th>
                <th style={{ textAlign: 'right' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map(c => (
                <tr key={c.id}>
                  <td>
                    <div style={{ fontWeight: 500, fontSize: '.88rem' }}>{c.name}</div>
                    <div style={{ fontFamily: 'var(--font-mono)', fontSize: '.65rem', color: 'var(--text-3)', marginTop: '.1rem' }}>{c.id.slice(0, 8)}…</div>
                  </td>
                  <td style={{ maxWidth: '220px' }}>
                    <div style={{ fontSize: '.82rem', color: 'var(--text-2)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {c.description || <span style={{ color: 'var(--text-3)', fontStyle: 'italic' }}>—</span>}
                    </div>
                  </td>
                  <td style={{ textAlign: 'center', fontFamily: 'var(--font-mono)', fontSize: '.82rem' }}>{c.display_order}</td>
                  <td>
                    {c.organizer_id
                      ? <span style={{ fontFamily: 'var(--font-mono)', fontSize: '.72rem', color: 'var(--text-2)' }}>Organiser</span>
                      : <span style={{ fontFamily: 'var(--font-mono)', fontSize: '.72rem', color: 'var(--accent)' }}>Admin</span>}
                  </td>
                  <td style={{ textAlign: 'center' }}>
                    <span className={`badge ${c.is_active ? 'badge-active' : 'badge-inactive'}`}>{c.is_active ? 'active' : 'inactive'}</span>
                  </td>
                  <td style={{ fontSize: '.8rem', color: 'var(--text-2)' }}>{fmtDate(c.created_at)}</td>
                  <td>
                    <div style={{ display: 'flex', gap: '.35rem', justifyContent: 'flex-end' }}>
                      <button className="btn btn-sm" onClick={() => openManage(c)}>⬡ Manage Items</button>
                      <button className="btn btn-sm" onClick={() => openEdit(c)}>✎ Edit</button>
                      {c.is_active && <button className="btn btn-sm btn-danger" onClick={() => deactivate(c)}>Deactivate</button>}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Create / Edit Modal */}
      {formModal && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setFormModal(false); }}>
          <div className="modal modal-sm">
            <div className="modal-header">
              <div className="modal-title">{editingId ? 'Edit Collection' : 'New Collection'}</div>
              <button className="modal-close" onClick={() => setFormModal(false)}>✕</button>
            </div>
            <div className="modal-body">
              <div className="field"><label>Name *</label><input type="text" maxLength={255} value={form.name} onChange={e => setF({ name: e.target.value })} placeholder="e.g. Summer Drop 2025" /></div>
              <div className="field"><label>Description <small style={{ color: 'var(--text-3)', fontWeight: 400 }}>(optional)</small></label><textarea rows={2} value={form.description} onChange={e => setF({ description: e.target.value })} placeholder="What makes this collection special…" /></div>
              <div className="field">
                <label>Display Order</label>
                <input type="number" min="0" value={form.display_order} onChange={e => setF({ display_order: e.target.value })} />
                <small style={{ color: 'var(--text-3)', fontSize: '.74rem' }}>Lower = shown first in the app</small>
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => setFormModal(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={submitForm} disabled={submitting}>{submitting ? 'Saving…' : editingId ? 'Save Changes' : 'Create Collection'}</button>
            </div>
          </div>
        </div>
      )}

      {/* Manage Items Modal */}
      {manageModal && manageCollection && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setManageModal(false); }}>
          <div className="modal" style={{ maxWidth: '760px', height: '80vh', display: 'flex', flexDirection: 'column' }}>
            <div className="modal-header" style={{ flexShrink: 0 }}>
              <div>
                <div className="modal-title">⬡ {manageCollection.name}</div>
                <div style={{ fontSize: '.76rem', color: 'var(--text-3)', marginTop: '.15rem' }}>Add or remove merch — changes save immediately</div>
              </div>
              <button className="modal-close" onClick={() => setManageModal(false)}>✕</button>
            </div>

            {manageLoading ? (
              <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}><div className="spinner" /></div>
            ) : (
              <div className="modal-body" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1.25rem', flex: 1, minHeight: 0, overflow: 'hidden' }}>
                {/* LEFT — in this collection */}
                <div style={{ display: 'flex', flexDirection: 'column', minHeight: 0 }}>
                  <div style={{ fontFamily: 'var(--font-mono)', fontSize: '.68rem', textTransform: 'uppercase', letterSpacing: '.08em', color: 'var(--text-3)', marginBottom: '.6rem', flexShrink: 0 }}>
                    In collection <span style={{ color: 'var(--accent)' }}>({currentItems.length})</span>
                  </div>
                  <div style={{ flex: 1, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: '.4rem', paddingRight: '.2rem' }}>
                    {currentItems.length === 0
                      ? <div style={{ color: 'var(--text-3)', fontSize: '.82rem', fontStyle: 'italic', padding: '.5rem 0' }}>No items yet — add from the right.</div>
                      : currentItems.map(item => (
                        <div key={item.id} style={{ display: 'flex', alignItems: 'center', gap: '.5rem', padding: '.45rem .6rem', background: 'var(--bg)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)' }}>
                          <div style={{ flex: 1, minWidth: 0 }}>
                            <div style={{ fontSize: '.83rem', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{item.name}</div>
                            <div style={{ fontSize: '.71rem', color: 'var(--text-3)', marginTop: '.08rem' }}>{item.event_title || 'Standalone'}{item.category ? ` · ${item.category}` : ''}</div>
                          </div>
                          <span style={{ fontFamily: 'var(--font-mono)', fontSize: '.76rem', color: 'var(--accent)', flexShrink: 0 }}>{fmtMoney(item.base_price)}</span>
                          <button className="btn btn-sm btn-danger" style={{ flexShrink: 0, padding: '1px 7px', fontSize: '.72rem' }} disabled={saving === item.id} onClick={() => removeFromCollection(item)}>
                            {saving === item.id ? '…' : '✕'}
                          </button>
                        </div>
                      ))
                    }
                  </div>
                </div>

                {/* RIGHT — available to add */}
                <div style={{ display: 'flex', flexDirection: 'column', minHeight: 0 }}>
                  <div style={{ fontFamily: 'var(--font-mono)', fontSize: '.68rem', textTransform: 'uppercase', letterSpacing: '.08em', color: 'var(--text-3)', marginBottom: '.6rem', flexShrink: 0 }}>
                    Add items
                  </div>
                  <input
                    type="text" placeholder="Search by name or event…" value={addSearch}
                    onChange={e => setAddSearch(e.target.value)}
                    style={{ marginBottom: '.6rem', padding: '.38rem .6rem', background: 'var(--bg)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', color: 'var(--text-1)', fontSize: '.83rem', outline: 'none', flexShrink: 0 }}
                  />
                  <div style={{ flex: 1, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: '.4rem', paddingRight: '.2rem' }}>
                    {availableToAdd.length === 0
                      ? <div style={{ color: 'var(--text-3)', fontSize: '.82rem', fontStyle: 'italic', padding: '.5rem 0' }}>
                          {addSearch ? 'No merch matches your search.' : 'All active merch is already in this collection.'}
                        </div>
                      : availableToAdd.map(item => (
                        <div key={item.id} style={{ display: 'flex', alignItems: 'center', gap: '.5rem', padding: '.45rem .6rem', background: 'var(--bg)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)' }}>
                          <div style={{ flex: 1, minWidth: 0 }}>
                            <div style={{ fontSize: '.83rem', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{item.name}</div>
                            <div style={{ fontSize: '.71rem', color: 'var(--text-3)', marginTop: '.08rem' }}>{item.event_title || 'Standalone'}{item.category ? ` · ${item.category}` : ''}</div>
                          </div>
                          <span style={{ fontFamily: 'var(--font-mono)', fontSize: '.76rem', color: 'var(--accent)', flexShrink: 0 }}>{fmtMoney(item.base_price)}</span>
                          <button className="btn btn-sm btn-primary" style={{ flexShrink: 0, padding: '1px 7px', fontSize: '.72rem' }} disabled={saving === item.id} onClick={() => addToCollection(item)}>
                            {saving === item.id ? '…' : '+ Add'}
                          </button>
                        </div>
                      ))
                    }
                  </div>
                </div>
              </div>
            )}

            <div className="modal-footer" style={{ flexShrink: 0 }}>
              <span style={{ fontSize: '.75rem', color: 'var(--text-3)' }}>{currentItems.length} item{currentItems.length !== 1 ? 's' : ''} in collection</span>
              <button className="btn" onClick={() => setManageModal(false)}>Done</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}