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
import { prefetchImages } from '../../lib/img';

// ─── Constants ────────────────────────────────────────────────────────────────

const ACCENT          = '#ff3c6e';
const SUGGESTED_COUNT = 12;   // how many random picks to show
const POOL_LIMIT      = 40;   // how many events to pull into the shuffle pool

// Row geometry. The thumbnail (68pt) is always taller than the text column
// (2 title lines at 21pt + a 15pt meta line), so the row height is fixed —
// which lets us hand FlatList a getItemLayout and skip on-the-fly measurement.
const ROW_HEIGHT = 68 + 9 * 2;   // nfThumb height + nfRow vertical padding

// How many rows are visible on a first screenful. These get priority="high" so
// expo-image's queue serves them before anything below the fold.
const ABOVE_FOLD = 7;

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
const SuggestedRow = React.memo(({ item, onPress, colors, priority }: {
  item: Event; onPress: () => void; colors: any; priority?: 'low' | 'normal' | 'high';
}) => {
  const meta = [formatDate(item.start_time), item.city].filter(Boolean).join(' · ');
  return (
    <TouchableOpacity style={sheet.nfRow} onPress={onPress} activeOpacity={0.7}>
      <View style={[sheet.nfThumb, { backgroundColor: colors.sectionCardBg }]}>
        {item.cover_image_url
          ? <CoverImage
              uri={item.cover_image_url}
              style={StyleSheet.absoluteFillObject}
              recyclingKey={item.id}
              priority={priority ?? 'normal'}
            />
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
}, (a, b) => a.item.id === b.item.id && a.priority === b.priority && a.colors === b.colors);

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
        const picks = shuffle(pool.current).slice(0, SUGGESTED_COUNT);
        setSuggested(picks);

        // Warm the disk cache for the rows that will be on screen the instant
        // the spinner goes away. Without this, every visible thumbnail started
        // its download only once its row had mounted, so the list rendered as
        // a column of empty grey rectangles that filled in one by one over
        // several seconds. The home feed already did this; search never did.
        //
        // Deliberately awaited (with a ceiling) rather than fired and
        // forgotten: holding the existing spinner for a beat longer reads as
        // "loading", whereas showing an empty list reads as "broken".
        await Promise.race([
          prefetchImages(picks.slice(0, ABOVE_FOLD).map((e) => e.cover_image_url)),
          new Promise((r) => setTimeout(r, 1200)),   // never block on a bad network
        ]);
        // Everything below the fold is warmed in the background — by the time
        // the user has scrolled, it's already on disk.
        void prefetchImages(picks.slice(ABOVE_FOLD).map((e) => e.cover_image_url));
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
    const found = await searchEvents(trimmed);
    // Same treatment as the recommended list: warm the first screenful before
    // swapping the spinner out, so results appear complete rather than filling
    // in row by row.
    await Promise.race([
      prefetchImages(found.slice(0, ABOVE_FOLD).map((e) => e.cover_image_url)),
      new Promise((r) => setTimeout(r, 900)),
    ]);
    void prefetchImages(found.slice(ABOVE_FOLD).map((e) => e.cover_image_url));
    setResults(found);
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
            renderItem={({ item, index }) => (
              <SuggestedRow
                item={item}
                onPress={() => handleEventPress(item)}
                colors={colors}
                priority={index < ABOVE_FOLD ? 'high' : 'normal'}
              />
            )}
            // Row height is fixed (see ROW_HEIGHT), so FlatList can place rows
            // without measuring them — no layout pass, no scroll-position
            // correction, and the first screenful commits in one batch.
            getItemLayout={(_, index) => ({ length: ROW_HEIGHT, offset: ROW_HEIGHT * index, index })}
            initialNumToRender={ABOVE_FOLD}
            maxToRenderPerBatch={6}
            windowSize={7}
            removeClippedSubviews
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
            renderItem={({ item, index }) => (
              <SuggestedRow
                item={item}
                onPress={() => handleEventPress(item)}
                colors={colors}
                priority={index < ABOVE_FOLD ? 'high' : 'normal'}
              />
            )}
            getItemLayout={(_, index) => ({ length: ROW_HEIGHT, offset: ROW_HEIGHT * index, index })}
            initialNumToRender={ABOVE_FOLD}
            maxToRenderPerBatch={6}
            windowSize={7}
            removeClippedSubviews
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