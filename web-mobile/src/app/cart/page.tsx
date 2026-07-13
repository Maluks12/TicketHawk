'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { useRouter } from 'next/navigation';
import {
  type CartItem, type CartSummary, type CartEventGroup,
  getCart, updateCartItem, updateCartItemAddress, removeCartItem, clearCart,
  cartItemLabel, cartItemSubtitle, fmtZAR, getCachedSessionId,
} from '@/lib/api';
import { useAuthStore } from '@/store/authStore';
import { useAppTheme } from '@/themes/colors';

interface AddressForm { street: string; suburb: string; city: string; province: string; postal_code: string; }
const emptyAddress = (): AddressForm => ({ street: '', suburb: '', city: '', province: '', postal_code: '' });
function parseStoredAddress(raw?: string | null): AddressForm | null {
  if (!raw) return null;
  try { const p = JSON.parse(raw); return { street: p.street ?? '', suburb: p.suburb ?? '', city: p.city ?? '', province: p.province ?? '', postal_code: p.postal_code ?? '' }; }
  catch { return null; }
}
function isAddressComplete(a: AddressForm) { return a.street.trim() !== '' && a.city.trim() !== '' && a.province.trim() !== ''; }

function DeliveryAddressRow({ item, sessionId, colors, isDark, onSaved }: { item: CartItem; sessionId: string | null; colors: any; isDark: boolean; onSaved: () => void }) {
  const stored = parseStoredAddress(item.delivery_address);
  const [form, setForm] = useState<AddressForm>(stored ?? emptyAddress());
  const [open, setOpen] = useState(!stored || !isAddressComplete(stored));
  const [saving, setSaving] = useState(false);
  const complete = isAddressComplete(form);
  const summaryLine = stored && isAddressComplete(stored) ? [stored.street, stored.suburb, stored.city, stored.province].filter(Boolean).join(', ') : null;
  const bg = isDark ? '#161616' : '#f9f9f9';
  const border = isDark ? '#2a2a2a' : '#e5e5e5';

  const handleSave = async () => {
    if (!complete) return;
    setSaving(true);
    try {
      await updateCartItemAddress(item.id, { street: form.street.trim(), suburb: form.suburb.trim() || undefined, city: form.city.trim(), province: form.province.trim(), postal_code: form.postal_code.trim() || undefined }, sessionId);
      setOpen(false); onSaved();
    } catch (err: any) { alert(err?.response?.data?.error ?? 'Could not save address.'); }
    finally { setSaving(false); }
  };

  const inputStyle = { background: isDark ? '#0f0f0f' : '#fff', color: colors.text, border: `1px solid ${border}`, borderRadius: 8, padding: '10px 12px', fontSize: 14, width: '100%', boxSizing: 'border-box' as const, outline: 'none' };

  return (
    <div style={{ marginBottom: 4 }}>
      <div onClick={() => setOpen(!open)} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', background: bg, border: `1px solid ${border}`, borderRadius: 12, padding: '12px 14px', cursor: 'pointer', marginBottom: open ? 2 : 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <span>📦</span>
          <div>
            <p style={{ margin: 0, fontSize: 13, fontWeight: 700, color: colors.text }}>Delivery address</p>
            {summaryLine ? <p style={{ margin: 0, fontSize: 11, color: colors.subText }}>{summaryLine}</p>
              : <p style={{ margin: 0, fontSize: 11, color: '#ff9500' }}>Required before checkout</p>}
          </div>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          {summaryLine && <span style={{ background: '#34c759', color: '#fff', borderRadius: '50%', width: 20, height: 20, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 10, fontWeight: 900 }}>✓</span>}
          <span style={{ color: colors.subText, fontSize: 10 }}>{open ? '▲' : '▼'}</span>
        </div>
      </div>
      {open && (
        <div style={{ background: bg, border: `1px solid ${border}`, borderRadius: 12, padding: 14, display: 'flex', flexDirection: 'column', gap: 10, marginBottom: 8 }}>
          {['street', 'suburb'].map((key) => (
            <input key={key} placeholder={key === 'street' ? 'Street address *' : 'Suburb'} value={(form as any)[key]} onChange={(e) => setForm((f) => ({ ...f, [key]: e.target.value }))} style={inputStyle} />
          ))}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
            <input placeholder="City *" value={form.city} onChange={(e) => setForm((f) => ({ ...f, city: e.target.value }))} style={inputStyle} />
            <input placeholder="Province *" value={form.province} onChange={(e) => setForm((f) => ({ ...f, province: e.target.value }))} style={inputStyle} />
          </div>
          <input placeholder="Postal code" value={form.postal_code} onChange={(e) => setForm((f) => ({ ...f, postal_code: e.target.value }))} style={inputStyle} />
          <button onClick={handleSave} disabled={saving || !complete} style={{ background: complete ? '#ff2d55' : '#ff2d5530', color: '#fff', border: 'none', borderRadius: 8, padding: '12px', fontSize: 13, fontWeight: 800, cursor: complete ? 'pointer' : 'not-allowed', opacity: saving ? 0.6 : 1 }}>
            {saving ? 'Saving…' : summaryLine ? 'Update address' : 'Save address'}
          </button>
        </div>
      )}
    </div>
  );
}

