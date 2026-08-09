'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { useRouter } from 'next/navigation';
import { X, ChevronLeft, ChevronRight } from 'lucide-react';
import { Check, Package } from '@phosphor-icons/react';
import { type MerchandiseItem, addMerchToCart, getCachedSessionId } from '@/lib/api';
import { useAuthStore } from '@/store/authStore';

// ── Palette (mirrors merch.tsx) ───────────────────────────────────────────────
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

function useCountdown(initial: number) {
  const [secs, setSecs] = useState(initial);
  useEffect(() => {
    if (secs <= 0) return;
    const t = setInterval(() => setSecs((s) => Math.max(0, s - 1)), 1000);
    return () => clearInterval(t);
  }, []);
  return secs;
}

type Variant = NonNullable<MerchandiseItem['variants']>[number];

function getVariantLabel(v: Variant): string {
  try {
    const attr = JSON.parse(v.attributes) as Record<string, string>;
    return Object.values(attr).join(' / ');
  } catch { return v.sku; }
}

// ── Item detail overlay ────────────────────────────────────────────────────────
function ItemDetail({ item, onClose }: { item: MerchandiseItem; onClose: () => void }) {
  const { bg, text: tc } = paletteFor(item);
  const { user } = useAuthStore();
  const router = useRouter();
  const images = parseImages(item.images);
  const [imgIdx, setImgIdx] = useState(0);
  const [selectedVariant, setSelectedVariant] = useState<Variant | null>(null);
  const [adding, setAdding] = useState(false);
  const [added, setAdded] = useState(false);
  const remain = useCountdown(item.seconds_remain ?? 0);
  const until  = useCountdown(item.seconds_until ?? 0);
  const isLive = item.sale_status === 'live';
  const variants = item.variants ?? [];
  const hasVariants = variants.length > 0;
  const displayPrice = selectedVariant?.price_override != null ? selectedVariant.price_override : item.effective_price;
  const buyDisabled = isLive && hasVariants && !selectedVariant;

  const handleBuy = async () => {
    if (!user) {
      if (confirm('Sign in to add to cart — go to login?')) {
        onClose();
        router.push('/auth/login');
      }
      return;
    }
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
    <div style={{ position: 'absolute', inset: 0, background: bg, display: 'flex', flexDirection: 'column', overflowY: 'auto', animation: 'slideIn 0.18s ease-out' }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', padding: '14px 16px 10px', flexShrink: 0 }}>
        <p style={{ margin: 0, fontSize: 10, fontWeight: 800, letterSpacing: 1.5, color: `${tc}aa`, flex: 1 }}>
          {isLive ? '● AVAILABLE NOW' : '◎ DROPPING SOON'}
        </p>
        {isLive && remain > 0 && <p style={{ margin: 0, fontSize: 10, fontWeight: 800, color: `${tc}aa`, marginRight: 12 }}>ENDS {formatCountdown(remain)}</p>}
        {!isLive && until > 0  && <p style={{ margin: 0, fontSize: 10, fontWeight: 800, color: `${tc}aa`, marginRight: 12 }}>IN {formatCountdown(until)}</p>}
        <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', padding: 4 }}>
          <X size={20} color={`${tc}80`} />
        </button>
      </div>

      {/* Price box */}
      <div style={{ margin: '0 16px 12px', padding: 16, border: `1px solid ${tc}40`, borderRadius: 4 }}>
        <p style={{ margin: 0, fontSize: 10, fontWeight: 800, letterSpacing: 1.5, color: `${tc}90` }}>
          {(item.category ?? 'MERCHANDISE').toUpperCase()}
        </p>
        <p style={{ margin: '4px 0', fontSize: 34, fontWeight: 900, color: tc }}>{fmtPrice(displayPrice)}</p>
        <p style={{ margin: 0, fontSize: 12, color: `${tc}90` }}>{item.name}</p>
      </div>

      {/* Images */}
      {images.length > 0 && (
        <div style={{ margin: '0 16px 12px', position: 'relative' }}>
          <div style={{ width: '100%', aspectRatio: '4/3', borderRadius: 12, overflow: 'hidden' }}>
            <img src={images[imgIdx]} alt={item.name} style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
          </div>
          {images.length > 1 && (
            <div style={{ display: 'flex', justifyContent: 'center', gap: 6, marginTop: 8 }}>
              {images.map((_, i) => (
                <button key={i} onClick={() => setImgIdx(i)}
                  style={{ height: 4, width: i === imgIdx ? 18 : 6, borderRadius: 2, background: i === imgIdx ? tc : `${tc}40`, border: 'none', cursor: 'pointer', padding: 0, transition: 'width 0.2s' }} />
              ))}
            </div>
          )}
        </div>
      )}

      {/* Title */}
      <p style={{ margin: '0 16px 12px', fontSize: 26, fontWeight: 800, color: tc }}>{item.name}</p>

      {/* Variants */}
      {hasVariants && (
        <div style={{ margin: '0 16px 12px', padding: '14px 0', borderTop: `1px solid ${tc}20`, borderBottom: `1px solid ${tc}20` }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10 }}>
            <p style={{ margin: 0, fontSize: 10, fontWeight: 800, letterSpacing: 1.5, color: `${tc}70` }}>SIZE</p>
            {selectedVariant && (
              <p style={{ margin: 0, fontSize: 10, fontWeight: 700, color: `${tc}bb` }}>
                {getVariantLabel(selectedVariant).toUpperCase()}
                {selectedVariant.stock_quantity > 0 && selectedVariant.stock_quantity <= 5 ? `  —  ONLY ${selectedVariant.stock_quantity} LEFT` : ''}
              </p>
            )}
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
            {variants.map((v) => {
              const label = getVariantLabel(v);
              const oos = v.stock_quantity === 0;
              const low = !oos && v.stock_quantity <= 5;
              const sel = selectedVariant?.id === v.id;
              return (
                <button key={v.id}
                  onClick={() => !oos && setSelectedVariant(sel ? null : v)}
                  disabled={oos}
                  style={{
                    padding: '9px 14px', borderRadius: 4, cursor: oos ? 'default' : 'pointer',
                    border: `${sel ? 2 : 1}px solid ${oos ? `${tc}20` : sel ? tc : `${tc}50`}`,
                    background: sel ? `${tc}18` : 'transparent',
                    color: oos ? `${tc}28` : sel ? tc : `${tc}cc`,
                    fontWeight: sel ? 900 : 700, fontSize: 11, letterSpacing: 0.8,
                  }}
                >
                  <span style={{ display: 'block' }}>{label.toUpperCase()}</span>
                  {low && <span style={{ display: 'block', fontSize: 7, color: '#ff9500', fontWeight: 800, marginTop: 2 }}>LOW</span>}
                  {oos && <span style={{ display: 'block', fontSize: 8, color: `${tc}30`, marginTop: 2 }}>SOLD OUT</span>}
                </button>
              );
            })}
          </div>
        </div>
      )}

      {/* Description */}
      {item.description && (
        <p style={{ margin: '0 16px 80px', fontSize: 15, lineHeight: 1.6, color: `${tc}cc` }}>{item.description}</p>
      )}

      {/* Bottom bar */}
      <div style={{ position: 'sticky', bottom: 0, background: bg, borderTop: `1px solid ${tc}20`, padding: '14px 16px 20px', display: 'flex', alignItems: 'center', gap: 10, flexShrink: 0 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <p style={{ margin: 0, fontSize: 12, fontWeight: 700, color: tc, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{item.name}</p>
          <p style={{ margin: 0, fontSize: 11, color: `${tc}80` }}>{fmtPrice(displayPrice)}</p>
        </div>
        <button
          onClick={isLive && !buyDisabled ? handleBuy : undefined}
          disabled={buyDisabled || adding}
          style={{
            padding: '9px 20px', borderRadius: 4, cursor: buyDisabled || !isLive ? 'default' : 'pointer',
            border: `1px solid ${buyDisabled ? `${tc}25` : `${tc}60`}`, background: 'transparent',
            color: tc, fontSize: 11, fontWeight: 800, letterSpacing: 1.5,
            opacity: buyDisabled || adding ? 0.45 : 1,
          }}
        >
          {adding ? '…' : added ? <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}><Check size={11} weight="bold" /> ADDED</span> : isLive ? (buyDisabled ? 'SELECT SIZE' : 'BUY') : 'ALERT'}
        </button>
      </div>
    </div>
  );
}

// ── CollectionModal ────────────────────────────────────────────────────────────
interface CollectionModalProps {
  collectionName: string;
  description?: string;
  items: MerchandiseItem[];
  onSelectItem: (item: MerchandiseItem) => void;
  onClose: () => void;
}

export function CollectionModal({ collectionName, description, items, onSelectItem, onClose }: CollectionModalProps) {
  const [activeIndex, setActiveIndex] = useState(0);
  const [detailItem, setDetailItem] = useState<MerchandiseItem | null>(null);
  const [slideDir, setSlideDir] = useState<'left' | 'right'>('right');
  const [animKey, setAnimKey] = useState(0);
  const dragStartX = useRef<number | null>(null);

  const activeItem = items[activeIndex] ?? items[0];
  const { bg, text: tc } = activeItem ? paletteFor(activeItem) : PALETTE.default;
  const isLive = activeItem?.sale_status === 'live';

  const prev = useCallback(() => {
    setSlideDir('left');
    setAnimKey((k) => k + 1);
    setActiveIndex((i) => Math.max(0, i - 1));
  }, []);

  const next = useCallback(() => {
    setSlideDir('right');
    setAnimKey((k) => k + 1);
    setActiveIndex((i) => Math.min(items.length - 1, i + 1));
  }, [items.length]);

  // Keyboard navigation
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'ArrowLeft') prev();
      else if (e.key === 'ArrowRight') next();
      else if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [prev, next, onClose]);

  return (
    <div style={{ position: 'fixed', inset: 0, zIndex: 60, display: 'flex', alignItems: 'flex-end', justifyContent: 'center' }}>
      {/* Backdrop */}
      <div style={{ position: 'absolute', inset: 0, background: 'rgba(0,0,0,0.7)' }} onClick={onClose} />

      {/* Sheet */}
      <div
        style={{
          position: 'relative', width: '100%', maxWidth: 520, background: bg,
          borderRadius: '20px 20px 0 0', maxHeight: '92vh', overflow: 'hidden',
          display: 'flex', flexDirection: 'column', animation: 'slideUp 0.25s ease-out',
        }}
        onTouchStart={(e) => { dragStartX.current = e.touches[0].clientX; }}
        onTouchEnd={(e) => {
          if (dragStartX.current === null) return;
          const delta = dragStartX.current - e.changedTouches[0].clientX;
          if (Math.abs(delta) > 50) { delta > 0 ? next() : prev(); }
          dragStartX.current = null;
        }}
        onMouseDown={(e) => { dragStartX.current = e.clientX; }}
        onMouseUp={(e) => {
          if (dragStartX.current === null) return;
          const delta = dragStartX.current - e.clientX;
          if (Math.abs(delta) > 60) { delta > 0 ? next() : prev(); }
          dragStartX.current = null;
        }}
      >
        <style>{`
          @keyframes slideUp { from { transform: translateY(100%); } to { transform: translateY(0); } }
          @keyframes slideIn { from { transform: translateX(100%); opacity: 0; } to { transform: translateX(0); opacity: 1; } }
          @keyframes carouselFromRight { from { transform: translateX(48px); opacity: 0; } to { transform: translateX(0); opacity: 1; } }
          @keyframes carouselFromLeft  { from { transform: translateX(-48px); opacity: 0; } to { transform: translateX(0); opacity: 1; } }
        `}</style>

        {/* Handle + header */}
        <div style={{ padding: '10px 16px 14px', flexShrink: 0, borderBottom: `1px solid ${tc}15` }}>
          <div style={{ width: 36, height: 4, borderRadius: 2, background: `${tc}55`, margin: '0 auto 14px' }} />
          <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12 }}>
            <div style={{ flex: 1 }}>
              <p style={{ margin: '0 0 2px', fontSize: 9, fontWeight: 800, letterSpacing: 2, color: `${tc}55` }}>COLLECTION</p>
              <p style={{ margin: 0, fontSize: 22, fontWeight: 900, color: tc, lineHeight: 1.15 }}>{collectionName.toUpperCase()}</p>
            </div>
            <div style={{ border: `1px solid ${tc}40`, padding: '6px 12px', borderRadius: 4, textAlign: 'center', marginTop: 2 }}>
              <p style={{ margin: 0, fontSize: 20, fontWeight: 900, color: tc, lineHeight: 1.1 }}>{items.length}</p>
              <p style={{ margin: 0, fontSize: 8, fontWeight: 700, letterSpacing: 1.2, color: `${tc}55` }}>ITEMS</p>
            </div>
            <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', padding: 4, marginTop: 2 }}>
              <X size={20} color={`${tc}80`} />
            </button>
          </div>
        </div>

        {/* Carousel */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '16px 16px 80px', position: 'relative', userSelect: 'none' }}>
          {activeItem && (() => {
            const images = parseImages(activeItem.images);
            const heroImage = images[0];
            const { bg: cardBg, text: cardTc } = paletteFor(activeItem);
            const variantLabels = (activeItem.variants ?? [])
              .filter((v) => v.stock_quantity > 0)
              .slice(0, 4)
              .map((v) => { try { return Object.values(JSON.parse(v.attributes)).join('/'); } catch { return v.sku; } });

            return (
              <div
                key={animKey}
                style={{ animation: `${slideDir === 'right' ? 'carouselFromRight' : 'carouselFromLeft'} 0.22s cubic-bezier(0.4,0,0.2,1)` }}
              >
                <div onClick={() => setDetailItem(activeItem)}
                  style={{ background: cardBg, borderRadius: 18, overflow: 'hidden', cursor: 'pointer', transition: 'transform 0.15s', marginBottom: 14 }}>
                  <div style={{ position: 'relative', paddingTop: '100%' }}>
                    {heroImage
                      ? <img src={heroImage} alt={activeItem.name} style={{ position: 'absolute', inset: 0, width: '100%', height: '100%', objectFit: 'cover' }} />
                      : <div style={{ position: 'absolute', inset: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Package size={48} weight="regular" /></div>
                    }
                    <div style={{ position: 'absolute', bottom: 0, left: 0, right: 0, height: 60, background: `linear-gradient(transparent, ${cardBg})` }} />
                  </div>
                  <div style={{ padding: 14 }}>
                    <p style={{ margin: '0 0 4px', fontSize: 9, fontWeight: 800, letterSpacing: 1.5, color: `${cardTc}70` }}>{(activeItem.category ?? 'DROP').toUpperCase()}</p>
                    <p style={{ margin: '0 0 10px', fontSize: 16, fontWeight: 700, color: cardTc, lineHeight: 1.3 }}>{activeItem.name}</p>
                    {variantLabels.length > 0 && (
                      <div style={{ display: 'flex', gap: 4, marginBottom: 10, flexWrap: 'wrap' }}>
                        {variantLabels.map((lbl, i) => (
                          <span key={i} style={{ border: `1px solid ${cardTc}35`, padding: '2px 7px', borderRadius: 2, fontSize: 8, fontWeight: 700, letterSpacing: 0.8, color: `${cardTc}aa` }}>
                            {String(lbl).toUpperCase()}
                          </span>
                        ))}
                      </div>
                    )}
                    <div style={{ display: 'flex', alignItems: 'center' }}>
                      <p style={{ margin: 0, flex: 1, fontSize: 20, fontWeight: 900, color: cardTc }}>{fmtPrice(activeItem.effective_price)}</p>
                      <span style={{ border: `1px solid ${cardTc}60`, padding: '6px 14px', borderRadius: 2, fontSize: 10, fontWeight: 800, letterSpacing: 1.5, color: cardTc }}>
                        {isLive ? 'VIEW' : 'ALERT'}
                      </span>
                    </div>
                  </div>
                </div>

                {/* Dot indicators */}
                {items.length > 1 && (
                  <div style={{ display: 'flex', justifyContent: 'center', gap: 5, marginTop: 4 }}>
                    {items.map((_, i) => (
                      <button
                        key={i}
                        onClick={() => { setSlideDir(i > activeIndex ? 'right' : 'left'); setAnimKey((k) => k + 1); setActiveIndex(i); }}
                        style={{
                          height: 4, width: i === activeIndex ? 20 : 6, borderRadius: 2,
                          background: i === activeIndex ? tc : `${tc}40`,
                          border: 'none', cursor: 'pointer', padding: 0,
                          transition: 'width 0.2s, background 0.2s',
                        }}
                      />
                    ))}
                  </div>
                )}
              </div>
            );
          })()}

          {/* Description */}
          {description && (
            <div style={{ marginTop: 16, paddingTop: 16, borderTop: `1px solid ${tc}18` }}>
              <p style={{ margin: '0 0 8px', fontSize: 9, fontWeight: 800, letterSpacing: 2, color: `${tc}45` }}>ABOUT THIS COLLECTION</p>
              <p style={{ margin: 0, fontSize: 15, lineHeight: 1.6, color: `${tc}cc` }}>{description}</p>
            </div>
          )}
        </div>

        {/* Bottom action bar */}
        <div style={{ position: 'absolute', bottom: 0, left: 0, right: 0, background: bg, borderTop: `1px solid ${tc}20`, padding: '14px 16px 24px', display: 'flex', alignItems: 'center', gap: 10 }}>
          {/* Prev / counter / next */}
          <button onClick={prev} disabled={activeIndex === 0}
            style={{ background: 'none', border: `1px solid ${tc}30`, borderRadius: '50%', width: 32, height: 32, display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: activeIndex === 0 ? 'default' : 'pointer', opacity: activeIndex === 0 ? 0.3 : 1 }}>
            <ChevronLeft size={16} color={tc} />
          </button>
          <div style={{ flex: 1, textAlign: 'center' }}>
            <p style={{ margin: 0, fontSize: 10, fontWeight: 800, letterSpacing: 1.5, color: `${tc}aa` }}>
              {String(activeIndex + 1).padStart(2,'0')} / {String(items.length).padStart(2,'0')}
            </p>
            {activeItem && <p style={{ margin: '2px 0 0', fontSize: 12, fontWeight: 700, color: tc, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{activeItem.name}</p>}
          </div>
          <button onClick={next} disabled={activeIndex === items.length - 1}
            style={{ background: 'none', border: `1px solid ${tc}30`, borderRadius: '50%', width: 32, height: 32, display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: activeIndex === items.length - 1 ? 'default' : 'pointer', opacity: activeIndex === items.length - 1 ? 0.3 : 1 }}>
            <ChevronRight size={16} color={tc} />
          </button>
        </div>

        {/* Detail overlay */}
        {detailItem && (
          <ItemDetail item={detailItem} onClose={() => setDetailItem(null)} />
        )}
      </div>
    </div>
  );
}