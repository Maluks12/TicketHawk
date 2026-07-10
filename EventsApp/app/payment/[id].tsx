/**
 * app/payment/[id].tsx
 *
 * Drives the hosted-checkout step via an in-app browser (expo-web-browser),
 * NOT a WebView — so Apple Pay, 3-D Secure, OTP, and PayPal all work.
 *
 * Params (pushed from purchase.tsx / cart/checkout.tsx):
 *   id       — bookingId
 *   url      — authorization_url (URL-encoded) from startBookingPayment
 *   amount   — total, for display
 *   currency — e.g. "ZAR"
 *   method   — card | apple_pay | paypal (for the header label)
 *
 * Flow:
 *   1. On mount, open the authorization_url in the in-app browser.
 *   2. The gateway redirects to the backend /payments/return, which verifies
 *      (Paystack) and bounces to eventsapp://payment/return; the browser closes.
 *   3. Poll GET /v1/bookings/:id/payment for the authoritative outcome.
 *      - 'returned' (browser hit our scheme → likely paid): poll up to ~150s.
 *      - 'dismissed' (user closed it): poll briefly in case they paid then
 *        closed; otherwise treat as cancelled.
 */

import React, { useEffect, useRef, useState, useCallback, useMemo } from 'react';
import {
  View, Text, ActivityIndicator, TouchableOpacity, StyleSheet, StatusBar,
} from 'react-native';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { CheckCircle, Warning, ArrowUUpLeft, HourglassMedium } from 'phosphor-react-native';
import { useAppTheme } from '../../theme/colors';
import {
  openGatewayCheckout,
  startBookingPayment,
  retryGuestPayment,
  pollPayment,
  fetchPaymentStatus,
  type PaymentStatus,
  type PaymentMethod,
} from '../../lib/payments';

type Phase = 'opening' | 'in_browser' | 'confirming' | 'done' | 'failed' | 'cancelled' | 'pending';

const METHOD_LABEL: Record<string, string> = {
  card: 'Card',
  apple_pay: 'Apple Pay',
  paypal: 'PayPal',
};

