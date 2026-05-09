/**
 * FilterModal.tsx  –  EventsApp/components/FilterModal.tsx
 *
 * Architecture: ONE Modal, three internal screens swapped instantly.
 *   'filter'   – main bottom sheet
 *   'location' – full-screen city search
 *   'calendar' – full-screen date-range calendar
 */

import React, { useState, useEffect, useRef, useCallback } from 'react';
import {
  View,
  Text,
  Modal,
  TouchableOpacity,
  Pressable,
  TextInput,
  FlatList,
  ScrollView,
  ActivityIndicator,
  Keyboard,
  Platform,
  KeyboardAvoidingView,
  StyleSheet,
  Dimensions,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import * as Location from 'expo-location';
import Constants from 'expo-constants';
import { Ionicons } from '@expo/vector-icons';
import { useAppTheme } from '../theme/colors';

const { width: SW } = Dimensions.get('window');
const CELL = Math.floor((SW - 48) / 7);

// ─── Types ────────────────────────────────────────────────────────────────────

export type DatePreset = 'any' | 'tomorrow' | 'weekend' | 'custom';

export interface FilterParams {
  city?: string;
  province?: string;   // province-level filter — maps to ?province= on the backend
  lat?: number;
  lng?: number;
  radius_km?: number;
  date: DatePreset;
  date_from?: string;
  date_to?: string;
}

interface CityResult {
  city: string;
  event_count: number;
}

interface FilterModalProps {
  visible: boolean;
  onClose: () => void;
  onApply: (params: FilterParams) => void;
  initialParams?: Partial<FilterParams>;
}

type Screen = 'filter' | 'location' | 'calendar';

// ─── API URL ──────────────────────────────────────────────────────────────────
//
// On a physical device "localhost" = the phone itself, not your dev machine.
// We fall back to the Expo debugger host (your machine's LAN IP) automatically.
//
function getBaseUrl(): string {
  // Explicit env var always wins
  if (process.env.EXPO_PUBLIC_API_URL) {
    return process.env.EXPO_PUBLIC_API_URL;
  }
  // In Expo Go / dev builds, Constants.expoConfig.hostUri is "192.168.x.x:8081"
  // Strip the port and use 8080 (your Go server)
  const debuggerHost =
    Constants.expoConfig?.hostUri ??
    (Constants as any).manifest?.debuggerHost ??
    (Constants as any).manifest2?.extra?.expoGo?.debuggerHost;

  if (debuggerHost) {
    const ip = debuggerHost.split(':')[0];
    return `http://${ip}:8080`;
  }
  return 'http://localhost:8080';
}

const BASE_URL = getBaseUrl();
console.log('[FilterModal] API base URL:', BASE_URL);

// ─── API ─────────────────────────────────────────────────────────────────────

async function fetchCities(query: string): Promise<CityResult[]> {
  try {
    const qs  = query.trim() ? `?q=${encodeURIComponent(query.trim())}` : '';
    const url = `${BASE_URL}/v1/cities${qs}`;
    const res  = await fetch(url);
    if (!res.ok) return [];
    const json = await res.json();
    // Backend returns { "cities": [...] }
    return Array.isArray(json.cities) ? json.cities : [];
  } catch {
    return [];
  }
}

// ─── Date helpers ─────────────────────────────────────────────────────────────

const MONTH_SHORT = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
const MONTH_FULL  = ['January','February','March','April','May','June','July','August','September','October','November','December'];
const DAY_LABELS  = ['S','M','T','W','T','F','S'];

function todayISO(): string {
  return new Date().toISOString().slice(0, 10);
}

function toISO(year: number, month: number, day: number): string {
  return `${year}-${String(month + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
}

/** "May 28, 2026" — calendar header */
function formatDisplay(iso: string): string {
  if (!iso) return 'Select date';
  const [y, m, d] = iso.split('-');
  return `${MONTH_SHORT[parseInt(m) - 1]} ${parseInt(d)}, ${y}`;
}

/**
 * "Apr 29 – May 15" — home header + Set Dates button label.
 * Exported so index.tsx can use it directly.
 */
export function formatDateRangeLabel(from: string, to: string): string {
  if (!from || !to) return 'All dates';
  const [, fm, fd] = from.split('-');
  const [, tm, td] = to.split('-');
  return `${MONTH_SHORT[parseInt(fm) - 1]} ${parseInt(fd)} – ${MONTH_SHORT[parseInt(tm) - 1]} ${parseInt(td)}`;
}

function buildMonths(count = 12): { year: number; month: number }[] {
  const now = new Date();
  return Array.from({ length: count }, (_, i) => {
    const d = new Date(now.getFullYear(), now.getMonth() + i, 1);
    return { year: d.getFullYear(), month: d.getMonth() };
  });
}

function buildDaysGrid(year: number, month: number): number[] {
  const firstDay    = new Date(year, month, 1).getDay();
  const daysInMonth = new Date(year, month + 1, 0).getDate();
  const grid: number[] = [];
  for (let i = 0; i < firstDay; i++) grid.push(0);
  for (let d = 1; d <= daysInMonth; d++) grid.push(d);
  return grid;
}

// ─── CalendarMonth ────────────────────────────────────────────────────────────

interface CalendarMonthProps {
  year: number; month: number;
  from: string; to: string;
  todayStr: string;
  onDayPress: (iso: string) => void;
  isDark: boolean;  
}

function CalendarMonth({ year, month, from, to, todayStr, onDayPress, isDark }: CalendarMonthProps) {
  const cal = calStyles(isDark);
  const grid = buildDaysGrid(year, month);
  return (
    <View style={{ marginBottom: 28 }}>
      <Text style={cal.monthName}>{MONTH_FULL[month]}</Text>
      <View style={cal.grid}>
        {grid.map((day, idx) => {
          if (!day) return <View key={`p${idx}`} style={{ width: CELL, height: CELL }} />;
          const iso     = toISO(year, month, day);
          const isPast  = iso < todayStr;
          const isFrom  = iso === from;
          const isTo    = iso === to;
          const inRange = !!(from && to && iso > from && iso < to);
          return (
            <TouchableOpacity
              key={iso}
              style={[
                cal.cell, { width: CELL, height: CELL },
                (isFrom || isTo) && cal.cellSelected,
                inRange          && cal.cellRange,
                isFrom && to     && cal.cellRangeStart,
                isTo   && from   && cal.cellRangeEnd,
              ]}
              onPress={() => !isPast && onDayPress(iso)}
              activeOpacity={isPast ? 1 : 0.7}
              disabled={isPast}
            >
              <Text style={[
                cal.dayText,
                isPast           && cal.dayPast,
                (isFrom || isTo) && cal.daySelected,
                inRange          && cal.dayRange,
              ]}>
                {day}
              </Text>
            </TouchableOpacity>
          );
        })}
      </View>
    </View>
  );
}

function calStyles(isDark: boolean) {
  const text = isDark ? '#ffffff' : '#0a0a0a';
  const textFaint = isDark ? 'rgba(255,255,255,0.2)' : 'rgba(10,10,10,0.2)';
  const rangeBg = isDark ? 'rgba(255,255,255,0.08)' : 'rgba(10,10,10,0.08)';
  return StyleSheet.create({
    monthName:      { fontSize: 16, fontWeight: '800', color: text, marginBottom: 10, paddingHorizontal: 24 },
    grid:           { flexDirection: 'row', flexWrap: 'wrap', paddingHorizontal: 24 },
    cell:           { alignItems: 'center', justifyContent: 'center' },
    cellSelected:   { backgroundColor: text, borderRadius: CELL / 2 },
    cellRange:      { backgroundColor: rangeBg, borderRadius: 0 },
    cellRangeStart: { borderTopLeftRadius: CELL / 2, borderBottomLeftRadius: CELL / 2, borderTopRightRadius: 0, borderBottomRightRadius: 0 },
    cellRangeEnd:   { borderTopRightRadius: CELL / 2, borderBottomRightRadius: CELL / 2, borderTopLeftRadius: 0, borderBottomLeftRadius: 0 },
    dayText:        { fontSize: 15, fontWeight: '500', color: text },
    dayPast:        { color: textFaint },
    daySelected:    { color: isDark ? '#000' : '#fff', fontWeight: '800' },
    dayRange:       { color: text },
  });
}

// ─── Province → cities mapping (client-side province search) ─────────────────

const PROVINCE_CITIES: Record<string, string[]> = {
  'Gauteng':           ['Johannesburg', 'Pretoria', 'Sandton', 'Soweto', 'Midrand', 'Centurion', 'Randburg', 'Roodepoort', 'Boksburg', 'Benoni', 'Germiston', 'Krugersdorp'],
  'Western Cape':      ['Cape Town', 'Stellenbosch', 'Paarl', 'George', 'Knysna', 'Mossel Bay', 'Hermanus', 'Worcester'],
  'KwaZulu-Natal':     ['Durban', 'Pietermaritzburg', 'Richards Bay', 'Newcastle', 'Umhlanga', 'Ballito'],
  'Eastern Cape':      ['Port Elizabeth', 'East London', 'Mthatha', 'Grahamstown', 'Uitenhage'],
  'Limpopo':           ['Polokwane', 'Tzaneen', 'Phalaborwa', 'Louis Trichardt'],
  'Mpumalanga':        ['Nelspruit', 'Witbank', 'Secunda', 'Barberton'],
  'North West':        ['Rustenburg', 'Klerksdorp', 'Mafikeng', 'Potchefstroom'],
  'Free State':        ['Bloemfontein', 'Welkom', 'Sasolburg', 'Kroonstad'],
  'Northern Cape':     ['Kimberley', 'Upington', 'Springbok'],
};

// A flat row item for the FlatList
type LocRow =
  | { type: 'province'; key: string; label: string }
  | { type: 'city';     key: string; label: string; eventCount: number };

/**
 * Build the flat list of province headers + city rows from:
 *  - the query string (used to match provinces client-side)
 *  - the backend city results (for city-level matches)
 */
function buildLocationResults(query: string, backendCities: CityResult[]): LocRow[] {
  if (!query) return [];

  const q = query.toLowerCase();
  const rows: LocRow[] = [];

  // 1. Province match — show province header + all cities in that province
  for (const [province, cities] of Object.entries(PROVINCE_CITIES)) {
    if (!province.toLowerCase().includes(q)) continue;
    rows.push({ type: 'province', key: `prov-${province}`, label: province });
    for (const city of cities) {
      rows.push({ type: 'city', key: `prov-city-${city}`, label: city, eventCount: 0 });
    }
  }

  // 2. Direct city matches from backend (deduplicate vs province-expanded cities)
  const alreadyAdded = new Set(rows.filter(r => r.type === 'city').map(r => r.label.toLowerCase()));
  for (const c of backendCities) {
    if (!alreadyAdded.has(c.city.toLowerCase())) {
      rows.push({ type: 'city', key: `city-${c.city}`, label: c.city, eventCount: Number(c.event_count ?? 0) });
    }
  }

  return rows;
}

// ─── Main FilterModal ─────────────────────────────────────────────────────────

export default function FilterModal({ visible, onClose, onApply, initialParams }: FilterModalProps) {

  const insets = useSafeAreaInsets();
  const { colors, isDark } = useAppTheme();

  const [screen, setScreen] = useState<Screen>('filter');

  const [selectedCity,     setSelectedCity]     = useState<string | null>(initialParams?.city     ?? null);
  const [selectedProvince, setSelectedProvince] = useState<string | null>(initialParams?.province ?? null);
  const [nearMeCoords,   setNearMeCoords]   = useState<{ lat: number; lng: number } | undefined>(
    initialParams?.lat != null ? { lat: initialParams.lat!, lng: initialParams.lng! } : undefined,
  );
  const [previousCities, setPreviousCities] = useState<string[]>(
    initialParams?.city ? [initialParams.city] : [],
  );
  const [datePreset, setDatePreset] = useState<DatePreset>(initialParams?.date ?? 'any');
  const [dateFrom,   setDateFrom]   = useState(initialParams?.date_from ?? '');
  const [dateTo,     setDateTo]     = useState(initialParams?.date_to   ?? '');

  // Location screen
  const [query,       setQuery]       = useState('');
  const [cities,      setCities]      = useState<CityResult[]>([]);
  const [cityLoading, setCityLoading] = useState(false);
  const [locLoading,  setLocLoading]  = useState(false);
  const [locInputFocused, setLocInputFocused] = useState(false);
  const inputRef = useRef<TextInput>(null);
  const debounce = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Calendar
  const todayStr = todayISO();
  const [calFrom, setCalFrom] = useState('');
  const [calTo,   setCalTo]   = useState('');
  const [calStep, setCalStep] = useState<'from' | 'to'>('from');
  const months = buildMonths(12);

  useEffect(() => {
    if (!visible) setTimeout(() => setScreen('filter'), 300);
  }, [visible]);

  // On mount of location screen: clear state, focus input — don't pre-load city list
  useEffect(() => {
    if (screen !== 'location') return;
    setQuery('');
    setCities([]);
    setLocInputFocused(false);
    setTimeout(() => inputRef.current?.focus(), 60);
  }, [screen]);

  useEffect(() => {
    if (screen === 'calendar') {
      setCalFrom(dateFrom);
      setCalTo(dateTo);
      setCalStep('from');
    }
  }, [screen]);

  // ── City search ───────────────────────────────────────────────────────────

  const loadCities = useCallback(async (q: string) => {
    setCityLoading(true);
    const results = await fetchCities(q);
    setCities(results);
    setCityLoading(false);
  }, []);

  function handleQueryChange(text: string) {
    setQuery(text);
    if (debounce.current) clearTimeout(debounce.current);
    if (!text.trim()) { setCities([]); return; }
    debounce.current = setTimeout(() => loadCities(text), 280);
  }

  function handleCitySelect(city: string) {
    setSelectedCity(city);
    setSelectedProvince(null);
    setNearMeCoords(undefined);
    if (!previousCities.includes(city)) {
      setPreviousCities(prev => [city, ...prev].slice(0, 5));
    }
    setScreen('filter');
  }

  function handleProvinceSelect(province: string) {
    setSelectedProvince(province);
    setSelectedCity(null);
    setNearMeCoords(undefined);
    setScreen('filter');
  }

  async function handleNearMe() {
    setLocLoading(true);
    try {
      const { status } = await Location.requestForegroundPermissionsAsync();
      if (status !== 'granted') { setLocLoading(false); return; }
      const loc = await Location.getCurrentPositionAsync({ accuracy: Location.Accuracy.Balanced });
      setSelectedCity(null);
      setNearMeCoords({ lat: loc.coords.latitude, lng: loc.coords.longitude });
      setScreen('filter');
    } catch { }
    finally { setLocLoading(false); }
  }

  // ── Calendar ──────────────────────────────────────────────────────────────

  function handleDayPress(iso: string) {
    if (calStep === 'from') {
      setCalFrom(iso); setCalTo(''); setCalStep('to');
    } else {
      if (iso < calFrom) { setCalFrom(iso); setCalTo(''); }
      else               { setCalTo(iso); setCalStep('from'); }
    }
  }

  function handleCalendarApply() {
    if (!calFrom || !calTo) return;
    setDatePreset('custom');
    setDateFrom(calFrom);
    setDateTo(calTo);
    setScreen('filter');
  }

  // ── Filter ────────────────────────────────────────────────────────────────

  function handleDatePreset(preset: DatePreset) {
    if (preset === 'custom') { setScreen('calendar'); return; }
    setDatePreset(preset); setDateFrom(''); setDateTo('');
  }

  function handleDone() {
    const params: FilterParams = { date: datePreset };
    if (selectedCity)        params.city     = selectedCity;
    else if (selectedProvince) params.province = selectedProvince;
    else if (nearMeCoords)   { params.lat = nearMeCoords.lat; params.lng = nearMeCoords.lng; params.radius_km = 50; }
    if (datePreset === 'custom' && dateFrom && dateTo) { params.date_from = dateFrom; params.date_to = dateTo; }
    onApply(params);
  }

  const locationIsSet = !!selectedCity || !!selectedProvince || !!nearMeCoords;
  const locationLabel = selectedCity ?? selectedProvince ?? (nearMeCoords ? 'Near me' : 'Near me');
  const customBtnLabel =
    datePreset === 'custom' && dateFrom && dateTo
      ? formatDateRangeLabel(dateFrom, dateTo)
      : 'Set dates';
  const calCanApply = !!calFrom && !!calTo && calFrom <= calTo;

  const PILLS: { key: DatePreset; label: string }[] = [
    { key: 'any',      label: 'Any date'  },
    { key: 'tomorrow', label: 'Tomorrow'  },
    { key: 'weekend',  label: 'Weekend'   },
  ];

  return (
    <Modal
      visible={visible}
      transparent={screen === 'filter'}
      animationType={screen === 'filter' ? 'slide' : 'none'}
      onRequestClose={() => screen === 'filter' ? onClose() : setScreen('filter')}
      statusBarTranslucent
    >

      {/* ══════════ FILTER SHEET ══════════ */}
     {screen === 'filter' && (
  <View style={s.filterRoot}>
    <Pressable style={s.backdrop} onPress={onClose} />
    <View style={[s.sheet, { backgroundColor: colors.background }]}>
      <View style={[s.handle, { backgroundColor: isDark ? 'rgba(255,255,255,0.2)' : 'rgba(0,0,0,0.12)' }]} />

      {/* ── Header ── */}
      <View style={s.filterHeader}>
        <Text style={[s.filterTitle, { color: colors.text }]}>Filters</Text>
        <View style={[s.filterPinCircle, { backgroundColor: colors.ctaBg }]}>
          <Ionicons name="location" size={16} color="#fff" />
        </View>
      </View>

      {/* ── Location ── */}
      <Text style={[s.sectionTitle, { color: colors.text }]}>Location</Text>
      <TouchableOpacity onPress={() => setScreen('location')} style={s.locationRow} activeOpacity={0.7}>
        <View style={[s.locationIcon, { backgroundColor: isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)' }]}>
          <Ionicons name="location-outline" size={20} color={colors.text} />
        </View>
        <Text style={[s.locationText, { color: isDark ? 'rgba(255,255,255,0.45)' : 'rgba(0,0,0,0.45)' },
          locationIsSet && { color: colors.text, fontWeight: '600' }]}>
          {locationLabel}
        </Text>
        <Text style={[s.chevron, { color: isDark ? 'rgba(255,255,255,0.3)' : 'rgba(0,0,0,0.3)' }]}>›</Text>
      </TouchableOpacity>

      <View style={[s.divider, { backgroundColor: colors.dividerColor }]} />

      {/* ── Dates ── */}
      <View style={s.datesHeader}>
        <Text style={[s.sectionTitle, { color: colors.text, marginBottom: 0 }]}>Dates</Text>
        <Ionicons name="calendar-outline" size={22} color={isDark ? 'rgba(255,255,255,0.5)' : 'rgba(0,0,0,0.4)'} />
      </View>

      {/* Segmented toggle: Any date | Pick date */}
      <View style={[s.segmentTrack, { backgroundColor: isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.07)' }]}>
        <TouchableOpacity
          style={[s.segmentBtn, datePreset === 'any' && { backgroundColor: colors.ctaBg }]}
          onPress={() => handleDatePreset('any')}
          activeOpacity={0.8}
        >
          <Text style={[s.segmentText, { color: datePreset === 'any' ? '#fff' : (isDark ? 'rgba(255,255,255,0.6)' : 'rgba(0,0,0,0.5)') }]}>
            Any date
          </Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={[s.segmentBtn, datePreset !== 'any' && { backgroundColor: isDark ? '#2a2a2a' : '#2a2a2a' }]}
          onPress={() => handleDatePreset('custom')}
          activeOpacity={0.8}
        >
          <Ionicons name="calendar-outline" size={14}
            color={datePreset !== 'any' ? '#fff' : (isDark ? 'rgba(255,255,255,0.5)' : 'rgba(0,0,0,0.4)')}
            style={{ marginRight: 5 }}
          />
          <Text style={[s.segmentText, { color: datePreset !== 'any' ? '#fff' : (isDark ? 'rgba(255,255,255,0.6)' : 'rgba(0,0,0,0.5)') }]}>
            {datePreset === 'custom' && dateFrom && dateTo ? formatDateRangeLabel(dateFrom, dateTo) : 'Pick date'}
          </Text>
        </TouchableOpacity>
      </View>

      {/* ── Done ── */}
      <View style={s.doneRow}>
        <TouchableOpacity onPress={handleDone} style={[s.doneBtn, { backgroundColor: colors.secondaryBg }]} activeOpacity={0.85}>
          <Text style={[s.doneBtnText, { color: colors.secondaryText }]}>DONE</Text>
        </TouchableOpacity>
      </View>
    </View>
  </View>
)}

      {/* ══════════ LOCATION SCREEN ══════════ */}
      {screen === 'location' && (
        <View style={[s.fullScreen, { paddingTop: insets.top, backgroundColor: colors.background }]}>
          <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>

            {/* ── Search bar — matches search tab design ── */}
            <View style={s.locTopBar}>
              <View style={[s.locSearchBox, locInputFocused && s.locSearchBoxFocused,
                { backgroundColor: isDark ? 'rgba(255,255,255,0.08)' : 'rgba(10,10,10,0.06)' }]}>
                <Ionicons name="search" size={17} color={isDark ? 'rgba(255,255,255,0.45)' : 'rgba(10,10,10,0.45)'} style={{ marginRight: 8 }} />
                <TextInput
                  ref={inputRef}
                  value={query}
                  onChangeText={handleQueryChange}
                  onFocus={() => setLocInputFocused(true)}
                  onBlur={() => setLocInputFocused(false)}
                  placeholder="Search by city or province…"
                  placeholderTextColor={isDark ? 'rgba(255,255,255,0.35)' : 'rgba(10,10,10,0.38)'}
                  style={[s.locSearchInput, { color: colors.text }]}
                  returnKeyType="search"
                  onSubmitEditing={Keyboard.dismiss}
                  autoCorrect={false}
                  autoCapitalize="words"
                />
                {query.length > 0 && (
                  <TouchableOpacity onPress={() => { setQuery(''); setCities([]); }} hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}>
                    <Ionicons name="close-circle" size={16} color={isDark ? 'rgba(255,255,255,0.35)' : 'rgba(10,10,10,0.35)'} />
                  </TouchableOpacity>
                )}
              </View>
              {/* Cancel only appears while input is focused */}
              {locInputFocused && (
                <TouchableOpacity
                  onPress={() => { Keyboard.dismiss(); setLocInputFocused(false); setScreen('filter'); }}
                  style={s.locCancelBtn}
                  activeOpacity={0.7}
                >
                  <Text style={s.locCancelText}>Cancel</Text>
                </TouchableOpacity>
              )}
            </View>
            <View style={s.hairline} />

            {/* ── Results ── */}
            {cityLoading ? (
              <ActivityIndicator size="small" color="#ff3c6e" style={{ marginTop: 36 }} />
            ) : (
              <FlatList
                data={buildLocationResults(query.trim(), cities)}
                keyExtractor={r => r.key}
                keyboardShouldPersistTaps="handled"
                contentContainerStyle={s.locListContent}
                renderItem={({ item }) => {
                  if (item.type === 'province') {
                    return (
                      <TouchableOpacity
                        onPress={() => handleProvinceSelect(item.label)}
                        style={s.locRow}
                        activeOpacity={0.7}
                      >
                        <View style={s.locRowLeft}>
                          <Ionicons name="map-outline" size={18} color={isDark ? 'rgba(255,255,255,0.35)' : 'rgba(10,10,10,0.35)'} style={{ marginRight: 12 }} />
                          <Text style={[s.locRowText, selectedProvince === item.label && s.locRowTextSelected, { color: colors.text }]}>
                            {item.label}
                          </Text>
                        </View>
                        <Text style={s.locProvinceTag}>Province</Text>
                      </TouchableOpacity>
                    );
                  }
                  return (
                    <TouchableOpacity onPress={() => handleCitySelect(item.label)} style={s.locRow} activeOpacity={0.7}>
                      <View style={s.locRowLeft}>
                        <Ionicons name="location-outline" size={18} color={isDark ? 'rgba(255,255,255,0.35)' : 'rgba(10,10,10,0.35)'} style={{ marginRight: 12 }} />
                        <Text style={[s.locRowText, selectedCity === item.label && s.locRowTextSelected, { color: colors.text }]}>
                          {item.label}
                        </Text>
                      </View>
                      {item.eventCount > 0 && (
                        <Text style={s.locRowCount}>{item.eventCount} events</Text>
                      )}
                    </TouchableOpacity>
                  );
                }}
                ListEmptyComponent={
                  query.trim() ? (
                    <Text style={[s.locEmpty, { color: colors.subText }]}>No results for "{query}"</Text>
                  ) : (
                    <Text style={[s.locEmpty, { color: colors.subText }]}>Start typing a city or province…</Text>
                  )
                }
                ItemSeparatorComponent={() => <View style={s.locDivider} />}
              />
            )}

            {/* ── Use Current Location ── */}
            <View>
              <View style={s.hairline} />
              <TouchableOpacity onPress={handleNearMe} style={s.currentLocBtn} activeOpacity={0.7}>
                {locLoading
                  ? <ActivityIndicator size="small" color={colors.text} style={{ marginRight: 8 }} />
                  : <Ionicons name="navigate" size={16} color={colors.text} style={{ marginRight: 6 }} />}
                <Text style={[s.currentLocText, { color: colors.text }]}>Use Current Location</Text>
              </TouchableOpacity>
            </View>
          </KeyboardAvoidingView>
        </View>
      )}

      {/* ══════════ CALENDAR SCREEN ══════════ */}
      {screen === 'calendar' && (
       <View style={[s.fullScreen, { paddingTop: insets.top, backgroundColor: colors.background }]}>
          <View style={s.calHeader}>
            <TouchableOpacity style={[s.calHeaderCol, calStep === 'from' && s.calHeaderColActive]} onPress={() => setCalStep('from')} activeOpacity={0.7}>
              <Text style={s.calHeaderLabel}>FROM</Text>
              <Text style={[s.calHeaderDate, !calFrom && s.calHeaderDateEmpty, { color: colors.text }]}>{calFrom ? formatDisplay(calFrom) : 'Select date'}</Text>
            </TouchableOpacity>
            <View style={s.calHeaderDivider} />
            <TouchableOpacity style={[s.calHeaderCol, calStep === 'to' && s.calHeaderColActive]} onPress={() => calFrom && setCalStep('to')} activeOpacity={calFrom ? 0.7 : 1}>
              <Text style={s.calHeaderLabel}>TO</Text>
              <Text style={[s.calHeaderDate, !calTo && s.calHeaderDateEmpty, { color: colors.text }]}>{calTo ? formatDisplay(calTo) : 'Select date'}</Text>
            </TouchableOpacity>
          </View>

          <View style={s.calDowRow}>
            {DAY_LABELS.map((d, i) => (
              <View key={i} style={{ width: CELL, alignItems: 'center' }}>
                <Text style={s.calDowText}>{d}</Text>
              </View>
            ))}
          </View>
          <View style={s.hairline} />

          <ScrollView style={{ flex: 1 }} showsVerticalScrollIndicator={false} contentContainerStyle={{ paddingTop: 16, paddingBottom: 8 }}>
            {months.map(({ year, month }) => (
              <CalendarMonth key={`${year}-${month}`} year={year} month={month} from={calFrom} to={calTo} todayStr={todayStr} onDayPress={handleDayPress} isDark={isDark} />
            ))}
          </ScrollView>

          <View style={s.calFooter}>
            <TouchableOpacity onPress={() => setScreen('filter')} activeOpacity={0.7}>
              <Text style={[s.calCancelText, { color: colors.text }]}>Cancel</Text>
            </TouchableOpacity>
            <TouchableOpacity onPress={handleCalendarApply} style={[s.calApplyBtn, !calCanApply && s.calApplyBtnDisabled]} activeOpacity={calCanApply ? 0.85 : 1}>
              <Text style={s.calApplyText}>Apply</Text>
            </TouchableOpacity>
          </View>
        </View>
      )}
    </Modal>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const s = StyleSheet.create({
  filterRoot:          { flex: 1, justifyContent: 'flex-end' },
  backdrop:            { ...StyleSheet.absoluteFillObject, backgroundColor: 'rgba(0,0,0,0.45)' },
  sheet:               { backgroundColor: '#fff', borderTopLeftRadius: 20, borderTopRightRadius: 20, paddingBottom: 44, shadowColor: '#000', shadowOffset: { width: 0, height: -4 }, shadowOpacity: 0.14, shadowRadius: 20, elevation: 24 },
  handle:              { width: 40, height: 4, borderRadius: 2, backgroundColor: 'rgba(0,0,0,0.12)', alignSelf: 'center', marginTop: 12, marginBottom: 24 },
  filterHeader:   { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: 24, marginBottom: 20 },
  filterTitle:    { fontSize: 18, fontWeight: '700' },
  filterPinCircle: { width: 32, height: 32, borderRadius: 16, alignItems: 'center', justifyContent: 'center' },
  sectionTitle:   { fontSize: 18, fontWeight: '800', paddingHorizontal: 24, marginBottom: 4 },
  locationRow:    { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 24, paddingVertical: 14 },
  locationIcon:   { width: 44, height: 44, borderRadius: 22, alignItems: 'center', justifyContent: 'center', marginRight: 14 },
  locationText:        { flex: 1, fontSize: 17, fontWeight: '400', color: 'rgba(10,10,10,0.45)' },
  locationTextSet:     { fontWeight: '600', color: '#0a0a0a' },
  chevron:             { fontSize: 22, color: 'rgba(10,10,10,0.3)' },
  divider:             { height: StyleSheet.hairlineWidth, backgroundColor: 'rgba(0,0,0,0.1)', marginHorizontal: 24, marginVertical: 8 },
  pillRow:             { flexDirection: 'row', paddingHorizontal: 24, gap: 10, marginTop: 12, marginBottom: 12 },
  pill:                { flex: 1, paddingVertical: 11, borderRadius: 24, borderWidth: 1, borderColor: 'rgba(10,10,10,0.2)', backgroundColor: '#fff', alignItems: 'center' },
  pillActive:          { borderWidth: 2, borderColor: '#0a0a0a' },
  pillText:            { fontSize: 14, fontWeight: '500', color: 'rgba(10,10,10,0.55)' },
  pillTextActive:      { fontWeight: '800', color: '#0a0a0a' },
  setDatesBtn:         { marginHorizontal: 24, paddingVertical: 15, borderRadius: 12, borderWidth: 1, borderColor: 'rgba(10,10,10,0.18)', alignItems: 'center', backgroundColor: '#fff' },
  setDatesBtnActive:   { borderWidth: 2, borderColor: '#0a0a0a' },
  setDatesText:        { fontSize: 15, fontWeight: '600', color: 'rgba(10,10,10,0.6)' },
  setDatesTextActive:  { fontWeight: '800', color: '#0a0a0a' },
  datesHeader:    { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: 24, marginTop: 8, marginBottom: 14 },
  segmentTrack:   { flexDirection: 'row', marginHorizontal: 24, borderRadius: 28, padding: 4, marginBottom: 28 },
  segmentBtn:     { flex: 1, flexDirection: 'row', alignItems: 'center', justifyContent: 'center', paddingVertical: 12, borderRadius: 24 },
  segmentText:    { fontSize: 14, fontWeight: '600' },
  doneRow:        { paddingHorizontal: 24, marginTop: 28 },
  doneBtn:        { borderRadius: 16, paddingVertical: 17, alignItems: 'center' },
  doneBtnText:    { fontWeight: '800', fontSize: 16, letterSpacing: 1 },
  fullScreen:          { flex: 1, backgroundColor: '#fff' },
  hairline:       { height: StyleSheet.hairlineWidth, backgroundColor: 'rgba(128,128,128,0.25)' },
  locTopBar:           { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 16, paddingTop: 12, paddingBottom: 10, gap: 10 },
  locSearchBox:        { flex: 1, flexDirection: 'row', alignItems: 'center', backgroundColor: 'rgba(10,10,10,0.06)', borderRadius: 14, paddingHorizontal: 14, height: 46, borderWidth: 1.5, borderColor: 'transparent' },
  locSearchBoxFocused: { borderColor: '#ff2d55', backgroundColor: 'transparent' },
  locSearchInput:      { flex: 1, fontSize: 15, color: '#0a0a0a', fontWeight: '400' },
  locCancelBtn:        { paddingLeft: 4, paddingVertical: 8 },
  locCancelText:       { fontSize: 16, color: '#ff3c6e', fontWeight: '600' },
  locListContent:      { paddingHorizontal: 20, paddingBottom: 40 },
  locRow:              { flexDirection: 'row', alignItems: 'center', paddingVertical: 15 },
  locRowLeft:          { flex: 1, flexDirection: 'row', alignItems: 'center' },
  locRowText:          { flex: 1, fontSize: 16, fontWeight: '400', color: '#0a0a0a' },
  locRowTextSelected:  { fontWeight: '700', color: '#ff3c6e' },
  locProvinceRowText:  { fontWeight: '600' },
  locProvinceTag: { fontSize: 11, fontWeight: '700', color: 'rgba(128,128,128,0.6)', letterSpacing: 0.8, textTransform: 'uppercase', marginLeft: 8 },
  locRowCount:    { fontSize: 12, color: 'rgba(128,128,128,0.6)', marginLeft: 8 },  locDivider: { height: StyleSheet.hairlineWidth, backgroundColor: 'rgba(128,128,128,0.15)', marginLeft: 46 },
  locEmpty:            { textAlign: 'center', color: 'rgba(10,10,10,0.35)', fontSize: 14, marginTop: 48, lineHeight: 22 },
  currentLocBtn:       { flexDirection: 'row', alignItems: 'center', justifyContent: 'center', paddingVertical: 18, gap: 8 },
  currentLocText:      { fontSize: 16, fontWeight: '600', color: '#0a0a0a' },
  calHeader:           { flexDirection: 'row', borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: 'rgba(128,128,128,0.2)' },
  calHeaderColActive:  { borderBottomWidth: 2, borderBottomColor: '#ff2d55' },
  calHeaderLabel:      { fontSize: 11, fontWeight: '800', letterSpacing: 1.5, color: 'rgba(128,128,128,0.6)', marginBottom: 5 },
  calHeaderDate:       { fontSize: 17, fontWeight: '800' },        // color set inline
  calHeaderDateEmpty:  { fontWeight: '400' }, 
  calHeaderDivider:    { width: StyleSheet.hairlineWidth, backgroundColor: 'rgba(0,0,0,0.15)', marginVertical: 14 },
  calHeaderCol:        { flex: 1, paddingVertical: 16, paddingHorizontal: 24 },
  calDowRow:           { flexDirection: 'row', paddingHorizontal: 24, paddingTop: 14, paddingBottom: 8 },
  calDowText:          { fontSize: 13, fontWeight: '600', color: 'rgba(128,128,128,0.6)' },
  calFooter:           { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: 24, paddingVertical: 16, borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: 'rgba(0,0,0,0.08)' },
  calApplyBtn:         { backgroundColor: '#1c2e61', borderRadius: 14, paddingVertical: 14, paddingHorizontal: 44 },
  calCancelText:       { fontSize: 16, fontWeight: '500' },
  calApplyBtnDisabled: { backgroundColor: 'rgba(10,10,10,0.2)' },
  calApplyText:        { color: '#fff', fontWeight: '800', fontSize: 15 },
});

// ─── Utility ──────────────────────────────────────────────────────────────────

export function buildFilterQueryString(params: FilterParams): string {
  const qs = new URLSearchParams();
  if (params.city)         qs.set('city',       params.city);
  if (params.province)     qs.set('province',   params.province);
  if (params.lat  != null) qs.set('lat',        String(params.lat));
  if (params.lng  != null) qs.set('lng',        String(params.lng));
  if (params.radius_km)    qs.set('radius_km',  String(params.radius_km));
  if (params.date && params.date !== 'any') qs.set('date', params.date);
  if (params.date === 'custom' && params.date_from) qs.set('date_from', params.date_from);
  if (params.date === 'custom' && params.date_to)   qs.set('date_to',   params.date_to);
  return qs.toString();
}