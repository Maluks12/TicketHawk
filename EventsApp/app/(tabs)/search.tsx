import React, { useState, useRef, useCallback, useEffect } from 'react';
import {
  View,
  Text,
  TextInput,
  TouchableOpacity,
  FlatList,
  Image,
  ActivityIndicator,
  StyleSheet,
  Keyboard,
  Animated,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { searchEvents, getSuggestionEvents, type Event } from '../../lib/api';
import { useEventStore } from '../../store/eventStore';

// ─── Constants ────────────────────────────────────────────────────────────────

const ACCENT    = '#ff3c6e';
const DARK      = '#0a0a0a';
const DARK_BG   = '#002047';
const GREY_TEXT = 'rgba(10,10,10,0.45)';
const BORDER    = 'rgba(10,10,10,0.10)';

const SUGGESTIONS = [
  {
    key: 'popular',
    label: 'Popular Events',
    icon: 'star' as const,
    iconColor: '#3b82f6',
    bgColor: 'rgba(59,130,246,0.12)',
  },
  {
    key: 'just_announced',
    label: 'Just Announced',
    icon: 'megaphone' as const,
    iconColor: '#a855f7',
    bgColor: 'rgba(168,85,247,0.12)',
  },
];

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatPrice(price: number) {
  if (price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

function formatDate(iso: string) {
  if (!iso) return '';
  const d = new Date(iso);
  return d.toLocaleDateString('en-ZA', { day: 'numeric', month: 'short', year: 'numeric' });
}

// ─── Sub-components ───────────────────────────────────────────────────────────

const SuggestionRow = ({
  item,
  onPress,
}: {
  item: (typeof SUGGESTIONS)[number];
  onPress: () => void;
}) => (
  <TouchableOpacity style={styles.suggRow} onPress={onPress} activeOpacity={0.7}>
    <View style={[styles.suggIcon, { backgroundColor: item.bgColor }]}>
      <Ionicons name={item.icon} size={20} color={item.iconColor} />
    </View>
    <Text style={styles.suggLabel}>{item.label}</Text>
    <Ionicons name="chevron-forward" size={18} color="rgba(10,10,10,0.25)" />
  </TouchableOpacity>
);

const EventResultRow = ({
  item,
  onPress,
}: {
  item: Event;
  onPress: () => void;
}) => (
  <TouchableOpacity style={styles.resultRow} onPress={onPress} activeOpacity={0.75}>
    {/* Thumbnail */}
    <View style={styles.resultThumb}>
      {item.cover_image_url ? (
        <Image source={{ uri: item.cover_image_url }} style={StyleSheet.absoluteFillObject} resizeMode="cover" />
      ) : (
        <View style={[StyleSheet.absoluteFillObject, { backgroundColor: '#e5e5e5' }]} />
      )}
    </View>

    {/* Info */}
    <View style={styles.resultInfo}>
      <Text style={styles.resultCat} numberOfLines={1}>{item.category?.toUpperCase()}</Text>
      <Text style={styles.resultTitle} numberOfLines={2}>{item.title}</Text>
      <Text style={styles.resultMeta} numberOfLines={1}>
        📍 {item.venue_name}{item.city ? `, ${item.city}` : ''}
      </Text>
      {item.start_time ? (
        <Text style={styles.resultDate}>{formatDate(item.start_time)}</Text>
      ) : null}
    </View>

    {/* Price */}
    <View style={styles.resultPriceCol}>
      <Text style={styles.resultPrice}>{formatPrice(item.price)}</Text>
    </View>
  </TouchableOpacity>
);

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function SearchScreen() {
  const router          = useRouter();
  const setSelectedEvent = useEventStore((s) => s.setSelectedEvent);

  const inputRef = useRef<TextInput>(null);
  const debounce = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [query,        setQuery]        = useState('');
  const [isFocused,    setIsFocused]    = useState(false);
  const [results,      setResults]      = useState<Event[]>([]);
  const [loading,      setLoading]      = useState(false);
  const [suggResults,  setSuggResults]  = useState<Event[]>([]);
  const [suggTitle,    setSuggTitle]    = useState('');
  const [suggLoading,  setSuggLoading]  = useState(false);
  const [view,         setView]         = useState<'idle' | 'results' | 'suggestion'>('idle');

  // Animated underline for active search bar
  const underlineAnim = useRef(new Animated.Value(0)).current;

  const animateFocus = (focused: boolean) => {
    Animated.timing(underlineAnim, {
      toValue: focused ? 1 : 0,
      duration: 180,
      useNativeDriver: false,
    }).start();
  };

  // ── Live search ────────────────────────────────────────────────────────────

  const runSearch = useCallback(async (q: string) => {
    if (!q.trim()) {
      setResults([]);
      setView('idle');
      return;
    }
    setLoading(true);
    setView('results');
    const hits = await searchEvents(q);
    setResults(hits);
    setLoading(false);
  }, []);

  const handleQueryChange = (text: string) => {
    setQuery(text);
    if (debounce.current) clearTimeout(debounce.current);
    debounce.current = setTimeout(() => runSearch(text), 320);
  };

  const handleClear = () => {
    setQuery('');
    setResults([]);
    setView('idle');
    inputRef.current?.focus();
  };

  const handleCancel = () => {
    Keyboard.dismiss();
    setQuery('');
    setResults([]);
    setView('idle');
    setIsFocused(false);
  };

  // ── Suggestion tap ─────────────────────────────────────────────────────────

  const handleSuggestion = async (key: string, label: string) => {
    Keyboard.dismiss();
    setSuggTitle(label);
    setSuggLoading(true);
    setView('suggestion');
    const events = await getSuggestionEvents(key as 'popular' | 'just_announced');
    setSuggResults(events);
    setSuggLoading(false);
  };

  // ── Event press ────────────────────────────────────────────────────────────

  const handleEventPress = (event: Event) => {
    setSelectedEvent(event);
    router.push(`/event/${event.id}`);
  };

  // ── Render ─────────────────────────────────────────────────────────────────

  const showCancel  = isFocused || query.length > 0;
  const activeItems = view === 'suggestion' ? suggResults : results;

  return (
    <SafeAreaView style={styles.safe}>

      {/* ── Header ── */}
      <View style={styles.header}>
        <Text style={styles.headerTitle}>Search</Text>
      </View>

      {/* ── Search bar ── */}
      <View style={styles.searchRow}>
        <View style={[styles.searchBox, isFocused && styles.searchBoxFocused]}>
          <Ionicons name="search" size={17} color={GREY_TEXT} style={{ marginRight: 8 }} />
          <TextInput
            ref={inputRef}
            value={query}
            onChangeText={handleQueryChange}
            onFocus={() => { setIsFocused(true); animateFocus(true); }}
            onBlur={() => { animateFocus(false); }}
            placeholder="Search events…"
            placeholderTextColor={GREY_TEXT}
            style={styles.searchInput}
            returnKeyType="search"
            onSubmitEditing={() => runSearch(query)}
            clearButtonMode="never"
            autoCorrect={false}
            autoCapitalize="none"
          />
          {query.length > 0 && (
            <TouchableOpacity onPress={handleClear} hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}>
              <Ionicons name="close-circle" size={16} color={GREY_TEXT} />
            </TouchableOpacity>
          )}
        </View>
        {showCancel && (
          <TouchableOpacity onPress={handleCancel} style={styles.cancelBtn} activeOpacity={0.7}>
            <Text style={styles.cancelText}>Cancel</Text>
          </TouchableOpacity>
        )}
      </View>

      {/* ── Content ── */}

      {/* IDLE: Suggestions */}
      {view === 'idle' && (
        <FlatList
          data={SUGGESTIONS}
          keyExtractor={(s) => s.key}
          contentContainerStyle={styles.suggContainer}
          ListHeaderComponent={
            <Text style={styles.suggHeading}>Suggestions</Text>
          }
          renderItem={({ item }) => (
            <SuggestionRow
              item={item}
              onPress={() => handleSuggestion(item.key, item.label)}
            />
          )}
          ItemSeparatorComponent={() => <View style={styles.divider} />}
          keyboardShouldPersistTaps="handled"
        />
      )}

      {/* RESULTS / SUGGESTION: Event list */}
      {(view === 'results' || view === 'suggestion') && (
        <>
          {/* Back link when browsing a suggestion */}
          {view === 'suggestion' && (
            <TouchableOpacity
              style={styles.backRow}
              onPress={() => { setView('idle'); setSuggResults([]); }}
              activeOpacity={0.7}
            >
              <Ionicons name="chevron-back" size={16} color={ACCENT} />
              <Text style={styles.backText}>  {suggTitle}</Text>
            </TouchableOpacity>
          )}

          {loading || suggLoading ? (
            <View style={styles.centred}>
              <ActivityIndicator size="large" color={ACCENT} />
            </View>
          ) : (
            <FlatList
              data={activeItems}
              keyExtractor={(e) => e.id}
              contentContainerStyle={styles.resultsList}
              keyboardShouldPersistTaps="handled"
              renderItem={({ item }) => (
                <EventResultRow item={item} onPress={() => handleEventPress(item)} />
              )}
              ItemSeparatorComponent={() => <View style={styles.divider} />}
              ListEmptyComponent={
                <View style={styles.centred}>
                  <Ionicons name="search-outline" size={48} color="rgba(10,10,10,0.15)" />
                  <Text style={styles.emptyTitle}>No events found</Text>
                  <Text style={styles.emptySubtitle}>
                    {view === 'results'
                      ? `No results for "${query}". Try a different keyword.`
                      : 'Check back later for updates.'}
                  </Text>
                </View>
              }
            />
          )}
        </>
      )}
    </SafeAreaView>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const styles = StyleSheet.create({

  safe: {
    flex: 1,
    backgroundColor: '#ffffff',
  },

  // Header
  header: {
    paddingHorizontal: 20,
    paddingTop: 12,
    paddingBottom: 4,
  },
  headerTitle: {
    fontSize: 28,
    fontWeight: '800',
    color: DARK,
    letterSpacing: 0.2,
  },

  // Search bar
  searchRow: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 16,
    paddingVertical: 10,
    gap: 10,
  },
  searchBox: {
    flex: 1,
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: 'rgba(10,10,10,0.06)',
    borderRadius: 14,
    paddingHorizontal: 14,
    height: 46,
    borderWidth: 1.5,
    borderColor: 'transparent',
  },
  searchBoxFocused: {
    borderColor: ACCENT,
    backgroundColor: '#fff',
  },
  searchInput: {
    flex: 1,
    fontSize: 15,
    color: DARK,
    fontWeight: '400',
  },
  cancelBtn: {
    paddingVertical: 6,
    paddingLeft: 2,
  },
  cancelText: {
    fontSize: 16,
    color: ACCENT,
    fontWeight: '600',
  },

  // Suggestions
  suggContainer: {
    paddingTop: 4,
    paddingBottom: 40,
  },
  suggHeading: {
    fontSize: 18,
    fontWeight: '800',
    color: DARK,
    paddingHorizontal: 20,
    paddingTop: 12,
    paddingBottom: 8,
  },
  suggRow: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 20,
    paddingVertical: 16,
    backgroundColor: '#fff',
  },
  suggIcon: {
    width: 42,
    height: 42,
    borderRadius: 21,
    alignItems: 'center',
    justifyContent: 'center',
    marginRight: 16,
  },
  suggLabel: {
    flex: 1,
    fontSize: 16,
    fontWeight: '500',
    color: DARK,
  },

  divider: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: BORDER,
    marginLeft: 20,
  },

  // Back nav
  backRow: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 16,
    paddingVertical: 10,
  },
  backText: {
    fontSize: 16,
    fontWeight: '700',
    color: ACCENT,
  },

  // Results
  resultsList: {
    paddingBottom: 100,
  },
  resultRow: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 16,
    paddingVertical: 14,
    gap: 14,
  },
  resultThumb: {
    width: 72,
    height: 60,
    borderRadius: 10,
    overflow: 'hidden',
    backgroundColor: '#e5e5e5',
    flexShrink: 0,
  },
  resultInfo: {
    flex: 1,
  },
  resultCat: {
    fontSize: 9,
    fontWeight: '800',
    color: ACCENT,
    letterSpacing: 1.4,
    marginBottom: 2,
  },
  resultTitle: {
    fontSize: 14,
    fontWeight: '700',
    color: DARK,
    lineHeight: 19,
    marginBottom: 3,
  },
  resultMeta: {
    fontSize: 11,
    color: GREY_TEXT,
    marginBottom: 1,
  },
  resultDate: {
    fontSize: 11,
    color: GREY_TEXT,
  },
  resultPriceCol: {
    alignItems: 'flex-end',
    justifyContent: 'center',
    minWidth: 56,
  },
  resultPrice: {
    fontSize: 13,
    fontWeight: '800',
    color: DARK,
  },

  // Empty / loading
  centred: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    paddingTop: 80,
    paddingHorizontal: 32,
  },
  emptyTitle: {
    fontSize: 17,
    fontWeight: '700',
    color: DARK,
    marginTop: 16,
    marginBottom: 6,
  },
  emptySubtitle: {
    fontSize: 13,
    color: GREY_TEXT,
    textAlign: 'center',
    lineHeight: 19,
  },
});