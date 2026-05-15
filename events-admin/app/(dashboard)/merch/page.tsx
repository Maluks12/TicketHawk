'use client';
import { useEffect, useState, useMemo } from 'react';
import { api } from '../../../lib/api';
import { fmtMoney, fmtDate, toLocalInput, Badge, formatAttrs } from '../../../lib/utils';
import { useToast } from '../../../lib/toast';
import { useConfirm } from '../../../lib/confirm';

// ─── Types ───────────────────────────────────────────────────────────────────
interface EventOption  { id: string; title: string; status: string; }
interface Collection   { id: string; name: string; description?: string; cover_image?: string; display_order: number; is_active: boolean; organizer_id?: string; created_at: string; updated_at: string; }
interface MerchItem    { id: string; name: string; description?: string; base_price: number; event_id?: string; event_title?: string; category?: string; collection_id?: string; is_active: boolean; fulfillment_mode: string; pickup_location?: string; delivery_fee?: number; sale_start?: string; sale_end?: string; images?: string; display_order: number; variants: Variant[]; }
interface Variant      { id: string; sku: string; attributes: string; stock_quantity: number; price_override?: number; is_active: boolean; }

type Placement = 'standalone' | 'event' | 'collection';
type ActiveTab = 'merch' | 'collections';

// ─── Empty form defaults ─────────────────────────────────────────────────────
const EMPTY_MERCH     = { name: '', description: '', base_price: '', category: '', display_order: '0', fulfillment_mode: 'delivery', pickup_location: '', delivery_fee: '0', sale_start: '', sale_end: '', images: '' };
const EMPTY_VARIANT   = { sku: '', attributes: '', stock_quantity: '0', price_override: '' };

type SizeMode = 'clothing' | 'shoes-uk' | 'shoes-eu' | 'custom';
const CLOTHING_SIZES = ['XS', 'S', 'M', 'L', 'XL', 'XXL', 'XXXL'];
const SHOE_SIZES_UK  = ['3', '4', '5', '6', '7', '8', '9', '10', '11', '12', '13'];
const SHOE_SIZES_EU  = ['35', '36', '37', '38', '39', '40', '41', '42', '43', '44', '45', '46', '47', '48'];
function detectSizeMode(cat: string): SizeMode {
  const c = (cat || '').toLowerCase();
  if (c.includes('shoe') || c.includes('footwear') || c.includes('sneaker') || c.includes('boot') || c.includes('trainer')) return 'shoes-uk';
  return 'clothing';
}
function sizeList(mode: SizeMode) {
  if (mode === 'shoes-uk') return SHOE_SIZES_UK;
  if (mode === 'shoes-eu') return SHOE_SIZES_EU;
  if (mode === 'clothing') return CLOTHING_SIZES;
  return [];
}
const EMPTY_COLL_FORM = { name: '', description: '', display_order: '0', cover_image: '' };

// ─── Shared sub-styles ───────────────────────────────────────────────────────
const placementCard = (active: boolean): React.CSSProperties => ({
  padding: '.65rem .8rem', borderRadius: 'var(--r)', cursor: 'pointer', textAlign: 'left',
  border: `1px solid ${active ? 'var(--accent)' : 'var(--border)'}`,
  background: active ? 'var(--accent-d)' : 'var(--surface-2)',
  transition: 'all .15s',
});

