import React, { useState, useRef, useCallback, useEffect } from 'react';
import {
  View, Text, TextInput, TouchableOpacity, FlatList,
  ActivityIndicator, StyleSheet, Keyboard, Animated,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { searchEvents, getActiveEvents, getTrendingEvents, type Event } from '../../lib/api';
import { useEventStore } from '../../store/eventStore';
import { s, sheet } from '../../styles/search';
import { useAppTheme } from '../../theme/colors';
import { CoverImage } from '../../components/CoverImage';

// ─── Constants ────────────────────────────────────────────────────────────────

const ACCENT          = '#ff3c6e';
const SUGGESTED_COUNT = 12;   // how many random picks to show
const POOL_LIMIT      = 40;   // how many events to pull into the shuffle pool

// ─── Helpers ──────────────────────────────────────────────────────────────────

const formatDate  = (iso: string)   => iso ? new Date(iso).toLocaleDateString('en-ZA', { day: 'numeric', month: 'short' }) : '';

// Fisher–Yates shuffle — returns a new array, never mutates the input.
function shuffle<T>(arr: T[]): T[] {
  const a = [...arr];
  for (let i = a.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [a[i], a[j]] = [a[j], a[i]];
  }
  return a;
}

// Merge several event lists into one de-duplicated pool (by id).
function dedupe(...lists: Event[][]): Event[] {
  const seen = new Set<string>();
  const out: Event[] = [];
  for (const list of lists) {
    for (const ev of list) {
      if (ev && ev.id && !seen.has(ev.id)) { seen.add(ev.id); out.push(ev); }
    }
  }
  return out;
}

// ─── Sub-components ───────────────────────────────────────────────────────────

// Recommendation / result row: landscape thumbnail · title · date & city · chevron.
const SuggestedRow = ({ item, onPress, colors }: { item: Event; onPress: () => void; colors: any }) => {
  const meta = [formatDate(item.start_time), item.city].filter(Boolean).join(' · ');
  return (
    <TouchableOpacity style={sheet.nfRow} onPress={onPress} activeOpacity={0.7}>
      <View style={[sheet.nfThumb, { backgroundColor: colors.sectionCardBg }]}>
        {item.cover_image_url
          ? <CoverImage uri={item.cover_image_url} style={StyleSheet.absoluteFillObject} recyclingKey={item.id} />
          : <View style={[StyleSheet.absoluteFillObject, { alignItems: 'center', justifyContent: 'center' }]}>
              <Ionicons name="ticket-outline" size={22} color={colors.iconColor} />
            </View>
        }
      </View>

      <View style={{ flex: 1, justifyContent: 'center' }}>
        <Text style={{ fontSize: 16, fontWeight: '700', color: colors.text, lineHeight: 21 }} numberOfLines={2}>
          {item.title}
        </Text>
        {meta ? (
          <Text style={{ fontSize: 12, color: colors.subText, marginTop: 3 }} numberOfLines={1}>
            {meta}
          </Text>
        ) : null}
      </View>

      <Ionicons name="chevron-forward" size={20} color={colors.iconColor} style={{ flexShrink: 0 }} />
    </TouchableOpacity>
  );
};

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function SearchScreen() {
  const router           = useRouter();
  const setSelectedEvent = useEventStore(st => st.setSelectedEvent);
  const inputRef         = useRef<TextInput>(null);
  const debounce         = useRef<ReturnType<typeof setTimeout> | null>(null);
  const underlineAnim    = useRef(new Animated.Value(0)).current;
  const { isDark, colors } = useAppTheme();

  const [query,     setQuery]     = useState('');
  const [isFocused, setIsFocused] = useState(false);
  const [results,   setResults]   = useState<Event[]>([]);
  const [loading,   setLoading]   = useState(false);
  const [view,      setView]      = useState<'idle' | 'typing' | 'results'>('idle');

  // Random "Recommended" pool
  const pool          = useRef<Event[]>([]);            // full fetched pool (kept for re-shuffles)
  const [suggested,   setSuggested]   = useState<Event[]>([]);
  const [suggLoading, setSuggLoading] = useState(true);

  // Fetch the pool once on mount, then show a random slice.
  useEffect(() => {
    let active = true;
    (async () => {
      try {
        const [activeRes, trendingRes] = await Promise.allSettled([
          getActiveEvents(POOL_LIMIT),
          getTrendingEvents(POOL_LIMIT),
        ]);
        const activeEvents   = activeRes.status   === 'fulfilled' ? (activeRes.value.data.data   ?? []) : [];
        const trendingEvents = trendingRes.status === 'fulfilled' ? (trendingRes.value.data.data ?? []) : [];
        if (!active) return;
        pool.current = dedupe(activeEvents, trendingEvents);
        setSuggested(shuffle(pool.current).slice(0, SUGGESTED_COUNT));
      } catch {
        if (active) setSuggested([]);
      } finally {
        if (active) setSuggLoading(false);
      }
    })();
    return () => { active = false; };
  }, []);

  const animateFocus = (focused: boolean) =>
    Animated.timing(underlineAnim, { toValue: focused ? 1 : 0, duration: 180, useNativeDriver: false }).start();

  const MIN_QUERY_LENGTH = 3;

  const runSearch = useCallback(async (q: string) => {
    const trimmed = q.trim();
    if (!trimmed) { setResults([]); setView('idle'); return; }
    if (trimmed.length < MIN_QUERY_LENGTH) { setResults([]); setView('typing'); return; }
    setLoading(true); setView('results');
    setResults(await searchEvents(trimmed));
    setLoading(false);
  }, []);

  const handleQueryChange = (text: string) => {
    setQuery(text);
    if (debounce.current) clearTimeout(debounce.current);
    debounce.current = setTimeout(() => runSearch(text), 320);
  };

  const handleClear  = () => { setQuery(''); setResults([]); setView('idle'); inputRef.current?.focus(); };
  const handleCancel = () => { Keyboard.dismiss(); setQuery(''); setResults([]); setView('idle'); setIsFocused(false); };

  const handleEventPress = (event: Event) => { setSelectedEvent(event); router.push(`/event/${event.id}`); };

  const showCancel  = isFocused || query.length > 0;
  const searchBoxBg = isFocused
    ? (isDark ? '#1c1c1c' : '#fff')
    : (isDark ? 'rgba(255,255,255,0.07)' : 'rgba(10,10,10,0.06)');

  return (
     <SafeAreaView style={{ flex: 1, backgroundColor: colors.background }} edges={['top']}>

      {/* Header */}
     <View style={{ paddingHorizontal: 20, paddingTop: 12, paddingBottom: 4, backgroundColor: colors.background }}>
        <Text style={{ fontSize: 28, fontWeight: '800', color: colors.text, letterSpacing: 0.2 }}>Search</Text>
      </View>

      {/* Search bar */}
      <View className={s.searchRow}>
           <View style={[sheet.searchBox, { backgroundColor: searchBoxBg }, isFocused && { borderColor: '#ff3c6e' }]}>
          <TextInput
            ref={inputRef}
            value={query}
            onChangeText={handleQueryChange}
            onFocus={() => { setIsFocused(true); animateFocus(true); }}
            onBlur={() => animateFocus(false)}
            placeholder="Search events…"
            placeholderTextColor={colors.iconColor} style={{ flex: 1, fontSize: 15, color: colors.text }} />
          {query.length > 0 && (
            <TouchableOpacity onPress={handleClear} hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}>
              <Ionicons name="close-circle" size={16} color={colors.iconColor} />
            </TouchableOpacity>
          )}
        </View>
        {showCancel && (
          <TouchableOpacity onPress={handleCancel} className={s.cancelBtn} activeOpacity={0.7}>
            <Text className={s.cancelText}>Cancel</Text>
          </TouchableOpacity>
        )}
      </View>

      {/* Idle: Netflix-style random recommendations */}
      {view === 'idle' && (
        suggLoading ? (
          <View className={s.centred}>
            <ActivityIndicator size="large" color={ACCENT} />
          </View>
        ) : (
          <FlatList
            data={suggested}
            keyExtractor={item => item.id}
            ListHeaderComponent={
              <Text style={{ fontSize: 20, fontWeight: '800', color: colors.text, paddingHorizontal: 20, paddingTop: 12, paddingBottom: 10 }}>
                Recommended Events
              </Text>
            }
            renderItem={({ item }) => (
              <SuggestedRow item={item} onPress={() => handleEventPress(item)} colors={colors} />
            )}
            keyboardShouldPersistTaps="handled"
            contentContainerStyle={{ paddingBottom: 24 }}
            ListEmptyComponent={
              <View className={s.centred}>
                <Ionicons name="sparkles-outline" size={44} color={colors.iconColor} />
                <Text style={{ fontSize: 17, fontWeight: '700', color: colors.text, marginTop: 16, marginBottom: 6 }}>Nothing to suggest yet</Text>
                <Text style={{ fontSize: 13, color: colors.subText, textAlign: 'center', lineHeight: 19 }}>
                  Search for an event above to get started.
                </Text>
              </View>
            }
          />
        )
      )}

      {/* Typing: show hint until 3 chars */}
      {view === 'typing' && (
        <View style={{ alignItems: 'center', paddingTop: 60, paddingHorizontal: 40 }}>
          <Ionicons name="search-outline" size={40} color={colors.iconColor} />
          <Text style={{ fontSize: 15, color: colors.subText, marginTop: 14, textAlign: 'center', lineHeight: 22 }}>
            Keep typing… results appear after{'\n'}
            <Text style={{ color: ACCENT, fontWeight: '700' }}>3 characters</Text>
          </Text>
        </View>
      )}

      {/* Results */}
      {view === 'results' && (
        loading ? (
          <View className={s.centred}>
            <ActivityIndicator size="large" color={ACCENT} />
          </View>
        ) : (
          <FlatList
            data={results}
            keyExtractor={e => e.id}
            keyboardDismissMode="on-drag"
            keyboardShouldPersistTaps="handled"
            renderItem={({ item }) => <SuggestedRow item={item} onPress={() => handleEventPress(item)} colors={colors} />}
            contentContainerStyle={{ paddingBottom: 24 }}
            ListEmptyComponent={
              <View className={s.centred}>
                <Ionicons name="search-outline" size={48} color={colors.iconColor} />
                <Text style={{ fontSize: 17, fontWeight: '700', color: colors.text, marginTop: 16, marginBottom: 6 }}>No events found</Text>
                <Text style={{ fontSize: 13, color: colors.subText, textAlign: 'center', lineHeight: 19 }}>
                  No results for "{query}". Try a different keyword.
                </Text>
              </View>
            }
          />
        )
      )}
    </SafeAreaView>
  );
}