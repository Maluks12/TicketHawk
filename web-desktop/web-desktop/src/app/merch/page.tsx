'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { ShoppingCart, X, ChevronRight, ChevronLeft } from 'lucide-react';
import { Package, Check, ShoppingBag, HourglassMedium, WarningCircle } from '@phosphor-icons/react';
import {
  getDropsFeed, addMerchToCart, getCachedSessionId,
  type MerchandiseItem,
} from '@/lib/api';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';
import { CollectionModal } from '@/components/collectionModal';

// ── Palette ───────────────────────────────────────────────────────────────────
const PALETTE: Record<string, { bg: string; text: string }> = {
  sneakers:    { bg: '#0d0d0d', text: '#ffffff' },
  apparel:     { bg: '#1a3a5c', text: '#ffffff' },
  culture:     { bg: '#1a6b3c', text: '#ffffff' },
  drops:       { bg: '#1a8fe8', text: '#000000' },
  accessories: { bg: '#4a1a6b', text: '#ffffff' },
  jersey:      { bg: '#7b1c2a', text: '#ffffff' },
  footwear:    { bg: '#2d2d2d', text: '#ffffff' },
  collectible: { bg: '#cc2200', text: '#ffffff' },
  default:     { bg: '#1a1a1a', text: '#ffffff' },
};

function paletteFor(item: MerchandiseItem) {
  const key = (item.category ?? 'default').toLowerCase();
  return PALETTE[key] ?? PALETTE.default;
}

// ── Helpers ───────────────────────────────────────────────────────────────────
function parseImages(raw?: string): string[] {
  if (!raw) return [];
  try {
    const p = JSON.parse(raw);
    return Array.isArray(p) ? p.filter(Boolean) : [];
  } catch { return []; }
}

function fmtPrice(p: number) { return `R${Math.round(p).toLocaleString('en-ZA')}`; }

