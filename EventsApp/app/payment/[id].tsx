/**
 * app/payment/[id].tsx
 *
 * Opens the Stitch Secure Fields payment page in a WebView.
 * The backend serves a thin HTML host page at:
 *   GET /v1/payments/:bookingId/page
 * which loads the Stitch JS SDK with the client_token already injected.
 *
 * Flow:
 *  1. This screen receives bookingId + clientToken via route params.
 *  2. It opens a WebView pointing at the backend payment page.
 *  3. On success Stitch redirects to eventsapp://payment/success?booking_id=...
 *  4. This screen catches that URL, switches to "confirming" state, and polls
 *     GET /v1/payments/:bookingId/status until payment_status === 'completed'.
 *  5. It then navigates to /(tabs)/tickets and removes itself from the stack.
 */

import React, { useEffect, useRef, useState, useCallback } from 'react';
import {
  View, Text, ActivityIndicator, TouchableOpacity,
  StyleSheet, Alert, Platform, StatusBar,
} from 'react-native';
import { WebView, type WebViewNavigation } from 'react-native-webview';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { pollPaymentStatus } from '../../lib/api';
import { API_BASE_URL } from '../../lib/api';

type Phase = 'loading' | 'paying' | 'confirming' | 'done' | 'failed';

export default function PaymentScreen() {
  const router  = useRouter();
  const insets  = useSafeAreaInsets();
  const { id: bookingId, token: clientToken, amount, currency } = useLocalSearchParams<{
    id: string; token: string; amount: string; currency: string;
  }>();

  const [phase, setPhase]   = useState<Phase>('loading');
  const [message, setMessage] = useState('Loading payment…');
  const webViewRef = useRef<WebView>(null);
  const pollingRef = useRef(false);

  // The backend serves a payment host page that injects the client_token
  // into the Stitch Secure Fields SDK.
  const paymentPageUrl = `${API_BASE_URL}/v1/payments/${bookingId}/page?client_token=${encodeURIComponent(clientToken ?? '')}`;

  const handleSuccess = useCallback(async () => {
    if (pollingRef.current) return;
    pollingRef.current = true;
    setPhase('confirming');
    setMessage('Confirming your payment…');
    try {
      const status = await pollPaymentStatus(bookingId, 2000, 120_000);
      if (status.payment_status === 'completed') {
        setPhase('done');
        setMessage('Payment confirmed! 🎟');
        setTimeout(() => {
          router.replace('/(tabs)/tickets');
        }, 1500);
      } else {
        setPhase('failed');
        setMessage('Payment could not be confirmed. Please check your tickets or try again.');
      }
    } catch {
      setPhase('failed');
      setMessage('Timed out waiting for payment confirmation. Your ticket may still arrive — check the Tickets tab.');
    }
  }, [bookingId, router]);

  const handleNavChange = useCallback((nav: WebViewNavigation) => {
    const url = nav.url ?? '';
    // Catch the Stitch redirect back into the app
    if (url.startsWith('eventsapp://payment/success') || url.includes('/payment/success')) {
      handleSuccess();
      return false; // prevent WebView from navigating to the deep link
    }
    if (url.startsWith('eventsapp://payment/cancel') || url.includes('/payment/cancel')) {
      Alert.alert('Payment cancelled', 'You can retry from the checkout screen.', [
        { text: 'OK', onPress: () => router.back() },
      ]);
      return false;
    }
    return true;
  }, [handleSuccess, router]);

  const fmtAmount = () => {
    const n = parseFloat(amount ?? '0');
    return `${currency ?? 'ZAR'} ${Math.round(n).toLocaleString('en-ZA')}`;
  };

  return (
    <View style={[styles.root, { paddingTop: insets.top }]}>
      <StatusBar barStyle="dark-content" />

      {/* ── Header ── */}
      <View style={styles.header}>
        {(phase === 'paying' || phase === 'loading') && (
          <TouchableOpacity onPress={() => {
            Alert.alert('Leave payment?', 'Your reservation will be held for a few minutes.', [
              { text: 'Stay', style: 'cancel' },
              { text: 'Leave', style: 'destructive', onPress: () => router.back() },
            ]);
          }} style={styles.cancelBtn}>
            <Text style={styles.cancelText}>Cancel</Text>
          </TouchableOpacity>
        )}
        <Text style={styles.headerTitle}>Secure Payment</Text>
        <Text style={styles.headerAmount}>{fmtAmount()}</Text>
      </View>

      {/* ── Overlay states ── */}
      {(phase === 'loading' || phase === 'confirming' || phase === 'done' || phase === 'failed') && (
        <View style={styles.overlay}>
          {phase !== 'failed' && <ActivityIndicator size="large" color={phase === 'done' ? '#00c853' : '#ff3c6e'} />}
          {phase === 'failed' && <Text style={styles.failIcon}>⚠️</Text>}
          <Text style={[styles.overlayText, phase === 'done' && styles.successText]}>{message}</Text>
          {phase === 'failed' && (
            <TouchableOpacity style={styles.retryBtn} onPress={() => router.back()}>
              <Text style={styles.retryText}>Go back</Text>
            </TouchableOpacity>
          )}
          {phase === 'failed' && (
            <TouchableOpacity style={[styles.retryBtn, { marginTop: 10, backgroundColor: '#1a1a2e' }]}
              onPress={() => router.replace('/(tabs)/tickets')}>
              <Text style={styles.retryText}>Check Tickets tab</Text>
            </TouchableOpacity>
          )}
        </View>
      )}

      {/* ── WebView ── */}
      <WebView
        ref={webViewRef}
        source={{ uri: paymentPageUrl }}
        style={[styles.webview, (phase !== 'paying' && phase !== 'loading') && styles.hidden]}
        onLoadStart={() => setPhase('loading')}
        onLoadEnd={() => setPhase('paying')}
        onShouldStartLoadWithRequest={handleNavChange}
        onError={() => {
          setPhase('failed');
          setMessage('Failed to load payment page. Check your connection and try again.');
        }}
        javaScriptEnabled
        domStorageEnabled
        originWhitelist={['*']}
        // Allow the Stitch domain
        allowsInlineMediaPlayback
        mediaPlaybackRequiresUserAction={false}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  root:       { flex: 1, backgroundColor: '#fff' },
  header:     { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 16, paddingVertical: 12,
                borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: '#e5e5e5' },
  cancelBtn:  { marginRight: 'auto' as any },
  cancelText: { color: '#ff3c6e', fontSize: 15 },
  headerTitle:{ flex: 1, textAlign: 'center', fontSize: 16, fontWeight: '600', color: '#0a0a0a' },
  headerAmount:{ fontSize: 14, color: '#666', marginLeft: 'auto' as any },
  webview:    { flex: 1 },
  hidden:     { opacity: 0, height: 0, flex: 0 },
  overlay:    { ...StyleSheet.absoluteFillObject, zIndex: 10, backgroundColor: '#fff',
                alignItems: 'center', justifyContent: 'center', padding: 32, gap: 16 },
  overlayText:{ fontSize: 16, color: '#333', textAlign: 'center', lineHeight: 24 },
  successText:{ color: '#00c853', fontWeight: '700', fontSize: 18 },
  failIcon:   { fontSize: 40 },
  retryBtn:   { backgroundColor: '#ff3c6e', borderRadius: 12, paddingVertical: 12,
                paddingHorizontal: 28 },
  retryText:  { color: '#fff', fontWeight: '700', fontSize: 15 },
});