import React, { useState } from 'react';
import { View, Text, TouchableOpacity } from 'react-native';
import { useRouter } from 'expo-router';
import BrandLogo from './BrandLogo';

// ─────────────────────────────────────────────────────────────────────────────
// ComingSoon — full-screen cover for a home feed with no events in it at all.
//
// Purely presentational. The home screen decides when to mount it (nothing came
// back from the API) and unmounts it the moment the first event does, so this
// component knows nothing about the feed.
//
// It exists because the alternative is worse than blank: with zero events the
// hero falls back to its ListEmptyComponent, which reads "DISCOVER TOP EVENTS —
// find and book the best events happening near you" above an empty page. That
// promises a catalogue and then shows none of it.
//
// Structure deliberately mirrors the offline "Connect to Network" state in
// (tabs)/index.tsx: centred mark, title, one line of explanation, one primary
// action. A visitor who sees both should recognise them as the same kind of
// screen.
// ─────────────────────────────────────────────────────────────────────────────

export default function ComingSoon({
  colors,
  onRefresh,
}: {
  colors: any;
  onRefresh?: () => Promise<void> | void;
}) {
  const router = useRouter();
  const [refreshing, setRefreshing] = useState(false);

  const handleRefresh = async () => {
    if (refreshing || !onRefresh) return;
    setRefreshing(true);
    try { await onRefresh(); } finally { setRefreshing(false); }
  };

  return (
    <View
      style={{
        flex: 1,
        backgroundColor: colors.background,
        alignItems: 'center',
        justifyContent: 'center',
        paddingHorizontal: 40,
      }}
    >
      <BrandLogo size={78} />

      <Text
        style={{
          color: colors.text,
          fontSize: 26,
          fontWeight: '900',
          letterSpacing: -0.4,
          textAlign: 'center',
          textTransform: 'uppercase',
          marginTop: 22,
        }}
      >
        Events coming soon
        <Text style={{ color: '#FF2D55' }}>!</Text>
      </Text>

      <Text
        style={{
          color: colors.subText,
          fontSize: 13.5,
          lineHeight: 21,
          textAlign: 'center',
          marginTop: 12,
        }}
      >
        Nothing is on sale just yet. The first events land here as soon as
        organisers open their doors.
      </Text>

      <TouchableOpacity
        onPress={handleRefresh}
        disabled={refreshing}
        activeOpacity={0.85}
        style={{
          marginTop: 26,
          paddingHorizontal: 28,
          paddingVertical: 13,
          borderRadius: 12,
          backgroundColor: colors.ctaBg,
          opacity: refreshing ? 0.6 : 1,
        }}
      >
        <Text style={{ color: colors.ctaText, fontSize: 14, fontWeight: '800', letterSpacing: 0.3 }}>
          {refreshing ? 'Checking…' : 'Check Again'}
        </Text>
      </TouchableOpacity>

      {/* Someone who already holds a ticket can still reach it. Without this the
          screen is a dead end for exactly the people with most at stake. */}
      <TouchableOpacity
        // navigate, not push: (tabs) is already mounted and this screen lives
        // INSIDE it, so pushing another (tabs) route would remount the tab group
        // and refetch the whole feed. See the note in app/payment/[id].tsx.
        onPress={() => router.navigate('/(tabs)/tickets')}
        activeOpacity={0.7}
        style={{ marginTop: 16, paddingVertical: 8, paddingHorizontal: 12 }}
      >
        <Text style={{ color: colors.subText, fontSize: 13, fontWeight: '600' }}>
          View my tickets
        </Text>
      </TouchableOpacity>
    </View>
  );
}