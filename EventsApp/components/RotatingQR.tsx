import React, {
  useState, useEffect, useRef, useCallback,
} from 'react';
import {
  View, Text, Animated, TouchableOpacity,
  ActivityIndicator, AppState, StyleSheet,
} from 'react-native';
import QRCode from 'react-native-qrcode-svg';
import { Ionicons } from '@expo/vector-icons';
import { api } from '../lib/api';
import { getOfflineIndividualToken, getOfflinePoolToken } from '../lib/offline';

// ── Types ─────────────────────────────────────────────────────────────────────

interface RotatingQRToken {
  token:      string;
  expires_at: string;
  window_sec: number;
  issued_at:  string;
  remaining?: number; // pool tokens only — admissions left in the pool
}

interface Props {
  /** Outer size in px — matches the existing qrImage width (SW - 96). */
  size: number;
  /** Individual ticket mode: the assignment to render a QR for. */
  assignmentId?: string;
  /** When present, fetch via the guest endpoints (no auth token required). */
  guest?: { bookingNumber: string; email: string };
  /**
   * Pool mode: render the buyer's GROUP pass instead of a single ticket.
   * Authed buyer → pass { bookingId }. Guest buyer → pass {} and supply `guest`.
   */
  pool?: { bookingId?: string };
  /** Pool mode only: reports admissions remaining each refresh. */
  onRemaining?: (n: number) => void;
}

// ── Component ─────────────────────────────────────────────────────────────────