export default function MerchPage() {
  const toast   = useToast();
  const confirm = useConfirm();

  // ── Tab ──────────────────────────────────────────────────────────────────
  const [activeTab, setActiveTab] = useState<ActiveTab>('merch');

  // ── Shared data ──────────────────────────────────────────────────────────
  const [events,      setEvents]      = useState<EventOption[]>([]);
  const [collections, setCollections] = useState<Collection[]>([]);
  const [items,       setItems]       = useState<MerchItem[]>([]);
  const [loadingMerch, setLoadingMerch] = useState(false);
  const [loadingColls, setLoadingColls] = useState(false);

  // ── Merch tab filters ────────────────────────────────────────────────────
  const [search,       setSearch]       = useState('');
  const [eventFilter,  setEventFilter]  = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'inactive'>('all');

  // ── Merch create/edit modal ──────────────────────────────────────────────
  const [merchModal,    setMerchModal]    = useState(false);
  const [editingMerchId, setEditingMerchId] = useState<string | null>(null);
  const [mForm,         setMFormState]   = useState({ ...EMPTY_MERCH });
  const [placement,     setPlacement]    = useState<Placement>('standalone');
  const [plEventId,     setPlEventId]    = useState('');
  const [plCollectionId, setPlCollectionId] = useState('');
  const [mSubmitting,   setMSubmitting]  = useState(false);

  // Inline new-collection creation inside the merch modal
  const [showNewCollInline,  setShowNewCollInline]  = useState(false);
  const [inlineCollName,     setInlineCollName]     = useState('');
  const [inlineCollDesc,     setInlineCollDesc]     = useState('');
  const [inlineCollSaving,   setInlineCollSaving]   = useState(false);

  // ── Variant modal ────────────────────────────────────────────────────────
  const [variantModal,    setVariantModal]    = useState(false);
  const [variantMerchId,  setVariantMerchId]  = useState<string | null>(null);
  const [variantMerchItem,setVariantMerchItem]= useState<MerchItem | null>(null);
  const [vForm,           setVForm]           = useState({ ...EMPTY_VARIANT });
  const [vSubmitting,     setVSubmitting]     = useState(false);
  // size-picker state
  const [sizeMode,        setSizeMode]        = useState<SizeMode>('clothing');
  const [selectedSizes,   setSelectedSizes]   = useState<Set<string>>(new Set());
  const [variantColour,   setVariantColour]   = useState('');
  const [globalStock,     setGlobalStock]     = useState('10');
  const [globalPrice,     setGlobalPrice]     = useState('');
  const [skuPrefix,       setSkuPrefix]       = useState('');

  // edit variant modal
  const [editVariantModal, setEditVariantModal] = useState(false);
  const [editVariant,      setEditVariant]      = useState<Variant | null>(null);
  const [evStock,          setEvStock]          = useState('');
  const [evPrice,          setEvPrice]          = useState('');
  const [evActive,         setEvActive]         = useState(true);
  const [evSubmitting,     setEvSubmitting]     = useState(false);

  // image uploader
  const [uploadedImages,   setUploadedImages]   = useState<string[]>([]);
  const [imgUploading,     setImgUploading]     = useState(false);

  // ── Collections tab state ────────────────────────────────────────────────
  const [collSearch,   setCollSearch]  = useState('');
  const [collFilter,   setCollFilter]  = useState<'all' | 'active' | 'inactive'>('all');
  const [collFormModal, setCollFormModal] = useState(false);
  const [editingCollId, setEditingCollId] = useState<string | null>(null);
  const [collForm,      setCollFormState] = useState({ ...EMPTY_COLL_FORM });
  const [collSubmitting, setCollSubmitting] = useState(false);

  // Manage-items modal
  const [manageModal,      setManageModal]      = useState(false);
  const [manageCollection, setManageCollection] = useState<Collection | null>(null);
  const [collItemsCurrent, setCollItemsCurrent] = useState<MerchItem[]>([]);
  const [manageLoading,    setManageLoading]    = useState(false);
  const [addSearch,        setAddSearch]        = useState('');
  const [saving,           setSaving]           = useState<string | null>(null);

  // ── Initial load ─────────────────────────────────────────────────────────
  useEffect(() => {
    api.get<{ data: EventOption[] }>('/admin/events?limit=100')
      .then(r => setEvents(r.data.data || [])).catch(() => {});
    loadAllMerch();
    loadCollections();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  async function loadAllMerch() {
    setLoadingMerch(true);
    try {
      const { data } = await api.get<{ data: MerchItem[] }>('/admin/merch');
      setItems(data.data || []);
    } catch { toast('Failed to load merchandise', 'error'); }
    finally { setLoadingMerch(false); }
  }

  async function loadCollections() {
    setLoadingColls(true);
    try {
      const { data } = await api.get<{ data: Collection[] }>('/admin/collections');
      setCollections(data.data || []);
    } catch { toast('Failed to load collections', 'error'); }
    finally { setLoadingColls(false); }
  }

  // ── Helpers ──────────────────────────────────────────────────────────────
  function setMF(patch: Partial<typeof EMPTY_MERCH>) { setMFormState(p => ({ ...p, ...patch })); }
  function collectionName(id?: string) { return id ? (collections.find(c => c.id === id)?.name ?? null) : null; }
  const activeCollections = useMemo(() => collections.filter(c => c.is_active), [collections]);

  // ── Merch tab — filtered items ───────────────────────────────────────────
  const filteredItems = useMemo(() => {
    let r = items;
    const q = search.toLowerCase();
    if (q) r = r.filter(m => m.name.toLowerCase().includes(q) || (m.event_title || '').toLowerCase().includes(q) || (m.category || '').toLowerCase().includes(q));
    if (eventFilter === '__standalone__') r = r.filter(m => !m.event_id && !m.collection_id);
    else if (eventFilter) r = r.filter(m => m.event_id === eventFilter);
    if (statusFilter === 'active')   r = r.filter(m => m.is_active);
    if (statusFilter === 'inactive') r = r.filter(m => !m.is_active);
    return r;
  }, [items, search, eventFilter, statusFilter]);

  // ── Merch modal open / close ─────────────────────────────────────────────
  function openCreateMerch() {
    setEditingMerchId(null);
    setMFormState({ ...EMPTY_MERCH });
    setUploadedImages([]);
    setPlacement('standalone');
    setPlEventId('');
    setPlCollectionId('');
    setShowNewCollInline(false);
    setInlineCollName('');
    setInlineCollDesc('');
    setMerchModal(true);
  }

  function openEditMerch(item: MerchItem) {
    setEditingMerchId(item.id);
    setMerchModal(true);
    if (item.event_id)           { setPlacement('event');      setPlEventId(item.event_id);          setPlCollectionId(''); }
    else if (item.collection_id) { setPlacement('collection'); setPlCollectionId(item.collection_id); setPlEventId('');   }
    else                         { setPlacement('standalone'); setPlEventId('');                      setPlCollectionId(''); }
    setShowNewCollInline(false);

    // All data is already in the item passed from the list — no extra API call needed
    setMFormState({
      name:             item.name             || '',
      description:      item.description      || '',
      base_price:       String(item.base_price ?? ''),
      category:         item.category         || '',
      display_order:    String(item.display_order ?? 0),
      fulfillment_mode: item.fulfillment_mode  || 'delivery',
      pickup_location:  item.pickup_location   || '',
      delivery_fee:     String(item.delivery_fee ?? 0),
      sale_start:       toLocalInput(item.sale_start),
      sale_end:         toLocalInput(item.sale_end),
      images:           '',
    });

    // Parse existing images — only keep valid HTTP(S) URLs,
    // and recover from double-encoded arrays or quoted strings
    try {
      const raw = item.images as unknown;
      if (!raw) { setUploadedImages([]); return; }

      const parsed: unknown = typeof raw === 'string' ? JSON.parse(raw) : raw;
      if (!Array.isArray(parsed)) { setUploadedImages([]); return; }

      const isURL = (s: unknown): s is string =>
        typeof s === 'string' && /^https?:\/\/.+/.test(s.trim());

      const urls = (parsed as unknown[]).flatMap((entry): string[] => {
        if (typeof entry !== 'string' || !entry.trim()) return [];

        // Case 1: inner JSON array — e.g. entry = '["https://..."]'
        if (entry.startsWith('[')) {
          try {
            const inner = JSON.parse(entry);
            if (Array.isArray(inner)) return (inner as unknown[]).filter(isURL) as string[];
          } catch {}
        }

        // Case 2: quoted string — e.g. entry = '"https://..."'
        if (entry.startsWith('"') && entry.endsWith('"')) {
          try {
            const unquoted = JSON.parse(entry) as unknown;
            if (isURL(unquoted)) return [unquoted];
          } catch {}
        }

        // Case 3: plain valid URL
        return isURL(entry) ? [entry.trim()] : [];
      });

      setUploadedImages(urls);
    } catch { setUploadedImages([]); }
  }

  // ── Inline collection creation ───────────────────────────────────────────
  async function createCollectionInline() {
    if (!inlineCollName.trim()) { toast('Collection name is required', 'error'); return; }
    setInlineCollSaving(true);
    const nameToFind = inlineCollName.trim();
    try {
      await api.post('/admin/collections', { name: nameToFind, description: inlineCollDesc.trim() || undefined, display_order: 0 });
      toast(`Collection "${nameToFind}" created`, 'success');
      const { data } = await api.get<{ data: Collection[] }>('/admin/collections');
      const allColls = data.data || [];
      setCollections(allColls);
      const newColl = allColls.find(c => c.name === nameToFind && c.is_active);
      if (newColl) setPlCollectionId(newColl.id);
      setShowNewCollInline(false);
      setInlineCollName('');
      setInlineCollDesc('');
    } catch (e: unknown) {
      toast((e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Failed to create collection', 'error');
    } finally { setInlineCollSaving(false); }
  }

  // ── Submit merch ─────────────────────────────────────────────────────────
  async function submitMerch() {
    if (!mForm.name.trim()) { toast('Name is required', 'error'); return; }
    if (!mForm.base_price || isNaN(parseFloat(mForm.base_price))) { toast('Base price is required', 'error'); return; }
    if (placement === 'event' && !plEventId) { toast('Please select an event', 'error'); return; }
    if (placement === 'collection' && !plCollectionId) { toast('Please select or create a collection', 'error'); return; }
    setMSubmitting(true);
    try {
      const toISO = (v: string) => v ? new Date(v).toISOString() : null;
      const body: Record<string, unknown> = {
        name: mForm.name.trim(),
        base_price: parseFloat(mForm.base_price),
        display_order: parseInt(mForm.display_order) || 0,
        fulfillment_mode: mForm.fulfillment_mode,
        delivery_fee: parseFloat(mForm.delivery_fee) || 0,
      };
      if (mForm.description)    body.description    = mForm.description;
      if (mForm.category)       body.category       = mForm.category;
      if (mForm.pickup_location) body.pickup_location = mForm.pickup_location;
      if (mForm.sale_start)     body.sale_start     = toISO(mForm.sale_start);
      if (mForm.sale_end)       body.sale_end       = toISO(mForm.sale_end);
      if (uploadedImages.length > 0) body.images = JSON.stringify(uploadedImages);

      let savedId = editingMerchId;

      if (editingMerchId) {
        await api.put(`/admin/merch/${editingMerchId}`, body);
        toast('Merch updated', 'success');
      } else {
        let res;
        if (placement === 'event' && plEventId) {
          res = await api.post<{ id: string }>(`/organiser/events/${plEventId}/merch`, body);
        } else {
          res = await api.post<{ id: string }>('/admin/merch', body);
        }
        savedId = (res.data as { id?: string })?.id ?? null;
        toast('Merch item created', 'success');
      }

      // Handle collection assignment
      if (savedId) {
        const prevItem = items.find(i => i.id === savedId);
        if (placement === 'collection' && plCollectionId) {
          await api.put(`/admin/merch/${savedId}/collection`, { collection_id: plCollectionId });
        } else if (prevItem?.collection_id && placement !== 'collection') {
          // Item was in a collection, now it's not — clear it
          await api.put(`/admin/merch/${savedId}/collection`, { collection_id: null });
        }
      }

      setMerchModal(false);
      loadAllMerch();
    } catch (e: unknown) {
      toast((e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Save failed', 'error');
    } finally { setMSubmitting(false); }
  }

  async function uploadImage(file: File) {
    setImgUploading(true);
    try {
      const fd = new FormData();
      fd.append('image', file);
      const token   = typeof window !== 'undefined' ? localStorage.getItem('evt_token') : null;
      const baseURL = (process.env.NEXT_PUBLIC_API_URL || '').replace(/\/$/, '');
      const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

      let imageUrl: string | null = null;

      // ── 1. Try R2 via the Go backend ────────────────────────────────────
      try {
        const res = await fetch(`${baseURL}/admin/upload`, { method: 'POST', headers, body: fd });
        if (res.ok) {
          const json = await res.json() as { image_url?: string };
          imageUrl = json.image_url ?? null;
        } else {
          const json = await res.json().catch(() => null) as { error?: string } | null;
          throw new Error(json?.error ?? `Backend returned HTTP ${res.status}`);
        }
      } catch {
        // ── 2. R2 unavailable (local dev) — fall back to Next.js local storage
        const fd2   = new FormData();
        fd2.append('image', file);
        const local = await fetch('/api/upload', { method: 'POST', body: fd2 });
        const json  = await local.json() as { image_url?: string; error?: string };
        if (!local.ok) throw new Error(json.error ?? 'Local upload failed');
        imageUrl = json.image_url ?? null;
      }

      if (imageUrl) {
        setUploadedImages(prev => [...prev, imageUrl!]);
        toast('Image uploaded', 'success');
      } else {
        toast('Upload succeeded but no URL was returned', 'error');
      }
    } catch (e: unknown) {
      toast((e instanceof Error ? e.message : null) ?? 'Image upload failed', 'error');
    } finally { setImgUploading(false); }
  }

  function openEditVariant(v: Variant) {
    setEditVariant(v);
    setEvStock(String(v.stock_quantity));
    setEvPrice(v.price_override != null ? String(v.price_override) : '');
    setEvActive(v.is_active);
    setEditVariantModal(true);
  }

  async function submitEditVariant() {
    if (!editVariant) return;
    setEvSubmitting(true);
    try {
      const body: Record<string, unknown> = { is_active: evActive };
      const stock = parseInt(evStock);
      if (!isNaN(stock) && stock >= 0) body.stock_quantity = stock;
      if (evPrice.trim()) body.price_override = parseFloat(evPrice);
      await api.put(`/admin/merch/variants/${editVariant.id}`, body);
      toast('Variant updated', 'success');
      setEditVariantModal(false);
      loadAllMerch();
    } catch (e: unknown) {
      toast((e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Update failed', 'error');
    } finally { setEvSubmitting(false); }
  }

  function deactivateMerch(id: string, name: string) {
    confirm('Deactivate Merchandise', `"${name}" will be hidden from all listings. Existing orders are not affected.`, async () => {
      await api.delete(`/organiser/merch/${id}`);
      toast('Item deactivated', 'success');
      loadAllMerch();
    });
  }

  // ── Variant modal ────────────────────────────────────────────────────────
  function openVariant(m: MerchItem) {
    setVariantMerchId(m.id);
    setVariantMerchItem(m);
    setVForm({ ...EMPTY_VARIANT });
    const mode = detectSizeMode(m.category || '');
    setSizeMode(mode);
    setSelectedSizes(new Set());
    setVariantColour('');
    setGlobalStock('10');
    setGlobalPrice('');
    // Auto-generate SKU prefix from item name initials
    const prefix = m.name.split(' ').map((w: string) => w[0] || '').join('').toUpperCase().slice(0, 5);
    setSkuPrefix(prefix);
    setVariantModal(true);
  }

  async function submitVariant() {
    setVSubmitting(true);
    try {
      if (sizeMode === 'custom') {
        const attrs = vForm.attributes.trim();
        try { JSON.parse(attrs); } catch { toast('Attributes must be valid JSON, e.g. {"size":"L"}', 'error'); setVSubmitting(false); return; }
        const body: Record<string, unknown> = { sku: vForm.sku.trim(), attributes: attrs, stock_quantity: parseInt(vForm.stock_quantity) || 0 };
        if (vForm.price_override) body.price_override = parseFloat(vForm.price_override);
        await api.post(`/organiser/merch/${variantMerchId}/variants`, body);
        toast('Variant added', 'success');
      } else {
        if (selectedSizes.size === 0) { toast('Select at least one size', 'error'); setVSubmitting(false); return; }
        const failed: string[] = [];
        for (const size of selectedSizes) {
          const attrs: Record<string, string> = { size };
          if (sizeMode === 'shoes-uk') attrs.size_system = 'UK';
          if (sizeMode === 'shoes-eu') attrs.size_system = 'EU';
          if (variantColour.trim()) attrs.colour = variantColour.trim();
          const colPart = variantColour.trim() ? `-${variantColour.trim().slice(0, 3).toUpperCase()}` : '';
          const sku = `${skuPrefix}-${size}${colPart}`.replace(/\s+/g, '-').toUpperCase();
          const body: Record<string, unknown> = { sku, attributes: JSON.stringify(attrs), stock_quantity: parseInt(globalStock) || 0 };
          if (globalPrice) body.price_override = parseFloat(globalPrice);
          try { await api.post(`/organiser/merch/${variantMerchId}/variants`, body); }
          catch { failed.push(size); }
        }
        if (failed.length) toast(`Created with errors on: ${failed.join(', ')}`, 'error');
        else toast(`${selectedSizes.size} size variant${selectedSizes.size !== 1 ? 's' : ''} added`, 'success');
      }
      setVariantModal(false);
      loadAllMerch();
    } catch (e: unknown) {
      toast((e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Failed to add variant', 'error');
    } finally { setVSubmitting(false); }
  }

  // ── Collections tab ──────────────────────────────────────────────────────
  const filteredCollections = useMemo(() => {
    return collections.filter(c => {
      if (collFilter === 'active'   && !c.is_active) return false;
      if (collFilter === 'inactive' &&  c.is_active) return false;
      if (collSearch && !c.name.toLowerCase().includes(collSearch.toLowerCase())) return false;
      return true;
    });
  }, [collections, collFilter, collSearch]);

  function openCreateCollection() { setEditingCollId(null); setCollFormState({ ...EMPTY_COLL_FORM }); setCollFormModal(true); }
  function openEditCollection(c: Collection) {
    setEditingCollId(c.id);
    setCollFormState({ name: c.name, description: c.description || '', display_order: String(c.display_order), cover_image: c.cover_image || '' });
    setCollFormModal(true);
  }

  async function submitCollection() {
    if (!collForm.name.trim()) { toast('Name is required', 'error'); return; }
    setCollSubmitting(true);
    try {
      const body = { name: collForm.name.trim(), description: collForm.description.trim() || undefined, display_order: parseInt(collForm.display_order) || 0, cover_image_url: collForm.cover_image.trim() || undefined };
      if (editingCollId) { await api.put(`/admin/collections/${editingCollId}`, body); toast('Collection updated', 'success'); }
      else               { await api.post('/admin/collections', body);                toast('Collection created', 'success'); }
      setCollFormModal(false); loadCollections();
    } catch (e: unknown) {
      toast((e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Save failed', 'error');
    } finally { setCollSubmitting(false); }
  }

  function deactivateCollection(c: Collection) {
    confirm('Deactivate Collection', `"${c.name}" will be hidden and all its items will be un-assigned (not deleted).`, async () => {
      try { await api.delete(`/admin/collections/${c.id}`); toast('Collection deactivated', 'success'); loadCollections(); }
      catch { toast('Failed to deactivate', 'error'); }
    });
  }

  // ── Manage-items modal ───────────────────────────────────────────────────
  async function openManage(c: Collection) {
    setManageCollection(c); setCollItemsCurrent([]); setAddSearch('');
    setManageModal(true); setManageLoading(true);
    try {
      const { data } = await api.get<{ items: MerchItem[] }>(`/collections/${c.id}`);
      setCollItemsCurrent(data.items || []);
    } catch { toast('Failed to load collection items', 'error'); }
    finally { setManageLoading(false); }
  }

  const currentIds      = useMemo(() => new Set(collItemsCurrent.map(i => i.id)), [collItemsCurrent]);
  const availableToAdd  = useMemo(() => {
    const q = addSearch.toLowerCase();
    return items.filter(m => m.is_active && !currentIds.has(m.id) &&
      (m.name.toLowerCase().includes(q) || (m.event_title || '').toLowerCase().includes(q)));
  }, [items, currentIds, addSearch]);

  async function addToCollection(item: MerchItem) {
    if (!manageCollection) return;
    setSaving(item.id);
    try {
      await api.put(`/admin/merch/${item.id}/collection`, { collection_id: manageCollection.id });
      setCollItemsCurrent(p => [...p, item]);
      toast(`"${item.name}" added`, 'success');
    } catch { toast('Failed to add item', 'error'); }
    finally { setSaving(null); }
  }

  async function removeFromCollection(item: MerchItem) {
    setSaving(item.id);
    try {
      await api.put(`/admin/merch/${item.id}/collection`, { collection_id: null });
      setCollItemsCurrent(p => p.filter(i => i.id !== item.id));
      toast(`"${item.name}" removed`, 'success');
    } catch { toast('Failed to remove item', 'error'); }
    finally { setSaving(null); }
  }

  const showPickup = mForm.fulfillment_mode !== 'delivery';

  // ─────────────────────────────────────────────────────────────────────────
  return (
    <div style={{ padding: '1.5rem', maxWidth: '1400px' }}>

      {/* ── Page header ── */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.25rem' }}>
        <div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.1rem', fontWeight: 600 }}>Merch &amp; Collections</div>
          <div style={{ fontSize: '.8rem', color: 'var(--text-2)', marginTop: '.15rem' }}>Manage merchandise and curated collections in one place</div>
        </div>
        {activeTab === 'merch'
          ? <button className="btn btn-primary btn-sm" onClick={openCreateMerch}>+ Add Merch</button>
          : <button className="btn btn-primary btn-sm" onClick={openCreateCollection}>+ New Collection</button>
        }
      </div>

      {/* ── Tabs ── */}
      <div className="tabs">
        <button className={`tab${activeTab === 'merch' ? ' active' : ''}`} onClick={() => setActiveTab('merch')}>
          ◇ Merchandise {!loadingMerch && `(${items.length})`}
        </button>
        <button className={`tab${activeTab === 'collections' ? ' active' : ''}`} onClick={() => setActiveTab('collections')}>
          ⬡ Collections {!loadingColls && `(${activeCollections.length} active)`}
        </button>
      </div>

      {/* ══════════════════════ MERCH TAB ══════════════════════ */}
      {activeTab === 'merch' && (
        <>
          {/* Filters */}
          <div className="table-toolbar" style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-lg)', marginBottom: '1rem', flexWrap: 'wrap' }}>
            <input className="search-input" type="text" placeholder="Search name, event, category…" value={search} onChange={e => setSearch(e.target.value)} style={{ flex: 1, minWidth: '180px' }} />
            <select className="filter-select" value={eventFilter} onChange={e => setEventFilter(e.target.value)}>
              <option value="">All merch</option>
              <option value="__standalone__">Standalone only</option>
              {events.map(e => <option key={e.id} value={e.id}>{e.title}</option>)}
            </select>
            {(['all', 'active', 'inactive'] as const).map(f => (
              <button key={f} className={`btn btn-sm${statusFilter === f ? ' btn-primary' : ''}`} onClick={() => setStatusFilter(f)}>
                {f.charAt(0).toUpperCase() + f.slice(1)}
              </button>
            ))}
          </div>

          {loadingMerch ? (
            <div style={{ textAlign: 'center', padding: '2rem' }}><div className="spinner" style={{ margin: 'auto' }} /></div>
          ) : filteredItems.length === 0 ? (
            <div style={{ textAlign: 'center', padding: '3rem', color: 'var(--text-3)', fontSize: '.9rem' }}>
              {items.length === 0 ? 'No merch items yet. Click "+ Add Merch" to get started.' : 'No items match your filters.'}
            </div>
          ) : (
            <div className="merch-grid">
              {filteredItems.map(m => (
                <div key={m.id} className="merch-card">
                  <div style={{ padding: '1rem', flex: 1 }}>
                    <div style={{ fontWeight: 500, marginBottom: '.2rem', fontSize: '.9rem', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={m.name}>{m.name}</div>
                    <div style={{ fontFamily: 'var(--font-mono)', color: 'var(--accent)', fontSize: '.9rem', marginBottom: '.5rem' }}>{fmtMoney(m.base_price)}</div>

                    <div style={{ display: 'flex', gap: '.4rem', flexWrap: 'wrap', marginBottom: '.45rem' }}>
                      <Badge status={m.is_active ? 'active' : 'inactive'} />
                      <Badge status={m.fulfillment_mode} />
                      {m.category && <span className="badge badge-draft">{m.category}</span>}
                    </div>

                    {/* Placement indicators */}
                    {m.event_title ? (
                      <div style={{ fontSize: '.74rem', color: 'var(--text-2)', marginBottom: '.25rem', display: 'flex', alignItems: 'center', gap: '.3rem' }}>
                        <span style={{ color: 'var(--text-3)' }}>◈</span>
                        <span style={{ fontFamily: 'var(--font-mono)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{m.event_title}</span>
                      </div>
                    ) : collectionName(m.collection_id) ? (
                      <div style={{ fontSize: '.74rem', color: 'var(--text-2)', marginBottom: '.25rem', display: 'flex', alignItems: 'center', gap: '.3rem' }}>
                        <span style={{ color: 'var(--text-3)' }}>⬡</span>
                        <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--accent)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{collectionName(m.collection_id)}</span>
                      </div>
                    ) : (
                      <div style={{ fontSize: '.74rem', color: 'var(--text-3)', marginBottom: '.25rem' }}>Standalone</div>
                    )}

                    {m.description && <div style={{ fontSize: '.78rem', color: 'var(--text-2)', marginBottom: '.3rem', overflow: 'hidden', display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical' as const }}>{m.description}</div>}
                    <div style={{ fontSize: '.74rem', color: 'var(--text-3)' }}>Sale: {m.sale_start ? `${fmtDate(m.sale_start)} → ${fmtDate(m.sale_end)}` : 'Always live'}</div>

                    {m.variants?.length > 0 && (
                      <div style={{ marginTop: '.6rem', display: 'flex', flexDirection: 'column', gap: '.35rem' }}>
                        {m.variants.map(v => (
                          <div key={v.id} className="variant-item" style={{ cursor: 'pointer' }} onClick={() => openEditVariant(v)}>
                            <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--accent)', flexShrink: 0 }}>{v.sku}</span>
                            <span style={{ color: 'var(--text-2)', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis' }}>{formatAttrs(v.attributes)}</span>
                            <span style={{ fontFamily: 'var(--font-mono)', fontSize: '.78rem', flexShrink: 0 }}>{v.stock_quantity}×</span>
                            {v.price_override != null && <span style={{ fontFamily: 'var(--font-mono)', fontSize: '.72rem', color: 'var(--accent)' }}>{fmtMoney(v.price_override)}</span>}
                            {!v.is_active && <span style={{ fontSize: '.68rem', color: 'var(--danger)' }}>off</span>}
                            <span style={{ fontSize: '.68rem', color: 'var(--text-3)', flexShrink: 0 }}>✎</span>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                  <div style={{ padding: '.65rem 1rem', borderTop: '1px solid var(--border)', display: 'flex', gap: '.4rem', alignItems: 'center', flexWrap: 'wrap' }}>
                    <button className="btn btn-sm" onClick={() => openEditMerch(m)}>✎ Edit</button>
                    <button className="btn btn-sm" onClick={() => openVariant(m)}>+ Variant</button>
                    {m.is_active && <button className="btn btn-sm btn-danger" onClick={() => deactivateMerch(m.id, m.name)}>Deactivate</button>}
                    <span style={{ fontSize: '.74rem', color: 'var(--text-3)', marginLeft: 'auto' }}>{m.variants?.length || 0} variants</span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
      )}

      {/* ══════════════════════ COLLECTIONS TAB ══════════════════════ */}
      {activeTab === 'collections' && (
        <>
          {/* Summary pills */}
          <div style={{ display: 'flex', gap: '1rem', marginBottom: '1rem', flexWrap: 'wrap' }}>
            {[
              { label: 'Total',    value: collections.length,                          accent: false },
              { label: 'Active',   value: activeCollections.length,                    accent: true  },
              { label: 'Inactive', value: collections.length - activeCollections.length, accent: false },
            ].map(s => (
              <div key={s.label} style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-lg)', padding: '.6rem 1rem', minWidth: '90px' }}>
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: '1.3rem', fontWeight: 600, color: s.accent ? 'var(--accent)' : 'var(--text-1)' }}>{s.value}</div>
                <div style={{ fontSize: '.72rem', color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '.06em' }}>{s.label}</div>
              </div>
            ))}
          </div>

          {/* Filters */}
          <div className="table-toolbar" style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-lg)', marginBottom: '1rem', flexWrap: 'wrap' }}>
            <input className="search-input" type="text" placeholder="Search collections…" value={collSearch} onChange={e => setCollSearch(e.target.value)} style={{ flex: 1, minWidth: '180px' }} />
            {(['all', 'active', 'inactive'] as const).map(f => (
              <button key={f} className={`btn btn-sm${collFilter === f ? ' btn-primary' : ''}`} onClick={() => setCollFilter(f)}>
                {f.charAt(0).toUpperCase() + f.slice(1)}
              </button>
            ))}
          </div>

          {loadingColls ? (
            <div style={{ textAlign: 'center', padding: '2rem' }}><div className="spinner" style={{ margin: 'auto' }} /></div>
          ) : filteredCollections.length === 0 ? (
            <div style={{ textAlign: 'center', padding: '3rem', color: 'var(--text-3)', fontSize: '.9rem' }}>
              {collections.length === 0 ? 'No collections yet. Click "+ New Collection" to create one.' : 'No collections match your filter.'}
            </div>
          ) : (
            <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-lg)', overflow: 'hidden' }}>
              <table className="table" style={{ width: '100%' }}>
                <thead>
                  <tr>
                    <th>Cover</th><th>Name</th><th>Description</th>
                    <th style={{ textAlign: 'center' }}>Order</th>
                    <th>Owner</th>
                    <th style={{ textAlign: 'center' }}>Status</th>
                    <th>Created</th>
                    <th style={{ textAlign: 'right' }}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredCollections.map(c => (
                    <tr key={c.id}>
                      <td style={{ padding: '.5rem 1rem', width: '52px' }}>
                        {c.cover_image ? (
                          <div style={{ width: '40px', height: '40px', borderRadius: 'var(--r)', overflow: 'hidden', border: '1px solid var(--border)', flexShrink: 0 }}>
                            <img src={c.cover_image} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} onError={e => { (e.target as HTMLImageElement).style.display = 'none'; }} />
                          </div>
                        ) : (
                          <div style={{ width: '40px', height: '40px', borderRadius: 'var(--r)', border: '1px dashed var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-3)', fontSize: '.8rem' }}>⬡</div>
                        )}
                      </td>
                      <td>
                        <div style={{ fontWeight: 500, fontSize: '.88rem' }}>{c.name}</div>
                        <div style={{ fontFamily: 'var(--font-mono)', fontSize: '.65rem', color: 'var(--text-3)', marginTop: '.1rem' }}>{c.id.slice(0, 8)}…</div>
                      </td>
                      <td style={{ maxWidth: '200px' }}>
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
                          <button className="btn btn-sm" onClick={() => openManage(c)}>⬡ Items</button>
                          <button className="btn btn-sm" onClick={() => openEditCollection(c)}>✎ Edit</button>
                          {c.is_active && <button className="btn btn-sm btn-danger" onClick={() => deactivateCollection(c)}>Deactivate</button>}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}

      {/* ══════════════════════ MERCH MODAL ══════════════════════ */}
      {merchModal && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setMerchModal(false); }}>
          <div className="modal">
            <div className="modal-header">
              <div className="modal-title">{editingMerchId ? 'Edit Merchandise' : 'Add Merchandise'}</div>
              <button className="modal-close" onClick={() => setMerchModal(false)}>✕</button>
            </div>
            <div className="modal-body">

              {/* ── Placement selector (new items only) ── */}
              {!editingMerchId && (
                <>
                  <div className="section-sep">Placement</div>
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '.5rem', marginBottom: '1rem' }}>
                    {([
                      { key: 'standalone', icon: '◇', label: 'Standalone',    desc: 'Independent item — no event or collection' },
                      { key: 'event',      icon: '◈', label: 'Event Merch',   desc: 'Tied to a specific event' },
                      { key: 'collection', icon: '⬡', label: 'In Collection', desc: 'Part of a curated merch collection' },
                    ] as { key: Placement; icon: string; label: string; desc: string }[]).map(opt => (
                      <button
                        key={opt.key}
                        onClick={() => { setPlacement(opt.key); setShowNewCollInline(false); }}
                        style={placementCard(placement === opt.key)}
                      >
                        <div style={{ fontSize: '.78rem', fontWeight: 600, color: placement === opt.key ? 'var(--accent)' : 'var(--text)', marginBottom: '.2rem' }}>
                          {opt.icon} {opt.label}
                        </div>
                        <div style={{ fontSize: '.68rem', color: 'var(--text-3)', lineHeight: 1.4 }}>{opt.desc}</div>
                      </button>
                    ))}
                  </div>

                  {/* Event picker */}
                  {placement === 'event' && (
                    <div className="field">
                      <label>Event *</label>
                      <select value={plEventId} onChange={e => setPlEventId(e.target.value)}>
                        <option value="">— Select an event —</option>
                        {events.map(e => <option key={e.id} value={e.id}>{e.title} ({e.status})</option>)}
                      </select>
                    </div>
                  )}

                  {/* Collection picker + inline creation */}
                  {placement === 'collection' && (
                    <div className="field" style={{ marginBottom: '1rem' }}>
                      <label>Collection *</label>
                      {!showNewCollInline ? (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: '.4rem' }}>
                          <select value={plCollectionId} onChange={e => setPlCollectionId(e.target.value)}>
                            <option value="">— Select a collection —</option>
                            {activeCollections.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
                          </select>
                          <button className="btn btn-sm" style={{ alignSelf: 'flex-start', fontSize: '.76rem' }} onClick={() => setShowNewCollInline(true)}>
                            + Create new collection
                          </button>
                        </div>
                      ) : (
                        <div style={{ background: 'var(--surface-2)', border: '1px solid var(--border)', borderRadius: 'var(--r)', padding: '.8rem' }}>
                          <div style={{ fontFamily: 'var(--font-mono)', fontSize: '.65rem', textTransform: 'uppercase', letterSpacing: '.07em', color: 'var(--text-3)', marginBottom: '.55rem' }}>New Collection</div>
                          <input type="text" placeholder="Collection name *" value={inlineCollName} onChange={e => setInlineCollName(e.target.value)}
                            style={{ width: '100%', background: 'var(--surface-3)', border: '1px solid var(--border)', borderRadius: 'var(--r)', padding: '.45rem .65rem', color: 'var(--text)', fontSize: '.85rem', outline: 'none', marginBottom: '.35rem' }} />
                          <input type="text" placeholder="Description (optional)" value={inlineCollDesc} onChange={e => setInlineCollDesc(e.target.value)}
                            style={{ width: '100%', background: 'var(--surface-3)', border: '1px solid var(--border)', borderRadius: 'var(--r)', padding: '.45rem .65rem', color: 'var(--text)', fontSize: '.85rem', outline: 'none', marginBottom: '.5rem' }} />
                          <div style={{ display: 'flex', gap: '.4rem' }}>
                            <button className="btn btn-sm btn-primary" onClick={createCollectionInline} disabled={inlineCollSaving}>
                              {inlineCollSaving ? 'Creating…' : 'Create Collection'}
                            </button>
                            <button className="btn btn-sm" onClick={() => { setShowNewCollInline(false); setInlineCollName(''); setInlineCollDesc(''); }}>Cancel</button>
                          </div>
                        </div>
                      )}
                    </div>
                  )}
                </>
              )}

              {/* Editing — show current placement as read-only context */}
              {editingMerchId && (
                <div className="alert alert-info" style={{ fontSize: '.76rem', marginBottom: '.75rem' }}>
                  Current placement:&nbsp;
                  <strong>
                    {placement === 'event'      ? `◈ Event merch` :
                     placement === 'collection' ? `⬡ ${collectionName(plCollectionId) || 'Collection'}` :
                     '◇ Standalone'}
                  </strong>
                  &nbsp;— to change placement, deactivate and recreate the item.
                </div>
              )}

              {/* ── Item details ── */}
              <div className="section-sep">Item Details</div>
              <div className="field"><label>Name *</label><input type="text" maxLength={255} value={mForm.name} onChange={e => setMF({ name: e.target.value })} placeholder="e.g. Festival T-Shirt" /></div>
              <div className="field"><label>Description</label><textarea rows={2} value={mForm.description} onChange={e => setMF({ description: e.target.value })} placeholder="Optional description…" /></div>
              <div className="field-row">
                <div className="field"><label>Base Price *</label><input type="number" min="0.01" step="0.01" value={mForm.base_price} onChange={e => setMF({ base_price: e.target.value })} placeholder="0.00" /></div>
                <div className="field">
                  <label>Category <small style={{ color: 'var(--text-3)', textTransform: 'none', fontWeight: 400 }}>(drives colour theme)</small></label>
                  <input type="text" value={mForm.category} onChange={e => setMF({ category: e.target.value })} placeholder="e.g. apparel, footwear" />
                </div>
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
              {/* ── Images ── */}
              <div className="field">
                <label>Images</label>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '.5rem', marginBottom: '.4rem' }}>
                  {uploadedImages.map((url, i) => (
                    <div key={i} style={{ position: 'relative', width: '76px', height: '76px', flexShrink: 0 }}>
                      <img src={url} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: 'var(--r)', border: '1px solid var(--border)', display: 'block' }}
                        onError={e => { (e.target as HTMLImageElement).style.opacity = '0.3'; }} />
                      <button
                        onClick={() => setUploadedImages(prev => prev.filter((_, j) => j !== i))}
                        style={{ position: 'absolute', top: '-5px', right: '-5px', width: '18px', height: '18px', borderRadius: '50%', background: 'var(--danger)', color: '#fff', border: 'none', cursor: 'pointer', fontSize: '10px', display: 'flex', alignItems: 'center', justifyContent: 'center', lineHeight: 1 }}>
                        ✕
                      </button>
                    </div>
                  ))}
                  {/* Upload button */}
                  <label style={{ width: '76px', height: '76px', border: '1px dashed var(--border)', borderRadius: 'var(--r)', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', cursor: imgUploading ? 'wait' : 'pointer', color: 'var(--text-3)', fontSize: '.68rem', gap: '.2rem', background: 'var(--surface-2)', flexShrink: 0 }}>
                    <input type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }} disabled={imgUploading}
                      onChange={e => { const f = e.target.files?.[0]; if (f) { uploadImage(f); e.target.value = ''; } }} />
                    {imgUploading
                      ? <div className="spinner" style={{ width: '18px', height: '18px' }} />
                      : <><span style={{ fontSize: '1.4rem', lineHeight: 1 }}>+</span><span>Upload</span></>
                    }
                  </label>
                </div>
                <small style={{ color: 'var(--text-3)', fontSize: '.72rem' }}>JPEG · PNG · WebP · Max 10 MB each</small>
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => setMerchModal(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={submitMerch} disabled={mSubmitting}>{mSubmitting ? 'Saving…' : editingMerchId ? 'Save Changes' : 'Create Item'}</button>
            </div>
          </div>
        </div>
      )}

      {/* ══════════════════════ VARIANT MODAL ══════════════════════ */}
      {variantModal && (() => {
        // Sizes already added to this item (to grey them out)
        const existingSizes = new Set<string>(
          (variantMerchItem?.variants || []).flatMap(v => {
            try { const a = JSON.parse(v.attributes); return a.size ? [String(a.size)] : []; }
            catch { return []; }
          })
        );
        const sizes = sizeList(sizeMode);
        const skuPreview = selectedSizes.size > 0
          ? [...selectedSizes].slice(0, 3).map(s => {
              const col = variantColour.trim() ? `-${variantColour.trim().slice(0,3).toUpperCase()}` : '';
              return `${skuPrefix}-${s}${col}`.toUpperCase();
            }).join(', ') + (selectedSizes.size > 3 ? '…' : '')
          : '—';

        return (
          <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setVariantModal(false); }}>
            <div className="modal modal-sm">
              <div className="modal-header">
                <div>
                  <div className="modal-title">Add Size Variants</div>
                  <div style={{ fontSize: '.74rem', color: 'var(--text-3)', marginTop: '.1rem' }}>{variantMerchItem?.name}</div>
                </div>
                <button className="modal-close" onClick={() => setVariantModal(false)}>✕</button>
              </div>
              <div className="modal-body">

                {/* ── Mode tabs ── */}
                <div style={{ display: 'flex', gap: '.35rem', marginBottom: '1.1rem', background: 'var(--surface-2)', padding: '.3rem', borderRadius: 'var(--r)', border: '1px solid var(--border)' }}>
                  {([
                    { key: 'clothing',  label: 'Clothing' },
                    { key: 'shoes-uk',  label: 'Shoes (UK)' },
                    { key: 'shoes-eu',  label: 'Shoes (EU)' },
                    { key: 'custom',    label: 'Custom' },
                  ] as { key: SizeMode; label: string }[]).map(opt => (
                    <button key={opt.key} onClick={() => { setSizeMode(opt.key); setSelectedSizes(new Set()); }}
                      style={{
                        flex: 1, padding: '.35rem .3rem', borderRadius: 'calc(var(--r) - 2px)', fontSize: '.76rem',
                        fontWeight: sizeMode === opt.key ? 600 : 400,
                        border: 'none', cursor: 'pointer',
                        background: sizeMode === opt.key ? 'var(--accent)' : 'transparent',
                        color:      sizeMode === opt.key ? '#000' : 'var(--text-2)',
                        transition: 'all .15s',
                      }}>
                      {opt.label}
                    </button>
                  ))}
                </div>

                {sizeMode !== 'custom' ? (
                  <>
                    {/* ── Size grid ── */}
                    <div style={{ fontFamily: 'var(--font-mono)', fontSize: '.64rem', textTransform: 'uppercase', letterSpacing: '.08em', color: 'var(--text-3)', marginBottom: '.5rem' }}>
                      Select sizes <span style={{ color: 'var(--accent)' }}>({selectedSizes.size} selected)</span>
                    </div>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: '.4rem', marginBottom: '1rem' }}>
                      {sizes.map(size => {
                        const isSelected = selectedSizes.has(size);
                        const isExisting = existingSizes.has(size);
                        return (
                          <button key={size} disabled={isExisting}
                            onClick={() => setSelectedSizes(prev => {
                              const next = new Set(prev);
                              if (next.has(size)) next.delete(size); else next.add(size);
                              return next;
                            })}
                            style={{
                              minWidth: '44px', padding: '.35rem .55rem', borderRadius: 'var(--r)', fontSize: '.82rem', fontWeight: 600,
                              cursor: isExisting ? 'not-allowed' : 'pointer',
                              border: `1px solid ${isSelected ? 'var(--accent)' : isExisting ? 'var(--border)' : 'var(--border)'}`,
                              background: isSelected ? 'var(--accent)' : isExisting ? 'var(--surface-2)' : 'var(--surface-3)',
                              color: isSelected ? '#000' : isExisting ? 'var(--text-3)' : 'var(--text)',
                              textDecoration: isExisting ? 'line-through' : 'none',
                              opacity: isExisting ? 0.45 : 1,
                              transition: 'all .12s',
                            }}>
                            {size}
                          </button>
                        );
                      })}
                    </div>
                    <div style={{ display: 'flex', gap: '.4rem', marginBottom: '.5rem' }}>
                      <button className="btn btn-sm" style={{ fontSize: '.74rem' }}
                        onClick={() => setSelectedSizes(new Set(sizes.filter(s => !existingSizes.has(s))))}>
                        Select all
                      </button>
                      <button className="btn btn-sm" style={{ fontSize: '.74rem' }}
                        onClick={() => setSelectedSizes(new Set())}>
                        Clear
                      </button>
                    </div>

                    <div className="section-sep" style={{ margin: '.75rem 0' }} />

                    {/* ── Colour (optional) ── */}
                    <div className="field-row" style={{ marginBottom: '.65rem' }}>
                      <div className="field" style={{ flex: 1.2 }}>
                        <label>Colour <small style={{ color: 'var(--text-3)', fontWeight: 400 }}>(optional)</small></label>
                        <input type="text" value={variantColour} onChange={e => setVariantColour(e.target.value)} placeholder="e.g. Black, Red" />
                      </div>
                      <div className="field" style={{ flex: 1 }}>
                        <label>Stock per size</label>
                        <input type="number" min="0" value={globalStock} onChange={e => setGlobalStock(e.target.value)} />
                      </div>
                      <div className="field" style={{ flex: 1 }}>
                        <label>Price override <small style={{ color: 'var(--text-3)' }}>(opt.)</small></label>
                        <input type="number" min="0.01" step="0.01" value={globalPrice} onChange={e => setGlobalPrice(e.target.value)} placeholder="Base price" />
                      </div>
                    </div>

                    {/* ── SKU prefix ── */}
                    <div className="field" style={{ marginBottom: '.5rem' }}>
                      <label>SKU Prefix <small style={{ color: 'var(--text-3)', fontWeight: 400 }}>— auto-generated per size</small></label>
                      <input type="text" value={skuPrefix} onChange={e => setSkuPrefix(e.target.value.toUpperCase())} placeholder="e.g. TSH" maxLength={12} />
                      <small style={{ color: 'var(--text-3)', fontSize: '.72rem', marginTop: '.25rem', display: 'block' }}>
                        Preview: <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--accent)' }}>{skuPreview}</span>
                      </small>
                    </div>
                  </>
                ) : (
                  /* ── Custom / one-off variant ── */
                  <>
                    <div className="alert alert-info" style={{ marginBottom: '.75rem', fontSize: '.78rem' }}>
                      Enter any attribute as JSON, e.g. <code style={{ fontFamily: 'var(--font-mono)' }}>{`{"size":"L","colour":"Black"}`}</code>
                    </div>
                    <div className="field"><label>SKU *</label><input type="text" maxLength={100} value={vForm.sku} onChange={e => setVForm(p => ({ ...p, sku: e.target.value }))} placeholder="e.g. TSH-L-BLK" /></div>
                    <div className="field"><label>Attributes (JSON) *</label><input type="text" value={vForm.attributes} onChange={e => setVForm(p => ({ ...p, attributes: e.target.value }))} placeholder={`{"size":"L","colour":"Black"}`} /></div>
                    <div className="field-row">
                      <div className="field"><label>Stock Quantity</label><input type="number" min="0" value={vForm.stock_quantity} onChange={e => setVForm(p => ({ ...p, stock_quantity: e.target.value }))} /></div>
                      <div className="field">
                        <label>Price Override <small style={{ color: 'var(--text-3)' }}>(optional)</small></label>
                        <input type="number" min="0.01" step="0.01" value={vForm.price_override} onChange={e => setVForm(p => ({ ...p, price_override: e.target.value }))} placeholder="Leave blank to use base" />
                      </div>
                    </div>
                  </>
                )}
              </div>
              <div className="modal-footer">
                <button className="btn" onClick={() => setVariantModal(false)}>Cancel</button>
                <button className="btn btn-primary" onClick={submitVariant} disabled={vSubmitting}>
                  {vSubmitting
                    ? 'Saving…'
                    : sizeMode === 'custom'
                      ? 'Add Variant'
                      : `Add ${selectedSizes.size > 0 ? selectedSizes.size : ''} Size${selectedSizes.size !== 1 ? 's' : ''}`}
                </button>
              </div>
            </div>
          </div>
        );
      })()}

      {/* ══════════════════════ COLLECTION CREATE/EDIT MODAL ══════════════════════ */}
      {collFormModal && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setCollFormModal(false); }}>
          <div className="modal modal-sm">
            <div className="modal-header">
              <div className="modal-title">{editingCollId ? 'Edit Collection' : 'New Collection'}</div>
              <button className="modal-close" onClick={() => setCollFormModal(false)}>✕</button>
            </div>
            <div className="modal-body">
              <div className="field"><label>Name *</label><input type="text" maxLength={255} value={collForm.name} onChange={e => setCollFormState(p => ({ ...p, name: e.target.value }))} placeholder="e.g. Summer Drop 2025" /></div>
              <div className="field">
                <label>Description <small style={{ color: 'var(--text-3)', fontWeight: 400 }}>(optional)</small></label>
                <textarea rows={2} value={collForm.description} onChange={e => setCollFormState(p => ({ ...p, description: e.target.value }))} placeholder="What makes this collection special…" />
              </div>
              <div className="field">
                <label>Cover Image <small style={{ color: 'var(--text-3)', fontWeight: 400 }}>(optional — URL)</small></label>
                <input type="text" value={collForm.cover_image} onChange={e => setCollFormState(p => ({ ...p, cover_image: e.target.value }))} placeholder="https://example.com/cover.jpg" />
                {collForm.cover_image.trim() && (
                  <div style={{ marginTop: '.6rem', borderRadius: 'var(--r)', overflow: 'hidden', border: '1px solid var(--border)', maxWidth: '100%', height: '140px', background: 'var(--surface-2)', position: 'relative' }}>
                    <img
                      src={collForm.cover_image.trim()}
                      alt="Cover preview"
                      style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
                      onError={e => { (e.target as HTMLImageElement).style.opacity = '0'; }}
                      onLoad={e  => { (e.target as HTMLImageElement).style.opacity = '1'; }}
                    />
                    <div style={{ position: 'absolute', bottom: '.4rem', right: '.5rem', fontFamily: 'var(--font-mono)', fontSize: '.6rem', color: 'var(--text-3)', background: 'var(--bg)', borderRadius: 'var(--r)', padding: '2px 6px', opacity: .8 }}>preview</div>
                  </div>
                )}
              </div>
              <div className="field">
                <label>Display Order</label>
                <input type="number" min="0" value={collForm.display_order} onChange={e => setCollFormState(p => ({ ...p, display_order: e.target.value }))} />
                <small style={{ color: 'var(--text-3)', fontSize: '.74rem' }}>Lower = shown first in the app</small>
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => setCollFormModal(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={submitCollection} disabled={collSubmitting}>
                {collSubmitting ? 'Saving…' : editingCollId ? 'Save Changes' : 'Create Collection'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ══════════════════════ MANAGE COLLECTION ITEMS MODAL ══════════════════════ */}
      {manageModal && manageCollection && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setManageModal(false); }}>
          <div className="modal" style={{ maxWidth: '760px', height: '80vh', display: 'flex', flexDirection: 'column' }}>
            <div className="modal-header" style={{ flexShrink: 0 }}>
              <div>
                <div className="modal-title">⬡ {manageCollection.name}</div>
                <div style={{ fontSize: '.76rem', color: 'var(--text-3)', marginTop: '.15rem' }}>Add or remove merch — changes apply immediately</div>
              </div>
              <button className="modal-close" onClick={() => setManageModal(false)}>✕</button>
            </div>

            {manageLoading ? (
              <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}><div className="spinner" /></div>
            ) : (
              <div className="modal-body" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1.25rem', flex: 1, minHeight: 0, overflow: 'hidden' }}>
                {/* LEFT — in collection */}
                <div style={{ display: 'flex', flexDirection: 'column', minHeight: 0 }}>
                  <div style={{ fontFamily: 'var(--font-mono)', fontSize: '.68rem', textTransform: 'uppercase', letterSpacing: '.08em', color: 'var(--text-3)', marginBottom: '.6rem', flexShrink: 0 }}>
                    In collection <span style={{ color: 'var(--accent)' }}>({collItemsCurrent.length})</span>
                  </div>
                  <div style={{ flex: 1, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: '.4rem', paddingRight: '.2rem' }}>
                    {collItemsCurrent.length === 0
                      ? <div style={{ color: 'var(--text-3)', fontSize: '.82rem', fontStyle: 'italic', padding: '.5rem 0' }}>No items yet — add from the right.</div>
                      : collItemsCurrent.map(item => (
                        <div key={item.id} style={{ display: 'flex', alignItems: 'center', gap: '.5rem', padding: '.45rem .6rem', background: 'var(--bg)', border: '1px solid var(--border)', borderRadius: 'var(--r)' }}>
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
                  <div style={{ fontFamily: 'var(--font-mono)', fontSize: '.68rem', textTransform: 'uppercase', letterSpacing: '.08em', color: 'var(--text-3)', marginBottom: '.6rem', flexShrink: 0 }}>Add items</div>
                  <input type="text" placeholder="Search by name or event…" value={addSearch} onChange={e => setAddSearch(e.target.value)}
                    style={{ marginBottom: '.6rem', padding: '.38rem .6rem', background: 'var(--bg)', border: '1px solid var(--border)', borderRadius: 'var(--r)', color: 'var(--text-1)', fontSize: '.83rem', outline: 'none', flexShrink: 0 }} />
                  <div style={{ flex: 1, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: '.4rem', paddingRight: '.2rem' }}>
                    {availableToAdd.length === 0
                      ? <div style={{ color: 'var(--text-3)', fontSize: '.82rem', fontStyle: 'italic', padding: '.5rem 0' }}>
                          {addSearch ? 'No merch matches your search.' : 'All active merch is already in this collection.'}
                        </div>
                      : availableToAdd.map(item => (
                        <div key={item.id} style={{ display: 'flex', alignItems: 'center', gap: '.5rem', padding: '.45rem .6rem', background: 'var(--bg)', border: '1px solid var(--border)', borderRadius: 'var(--r)' }}>
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
              <span style={{ fontSize: '.75rem', color: 'var(--text-3)' }}>{collItemsCurrent.length} item{collItemsCurrent.length !== 1 ? 's' : ''} in collection</span>
              <button className="btn" onClick={() => setManageModal(false)}>Done</button>
            </div>
          </div>
        </div>
      )}
      {editVariantModal && editVariant && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setEditVariantModal(false); }}>
          <div className="modal modal-sm">
            <div className="modal-header">
              <div>
                <div className="modal-title">Edit Variant</div>
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: '.74rem', color: 'var(--accent)', marginTop: '.1rem' }}>{editVariant.sku}</div>
              </div>
              <button className="modal-close" onClick={() => setEditVariantModal(false)}>✕</button>
            </div>
            <div className="modal-body">
              {/* Read-only attributes */}
              <div className="field">
                <label>Attributes <small style={{ color: 'var(--text-3)', fontWeight: 400 }}>(read-only)</small></label>
                <div style={{ padding: '.45rem .65rem', background: 'var(--surface-2)', border: '1px solid var(--border)', borderRadius: 'var(--r)', fontFamily: 'var(--font-mono)', fontSize: '.82rem', color: 'var(--text-2)' }}>
                  {formatAttrs(editVariant.attributes)}
                </div>
              </div>
              <div className="field-row">
                <div className="field">
                  <label>Stock Quantity</label>
                  <input type="number" min="0" value={evStock} onChange={e => setEvStock(e.target.value)} />
                </div>
                <div className="field">
                  <label>Price Override <small style={{ color: 'var(--text-3)' }}>(opt.)</small></label>
                  <input type="number" min="0.01" step="0.01" value={evPrice} onChange={e => setEvPrice(e.target.value)} placeholder="Use base price" />
                </div>
              </div>
              <div className="field">
                <label>Status</label>
                <div style={{ display: 'flex', gap: '.5rem' }}>
                  {[true, false].map(val => (
                    <button key={String(val)} onClick={() => setEvActive(val)}
                      style={{ flex: 1, padding: '.4rem', borderRadius: 'var(--r)', border: `1px solid ${evActive === val ? (val ? 'var(--accent)' : 'var(--danger)') : 'var(--border)'}`, background: evActive === val ? (val ? 'var(--accent-d)' : 'rgba(255,59,48,.15)') : 'var(--surface-2)', color: evActive === val ? (val ? 'var(--accent)' : 'var(--danger)') : 'var(--text-2)', cursor: 'pointer', fontSize: '.82rem', fontWeight: evActive === val ? 600 : 400 }}>
                      {val ? 'Active' : 'Inactive'}
                    </button>
                  ))}
                </div>
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => setEditVariantModal(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={submitEditVariant} disabled={evSubmitting}>
                {evSubmitting ? 'Saving…' : 'Save Changes'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}