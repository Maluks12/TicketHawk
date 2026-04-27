/**
 * FilterModal.tsx
 * Place this file at: EventsApp/components/FilterModal.tsx
 *
 * Fix: collapsed the double-Modal (backdrop + sheet) into a single Modal
 * with an inline Pressable backdrop — React Native stacked Modals are
 * unreliable on Android and cause the sheet to never appear.
 */

import React, { useState, useEffect, useCallback, useRef } from 'react';
import {
  View,
  Text,
  Modal,
  TouchableOpacity,
  Pressable,
  TextInput,
  FlatList,
  ActivityIndicator,
  Animated,
  Keyboard,
  Platform,
  KeyboardAvoidingView,
  StyleSheet,
} from 'react-native';
import * as Location from 'expo-location';

// ─── Types ────────────────────────────────────────────────────────────────────

export type DatePreset = 'any' | 'tomorrow' | 'weekend' | 'custom';

export interface FilterParams {
  city?: string;
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

// ─── API ─────────────────────────────────────────────────────────────────────

const BASE_URL = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080';

async function fetchCities(query: string): Promise<CityResult[]> {
  const qs = query.trim() ? `?q=${encodeURIComponent(query.trim())}` : '';
  const res = await fetch(`${BASE_URL}/cities${qs}`);
  if (!res.ok) throw new Error('Failed to load cities');
  const json = await res.json();
  return json.cities ?? [];
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

function formatCustomDate(iso: string): string {
  if (!iso) return '';
  const [y, m, d] = iso.split('-');
  const months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
  return `${months[parseInt(m, 10) - 1]} ${parseInt(d, 10)}, ${y}`;
}

function todayISO(offsetDays = 0): string {
  const d = new Date();
  d.setDate(d.getDate() + offsetDays);
  return d.toISOString().slice(0, 10);
}

// ─── LocationPicker ───────────────────────────────────────────────────────────

interface LocationPickerProps {
  visible: boolean;
  current: { city: string | null; isNearMe: boolean };
  onSelect: (city: string | null, coords?: { lat: number; lng: number }) => void;
  onClose: () => void;
}

function LocationPicker({ visible, current, onSelect, onClose }: LocationPickerProps) {
  const [query, setQuery] = useState('');
  const [cities, setCities] = useState<CityResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [locLoading, setLocLoading] = useState(false);
  const debounce = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!visible) { setQuery(''); return; }
    loadCities('');
  }, [visible]);

  async function loadCities(q: string) {
    setLoading(true);
    try { setCities(await fetchCities(q)); }
    catch { /* silently fail */ }
    finally { setLoading(false); }
  }

  function handleQueryChange(text: string) {
    setQuery(text);
    if (debounce.current) clearTimeout(debounce.current);
    debounce.current = setTimeout(() => loadCities(text), 320);
  }

  async function handleNearMe() {
    setLocLoading(true);
    try {
      const { status } = await Location.requestForegroundPermissionsAsync();
      if (status !== 'granted') { onSelect(null); return; }
      const loc = await Location.getCurrentPositionAsync({ accuracy: Location.Accuracy.Balanced });
      onSelect(null, { lat: loc.coords.latitude, lng: loc.coords.longitude });
    } catch {
      onSelect(null);
    } finally {
      setLocLoading(false);
    }
  }

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        style={{ flex: 1, justifyContent: 'flex-end' }}
      >
        <Pressable style={{ flex: 1 }} onPress={onClose} />
        <View style={lp.sheet}>
          {/* Handle */}
          <View style={lp.handle} />

          {/* Header */}
          <View style={lp.header}>
            <Text style={lp.title}>Choose Location</Text>
            <TouchableOpacity onPress={onClose}>
              <Text style={lp.done}>Done</Text>
            </TouchableOpacity>
          </View>

          {/* Search */}
          <View style={lp.searchBar}>
            <Text style={{ fontSize: 15, marginRight: 8 }}>🔍</Text>
            <TextInput
              value={query}
              onChangeText={handleQueryChange}
              placeholder="Search cities..."
              placeholderTextColor="rgba(10,10,10,0.3)"
              style={lp.searchInput}
              autoFocus
              returnKeyType="search"
              onSubmitEditing={Keyboard.dismiss}
            />
            {query.length > 0 && (
              <TouchableOpacity onPress={() => { setQuery(''); loadCities(''); }}>
                <Text style={{ color: 'rgba(10,10,10,0.4)', fontSize: 16, fontWeight: '600' }}>✕</Text>
              </TouchableOpacity>
            )}
          </View>

          {/* Near Me */}
          <TouchableOpacity onPress={handleNearMe} style={lp.nearMeRow}>
            <View style={lp.nearMeIcon}>
              {locLoading
                ? <ActivityIndicator size="small" color="#ff3c6e" />
                : <Text style={{ fontSize: 16 }}>📍</Text>}
            </View>
            <View>
              <Text style={lp.nearMeLabel}>Near me</Text>
              <Text style={lp.nearMeSub}>Use your current location</Text>
            </View>
            {current.isNearMe && (
              <Text style={lp.check}>✓</Text>
            )}
          </TouchableOpacity>

          {/* City List */}
          {loading
            ? <ActivityIndicator size="small" color="#ff3c6e" style={{ marginTop: 20 }} />
            : (
              <FlatList
                data={cities}
                keyExtractor={(item) => item.city}
                keyboardShouldPersistTaps="handled"
                contentContainerStyle={{ paddingHorizontal: 20 }}
                renderItem={({ item }) => (
                  <TouchableOpacity
                    onPress={() => onSelect(item.city)}
                    style={lp.cityRow}
                  >
                    <Text style={[lp.cityName, current.city === item.city && lp.cityNameActive]}>
                      {item.city}
                    </Text>
                    <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
                      {item.event_count > 0 && (
                        <Text style={lp.eventCount}>{item.event_count} events</Text>
                      )}
                      {current.city === item.city && (
                        <Text style={lp.check}>✓</Text>
                      )}
                    </View>
                  </TouchableOpacity>
                )}
                ListEmptyComponent={
                  <Text style={lp.empty}>No cities found</Text>
                }
              />
            )}
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