export function RotatingQR({ size, assignmentId, guest, pool, onRemaining }: Props) {
  const [token,     setToken]     = useState<string | null>(null);
  const [windowSec, setWindowSec] = useState(30);
  const [secsLeft,  setSecsLeft]  = useState(30);
  const [loading,   setLoading]   = useState(true);
  const [error,     setError]     = useState(false);
  // Offline fallback: the cached, self-contained token we show with no signal.
  const [offlineToken, setOfflineToken] = useState<string | null>(null);

  // Animated progress bar: 1 = full, 0 = empty
  const progressAnim = useRef(new Animated.Value(1)).current;
  const progressAnim2 = useRef(new Animated.Value(1)).current; // second bar for crossfade
  const activeBar = useRef<0 | 1>(0);

  const refreshTimerRef   = useRef<ReturnType<typeof setTimeout> | null>(null);
  const countdownRef      = useRef<ReturnType<typeof setInterval> | null>(null);
  const expiresAtRef      = useRef<number>(0); // timestamp ms
  const animRunningRef    = useRef<Animated.CompositeAnimation | null>(null);
  const mountedRef        = useRef(true);

  const clearTimers = useCallback(() => {
    if (refreshTimerRef.current)  clearTimeout(refreshTimerRef.current);
    if (countdownRef.current)     clearInterval(countdownRef.current);
    animRunningRef.current?.stop();
  }, []);

  // ── Animate the active progress bar from current → 0 ──────────────────────
  const animateBar = useCallback((durationMs: number, fromValue: number) => {
    const bar = activeBar.current === 0 ? progressAnim : progressAnim2;
    animRunningRef.current?.stop();
    bar.setValue(fromValue);
    animRunningRef.current = Animated.timing(bar, {
      toValue: 0,
      duration: Math.max(durationMs, 0),
      useNativeDriver: false,
    });
    animRunningRef.current.start();
  }, [progressAnim, progressAnim2]);

  // ── Load a cached offline token as a fallback when the network fails ───────
  const loadOfflineFallback = useCallback(async (): Promise<boolean> => {
    try {
      let tok: string | null = null;
      if (pool) {
        tok = await getOfflinePoolToken({
          bookingId: pool.bookingId,
          bookingNumber: guest?.bookingNumber,
        });
      } else if (assignmentId) {
        tok = await getOfflineIndividualToken(assignmentId);
      }
      if (!mountedRef.current) return false;
      if (tok) {
        setOfflineToken(tok);
        setError(false);
        setLoading(false);
        return true;
      }
    } catch {
      /* fall through to the error state */
    }
    return false;
  }, [pool, pool?.bookingId, guest?.bookingNumber, assignmentId]);

  // ── Fetch a new (live) token ──────────────────────────────────────────────
  const fetchToken = useCallback(async () => {
    try {
      // Resolve the endpoint by mode: pool vs individual, guest vs authed.
      let res;
      if (pool) {
        res = guest
          ? await api.get<RotatingQRToken>(
              `/v1/guest/bookings/${encodeURIComponent(guest.bookingNumber)}/pool-qr`,
              { params: { email: guest.email } },
            )
          : await api.get<RotatingQRToken>(`/v1/bookings/${pool.bookingId}/pool-qr`);
      } else {
        res = guest
          ? await api.get<RotatingQRToken>(
              `/v1/guest/bookings/${encodeURIComponent(guest.bookingNumber)}/assignments/${assignmentId}/qr`,
              { params: { email: guest.email } },
            )
          : await api.get<RotatingQRToken>(`/v1/tickets/${assignmentId}/qr`);
      }
      if (!mountedRef.current) return;

      const { token, expires_at, window_sec, remaining } = res.data;
      if (pool && typeof remaining === 'number') onRemaining?.(remaining);
      const exp   = new Date(expires_at).getTime();
      const now   = Date.now();
      const msLeft = Math.max(exp - now, 0);

      expiresAtRef.current = exp;
      setToken(token);
      setOfflineToken(null); // back online — drop the static fallback
      setWindowSec(window_sec);
      setSecsLeft(Math.ceil(msLeft / 1000));
      setError(false);
      setLoading(false);

      // Animate the bar over the remaining lifetime
      animateBar(msLeft, msLeft / (window_sec * 1000));

      // Countdown text: tick every second
      if (countdownRef.current) clearInterval(countdownRef.current);
      countdownRef.current = setInterval(() => {
        if (!mountedRef.current) return;
        const left = Math.max(0, Math.ceil((expiresAtRef.current - Date.now()) / 1000));
        setSecsLeft(left);
      }, 500); // 500 ms to avoid off-by-one flicker

      // Refresh 2 s before expiry (but no sooner than 1 s from now)
      if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
      refreshTimerRef.current = setTimeout(fetchToken, Math.max(msLeft - 2000, 1000));
    } catch {
      if (!mountedRef.current) return;

      // Network (or auth) failed — try the cached offline token before erroring.
      const haveOffline = await loadOfflineFallback();
      if (!mountedRef.current) return;

      if (!haveOffline) {
        setError(true);
        setLoading(false);
      }
      // Either way, keep trying to come back online so we resume the live QR
      // (and refresh the countdown) the moment signal returns.
      if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
      refreshTimerRef.current = setTimeout(fetchToken, haveOffline ? 15000 : 5000);
    }
  }, [assignmentId, animateBar, guest?.bookingNumber, guest?.email, pool, pool?.bookingId, onRemaining, loadOfflineFallback]);

  // ── Mount / unmount ────────────────────────────────────────────────────────
  useEffect(() => {
    mountedRef.current = true;
    fetchToken();
    return () => {
      mountedRef.current = false;
      clearTimers();
    };
  }, [fetchToken, clearTimers]);

  // ── Refresh when app comes back to foreground ──────────────────────────────
  useEffect(() => {
    const sub = AppState.addEventListener('change', (state) => {
      if (state === 'active') {
        // If we're on the offline fallback, or the stored expiry has passed,
        // try for a fresh live token immediately.
        if (offlineToken || Date.now() >= expiresAtRef.current - 1000) {
          clearTimers();
          fetchToken();
        }
      }
    });
    return () => sub.remove();
  }, [fetchToken, clearTimers, offlineToken]);

  // ── Derived values ─────────────────────────────────────────────────────────
  const urgency: 'ok' | 'warn' | 'crit' =
    secsLeft <= 4 ? 'crit' : secsLeft <= 9 ? 'warn' : 'ok';

  const barColor =
    urgency === 'crit' ? '#ff4d4d' :
    urgency === 'warn' ? '#ffb830' : '#00e5a0';

  const bar = activeBar.current === 0 ? progressAnim : progressAnim2;

  const qrPadding = 10; // white padding inside the qr container
  const qrCodeSize = size - qrPadding * 2;

  // ── Loading state ──────────────────────────────────────────────────────────
  if (loading) {
    return (
      <View style={[styles.placeholder, { width: size, height: size }]}>
        <ActivityIndicator color="rgba(255,255,255,0.35)" size="small" />
        <Text style={styles.placeholderText}>Loading ticket…</Text>
      </View>
    );
  }

  // ── Offline fallback: static cached token, no live countdown ───────────────
  if (offlineToken) {
    return (
      <View style={styles.wrapper}>
        <View style={[styles.qrBox, { width: size, height: size, padding: qrPadding }]}>
          <QRCode
            value={offlineToken}
            size={qrCodeSize}
            color="#0a0a0a"
            backgroundColor="#ffffff"
            quietZone={6}
          />
        </View>

        {/* Offline badge in place of the live progress bar */}
        <View style={[styles.barTrack, { width: size }]}>
          <View style={[styles.barFill, { backgroundColor: '#8a8f98', width: '100%' }]} />
        </View>

        <View style={styles.statusRow}>
          <Ionicons name="cloud-offline-outline" size={13} color="#c7ccd4" />
          <Text style={[styles.statusText, { color: '#c7ccd4' }]}>
            Offline ticket · valid for entry
          </Text>
        </View>
      </View>
    );
  }

  // ── Error state ────────────────────────────────────────────────────────────
  if (error || !token) {
    return (
      <TouchableOpacity
        style={[styles.placeholder, { width: size, height: size }]}
        onPress={() => { setLoading(true); setError(false); fetchToken(); }}
        activeOpacity={0.75}
      >
        <Ionicons name="wifi-outline" size={32} color="rgba(255,255,255,0.2)" />
        <Text style={styles.placeholderText}>Could not load QR</Text>
        <Text style={styles.retryText}>Tap to retry</Text>
      </TouchableOpacity>
    );
  }

  // ── Live QR ────────────────────────────────────────────────────────────────
  return (
    <View style={styles.wrapper}>

      {/* White box — necessary contrast for scanner cameras */}
      <View style={[styles.qrBox, { width: size, height: size, padding: qrPadding }]}>
        <QRCode
          value={token}
          size={qrCodeSize}
          color="#0a0a0a"
          backgroundColor="#ffffff"
          quietZone={6}
        />
      </View>

      {/* Progress bar */}
      <View style={[styles.barTrack, { width: size }]}>
        <Animated.View
          style={[
            styles.barFill,
            {
              backgroundColor: barColor,
              width: bar.interpolate({
                inputRange:  [0, 1],
                outputRange: ['0%', '100%'],
              }),
            },
          ]}
        />
      </View>

      {/* Status row */}
      <View style={styles.statusRow}>
        {/* Live dot */}
        <View style={[styles.liveDot, { backgroundColor: barColor }]} />
        <Text style={[styles.statusText, { color: barColor }]}>
          {urgency === 'crit'
            ? `Refreshing…`
            : `Live ticket · refreshes in ${secsLeft}s`}
        </Text>
      </View>
    </View>
  );
}

