'use client';
import { useState, useEffect, useCallback } from 'react';
import { api } from '../../../lib/api';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';
import { fmtMoney, fmtDate } from '../../../lib/utils';

type Tab = 'merch' | 'collections';

interface MerchItem {
  id: string; name: string; status: string; price: number; currency: string;
  stock_quantity: number | null; event_id: string | null; fulfillment_type: string;
  sale_start: string | null; sale_end: string | null; created_at: string;
}

interface Collection {
  id: string; name: string; slug: string; status: string;
  description: string | null; cover_image_url: string | null;
  item_count: number; created_at: string;
}

export default function MerchPage() {
  const toast = useToast();
  const confirm = useConfirm();
  const [tab, setTab] = useState<Tab>('merch');
  const [merch, setMerch] = useState<MerchItem[]>([]);
  const [collections, setCollections] = useState<Collection[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [showCreateCollection, setShowCreateCollection] = useState(false);

  const fetchMerch = useCallback(async () => {
    try {
      const res = await api.get('/organiser/merch');
      setMerch(res.data.merch ?? []);
    } catch { /* handled below */ }
  }, []);

  const fetchCollections = useCallback(async () => {
    try {
      const res = await api.get('/organiser/collections');
      setCollections(res.data.collections ?? []);
    } catch { /* handled below */ }
  }, []);

  useEffect(() => {
    setLoading(true);
    Promise.all([fetchMerch(), fetchCollections()])
      .catch(() => toast('Failed to load merch data', 'error'))
      .finally(() => setLoading(false));
  }, []);

  async function deactivateMerch(id: string) {
    confirm('Deactivate item', 'This will hide the item from customers.', async () => {
      await api.delete(`/organiser/merch/${id}`);
      toast('Item deactivated', 'success');
      fetchMerch();
    });
  }

  async function deactivateCollection(id: string) {
    confirm('Deactivate collection', 'This will hide the collection from customers.', async () => {
      await api.delete(`/organiser/collections/${id}`);
      toast('Collection deactivated', 'success');
      fetchCollections();
    });
  }

  const filteredMerch = merch.filter(m =>
    !search || m.name.toLowerCase().includes(search.toLowerCase())
  );
  const filteredCollections = collections.filter(c =>
    !search || c.name.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <div className="page-title">Merch</div>
          <div style={{ fontSize: 13, color: '#606770', marginTop: 2 }}>
            {merch.length} items · {collections.length} collections
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          {tab === 'collections' && (
            <button className="btn btn-primary" onClick={() => setShowCreateCollection(true)}>
              + New collection
            </button>
          )}
        </div>
      </div>

      {/* Tab bar */}
      <div className="tab-bar">
        <button className={`tab-item${tab === 'merch' ? ' active' : ''}`} onClick={() => setTab('merch')}>
          Merch items
          {merch.length > 0 && <span style={{ marginLeft: 6, background: tab === 'merch' ? '#1877f2' : '#606770', color: '#fff', borderRadius: 99, padding: '1px 6px', fontSize: 11 }}>{merch.length}</span>}
        </button>
        <button className={`tab-item${tab === 'collections' ? ' active' : ''}`} onClick={() => setTab('collections')}>
          Collections
          {collections.length > 0 && <span style={{ marginLeft: 6, background: tab === 'collections' ? '#1877f2' : '#606770', color: '#fff', borderRadius: 99, padding: '1px 6px', fontSize: 11 }}>{collections.length}</span>}
        </button>
        <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', paddingBottom: 2 }}>
          <div className="search-box" style={{ minWidth: 180 }}>
            <svg viewBox="0 0 20 20" fill="#606770" width="14" height="14">
              <path fillRule="evenodd" d="M8 4a4 4 0 100 8 4 4 0 000-8zM2 8a6 6 0 1110.89 3.476l4.817 4.817a1 1 0 01-1.414 1.414l-4.816-4.816A6 6 0 012 8z" clipRule="evenodd"/>
            </svg>
            <input value={search} onChange={e => setSearch(e.target.value)}
              placeholder={tab === 'merch' ? 'Search items…' : 'Search collections…'} />
          </div>
        </div>
      </div>

      <div style={{ flex: 1, overflow: 'auto', background: '#fff' }}>
        {loading ? (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: 200 }}>
            <div className="spinner" />
          </div>
        ) : tab === 'merch' ? (
          filteredMerch.length === 0 ? (
            <div className="empty-state">
              <div className="empty-icon">👕</div>
              <div className="empty-title">No merch items yet</div>
              <div className="empty-desc">Merch items are added per event from the event detail page.</div>
            </div>
          ) : (
            <table className="meta-table">
              <thead>
                <tr>
                  <th>Item</th>
                  <th>Status</th>
                  <th>Price</th>
                  <th>Fulfillment</th>
                  <th>Stock</th>
                  <th>Sale window</th>
                  <th className="col-actions">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filteredMerch.map(m => (
                  <tr key={m.id}>
                    <td>
                      <div style={{ fontWeight: 600 }}>{m.name}</div>
                      <div style={{ fontSize: 11, color: '#8a8d91' }}>
                        {m.event_id ? 'Event merch' : 'Standalone'}
                      </div>
                    </td>
                    <td><span className={`badge badge-${m.status}`}>{m.status}</span></td>
                    <td style={{ fontWeight: 600 }}>{fmtMoney(m.price, m.currency)}</td>
                    <td style={{ color: '#606770', fontSize: 13 }}>{m.fulfillment_type ?? '—'}</td>
                    <td>{m.stock_quantity ?? 'Unlimited'}</td>
                    <td style={{ fontSize: 12, color: '#606770' }}>
                      {m.sale_start ? `${fmtDate(m.sale_start)} → ${m.sale_end ? fmtDate(m.sale_end) : '∞'}` : 'Always'}
                    </td>
                    <td>
                      <div style={{ display: 'flex', gap: 4, justifyContent: 'flex-end' }}>
                        <button className="btn btn-xs btn-danger" onClick={() => deactivateMerch(m.id)}>
                          Deactivate
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )
        ) : (
          filteredCollections.length === 0 ? (
            <div className="empty-state">
              <div className="empty-icon">📦</div>
              <div className="empty-title">No collections yet</div>
              <div className="empty-desc">Group your merch items into collections for a better shopping experience.</div>
              <button className="btn btn-primary" style={{ marginTop: 8 }} onClick={() => setShowCreateCollection(true)}>
                Create your first collection
              </button>
            </div>
          ) : (
            <table className="meta-table">
              <thead>
                <tr>
                  <th>Collection</th>
                  <th>Status</th>
                  <th>Items</th>
                  <th>Created</th>
                  <th className="col-actions">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filteredCollections.map(c => (
                  <tr key={c.id}>
                    <td>
                      <div style={{ fontWeight: 600 }}>{c.name}</div>
                      {c.description && <div style={{ fontSize: 12, color: '#8a8d91', marginTop: 2 }}>{c.description.slice(0, 60)}…</div>}
                    </td>
                    <td><span className={`badge badge-${c.status}`}>{c.status}</span></td>
                    <td>{c.item_count ?? 0}</td>
                    <td style={{ color: '#606770' }}>{fmtDate(c.created_at)}</td>
                    <td>
                      <div style={{ display: 'flex', gap: 4, justifyContent: 'flex-end' }}>
                        <button className="btn btn-xs btn-danger" onClick={() => deactivateCollection(c.id)}>
                          Deactivate
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )
        )}
      </div>

      {showCreateCollection && (
        <CreateCollectionModal
          onClose={() => setShowCreateCollection(false)}
          onCreated={() => { setShowCreateCollection(false); fetchCollections(); toast('Collection created', 'success'); }}
        />
      )}
    </div>
  );
}

function CreateCollectionModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  async function handleSubmit() {
    if (!name.trim()) { setError('Name is required'); return; }
    setSubmitting(true); setError('');
    try {
      await api.post('/organiser/collections', { name: name.trim(), description: description.trim() || undefined });
      onCreated();
    } catch (e: unknown) {
      setError((e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? 'Failed to create collection');
    } finally { setSubmitting(false); }
  }

  return (
    <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="modal modal-sm">
        <div className="modal-header">
          <div className="modal-title">New collection</div>
          <button className="btn-icon" onClick={onClose}>✕</button>
        </div>
        <div className="modal-body">
          <div className="field">
            <label>Collection name</label>
            <input className="field input" value={name} onChange={e => setName(e.target.value)} autoFocus placeholder="e.g. Summer Drop 2025" style={{ width: '100%', padding: '9px 12px', border: '1px solid #ccd0d5', borderRadius: 6, fontSize: 14, outline: 'none', fontFamily: 'inherit' }} />
          </div>
          <div className="field" style={{ marginBottom: 0 }}>
            <label>Description (optional)</label>
            <textarea value={description} onChange={e => setDescription(e.target.value)} placeholder="Brief description of this collection…" rows={3} style={{ width: '100%', padding: '9px 12px', border: '1px solid #ccd0d5', borderRadius: 6, fontSize: 14, outline: 'none', fontFamily: 'inherit', resize: 'vertical' }} />
          </div>
          {error && <div style={{ color: '#c41c1c', fontSize: 13, marginTop: 8 }}>{error}</div>}
        </div>
        <div className="modal-footer">
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" onClick={handleSubmit} disabled={submitting}>
            {submitting ? 'Creating…' : 'Create collection'}
          </button>
        </div>
      </div>
    </div>
  );
}