const lp = StyleSheet.create({
  sheet:         { backgroundColor: '#fff', borderTopLeftRadius: 20, borderTopRightRadius: 20, paddingTop: 12, paddingBottom: 40, maxHeight: '85%' },
  handle:        { width: 40, height: 4, borderRadius: 2, backgroundColor: 'rgba(0,0,0,0.12)', alignSelf: 'center', marginBottom: 20 },
  header:        { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: 24, marginBottom: 16 },
  title:         { fontSize: 20, fontWeight: '800', color: '#0a0a0a', letterSpacing: 0.3 },
  done:          { fontSize: 13, fontWeight: '700', color: '#ff3c6e' },
  searchBar:     { marginHorizontal: 20, marginBottom: 12, flexDirection: 'row', alignItems: 'center', backgroundColor: 'rgba(0,0,0,0.05)', borderRadius: 10, paddingHorizontal: 16, height: 44 },
  searchInput:   { flex: 1, fontSize: 15, color: '#0a0a0a' },
  nearMeRow:     { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 20, paddingVertical: 14, borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: 'rgba(0,0,0,0.1)' },
  nearMeIcon:    { width: 36, height: 36, borderRadius: 18, backgroundColor: 'rgba(255,60,110,0.1)', alignItems: 'center', justifyContent: 'center', marginRight: 12 },
  nearMeLabel:   { fontSize: 15, fontWeight: '700', color: '#0a0a0a' },
  nearMeSub:     { fontSize: 12, color: 'rgba(10,10,10,0.4)', marginTop: 1 },
  check:         { marginLeft: 'auto', color: '#ff3c6e', fontSize: 15, fontWeight: '800' },
  cityRow:       { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingVertical: 14, borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: 'rgba(0,0,0,0.07)' },
  cityName:      { fontSize: 15, fontWeight: '500', color: 'rgba(10,10,10,0.65)' },
  cityNameActive:{ fontWeight: '800', color: '#0a0a0a' },
  eventCount:    { fontSize: 11, color: 'rgba(10,10,10,0.35)', fontWeight: '600' },
  empty:         { textAlign: 'center', color: 'rgba(10,10,10,0.3)', fontSize: 13, marginTop: 24 },
});

// ─── DateRangePicker ──────────────────────────────────────────────────────────

interface DateRangePickerProps {
  visible: boolean;
  dateFrom: string;
  dateTo: string;
  onApply: (from: string, to: string) => void;
  onClose: () => void;
}