// ── Styles ────────────────────────────────────────────────────────────────────

const styles = StyleSheet.create({
  wrapper: {
    alignItems: 'center',
    gap: 10,
  },
  qrBox: {
    backgroundColor: '#ffffff',
    borderRadius: 12,
    alignItems: 'center',
    justifyContent: 'center',
    // subtle shadow so it lifts off the dark card
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.35,
    shadowRadius: 8,
    elevation: 4,
  },
  barTrack: {
    height: 3,
    borderRadius: 99,
    backgroundColor: 'rgba(255,255,255,0.08)',
    overflow: 'hidden',
  },
  barFill: {
    height: '100%',
    borderRadius: 99,
  },
  statusRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  liveDot: {
    width: 6,
    height: 6,
    borderRadius: 3,
  },
  statusText: {
    fontSize: 11,
    fontWeight: '600',
    letterSpacing: 0.2,
  },
  // ── Placeholder states ──────────────────────────────────────────────────────
  placeholder: {
    borderRadius: 12,
    backgroundColor: 'rgba(255,255,255,0.04)',
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.06)',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
  },
  placeholderText: {
    color: 'rgba(255,255,255,0.25)',
    fontSize: 13,
    fontWeight: '500',
  },
  retryText: {
    color: 'rgba(255,255,255,0.15)',
    fontSize: 11,
    marginTop: 2,
  },
});