export default function PaymentScreen() {
  const { isDark } = useAppTheme();
  const styles = React.useMemo(() => makeStyles(isDark), [isDark]);
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const { id: bookingId, url, amount, currency, method, bookingNumber, email } = useLocalSearchParams<{
    id: string; url: string; amount: string; currency: string; method?: string;
    bookingNumber?: string; email?: string;
  }>();

  // Guests have no auth token; their payment is addressed by booking number +
  // email against the /v1/guest/* routes. Present only for guest checkouts
  // (set by purchase.tsx).
  const guest = useMemo(
    () => (bookingNumber && email ? { bookingNumber, email } : undefined),
    [bookingNumber, email],
  );

  const [phase, setPhase]     = useState<Phase>('opening');
  const [message, setMessage] = useState('Opening secure checkout…');
  const startedRef = useRef(false);

  const authorizationUrl = decodeURIComponent(url ?? '');

  const doneTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Return to the Tickets tab WITHOUT remounting the tab group. event → purchase
  // → payment live in the ROOT stack on top of (tabs); a plain
  // router.replace('/(tabs)/tickets') swaps in a FRESH (tabs) instance, which
  // remounts Home (it then refetches every event) and corrupts the back stack
  // (the "GO_BACK was not handled" glitch). Instead pop this stack back to the
  // already-mounted (tabs), then just switch the active tab.
  const goToTickets = useCallback(() => {
    if (doneTimerRef.current) { clearTimeout(doneTimerRef.current); doneTimerRef.current = null; }
    try { if (router.canDismiss()) router.dismissAll(); } catch {}
    router.navigate('/(tabs)/tickets');
  }, [router]);

  // Back to checkout, guarded so we never dispatch GO_BACK on an empty stack.
  const leaveToCheckout = useCallback(() => {
    if (doneTimerRef.current) { clearTimeout(doneTimerRef.current); doneTimerRef.current = null; }
    if (router.canGoBack()) router.back();
    else goToTickets();
  }, [router, goToTickets]);

  // If the user leaves before the success redirect fires, cancel it so we never
  // navigate from an unmounted screen.
  useEffect(() => () => {
    if (doneTimerRef.current) clearTimeout(doneTimerRef.current);
  }, []);

  const finishFromStatus = useCallback((status: PaymentStatus | null, cancelledFallback: boolean) => {
    if (status?.payment_status === 'completed') {
      setPhase('done');
      setMessage('Payment confirmed!');
      doneTimerRef.current = setTimeout(goToTickets, 1400);
    } else if (status?.payment_status === 'failed') {
      setPhase('failed');
      setMessage('Your payment didn’t go through. You can try again.');
    } else if (status?.payment_status === 'pending' && !cancelledFallback) {
      // EFT and other bank-rail methods can sit in "pending" after the gateway
      // hands back — the transfer is initiated but the bank hasn't confirmed yet.
      // This is NOT a failure: when it settles, Paystack's webhook completes the
      // booking and issues tickets. Show a calm processing state with no Retry so
      // the customer doesn't pay a second time. (Only when the gateway actually
      // redirected back — a plain browser-dismiss still reads as cancelled below.)
      setPhase('pending');
      setMessage('Your EFT is processing. We’ll email your tickets the moment your bank confirms — usually within a few minutes. No need to pay again.');
    } else if (cancelledFallback) {
      setPhase('cancelled');
      setMessage('Payment cancelled. You can try again whenever you’re ready.');
    } else {
      setPhase('failed');
      setMessage('We’re still confirming your payment. Your ticket may arrive shortly — check the Tickets tab.');
    }
  }, [goToTickets]);

  const run = useCallback(async (checkoutUrl: string) => {
    if (!checkoutUrl) {
      setPhase('failed');
      setMessage('Missing checkout link. Please go back and try again.');
      return;
    }

    setPhase('in_browser');
    setMessage('Complete your payment in the secure window…');

    const outcome = await openGatewayCheckout(checkoutUrl);

    setPhase('confirming');
    setMessage('Confirming your payment…');

    if (outcome.type === 'returned') {
      // Gateway redirected back through our scheme — they most likely paid.
      try {
        const status = await pollPayment(bookingId, 2500, 150_000, guest);
        finishFromStatus(status, false);
      } catch {
        finishFromStatus(null, false);
      }
    } else {
      // User closed the browser. Poll briefly in case they paid then dismissed;
      // the backend verifies Paystack synchronously on return, so a real payment
      // resolves fast. Otherwise treat as cancelled.
      try {
        const status = await pollPayment(bookingId, 1500, 9_000, guest);
        finishFromStatus(status, true);
      } catch {
        // brief poll timed out → one last direct read, else cancelled
        try {
          const status = await fetchPaymentStatus(bookingId, guest);
          finishFromStatus(status, true);
        } catch {
          finishFromStatus(null, true);
        }
      }
    }
  }, [bookingId, finishFromStatus, guest]);

  useEffect(() => {
    if (startedRef.current) return;
    startedRef.current = true;
    run(authorizationUrl);
  }, [run, authorizationUrl]);

  // Retry re-initialises the payment so we get a FRESH gateway order and the
  // payment record resets to pending. Reusing the old order would just resolve
  // to failed again — a cancelled/declined order can't be re-approved, and the
  // backend short-circuits a payment that's already in a terminal state.
  const retry = useCallback(async () => {
    setPhase('opening');
    setMessage('Restarting secure checkout…');
    try {
      const started = guest
        ? await retryGuestPayment(guest.bookingNumber, guest.email)
        : await startBookingPayment(bookingId, (method as PaymentMethod) || 'card');
      run(started.authorization_url);
    } catch {
      setPhase('failed');
      setMessage('Couldn’t restart checkout. Please go back and start over.');
    }
  }, [bookingId, method, guest, run]);

  const fmtAmount = () => {
    const n = parseFloat(amount ?? '0');
    if (Number.isNaN(n)) return currency ?? 'ZAR';
    return `${currency ?? 'ZAR'} ${Math.round(n).toLocaleString('en-ZA')}`;
  };

  const methodLabel = method ? (METHOD_LABEL[method] ?? '') : '';
  const spinning = phase === 'opening' || phase === 'in_browser' || phase === 'confirming';
  const canRetry = phase === 'failed' || phase === 'cancelled';

  return (
    <View style={[styles.root, { paddingTop: insets.top }]}>
      <StatusBar barStyle={isDark ? 'light-content' : 'dark-content'} />

      <View style={styles.header}>
        <Text style={styles.headerTitle}>
          {methodLabel ? `Pay with ${methodLabel}` : 'Secure Payment'}
        </Text>
      </View>

      <View style={styles.body}>
        <Text style={styles.bigAmount}>{fmtAmount()}</Text>
        {spinning && <ActivityIndicator size="large" color="#ff3c6e" />}
        {phase === 'done' && <CheckCircle size={56} color="#00a843" weight="fill" />}
        {phase === 'failed' && <Warning size={56} color="#f0a020" weight="fill" />}
        {phase === 'cancelled' && <ArrowUUpLeft size={48} color="#6b7280" weight="bold" />}
        {phase === 'pending' && <HourglassMedium size={48} color="#f0a020" weight="fill" />}

        <Text style={[styles.message, phase === 'done' && styles.successText]}>{message}</Text>

        {phase === 'pending' && (
          <TouchableOpacity style={styles.primaryBtn} onPress={goToTickets}>
            <Text style={styles.primaryText}>Go to my tickets</Text>
          </TouchableOpacity>
        )}

        {canRetry && (
          <>
            <TouchableOpacity
              style={styles.primaryBtn}
              onPress={retry}
            >
              <Text style={styles.primaryText}>Try again</Text>
            </TouchableOpacity>
            <TouchableOpacity style={styles.secondaryBtn} onPress={leaveToCheckout}>
              <Text style={styles.secondaryText}>Back to checkout</Text>
            </TouchableOpacity>
            <TouchableOpacity style={styles.linkBtn} onPress={goToTickets}>
              <Text style={styles.linkText}>Check Tickets tab</Text>
            </TouchableOpacity>
          </>
        )}
      </View>
    </View>
  );
}