function CartItemRow({ item, onQuantityChange, onRemove, updating, colors }: { item: CartItem; onQuantityChange: (id: string, qty: number) => void; onRemove: (id: string) => void; updating: boolean; colors: any }) {
  const lineTotal = (item as any).unit_price_snapshot * item.quantity + ((item as any).delivery_fee_snapshot ?? 0);
  const sub = cartItemSubtitle(item);
  const isTicket = item.item_type === 'ticket';

  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12, padding: '14px 16px', borderRadius: 14, border: `1px solid ${colors.dividerColor}`, background: colors.cardBg, marginBottom: 4 }}>
      <div style={{ width: 28, height: 28, borderRadius: 8, background: isTicket ? '#ff2d55' : '#1c2e61', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, marginTop: 2, fontSize: 13 }}>
        {isTicket ? '🎟' : '👜'}
      </div>
      <div style={{ flex: 1 }}>
        <p style={{ margin: 0, fontSize: 15, fontWeight: 700, color: colors.text, lineHeight: 1.3 }}>{cartItemLabel(item)}</p>
        {sub && <p style={{ margin: '2px 0 0', fontSize: 12, color: colors.subText }}>{sub}</p>}
        {(item as any).delivery_fee_snapshot > 0 && <p style={{ margin: '2px 0 0', fontSize: 11, color: colors.subText }}>+ {fmtZAR((item as any).delivery_fee_snapshot)} delivery</p>}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: 10 }}>
          <div style={{ display: 'flex', alignItems: 'center', border: `1px solid ${colors.dividerColor}`, borderRadius: 10, overflow: 'hidden' }}>
            <button onClick={() => item.quantity > 1 ? onQuantityChange(item.id, item.quantity - 1) : onRemove(item.id)} disabled={updating} style={{ width: 32, height: 32, border: 'none', background: colors.sectionCardBg, cursor: 'pointer', fontSize: 15, color: colors.text }}>
              {item.quantity === 1 ? '🗑' : '−'}
            </button>
            <span style={{ width: 36, textAlign: 'center', fontSize: 15, fontWeight: 700, color: colors.text }}>{updating ? '…' : item.quantity}</span>
            <button onClick={() => onQuantityChange(item.id, item.quantity + 1)} disabled={updating || item.quantity >= 10} style={{ width: 32, height: 32, border: 'none', background: colors.sectionCardBg, cursor: 'pointer', fontSize: 15, color: item.quantity >= 10 ? colors.subText : colors.text }}>+</button>
          </div>
          <span style={{ fontSize: 15, fontWeight: 800, color: '#ff2d55' }}>{fmtZAR(lineTotal)}</span>
        </div>
      </div>
      <button onClick={() => { if (confirm(`Remove "${cartItemLabel(item)}" from your cart?`)) onRemove(item.id); }} style={{ background: 'none', border: 'none', cursor: 'pointer', color: colors.subText, padding: 4, marginTop: 2, fontSize: 16 }}>🗑</button>
    </div>
  );
}

