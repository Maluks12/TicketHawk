import React, { useState, useRef, useCallback } from 'react';
import {
  View, Text, TextInput, TouchableOpacity, FlatList,
  Image, ActivityIndicator, StyleSheet, Keyboard, Animated,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { searchEvents, getSuggestionEvents, type Event } from '../../lib/api';
import { useEventStore } from '../../store/eventStore';
import { s, sheet } from '../../styles/search';

// ─── Constants ────────────────────────────────────────────────────────────────

const ACCENT    = '#ff3c6e';
const GREY_TEXT = 'rgba(10,10,10,0.45)';

const SUGGESTIONS = [
  { key: 'popular',        label: 'Popular Events',  icon: 'star'     as const, iconColor: '#3b82f6', bgColor: 'rgba(59,130,246,0.12)'  },
  { key: 'just_announced', label: 'Just Announced',  icon: 'megaphone'as const, iconColor: '#a855f7', bgColor: 'rgba(168,85,247,0.12)' },
];

// ─── Helpers ──────────────────────────────────────────────────────────────────

const formatPrice = (price: number) => price === 0 ? 'Free' : `R${Math.round(price).toLocaleString('en-ZA')}`;
const formatDate  = (iso: string)   => iso ? new Date(iso).toLocaleDateString('en-ZA', { day: 'numeric', month: 'short', year: 'numeric' }) : '';

// ─── Sub-components ───────────────────────────────────────────────────────────

const SuggestionRow = ({ item, onPress }: { item: typeof SUGGESTIONS[number]; onPress: () => void }) => (
  <TouchableOpacity className={s.suggRow} onPress={onPress} activeOpacity={0.7}>
    <View style={[sheet.suggIcon, { backgroundColor: item.bgColor }]}>
      <Ionicons name={item.icon} size={20} color={item.iconColor} />
    </View>
    <Text className={s.suggLabel}>{item.label}</Text>
    <Ionicons name="chevron-forward" size={18} color="rgba(10,10,10,0.25)" />
  </TouchableOpacity>
);

const EventResultRow = ({ item, onPress }: { item: Event; onPress: () => void }) => (
  <TouchableOpacity className={s.resultRow} onPress={onPress} activeOpacity={0.75}>
    <View style={sheet.resultThumb}>
      {item.cover_image_url
        ? <Image source={{ uri: item.cover_image_url }} style={StyleSheet.absoluteFillObject} resizeMode="cover" />
        : <View style={[StyleSheet.absoluteFillObject, { backgroundColor: '#e5e5e5' }]} />
      }
    </View>
    <View className={s.resultInfo}>
      <Text className={s.resultCat} numberOfLines={1}>{item.category?.toUpperCase()}</Text>
      <Text className={s.resultTitle} numberOfLines={2}>{item.title}</Text>
      <Text className={s.resultMeta} numberOfLines={1}>📍 {item.venue_name}{item.city ? `, ${item.city}` : ''}</Text>
      {item.start_time ? <Text className={s.resultDate}>{formatDate(item.start_time)}</Text> : null}
    </View>
    <View className={s.resultPriceCol}>
      <Text className={s.resultPrice}>{formatPrice(item.price)}</Text>
    </View>
  </TouchableOpacity>
);

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function SearchScreen() {
  const router           = useRouter();
  const setSelectedEvent = useEventStore(st => st.setSelectedEvent);
  const inputRef         = useRef<TextInput>(null);
  const debounce         = useRef<ReturnType<typeof setTimeout> | null>(null);
  const underlineAnim    = useRef(new Animated.Value(0)).current;

  const [query,       setQuery]       = useState('');
  const [isFocused,   setIsFocused]   = useState(false);
  const [results,     setResults]     = useState<Event[]>([]);
  const [loading,     setLoading]     = useState(false);
  const [suggResults, setSuggResults] = useState<Event[]>([]);
  const [suggTitle,   setSuggTitle]   = useState('');
  const [suggLoading, setSuggLoading] = useState(false);
  const [view,        setView]        = useState<'idle' | 'results' | 'suggestion'>('idle');

  const animateFocus = (focused: boolean) =>
    Animated.timing(underlineAnim, { toValue: focused ? 1 : 0, duration: 180, useNativeDriver: false }).start();

  const runSearch = useCallback(async (q: string) => {
    if (!q.trim()) { setResults([]); setView('idle'); return; }
    setLoading(true); setView('results');
    setResults(await searchEvents(q));
    setLoading(false);
  }, []);

  const handleQueryChange = (text: string) => {
    setQuery(text);
    if (debounce.current) clearTimeout(debounce.current);
    debounce.current = setTimeout(() => runSearch(text), 320);
  };

  const handleClear  = () => { setQuery(''); setResults([]); setView('idle'); inputRef.current?.focus(); };
  const handleCancel = () => { Keyboard.dismiss(); setQuery(''); setResults([]); setView('idle'); setIsFocused(false); };

  const handleSuggestion = async (key: string, label: string) => {
    Keyboard.dismiss(); setSuggTitle(label); setSuggLoading(true); setView('suggestion');
    setSuggResults(await getSuggestionEvents(key as 'popular' | 'just_announced'));
    setSuggLoading(false);
  };

  const handleEventPress = (event: Event) => { setSelectedEvent(event); router.push(`/event/${event.id}`); };

  const showCancel  = isFocused || query.length > 0;
  const activeItems = view === 'suggestion' ? suggResults : results;

  return (
    <SafeAreaView className={s.safe}>

      {/* Header */}
      <View className={s.header}>
        <Text className={s.headerTitle}>Search</Text>
      </View>

      {/* Search bar */}
      <View className={s.searchRow}>
        <View style={[sheet.searchBox, isFocused && sheet.searchBoxFocused]}>
          <Ionicons name="search" size={17} color={GREY_TEXT} style={{ marginRight: 8 }} />
          <TextInput
            ref={inputRef}
            value={query}
            onChangeText={handleQueryChange}
            onFocus={() => { setIsFocused(true); animateFocus(true); }}
            onBlur={() => animateFocus(false)}
            placeholder="Search events…"
            placeholderTextColor={GREY_TEXT}
            className={s.searchInput}
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
          <TouchableOpacity onPress={handleCancel} className={s.cancelBtn} activeOpacity={0.7}>
            <Text className={s.cancelText}>Cancel</Text>
          </TouchableOpacity>
        )}
      </View>

      {/* Idle: suggestions */}
      {view === 'idle' && (
        <FlatList
          data={SUGGESTIONS}
          keyExtractor={item => item.key}
          ListHeaderComponent={<Text className={s.suggHeading}>Suggestions</Text>}
          renderItem={({ item }) => (
            <SuggestionRow item={item} onPress={() => handleSuggestion(item.key, item.label)} />
          )}
          ItemSeparatorComponent={() => <View style={sheet.divider} />}
          keyboardShouldPersistTaps="handled"
        />
      )}

      {/* Results / suggestion */}
      {(view === 'results' || view === 'suggestion') && (
        <>
          {view === 'suggestion' && (
            <TouchableOpacity className={s.backRow} onPress={() => { setView('idle'); setSuggResults([]); }} activeOpacity={0.7}>
              <Ionicons name="chevron-back" size={16} color={ACCENT} />
              <Text className={s.backText}>  {suggTitle}</Text>
            </TouchableOpacity>
          )}

          {loading || suggLoading ? (
            <View className={s.centred}>
              <ActivityIndicator size="large" color={ACCENT} />
            </View>
          ) : (
            <FlatList
              data={activeItems}
              keyExtractor={e => e.id}
              keyboardShouldPersistTaps="handled"
              renderItem={({ item }) => <EventResultRow item={item} onPress={() => handleEventPress(item)} />}
              ItemSeparatorComponent={() => <View style={sheet.divider} />}
              ListEmptyComponent={
                <View className={s.centred}>
                  <Ionicons name="search-outline" size={48} color="rgba(10,10,10,0.15)" />
                  <Text className={s.emptyTitle}>No events found</Text>
                  <Text className={s.emptySubtitle}>
                    {view === 'results' ? `No results for "${query}". Try a different keyword.` : 'Check back later for updates.'}
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