const makeStyles = (dark: boolean) => StyleSheet.create({
  root:         { flex: 1, backgroundColor: dark ? '#0e0e12' : '#fff' },
  header:       { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 16, paddingVertical: 14,
                  borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: dark ? '#2e2e35' : '#e5e5e5' },
  headerTitle:  { flex: 1, fontSize: 16, fontWeight: '600', color: dark ? '#ffffff' : '#0a0a0a' },
  bigAmount:    { fontSize: 32, fontWeight: '800', color: dark ? '#ffffff' : '#0a0a0a', letterSpacing: -0.6, marginBottom: 4 },
  body:         { flex: 1, alignItems: 'center', justifyContent: 'center', padding: 32, gap: 18 },
  message:      { fontSize: 16, color: dark ? '#c8c8cf' : '#333', textAlign: 'center', lineHeight: 24 },
  successText:  { color: '#00a843', fontWeight: '700', fontSize: 18 },
  primaryBtn:   { backgroundColor: '#ff3c6e', borderRadius: 12, paddingVertical: 13, paddingHorizontal: 36, marginTop: 6 },
  primaryText:  { color: '#fff', fontWeight: '700', fontSize: 15 },
  secondaryBtn: { borderRadius: 12, paddingVertical: 12, paddingHorizontal: 36, borderWidth: 1, borderColor: dark ? '#c8c8cf' : '#1a1a2e' },
  secondaryText:{ color: dark ? '#e5e5ea' : '#1a1a2e', fontWeight: '600', fontSize: 15 },
  linkBtn:      { paddingVertical: 8 },
  linkText:     { color: '#888', fontSize: 14, textDecorationLine: 'underline' },
});