function DateRangePicker({ visible, dateFrom, dateTo, onApply, onClose }: DateRangePickerProps) {
  const [from, setFrom] = useState(dateFrom || todayISO());
  const [to, setTo]     = useState(dateTo  || todayISO(7));

  useEffect(() => {
    if (visible) {
      setFrom(dateFrom || todayISO());
      setTo(dateTo || todayISO(7));
    }
  }, [visible]);

  const isValid = from && to && from <= to;

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
      <Pressable
        style={{ flex: 1, justifyContent: 'flex-end', backgroundColor: 'rgba(0,0,0,0.4)' }}
        onPress={onClose}
      >
        <Pressable onPress={() => {}} style={dr.sheet}>
          <View style={dr.handle} />
          <Text style={dr.title}>Set Dates</Text>

          <Text style={dr.label}>FROM</Text>
          <TextInput
            value={from}
            onChangeText={setFrom}
            placeholder="YYYY-MM-DD"
            placeholderTextColor="rgba(10,10,10,0.25)"
            style={[dr.input, { borderColor: from ? '#0a0a0a' : 'rgba(10,10,10,0.15)' }]}
            keyboardType="numeric"
          />

          <Text style={dr.label}>TO</Text>
          <TextInput
            value={to}
            onChangeText={setTo}
            placeholder="YYYY-MM-DD"
            placeholderTextColor="rgba(10,10,10,0.25)"
            style={[dr.input, { borderColor: to ? '#0a0a0a' : 'rgba(10,10,10,0.15)', marginBottom: 6 }]}
            keyboardType="numeric"
          />

          {!isValid && from && to && (
            <Text style={{ fontSize: 12, color: '#ff3c6e', marginBottom: 4 }}>
              End date must be on or after start date
            </Text>
          )}
          {isValid && (
            <Text style={dr.preview}>
              {formatCustomDate(from)} → {formatCustomDate(to)}
            </Text>
          )}

          <TouchableOpacity
            onPress={() => isValid && onApply(from, to)}
            style={[dr.applyBtn, { backgroundColor: isValid ? '#0a0a0a' : 'rgba(10,10,10,0.2)' }]}
          >
            <Text style={dr.applyText}>Apply Dates</Text>
          </TouchableOpacity>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const dr = StyleSheet.create({
  sheet:    { backgroundColor: '#fff', borderTopLeftRadius: 20, borderTopRightRadius: 20, paddingHorizontal: 24, paddingTop: 12, paddingBottom: 40 },
  handle:   { width: 40, height: 4, borderRadius: 2, backgroundColor: 'rgba(0,0,0,0.12)', alignSelf: 'center', marginBottom: 20 },
  title:    { fontSize: 20, fontWeight: '800', color: '#0a0a0a', marginBottom: 24 },
  label:    { fontSize: 11, fontWeight: '800', letterSpacing: 1.5, color: 'rgba(10,10,10,0.4)', marginBottom: 8 },
  input:    { borderWidth: 1.5, borderRadius: 10, paddingHorizontal: 16, paddingVertical: 14, fontSize: 16, color: '#0a0a0a', fontWeight: '600', marginBottom: 20 },
  preview:  { fontSize: 13, color: 'rgba(10,10,10,0.45)', marginBottom: 24, marginTop: 6 },
  applyBtn: { borderRadius: 14, paddingVertical: 16, alignItems: 'center' },
  applyText:{ color: '#fff', fontWeight: '800', fontSize: 14, letterSpacing: 0.5 },
});

// ─── Main FilterModal ─────────────────────────────────────────────────────────

export default function FilterModal({ visible, onClose, onApply, initialParams }: FilterModalProps) {
  const [selectedCity, setSelectedCity] = useState<string | null>(initialParams?.city ?? null);
  const [nearMeCoords, setNearMeCoords] = useState<{ lat: number; lng: number } | undefined>(
    initialParams?.lat != null && initialParams?.lng != null
      ? { lat: initialParams.lat, lng: initialParams.lng }
      : undefined
  );

  const [datePreset, setDatePreset] = useState<DatePreset>(initialParams?.date ?? 'any');
  const [dateFrom, setDateFrom]     = useState(initialParams?.date_from ?? '');
  const [dateTo, setDateTo]         = useState(initialParams?.date_to   ?? '');

  const [locationPickerOpen, setLocationPickerOpen] = useState(false);
  const [datePickerOpen, setDatePickerOpen]         = useState(false);

  const isNearMe     = !selectedCity && nearMeCoords != null;
  const locationIsSet = !!selectedCity || !!nearMeCoords;
  const locationLabel = selectedCity ?? (nearMeCoords ? 'Near me' : 'Near me');

  function handleLocationSelect(city: string | null, coords?: { lat: number; lng: number }) {
    setSelectedCity(city);
    setNearMeCoords(coords);
    setLocationPickerOpen(false);
  }

  function handleDatePreset(preset: DatePreset) {
    if (preset === 'custom') { setDatePickerOpen(true); return; }
    setDatePreset(preset);
    setDateFrom('');
    setDateTo('');
  }

  function handleDateRangeApply(from: string, to: string) {
    setDatePreset('custom');
    setDateFrom(from);
    setDateTo(to);
    setDatePickerOpen(false);
  }

  function handleDone() {
    const params: FilterParams = { date: datePreset };
    if (selectedCity) {
      params.city = selectedCity;
    } else if (nearMeCoords) {
      params.lat = nearMeCoords.lat;
      params.lng = nearMeCoords.lng;
      params.radius_km = 50;
    }
    if (datePreset === 'custom' && dateFrom && dateTo) {
      params.date_from = dateFrom;
      params.date_to   = dateTo;
    }
    onApply(params);
  }

  const PILL_PRESETS: { key: DatePreset; label: string }[] = [
    { key: 'any',      label: 'Any date'  },
    { key: 'tomorrow', label: 'Tomorrow'  },
    { key: 'weekend',  label: 'Weekend'   },
  ];

  const customLabel =
    datePreset === 'custom' && dateFrom && dateTo
      ? `${formatCustomDate(dateFrom)} – ${formatCustomDate(dateTo)}`
      : 'Set dates';

  return (
    <>
      {/* ── Single Modal ─────────────────────────────────────────────────── */}
      <Modal
        visible={visible}
        transparent
        animationType="slide"
        onRequestClose={onClose}
        statusBarTranslucent
      >
        <View style={fm.root}>
          {/* Tappable backdrop */}
          <Pressable style={fm.backdrop} onPress={onClose} />

          {/* Sheet */}
          <View style={fm.sheet}>
            {/* Drag handle */}
            <View style={fm.handle} />

            {/* ── Location ── */}
            <Text style={fm.sectionTitle}>Location</Text>

            <TouchableOpacity
              onPress={() => setLocationPickerOpen(true)}
              style={fm.locationRow}
              activeOpacity={0.7}
            >
              <View style={fm.locationIcon}>
                <Text style={{ fontSize: 20 }}>📍</Text>
              </View>
              <Text style={[fm.locationText, locationIsSet && fm.locationTextSet]}>
                {locationLabel}
              </Text>
              <Text style={fm.chevron}>›</Text>
            </TouchableOpacity>

            {/* Divider */}
            <View style={fm.divider} />

            {/* ── Dates ── */}
            <Text style={[fm.sectionTitle, { marginTop: 8 }]}>Dates</Text>

            {/* Pills row */}
            <View style={fm.pillRow}>
              {PILL_PRESETS.map(({ key, label }) => {
                const active = datePreset === key;
                return (
                  <TouchableOpacity
                    key={key}
                    onPress={() => handleDatePreset(key)}
                    style={[fm.pill, active && fm.pillActive]}
                    activeOpacity={0.75}
                  >
                    <Text style={[fm.pillText, active && fm.pillTextActive]}>{label}</Text>
                  </TouchableOpacity>
                );
              })}
            </View>

            {/* Set dates button */}
            <TouchableOpacity
              onPress={() => handleDatePreset('custom')}
              style={[fm.setDatesBtn, datePreset === 'custom' && fm.setDatesBtnActive]}
              activeOpacity={0.75}
            >
              <Text style={[fm.setDatesText, datePreset === 'custom' && fm.setDatesTextActive]}>
                {customLabel}
              </Text>
            </TouchableOpacity>

            {/* Done */}
            <View style={fm.doneRow}>
              <TouchableOpacity onPress={handleDone} style={fm.doneBtn} activeOpacity={0.85}>
                <Text style={fm.doneBtnText}>Done</Text>
              </TouchableOpacity>
            </View>
          </View>
        </View>
      </Modal>

      {/* Sub-modals (rendered outside the main Modal to avoid nesting issues) */}
      <LocationPicker
        visible={locationPickerOpen}
        current={{ city: selectedCity, isNearMe }}
        onSelect={handleLocationSelect}
        onClose={() => setLocationPickerOpen(false)}
      />
      <DateRangePicker
        visible={datePickerOpen}
        dateFrom={dateFrom}
        dateTo={dateTo}
        onApply={handleDateRangeApply}
        onClose={() => setDatePickerOpen(false)}
      />
    </>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const fm = StyleSheet.create({
  root: {
    flex: 1,
    justifyContent: 'flex-end',
  },
  backdrop: {
    ...StyleSheet.absoluteFillObject,
    backgroundColor: 'rgba(0,0,0,0.45)',
  },
  sheet: {
    backgroundColor: '#fff',
    borderTopLeftRadius: 20,
    borderTopRightRadius: 20,
    paddingBottom: 44,
    shadowColor: '#000',
    shadowOffset: { width: 0, height: -4 },
    shadowOpacity: 0.14,
    shadowRadius: 20,
    elevation: 24,
  },
  handle: {
    width: 40,
    height: 4,
    borderRadius: 2,
    backgroundColor: 'rgba(0,0,0,0.12)',
    alignSelf: 'center',
    marginTop: 12,
    marginBottom: 24,
  },
  sectionTitle: {
    fontSize: 22,
    fontWeight: '800',
    color: '#0a0a0a',
    paddingHorizontal: 24,
    marginBottom: 4,
  },
  locationRow: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 24,
    paddingVertical: 14,
  },
  locationIcon: {
    width: 44,
    height: 44,
    borderRadius: 22,
    backgroundColor: 'rgba(10,10,10,0.06)',
    alignItems: 'center',
    justifyContent: 'center',
    marginRight: 14,
  },
  locationText: {
    flex: 1,
    fontSize: 17,
    fontWeight: '400',
    color: 'rgba(10,10,10,0.45)',
  },
  locationTextSet: {
    fontWeight: '600',
    color: '#0a0a0a',
  },
  chevron: {
    fontSize: 22,
    color: 'rgba(10,10,10,0.3)',
  },
  divider: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: 'rgba(0,0,0,0.1)',
    marginHorizontal: 24,
    marginVertical: 8,
  },
  pillRow: {
    flexDirection: 'row',
    paddingHorizontal: 24,
    gap: 10,
    marginTop: 12,
    marginBottom: 12,
  },
  pill: {
    flex: 1,
    paddingVertical: 11,
    borderRadius: 24,
    borderWidth: 1,
    borderColor: 'rgba(10,10,10,0.2)',
    backgroundColor: '#fff',
    alignItems: 'center',
  },
  pillActive: {
    borderWidth: 2,
    borderColor: '#0a0a0a',
  },
  pillText: {
    fontSize: 14,
    fontWeight: '500',
    color: 'rgba(10,10,10,0.55)',
  },
  pillTextActive: {
    fontWeight: '800',
    color: '#0a0a0a',
  },
  setDatesBtn: {
    marginHorizontal: 24,
    paddingVertical: 15,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: 'rgba(10,10,10,0.18)',
    alignItems: 'center',
    backgroundColor: '#fff',
  },
  setDatesBtnActive: {
    borderWidth: 2,
    borderColor: '#0a0a0a',
  },
  setDatesText: {
    fontSize: 15,
    fontWeight: '600',
    color: 'rgba(10,10,10,0.6)',
  },
  setDatesTextActive: {
    fontWeight: '800',
    color: '#0a0a0a',
  },
  doneRow: {
    paddingHorizontal: 24,
    marginTop: 24,
    alignItems: 'flex-end',
  },
  doneBtn: {
    backgroundColor: '#0a0a0a',
    borderRadius: 14,
    paddingVertical: 14,
    paddingHorizontal: 40,
  },
  doneBtnText: {
    color: '#fff',
    fontWeight: '800',
    fontSize: 15,
    letterSpacing: 0.3,
  },
});

// ─── Utility ──────────────────────────────────────────────────────────────────

export function buildFilterQueryString(params: FilterParams): string {
  const qs = new URLSearchParams();
  if (params.city)       qs.set('city',      params.city);
  if (params.lat != null) qs.set('lat',      String(params.lat));
  if (params.lng != null) qs.set('lng',      String(params.lng));
  if (params.radius_km)  qs.set('radius_km', String(params.radius_km));
  if (params.date && params.date !== 'any') qs.set('date', params.date);
  if (params.date === 'custom' && params.date_from) qs.set('date_from', params.date_from);
  if (params.date === 'custom' && params.date_to)   qs.set('date_to',   params.date_to);
  return qs.toString();
}