function formatCountdown(total: number): string {
  const d = Math.floor(total / 86400);
  const h = Math.floor((total % 86400) / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (d > 0) return `${d}D  ${String(h).padStart(2,'0')}H  ${String(m).padStart(2,'0')}M`;
  return `${String(h).padStart(2,'0')}:${String(m).padStart(2,'0')}:${String(s).padStart(2,'0')}`;
}

// ── Countdown hook ────────────────────────────────────────────────────────────
function useCountdown(initial: number) {
  const [secs, setSecs] = useState(initial);
  useEffect(() => {
    if (secs <= 0) return;
    const t = setInterval(() => setSecs((s) => Math.max(0, s - 1)), 1000);
    return () => clearInterval(t);
  }, []);
  return secs;
}

// ── Variant helpers ───────────────────────────────────────────────────────────
type Variant = NonNullable<MerchandiseItem['variants']>[number];

function getVariantLabel(v: Variant): string {
  try {
    const attr = JSON.parse(v.attributes) as Record<string, string>;
    return Object.values(attr).join(' / ');
  } catch { return v.sku; }
}

// ── Collection grouping ───────────────────────────────────────────────────────
interface CollectionEntry {
  representativeItem: MerchandiseItem;
  items: MerchandiseItem[];
  isCollection: boolean;
  collectionName?: string;
  collectionCoverImageUrl?: string;
}

function buildCollections(items: MerchandiseItem[]): CollectionEntry[] {
  const collMap = new Map<string, MerchandiseItem[]>();
  const evtMap  = new Map<string, MerchandiseItem[]>();
  for (const item of items) {
    if (item.collection_id) {
      if (!collMap.has(item.collection_id)) collMap.set(item.collection_id, []);
      collMap.get(item.collection_id)!.push(item);
    } else {
      const key = item.event_id || item.id;
      if (!evtMap.has(key)) evtMap.set(key, []);
      evtMap.get(key)!.push(item);
    }
  }
  const entries: CollectionEntry[] = [];
  collMap.forEach((group) => {
    const sorted = [...group].sort((a, b) => a.display_order - b.display_order);
    entries.push({
      representativeItem:      sorted[0],
      items:                   sorted,
      isCollection:            true,
      collectionName:          sorted[0].collection_name,
      collectionCoverImageUrl: sorted[0].collection_cover_image_url || undefined,
    });
  });
  evtMap.forEach((group) => {
    const sorted = [...group].sort((a, b) => a.display_order - b.display_order);
    entries.push({ representativeItem: sorted[0], items: sorted, isCollection: sorted.length > 1 });
  });
  entries.sort((a, b) => a.representativeItem.display_order - b.representativeItem.display_order);
  return entries;
}

// ── Image collage ─────────────────────────────────────────────────────────────
function Collage({ images }: { images: string[] }) {
  if (!images.length) {
    return <div className="w-full aspect-square rounded-[10px] bg-white/5 flex items-center justify-center"><Package size={36} weight="regular" /></div>;
  }
  if (images.length === 1) {
    return (
      <div className="w-full aspect-square rounded-[10px] overflow-hidden">
        <img src={images[0]} alt="" className="w-full h-full object-cover" />
      </div>
    );
  }
  return (
    <div className="w-full aspect-square rounded-[10px] overflow-hidden flex gap-0.5">
      <img src={images[0]} alt="" className="flex-1 h-full object-cover" />
      <div className="w-[45%] flex flex-col gap-0.5">
        {images.slice(1, 3).map((u, i) => (
          <img key={i} src={u} alt="" className="flex-1 w-full object-cover" />
        ))}
      </div>
    </div>
  );
}

// ── Available card ────────────────────────────────────────────────────────────
function AvailableCard({ entry, onPress }: { entry: CollectionEntry; onPress: () => void }) {
  const item = entry.representativeItem;
  const { bg, text: tc } = paletteFor(item);
  const images  = entry.isCollection && entry.collectionCoverImageUrl
    ? [entry.collectionCoverImageUrl]
    : parseImages(item.images);
  const remain  = useCountdown(item.seconds_remain ?? 0);
  const urgent  = !!item.sale_end && remain < 86400;
  const title   = entry.isCollection
    ? (entry.collectionName || item.event_name || item.name) : item.name;

  return (
    <div
      onClick={onPress}
      className="w-full rounded-[14px] overflow-hidden text-left transition-transform hover:scale-[1.01] active:scale-[0.99] cursor-pointer"
      style={{ backgroundColor: bg }}
    >
      {/* Top bar */}
      <div className="flex items-center gap-2 px-4 pt-4 pb-3">
        <span className="text-[10px] font-black tracking-[1.5px]" style={{ color: tc }}>
          {(item.category ?? 'DROP').toUpperCase()}
        </span>
        {entry.isCollection && (
          <span
            className="border px-1.5 py-0.5 text-[10px] font-black rounded-sm ml-1"
            style={{ borderColor: `${tc}50`, color: tc }}
          >
            {entry.items.length} ITEMS
          </span>
        )}
        <span className="flex-1" />
        {!!item.sale_end && (
          <div className="border px-2 py-1 rounded-sm text-right" style={{ borderColor: urgent ? '#ff3c6e' : `${tc}40` }}>
            <p className="text-[8px] font-black tracking-wider" style={{ color: urgent ? '#ff3c6e' : `${tc}80` }}>ENDS IN</p>
            <p className="text-[11px] font-black font-mono" style={{ color: urgent ? '#ff3c6e' : tc }}>{formatCountdown(remain)}</p>
          </div>
        )}
      </div>

      {/* Image */}
      <div className="mx-4"><Collage images={images} /></div>

      {/* Title */}
      <p className="px-4 pt-3 pb-1 text-lg font-black leading-snug line-clamp-2" style={{ color: tc }}>{title}</p>

      {/* Bottom */}
      <div
        className="flex items-center gap-3 px-4 pb-4 pt-3 mt-1 border-t"
        style={{ borderColor: `${tc}25` }}
      >
        <span className="text-[10px] font-bold tracking-wide" style={{ color: `${tc}aa` }}>
          {entry.isCollection ? `${entry.items.length} ITEMS` : (item.category ?? 'MERCHANDISE').toUpperCase()}
        </span>
        <span className="flex-1" />
        <button
          className="border px-4 py-2 rounded-sm text-[11px] font-black tracking-wide"
          style={{ borderColor: `${tc}60`, color: tc }}
          onClick={onPress}
        >
          {entry.isCollection ? 'VIEW ALL' : `BUY  ${fmtPrice(item.effective_price)}`}
        </button>
      </div>
    </div>
  );
}

// ── Upcoming card ─────────────────────────────────────────────────────────────
function UpcomingCard({ entry, onPress }: { entry: CollectionEntry; onPress: () => void }) {
  const item = entry.representativeItem;
  const { bg, text: tc } = paletteFor(item);
  const images = entry.isCollection && entry.collectionCoverImageUrl
    ? [entry.collectionCoverImageUrl]
    : parseImages(item.images);
  const until = useCountdown(item.seconds_until ?? 0);
  const title = entry.isCollection
    ? (entry.collectionName || item.event_name || item.name) : item.name;

  return (
    <button
      onClick={onPress}
      className="w-full rounded-[14px] overflow-hidden text-left transition-transform hover:scale-[1.01] active:scale-[0.99]"
      style={{ backgroundColor: bg }}
    >
      <div className="flex items-center gap-2 px-4 pt-4 pb-2">
        <span className="text-[10px] font-black tracking-[1.5px]" style={{ color: tc }}>
          {(item.category ?? 'DROP').toUpperCase()}
        </span>
        {entry.isCollection && (
          <span className="border px-1.5 py-0.5 text-[10px] font-black rounded-sm ml-1" style={{ borderColor: `${tc}50`, color: tc }}>
            {entry.items.length} ITEMS
          </span>
        )}
        <span className="flex-1" />
        <span className="text-[10px] font-bold" style={{ color: `${tc}80` }}>DROPS IN</span>
      </div>

      {/* Countdown banner */}
      <div className="mx-4 border-y py-2.5" style={{ borderColor: `${tc}15` }}>
        <p className="text-2xl font-black font-mono tracking-widest text-center" style={{ color: tc }}>
          {formatCountdown(until)}
        </p>
      </div>

      <div className="mx-4 mt-3"><Collage images={images} /></div>
      <p className="px-4 pt-3 pb-1 text-lg font-black leading-snug line-clamp-2" style={{ color: tc }}>{title}</p>

      <div className="flex items-center gap-3 px-4 pb-4 pt-3 mt-1 border-t" style={{ borderColor: `${tc}25` }}>
        <span className="text-[10px] font-bold tracking-wide" style={{ color: `${tc}aa` }}>
          {entry.isCollection ? `${entry.items.length} ITEMS` : (item.category ?? 'MERCHANDISE').toUpperCase()}
        </span>
        <span className="flex-1" />
        {item.sale_start && (
          <span className="text-[10px] font-bold" style={{ color: `${tc}80` }}>
            {new Date(item.sale_start).toLocaleDateString('en-ZA', { day:'2-digit', month:'short', year:'numeric' }).toUpperCase()}
          </span>
        )}
        <button
          className="border px-4 py-2 rounded-sm text-[11px] font-black tracking-wide ml-2"
          style={{ borderColor: `${tc}60`, color: tc }}
          onClick={onPress}
        >
          {entry.isCollection ? 'VIEW ALL' : 'SET ALERT'}
        </button>
      </div>
    </button>
  );
}

// ── Detail drawer ─────────────────────────────────────────────────────────────
function DetailDrawer({ item, onClose }: { item: MerchandiseItem; onClose: () => void }) {
  const { bg, text: tc } = paletteFor(item);
  const { user } = useAuthStore();
  const router = useRouter();
  const images = parseImages(item.images);
  const [selectedVariant, setSelectedVariant] = useState<Variant | null>(null);
  const [adding, setAdding] = useState(false);
  const [added, setAdded] = useState(false);
  const [imgIdx, setImgIdx] = useState(0);
  const imgDragStart = useCallback((x: number) => { (window as any).__imgDragX = x; }, []);
  const imgDragEnd = useCallback((x: number) => {
    const start = (window as any).__imgDragX;
    if (start == null) return;
    const delta = start - x;
    if (delta > 50 && imgIdx < images.length - 1) setImgIdx((i) => i + 1);
    else if (delta < -50 && imgIdx > 0) setImgIdx((i) => i - 1);
    (window as any).__imgDragX = null;
  }, [imgIdx, images.length]);
  const remain = useCountdown(item.seconds_remain ?? 0);
  const until  = useCountdown(item.seconds_until ?? 0);
  const isLive = item.sale_status === 'live';

  // Keyboard navigation for images & close
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'ArrowLeft')  setImgIdx((i) => Math.max(0, i - 1));
      if (e.key === 'ArrowRight') setImgIdx((i) => Math.min(images.length - 1, i + 1));
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [images.length, onClose]);

  const displayPrice = selectedVariant?.price_override != null
    ? selectedVariant.price_override : item.effective_price;

  const variants = item.variants ?? [];
  const hasVariants = variants.length > 0;
  const buyDisabled = isLive && hasVariants && !selectedVariant;

  const handleBuy = async () => {
    if (!user) { router.push('/auth/login'); return; }
    setAdding(true);
    try {
      const sid = await getCachedSessionId();
      await addMerchToCart({
        item_type: 'merchandise',
        event_id: item.event_id,
        merchandise_variant_id: selectedVariant?.id,
        quantity: 1,
        fulfillment_method: 'pickup',
      }, sid);
      setAdded(true);
      setTimeout(() => { setAdded(false); onClose(); router.push('/cart'); }, 900);
    } catch { setAdding(false); }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-end md:items-center justify-center">
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/70" onClick={onClose} />

      {/* Sheet */}
      <div
        className="relative w-full md:max-w-lg md:rounded-[20px] rounded-t-[20px] overflow-hidden flex flex-col max-h-[92vh]"
        style={{ backgroundColor: bg }}
      >
        {/* Handle + header */}
        <div className="flex items-center px-5 pt-4 pb-3">
          <div className="w-10 h-1 rounded-full mx-auto absolute top-3 left-1/2 -translate-x-1/2 opacity-30" style={{ backgroundColor: tc }} />
          <p className="text-[10px] font-black tracking-[1.5px]" style={{ color: `${tc}aa` }}>
            {isLive ? '● AVAILABLE NOW' : '◎ DROPPING SOON'}
          </p>
          <span className="flex-1" />
          {isLive && item.seconds_remain != null && (
            <p className="text-[10px] font-black font-mono" style={{ color: `${tc}aa` }}>ENDS {formatCountdown(remain)}</p>
          )}
          {!isLive && item.seconds_until != null && (
            <p className="text-[10px] font-black font-mono" style={{ color: `${tc}aa` }}>IN {formatCountdown(until)}</p>
          )}
          <button onClick={onClose} className="ml-4 p-1"><X size={18} color={`${tc}80`} /></button>
        </div>

        {/* Scrollable body */}
        <div className="overflow-y-auto px-5 pb-4 flex-1">
          {/* Price header */}
          <div className="border rounded-sm px-4 py-3 mb-4" style={{ borderColor: `${tc}40` }}>
            <p className="text-[10px] font-bold tracking-wide mb-0.5" style={{ color: `${tc}90` }}>
              {(item.category ?? 'MERCHANDISE').toUpperCase()}
            </p>
            <p className="text-2xl font-black" style={{ color: tc }}>{fmtPrice(displayPrice)}</p>
            <p className="text-sm mt-0.5 line-clamp-2" style={{ color: `${tc}90` }}>{item.name}</p>
          </div>

          {/* Images */}
          {images.length > 0 && (
            <div className="mb-4">
              <div
                className="w-full aspect-[4/3] rounded-[12px] overflow-hidden relative select-none"
                onMouseDown={(e) => imgDragStart(e.clientX)}
                onMouseUp={(e) => imgDragEnd(e.clientX)}
                onTouchStart={(e) => imgDragStart(e.touches[0].clientX)}
                onTouchEnd={(e) => imgDragEnd(e.changedTouches[0].clientX)}
              >
                <img src={images[imgIdx]} alt={item.name} className="w-full h-full object-cover" />
                {images.length > 1 && (
                  <>
                    <button
                      onClick={() => setImgIdx((i) => Math.max(0, i - 1))}
                      disabled={imgIdx === 0}
                      className="absolute left-2 top-1/2 -translate-y-1/2 w-8 h-8 rounded-full flex items-center justify-center transition-opacity"
                      style={{ background: `${bg}cc`, border: `1px solid ${tc}30`, opacity: imgIdx === 0 ? 0.2 : 0.85 }}
                    >
                      <ChevronLeft size={16} color={tc} />
                    </button>
                    <button
                      onClick={() => setImgIdx((i) => Math.min(images.length - 1, i + 1))}
                      disabled={imgIdx === images.length - 1}
                      className="absolute right-2 top-1/2 -translate-y-1/2 w-8 h-8 rounded-full flex items-center justify-center transition-opacity"
                      style={{ background: `${bg}cc`, border: `1px solid ${tc}30`, opacity: imgIdx === images.length - 1 ? 0.2 : 0.85 }}
                    >
                      <ChevronRight size={16} color={tc} />
                    </button>
                  </>
                )}
              </div>
              {images.length > 1 && (
                <div className="flex gap-2 mt-2 justify-center">
                  {images.map((_, i) => (
                    <button
                      key={i}
                      onClick={() => setImgIdx(i)}
                      className="h-1 rounded-full transition-all"
                      style={{ width: i === imgIdx ? 18 : 6, backgroundColor: i === imgIdx ? tc : `${tc}40` }}
                    />
                  ))}
                </div>
              )}
            </div>
          )}

          {/* Title */}
          <p className="text-xl font-black mb-4" style={{ color: tc }}>{item.name}</p>

          {/* Variants */}
          {hasVariants && (
            <div className="mb-4 border-y py-4" style={{ borderColor: `${tc}20` }}>
              <div className="flex items-center justify-between mb-3">
                <p className="text-[11px] font-black tracking-wide" style={{ color: `${tc}70` }}>SELECT SIZE</p>
                {selectedVariant && (
                  <p className="text-[10px] font-bold" style={{ color: `${tc}bb` }}>
                    {getVariantLabel(selectedVariant).toUpperCase()}
                    {selectedVariant.stock_quantity <= 5 && selectedVariant.stock_quantity > 0
                      ? `  —  ONLY ${selectedVariant.stock_quantity} LEFT` : ''}
                  </p>
                )}
              </div>
              <div className="flex flex-wrap gap-2">
                {variants.map((v) => {
                  const label      = getVariantLabel(v);
                  const outOfStock = v.stock_quantity === 0;
                  const lowStock   = !outOfStock && v.stock_quantity <= 5;
                  const isSel      = selectedVariant?.id === v.id;
                  return (
                    <button
                      key={v.id}
                      onClick={() => !outOfStock && setSelectedVariant(isSel ? null : v)}
                      disabled={outOfStock}
                      className="px-4 py-2 rounded-sm border text-[11px] font-bold relative"
                      style={{
                        borderColor: outOfStock ? `${tc}20` : isSel ? tc : `${tc}50`,
                        borderWidth: isSel ? 2 : 1,
                        backgroundColor: isSel ? `${tc}18` : 'transparent',
                        color: outOfStock ? `${tc}28` : isSel ? tc : `${tc}cc`,
                      }}
                    >
                      {label.toUpperCase()}
                      {lowStock && <span className="block text-[8px] font-black text-[#ff9500]">LOW</span>}
                      {outOfStock && <span className="block text-[8px]" style={{ color: `${tc}30` }}>SOLD OUT</span>}
                    </button>
                  );
                })}
              </div>
            </div>
          )}
           

          {/* Details grid */}
          {[
            ['PRICE',    fmtPrice(displayPrice)],
            ['CATEGORY', (item.category ?? '—').toUpperCase()],
            ['STATUS',   item.sale_status.toUpperCase()],
            ['RELEASE',  item.sale_start
              ? new Date(item.sale_start).toLocaleDateString('en-ZA', { day:'2-digit', month:'short', year:'numeric' }).toUpperCase()
              : 'NOW'],
          ].map(([label, value]) => (
            <div key={label} className="flex items-center justify-between py-2.5 border-b" style={{ borderColor: `${tc}20` }}>
              <p className="text-[11px] font-bold tracking-wide" style={{ color: `${tc}70` }}>{label}</p>
              <p className="text-[13px] font-black" style={{ color: tc }}>{value}</p>
            </div>
          ))}

          {item.description && (
            <p className="mt-4 text-sm leading-relaxed" style={{ color: `${tc}cc` }}>{item.description}</p>
          )}
        </div>

        {/* Sticky bottom bar */}
        <div
          className="flex items-center gap-4 px-5 py-4 border-t"
          style={{ borderColor: `${tc}20`, backgroundColor: bg }}
        >
          <div className="flex-1 min-w-0">
            <p className="text-sm font-black truncate" style={{ color: tc }}>{item.name}</p>
            <p className="text-[12px] font-bold" style={{ color: `${tc}80` }}>{fmtPrice(displayPrice)}</p>
          </div>
          <button
            onClick={isLive && !buyDisabled ? handleBuy : undefined}
            disabled={buyDisabled || adding}
            className="border px-6 py-2.5 rounded-sm text-[12px] font-black tracking-wide transition-opacity"
            style={{
              borderColor: buyDisabled ? `${tc}25` : `${tc}60`,
              color: tc,
              opacity: buyDisabled || adding ? 0.45 : 1,
            }}
          >
            {adding ? '...' : added ? <span className="inline-flex items-center gap-1"><Check size={11} weight="bold" /> ADDED</span> : isLive ? (buyDisabled ? 'SELECT SIZE' : 'BUY') : 'ALERT'}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Empty state ───────────────────────────────────────────────────────────────
function EmptyState({ tab }: { tab: 'available' | 'upcoming' }) {
  const { colors } = useAppTheme();
  return (
    <div className="flex flex-col items-center justify-center py-24 text-center px-8">
      <div className="mb-5">{tab === 'available' ? <ShoppingBag size={48} weight="regular" /> : <HourglassMedium size={48} weight="regular" />}</div>
      <p className="text-xl font-black mb-2" style={{ color: colors.text }}>
        {tab === 'available' ? 'No drops live right now' : 'Nothing dropping soon'}
      </p>
      <p className="text-sm leading-relaxed" style={{ color: colors.subText }}>
        Check back later — drops are time-limited and rotate often.
      </p>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────
type Tab = 'available' | 'upcoming';

export default function MerchPage() {
  const router = useRouter();
  const { colors } = useAppTheme();

  const [tab,       setTab]       = useState<Tab>('available');
  const [available, setAvailable] = useState<MerchandiseItem[]>([]);
  const [upcoming,  setUpcoming]  = useState<MerchandiseItem[]>([]);
  const [loading,   setLoading]   = useState(true);
  const [error,     setError]     = useState<string | null>(null);
  const [selected,  setSelected]  = useState<MerchandiseItem | null>(null);
  const [collectionEntry, setCollectionEntry] = useState<CollectionEntry | null>(null);

  const fetchDrops = useCallback(async () => {
    try {
      setError(null);
      const res = await getDropsFeed();
      // The web api.ts wraps the response differently — handle both shapes
      const data = (res as any).data ?? res;
      setAvailable((data as any).available ?? []);
      setUpcoming((data as any).upcoming ?? []);
    } catch (e: any) { setError(e?.message ?? 'Failed to load drops.'); }
  }, []);

  useEffect(() => { fetchDrops().finally(() => setLoading(false)); }, [fetchDrops]);

  const availableCollections = useMemo(() => buildCollections(available), [available]);
  const upcomingCollections  = useMemo(() => buildCollections(upcoming),  [upcoming]);
  const visible = tab === 'available' ? availableCollections : upcomingCollections;

   const handleEntryPress = (entry: CollectionEntry) => {
    if (entry.isCollection && entry.items.length > 1) {
      setCollectionEntry(entry);
    } else {
      setSelected(entry.representativeItem);
    }
  };

  return (
    <div style={{ backgroundColor: colors.background, minHeight: '100vh' }}>

      {/* ── Header ────────────────────────────────────────────────────────── */}
      <div
        className="sticky top-0 md:top-16 z-40 px-5"
        style={{ backgroundColor: colors.background, borderBottom: `1px solid ${colors.dividerColor}` }}
      >
        <div className="flex items-center justify-between py-4">
          <h1 className="text-2xl font-black tracking-wide" style={{ color: colors.text }}>DROPS</h1>
          <button onClick={() => router.push('/cart')} className="relative p-1">
            <ShoppingCart size={24} color={colors.text} />
          </button>
        </div>

        {/* Toggle */}
        <div
          className="flex rounded-[10px] p-1 mb-3"
          style={{ backgroundColor: colors.sectionCardBg }}
        >
          {(['available', 'upcoming'] as Tab[]).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className="flex-1 py-2 rounded-[8px] text-[11px] font-black tracking-[1px] transition-all"
              style={{
                backgroundColor: tab === t ? colors.text : 'transparent',
                color: tab === t ? colors.background : colors.subText,
              }}
            >
              {t === 'available' ? 'AVAILABLE NOW' : 'DROPPING SOON'}
            </button>
          ))}
        </div>
      </div>

      {/* ── Error ────────────────────────────────────────────────────────── */}
      {error && (
        <div className="mx-5 mt-4 p-3.5 rounded-sm border" style={{ backgroundColor: 'rgba(255,60,110,0.12)', borderColor: colors.dividerColor }}>
          <p className="font-black text-[12px] tracking-[1px] mb-1 flex items-center gap-1.5" style={{ color: colors.text }}><WarningCircle size={13} weight="fill" /> FAILED TO LOAD</p>
          <p className="text-[11px] mb-2" style={{ color: colors.subText }}>{error}</p>
          <button onClick={() => fetchDrops()} className="border px-3 py-1.5 rounded-sm text-[10px] font-black tracking-[1.5px]" style={{ borderColor: colors.text, color: colors.text }}>RETRY</button>
        </div>
      )}

      {/* ── Content ──────────────────────────────────────────────────────── */}
      {loading ? (
        <div className="flex items-center justify-center py-24">
          <div className="w-10 h-10 border-2 border-[#ff3c6e] border-t-transparent rounded-full animate-spin" />
        </div>
      ) : visible.length === 0 ? (
        <EmptyState tab={tab} />
      ) : (
        <div className="px-4 py-4 grid grid-cols-1 md:grid-cols-2 gap-4 max-w-3xl mx-auto">
          {visible.map((entry) =>
            entry.representativeItem.sale_status === 'live'
              ? <AvailableCard key={entry.representativeItem.id} entry={entry} onPress={() => handleEntryPress(entry)} />
              : <UpcomingCard  key={entry.representativeItem.id} entry={entry} onPress={() => handleEntryPress(entry)} />
          )}
        </div>
      )}

      {/* ── Detail drawer ─────────────────────────────────────────────────── */}
      {selected && <DetailDrawer item={selected} onClose={() => setSelected(null)} />}
      {collectionEntry && (
        <CollectionModal
          collectionName={collectionEntry.collectionName ?? collectionEntry.representativeItem.event_name ?? 'Collection'}
          description={undefined}
          items={collectionEntry.items}
          onSelectItem={(item) => { setCollectionEntry(null); setSelected(item); }}
          onClose={() => setCollectionEntry(null)}
        />
      )}
    </div>
  );
}