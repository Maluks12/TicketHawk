'use client';

import { useState } from 'react';
import type { MerchandiseItem, MerchandiseVariant } from '@/lib/api';

interface CartConfirmSheetProps {
  item: MerchandiseItem | null;
  selectedVariant: MerchandiseVariant | null;
  onClose: () => void;
  onAddToCart: (variant: MerchandiseVariant, quantity: number) => Promise<void>;
}

export function CartConfirmSheet({ item, selectedVariant, onClose, onAddToCart }: CartConfirmSheetProps) {
  const [qty,     setQty]     = useState(1);
  const [loading, setLoading] = useState(false);

  if (!item || !selectedVariant) return null;

  const handleAdd = async () => {
    setLoading(true);
    try { await onAddToCart(selectedVariant, qty); onClose(); }
    finally { setLoading(false); }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/60 backdrop-blur-sm"
         onClick={onClose}>
      <div className="w-full max-w-lg bg-white dark:bg-neutral-900 rounded-t-2xl p-6 pb-10 space-y-5"
           onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between">
          <h2 className="text-base font-black tracking-wide dark:text-white">{item.name}</h2>
          <button onClick={onClose} className="text-xl font-bold dark:text-white">✕</button>
        </div>

        {/* Variant info */}
        <div className="flex gap-2 text-xs font-semibold text-gray-500 dark:text-gray-400">
          {(() => {
            try {
              const attrs = JSON.parse(selectedVariant.attributes) as Record<string, string>;
              return Object.entries(attrs).map(([k, v]) => (
                <span key={k}>{k.charAt(0).toUpperCase() + k.slice(1)}: {v}</span>
              ));
            } catch {
              return selectedVariant.sku ? <span>{selectedVariant.sku}</span> : null;
            }
          })()}
          <span className="ml-auto">R{(selectedVariant.price_override ?? item.effective_price).toLocaleString('en-ZA')}</span>
        </div>

        {/* Quantity */}
        <div className="flex items-center gap-4">
          <span className="text-xs font-bold tracking-widest text-gray-400">QTY</span>
          <button onClick={() => setQty(q => Math.max(1, q - 1))}
                  className="w-8 h-8 rounded-full border flex items-center justify-center font-bold dark:border-white/20 dark:text-white">−</button>
          <span className="text-base font-bold dark:text-white">{qty}</span>
          <button onClick={() => setQty(q => Math.min(selectedVariant.stock_quantity, q + 1))}
                  className="w-8 h-8 rounded-full border flex items-center justify-center font-bold dark:border-white/20 dark:text-white">+</button>
          <span className="text-xs text-gray-400 ml-auto">{selectedVariant.stock_quantity} left</span>
        </div>

        <button onClick={handleAdd} disabled={loading}
                className="w-full bg-[#FF2D55] text-white font-extrabold text-xs tracking-[1.5px] py-3 rounded-md disabled:opacity-50">
          {loading ? 'Adding…' : `ADD TO CART — R${((selectedVariant.price_override ?? item.effective_price) * qty).toLocaleString('en-ZA')}`}
        </button>
      </div>
    </div>
  );
}