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
import { useAppTheme } from '../../theme/colors'; // ADD

// ─── Constants ────────────────────────────────────────────────────────────────

const ACCENT    = '#ff3c6e';

const SUGGESTIONS = [
  { key: 'popular',        label: 'Popular Events',  icon: 'star'     as const, iconColor: '#3b82f6', bgColor: 'rgba(59,130,246,0.12)'  },
  { key: 'just_announced', label: 'Just Announced',  icon: 'megaphone'as const, iconColor: '#a855f7', bgColor: 'rgba(168,85,247,0.12)' },
];

// ─── Helpers ──────────────────────────────────────────────────────────────────

const formatPrice = (price: number) => price === 0 ? 'Free' : `R${Math.round(price).toLocaleString('en-ZA')}`;
const formatDate  = (iso: string)   => iso ? new Date(iso).toLocaleDateString('en-ZA', { day: 'numeric', month: 'short', year: 'numeric' }) : '';

// ─── Sub-components ───────────────────────────────────────────────────────────

const SuggestionRow = ({ item, onPress, colors }: { item: typeof SUGGESTIONS[number]; onPress: () => void; colors: any }) => (
  <TouchableOpacity style={{ flexDirection: 'row', alignItems: 'center', paddingHorizontal: 20, paddingVertical: 16 }} onPress={onPress} activeOpacity={0.7}>
    <View style={[sheet.suggIcon, { backgroundColor: item.bgColor }]}>
      <Ionicons name={item.icon} size={20} color={item.iconColor} />
    </View>
    <Text style={{ flex: 1, fontSize: 16, fontWeight: '500', color: colors.text }}>{item.label}</Text>
    <Ionicons name="chevron-forward" size={18} color={colors.iconColor} />
  </TouchableOpacity>
);

const EventResultRow = ({ item, onPress, colors }: { item: Event; onPress: () => void; colors: any }) => (
  <TouchableOpacity style={{ flexDirection: 'row', alignItems: 'center', paddingHorizontal: 16, paddingVertical: 14, gap: 14 }} onPress={onPress} activeOpacity={0.75}>
    <View style={sheet.resultThumb}>
      {item.cover_image_url
        ? <Image source={{ uri: item.cover_image_url }} style={StyleSheet.absoluteFillObject} resizeMode="cover" />
        : <View style={[StyleSheet.absoluteFillObject, { backgroundColor: colors.sectionCardBg }]} />
      }
    </View>
    <View style={{ flex: 1 }}>
      <Text style={{ fontSize: 9, fontWeight: '800', color: '#ff3c6e', letterSpacing: 1.4, textTransform: 'uppercase', marginBottom: 2 }} numberOfLines={1}>{item.category?.toUpperCase()}</Text>
      <Text style={{ fontSize: 14, fontWeight: '700', color: colors.text, lineHeight: 19, marginBottom: 3 }} numberOfLines={2}>{item.title}</Text>
      <Text style={{ fontSize: 11, color: colors.subText, marginBottom: 1 }} numberOfLines={1}>📍 {item.venue_name}{item.city ? `, ${item.city}` : ''}</Text>
      {item.start_time ? <Text style={{ fontSize: 11, color: colors.subText }}>{formatDate(item.start_time)}</Text> : null}
    </View>
    <View style={{ alignItems: 'flex-end', minWidth: 56 }}>
      <Text style={{ fontSize: 13, fontWeight: '800', color: colors.text }}>{formatPrice(item.price)}</Text>
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
  const { isDark, colors } = useAppTheme();

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

      {/* Idle: suggestions */}
      {view === 'idle' && (
        <FlatList
          data={SUGGESTIONS}
          keyExtractor={item => item.key}
          ListHeaderComponent={<Text style={{ fontSize: 18, fontWeight: '800', color: colors.text, paddingHorizontal: 20, paddingTop: 12, paddingBottom: 8 }}>Suggestions</Text>}
          renderItem={({ item }) => (
           <SuggestionRow item={item} onPress={() => handleSuggestion(item.key, item.label)} colors={colors} />
          )}
         ItemSeparatorComponent={() => <View style={{ height: StyleSheet.hairlineWidth, backgroundColor: colors.dividerColor, marginLeft: 20 }} />}
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
              renderItem={({ item }) => <EventResultRow item={item} onPress={() => handleEventPress(item)} colors={colors} />}
              ItemSeparatorComponent={() => <View style={{ height: StyleSheet.hairlineWidth, backgroundColor: colors.dividerColor, marginLeft: 20 }} />}
              ListEmptyComponent={
                <View className={s.centred}>
                  <Ionicons name="search-outline" size={48} color={colors.iconColor} />
                  <Text style={{ fontSize: 17, fontWeight: '700', color: colors.text, marginTop: 16, marginBottom: 6 }}>No events found</Text>
                  <Text style={{ fontSize: 13, color: colors.subText, textAlign: 'center', lineHeight: 19 }}>
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