export default function CartPage() {
  const router = useRouter();
  const { colors, isDark } = useAppTheme();
  const { user } = useAuthStore();
  const [cart, setCart] = useState<CartSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [updatingId, setUpdatingId] = useState<string | null>(null);
  const sidRef = useRef<string | null>(null);

  const fetchCart = useCallback(async (quiet = false) => {
    if (!quiet) setLoading(true);
    try {
      if (sidRef.current === null) sidRef.current = await getCachedSessionId();
      const data = await getCart(sidRef.current);
      setCart(data);
    } catch { setCart(null); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { fetchCart(); }, [fetchCart]);

  const handleQuantityChange = async (itemId: string, qty: number) => {
    setUpdatingId(itemId);
    try { await updateCartItem(itemId, qty, sidRef.current); await fetchCart(true); }
    catch (err: any) { alert(err?.response?.data?.error ?? 'Could not update quantity'); }
    finally { setUpdatingId(null); }
  };

  const handleRemove = async (itemId: string) => {
    setUpdatingId(itemId);
    try { await removeCartItem(itemId, sidRef.current); await fetchCart(true); }
    catch { alert('Could not remove item'); }
    finally { setUpdatingId(null); }
  };

  const handleClearCart = async () => {
    if (!confirm('Remove all items from your cart?')) return;
    try { await clearCart(sidRef.current); setCart(null); }
    catch { alert('Could not clear cart'); }
  };

  const bg = isDark ? '#0a0a0a' : '#f5f5f7';
  const cardBg = isDark ? '#111' : '#fff';
  const border = isDark ? '#222' : '#e5e5e5';

  if (!user) return (
    <div style={{ minHeight: '100vh', background: bg, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 14, padding: 40 }}>
      <span style={{ fontSize: 48 }}>🛒</span>
      <h2 style={{ margin: 0, color: colors.text, fontSize: 22, fontWeight: 800 }}>Sign in to view your cart</h2>
      <p style={{ margin: 0, color: colors.subText, textAlign: 'center', maxWidth: 320 }}>Log in or create an account to add items to your cart and check out.</p>
      <button onClick={() => router.push('/auth/login')} style={{ background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 12, padding: '14px 28px', fontSize: 15, fontWeight: 800, cursor: 'pointer', marginTop: 6, width: '100%', maxWidth: 320 }}>Log in / Sign up</button>
    </div>
  );

  const allItems = (cart as any)?.items_by_event?.flatMap((g: any) => g.items) ?? [];
  const totalItems = cart?.total_items ?? 0;
  const totalAmount = (cart as any)?.total_amount ?? 0;
  const hasItems = totalItems > 0;
  const missingAddress = allItems.some((i: CartItem) => i.item_type === 'merchandise' && i.fulfillment_method === 'delivery' && !i.delivery_address);

  return (
    <div style={{ minHeight: '100vh', background: bg }}>
      <div style={{ maxWidth: 720, margin: '0 auto', padding: '0 16px 160px' }}>
        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '24px 0 16px', borderBottom: `1px solid ${border}` }}>
          <button onClick={() => router.back()} style={{ background: 'none', border: 'none', cursor: 'pointer', color: colors.text, fontSize: 22, padding: 4 }}>←</button>
          <h1 style={{ margin: 0, fontSize: 28, fontWeight: 800, color: colors.text, flex: 1 }}>Cart</h1>
          {hasItems && <span style={{ background: '#ff2d55', color: '#fff', borderRadius: 11, padding: '2px 8px', fontSize: 12, fontWeight: 800 }}>{totalItems}</span>}
          {hasItems && <button onClick={handleClearCart} style={{ background: 'none', border: 'none', cursor: 'pointer', color: colors.subText, fontSize: 14, fontWeight: 600 }}>Clear</button>}
        </div>

        {loading ? (
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', minHeight: 300, gap: 12 }}>
            <div style={{ width: 36, height: 36, border: '3px solid #ff2d55', borderTopColor: 'transparent', borderRadius: '50%', animation: 'spin 0.8s linear infinite' }} />
            <p style={{ color: colors.subText, margin: 0 }}>Loading cart…</p>
          </div>
        ) : !hasItems ? (
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', minHeight: 300, gap: 14, paddingTop: 40 }}>
            <span style={{ fontSize: 48 }}>🛒</span>
            <h2 style={{ margin: 0, color: colors.text, fontSize: 22, fontWeight: 800 }}>Your cart is empty</h2>
            <p style={{ margin: 0, color: colors.subText, textAlign: 'center' }}>Browse events and merch drops to add items here</p>
            <button onClick={() => router.push('/')} style={{ background: '#ff2d55', color: '#fff', border: 'none', borderRadius: 12, padding: '13px 28px', fontSize: 15, fontWeight: 800, cursor: 'pointer', marginTop: 6 }}>Explore Events</button>
          </div>
        ) : (
          <>
            {(cart as any).items_by_event.map((group: any, idx: number) => (
              <div key={group.event_id ?? `standalone-merch-${idx}`} style={{ marginTop: 16 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', background: cardBg, borderRadius: 12, padding: '10px 16px', marginBottom: 6 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <div style={{ width: 8, height: 8, borderRadius: 4, background: '#ff2d55' }} />
                    <span style={{ fontSize: 13, fontWeight: 700, color: colors.text }}>{group.event_title || 'Items'}</span>
                  </div>
                  <span style={{ fontSize: 13, fontWeight: 600, color: colors.subText }}>{fmtZAR(group.subtotal)}</span>
                </div>
                {group.items.map((item: CartItem) => (
                  <div key={item.id}>
                    <CartItemRow item={item} onQuantityChange={handleQuantityChange} onRemove={handleRemove} updating={updatingId === item.id} colors={colors} />
                    {item.item_type === 'merchandise' && item.fulfillment_method === 'delivery' && (
                      <DeliveryAddressRow item={item} sessionId={sidRef.current} colors={colors} isDark={isDark} onSaved={() => fetchCart(true)} />
                    )}
                  </div>
                ))}
              </div>
            ))}

            {missingAddress && (
              <div style={{ display: 'flex', gap: 8, alignItems: 'center', background: isDark ? '#2a1a00' : '#fff8ec', borderRadius: 10, padding: 14, margin: '8px 0' }}>
                <span>⚠️</span>
                <p style={{ margin: 0, fontSize: 13, fontWeight: 600, color: isDark ? '#ffb340' : '#a05000' }}>Add a delivery address above before you can check out</p>
              </div>
            )}

            {/* Order summary */}
            <div style={{ background: cardBg, border: `1px solid ${border}`, borderRadius: 16, padding: 18, marginTop: 12 }}>
              <p style={{ margin: '0 0 12px', fontSize: 11, fontWeight: 800, letterSpacing: 1.2, color: colors.subText }}>ORDER SUMMARY</p>
              {allItems.map((item: CartItem) => (
                <div key={item.id} style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                  <span style={{ fontSize: 14, fontWeight: 500, color: colors.text }}>{cartItemLabel(item)} × {item.quantity}</span>
                  <span style={{ fontSize: 14, fontWeight: 600, color: colors.text }}>{fmtZAR((item as any).unit_price_snapshot * item.quantity)}</span>
                </div>
              ))}
              {allItems.some((i: any) => i.delivery_fee_snapshot > 0) && (
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                  <span style={{ fontSize: 14, color: colors.subText }}>Delivery fees</span>
                  <span style={{ fontSize: 14, color: colors.subText }}>{fmtZAR(allItems.reduce((s: number, i: any) => s + i.delivery_fee_snapshot, 0))}</span>
                </div>
              )}
              <div style={{ height: 1, background: border, margin: '8px 0' }} />
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ fontSize: 16, fontWeight: 700, color: colors.text }}>Total</span>
                <span style={{ fontSize: 18, fontWeight: 800, color: '#ff2d55' }}>{fmtZAR(totalAmount)}</span>
              </div>
            </div>

            <div style={{ display: 'flex', alignItems: 'center', gap: 8, background: colors.sectionCardBg, borderRadius: 10, padding: 12, marginTop: 8 }}>
              <span style={{ fontSize: 13 }}>🔒</span>
              <p style={{ margin: 0, fontSize: 12, color: colors.subText }}>Tickets are reserved for 10 minutes once you begin checkout</p>
            </div>
          </>
        )}
      </div>

      {/* Sticky checkout bar */}
      {hasItems && !loading && (
        <div style={{ position: 'fixed', bottom: 0, left: 0, right: 0, background: isDark ? '#0a0a0a' : '#fff', borderTop: `1px solid ${border}`, padding: '14px 24px 24px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', zIndex: 50 }}>
          <div>
            <p style={{ margin: 0, fontSize: 12, fontWeight: 600, color: colors.subText }}>Total</p>
            <p style={{ margin: 0, fontSize: 22, fontWeight: 800, color: colors.text }}>{fmtZAR(totalAmount)}</p>
          </div>
          <button
            onClick={() => { if (missingAddress) { alert('Add a delivery address for all delivery items before checking out.'); return; } router.push('/cart/checkout'); }}
            style={{ background: missingAddress ? '#ff2d5570' : '#ff2d55', color: '#fff', border: 'none', borderRadius: 14, padding: '14px 24px', fontSize: 16, fontWeight: 800, cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 8 }}>
            Checkout →
          </button>
        </div>
      )}
    </div>
  );
}