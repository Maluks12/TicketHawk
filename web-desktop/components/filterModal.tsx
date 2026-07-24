'use client';

import { useState, useEffect, useRef } from 'react';
import { MapPin, Calendar, X, ChevronLeft, ChevronRight } from 'lucide-react';
import { useAppTheme } from '@/themes/colors';

// ── Types & exports ───────────────────────────────────────────────────────────
export type DatePreset = 'any' | 'today' | 'tomorrow' | 'weekend' | 'custom';

export interface FilterParams {
  city?: string;
  province?: string;
  lat?: number;
  lng?: number;
  radius_km?: number;
  date: DatePreset;
  date_from?: string;
  date_to?: string;
}

interface FilterModalProps {
  visible: boolean;
  onClose: () => void;
  onApply: (params: FilterParams) => void;
  initialParams?: Partial<FilterParams>;
}

export function getBaseUrl(): string {
  return process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080';
}

export function formatDateRangeLabel(from: string, to: string): string {
  const f = new Date(from).toLocaleDateString('en-ZA', { day: 'numeric', month: 'short' });
  const t = new Date(to).toLocaleDateString('en-ZA', { day: 'numeric', month: 'short' });
  return `${f} – ${t}`;
}

export function buildFilterQueryString(params: FilterParams): string {
  const qs = new URLSearchParams();
  if (params.city)         qs.set('city',      params.city);
  if (params.province)     qs.set('province',  params.province);
  if (params.lat  != null) qs.set('lat',       String(params.lat));
  if (params.lng  != null) qs.set('lng',       String(params.lng));
  if (params.radius_km)    qs.set('radius_km', String(params.radius_km));
  if (params.date && params.date !== 'any') qs.set('date', params.date);
  if (params.date === 'custom' && params.date_from) qs.set('date_from', params.date_from);
  if (params.date === 'custom' && params.date_to)   qs.set('date_to',   params.date_to);
  return qs.toString();
}

// ── South African cities (sorted) ─────────────────────────────────────────────
const SA_CITIES = [
  'Beaufort West', 'Bela-Bela', 'Bellville', 'Benoni', 'Bhisho',
  'Bloemfontein', 'Boksburg', 'Brakpan', 'Brits', 'Butterworth',
  'Cape Town', 'Centurion', 'Chatsworth', 'Cofimvaba', 'Cradock',
  'De Aar', 'Durban', 'East London', 'Edenvale', 'eMalahleni',
  'Empangeni', 'George', 'Germiston', 'Gqeberha', 'Hermanus',
  'Johannesburg', 'Khayelitsha', 'Kimberley', 'King Williams Town',
  'Klerksdorp', 'Knysna', 'Kroonstad', 'Krugersdorp', 'Ladysmith',
  'Lephalale', 'Mahikeng', 'Mafikeng', 'Mamelodi', 'Mbombela',
  'Middelburg', 'Midrand', "Mitchell's Plain", 'Mokopane', 'Mossel Bay',
  'Mthatha', 'Nelspruit', 'Newcastle', 'Oudtshoorn', 'Paarl',
  'Phalaborwa', 'Phuthaditjhaba', 'Pietermaritzburg', 'Pinetown',
  'Polokwane', 'Port Elizabeth', 'Potchefstroom', 'Pretoria',
  'Queenstown', 'Randburg', 'Richards Bay', 'Roodepoort', 'Rustenburg',
  'Sandton', 'Sasolburg', 'Secunda', 'Somerset West', 'Soshanguve',
  'Soweto', 'Springs', 'Standerton', 'Stellenbosch', 'Thohoyandou',
  'Tongaat', 'Tshwane', 'Tzaneen', 'Umlazi', 'Upington',
  'Vanderbijlpark', 'Vereeniging', 'Welkom', 'Witbank', 'Worcester',
].sort();

// ── City autocomplete ─────────────────────────────────────────────────────────
function highlightMatch(city: string, query: string) {
  const idx = city.toLowerCase().indexOf(query.toLowerCase());
  if (idx === -1) return <span>{city}</span>;
  return (
    <>
      {city.slice(0, idx)}
      <span className="font-bold" style={{ color: '#FF2D55' }}>
        {city.slice(idx, idx + query.length)}
      </span>
      {city.slice(idx + query.length)}
    </>
  );
}

function CityAutocomplete({
  value,
  onChange,
}: {
  value: string;
  onChange: (v: string) => void;
}) {
  const { colors } = useAppTheme();
  const [isFocused,   setIsFocused]   = useState(false);
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const justSelected = useRef(false);

  const open = isFocused && suggestions.length > 0;

  useEffect(() => {
    if (justSelected.current) { justSelected.current = false; return; }
    if (value.length >= 2) {
      setSuggestions(
        SA_CITIES
          .filter(c => c.toLowerCase().includes(value.toLowerCase()))
          .slice(0, 7),
      );
    } else {
      setSuggestions([]);
    }
  }, [value]);

  return (
    <div className="relative">
      <input
        className="w-full rounded-full px-4 py-2.5 text-sm focus:outline-none transition-colors duration-150"
        style={{
          border: `1px solid ${colors.dividerColor}`,
          backgroundColor: colors.sectionCardBg,
          color: colors.text,
        }}
        placeholder="e.g. Cape Town"
        value={value}
        autoComplete="off"
        spellCheck={false}
        onChange={(e) => onChange(e.target.value)}
        onFocus={() => setIsFocused(true)}
        onBlur={() => setIsFocused(false)}
      />

      {open && (
        <div
          className="absolute top-full left-0 right-0 mt-1.5 rounded-2xl shadow-2xl z-30 overflow-hidden"
          style={{ backgroundColor: colors.cardBg, border: `1px solid ${colors.dividerColor}` }}
        >
          {suggestions.map((city) => (
            <button
              key={city}
              type="button"
              className="w-full text-left px-4 py-2.5 text-sm transition-colors flex items-center gap-2.5"
              style={{ color: colors.text }}
              onMouseDown={(e) => e.preventDefault()} /* keep input focused */
              onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = colors.sectionCardBg; }}
              onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = 'transparent'; }}
              onClick={() => {
                justSelected.current = true;
                onChange(city);
                setSuggestions([]);
                setIsFocused(false);
              }}
            >
              <MapPin size={11} className="shrink-0" color={colors.iconColor} />
              <span>{highlightMatch(city, value)}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// ── Calendar range picker ─────────────────────────────────────────────────────
const WEEKDAY_LABELS = ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa'];
const MONTH_NAMES = [
  'January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December',
];

function getTodayStr() {
  const t = new Date();
  return `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`;
}

function makeDateStr(year: number, month: number, day: number) {
  return `${year}-${String(month + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
}

function RangePicker({
  from,
  to,
  onChange,
}: {
  from: string;
  to: string;
  onChange: (from: string, to: string) => void;
}) {
  const { colors } = useAppTheme();
  const today = getTodayStr();

  const [viewYear,  setViewYear]  = useState(() =>
    from ? parseInt(from.split('-')[0]) : new Date().getFullYear(),
  );
  const [viewMonth, setViewMonth] = useState(() =>
    from ? parseInt(from.split('-')[1]) - 1 : new Date().getMonth(),
  );
  const [start,     setStart]     = useState(from);
  const [end,       setEnd]       = useState(to);
  const [selecting, setSelecting] = useState<'start' | 'end'>(
    from && !to ? 'end' : 'start',
  );
  const [hover, setHover] = useState('');

  const goPrev = () => {
    if (viewMonth === 0) { setViewYear(y => y - 1); setViewMonth(11); }
    else setViewMonth(m => m - 1);
  };
  const goNext = () => {
    if (viewMonth === 11) { setViewYear(y => y + 1); setViewMonth(0); }
    else setViewMonth(m => m + 1);
  };

  const handleDayClick = (dateStr: string) => {
    if (dateStr < today) return;

    if (selecting === 'start' || (start && end)) {
      // Begin fresh selection
      setStart(dateStr); setEnd(''); setSelecting('end');
      onChange(dateStr, '');
    } else {
      // Pick end
      if (dateStr < start) {
        setEnd(start); setStart(dateStr);
        onChange(dateStr, start);
        setSelecting('start');
      } else if (dateStr === start) {
        // no-op: same day, stay in end-picking mode
      } else {
        setEnd(dateStr);
        onChange(start, dateStr);
        setSelecting('start');
      }
    }
  };

  const daysInMonth     = new Date(viewYear, viewMonth + 1, 0).getDate();
  const firstDayOfMonth = new Date(viewYear, viewMonth, 1).getDay(); // 0 = Sun

  // Preview range while hovering (only in 'end' mode after start is picked)
  const effectiveEnd =
    end || (selecting === 'end' && hover && hover > start ? hover : '');

  // Build cell array: null = padding, number = day
  const cells: (number | null)[] = Array(firstDayOfMonth).fill(null);
  for (let d = 1; d <= daysInMonth; d++) cells.push(d);

  const BAND = 'rgba(255,45,85,0.18)';

  return (
    <div className="select-none">
      {/* Month navigation */}
      <div className="flex items-center justify-between mb-3">
        <button
          type="button"
          onClick={goPrev}
          className="w-7 h-7 flex items-center justify-center rounded-full transition-colors"
          style={{ color: colors.subText }}
          onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = colors.sectionCardBg; e.currentTarget.style.color = colors.text; }}
          onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = 'transparent'; e.currentTarget.style.color = colors.subText; }}
        >
          <ChevronLeft size={14} />
        </button>
        <span className="text-[13px] font-bold" style={{ color: colors.text }}>
          {MONTH_NAMES[viewMonth]} {viewYear}
        </span>
        <button
          type="button"
          onClick={goNext}
          className="w-7 h-7 flex items-center justify-center rounded-full transition-colors"
          style={{ color: colors.subText }}
          onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = colors.sectionCardBg; e.currentTarget.style.color = colors.text; }}
          onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = 'transparent'; e.currentTarget.style.color = colors.subText; }}
        >
          <ChevronRight size={14} />
        </button>
      </div>

      {/* Weekday headers */}
      <div className="grid grid-cols-7 mb-1">
        {WEEKDAY_LABELS.map(d => (
          <div key={d} className="text-center text-[9px] font-black tracking-wider py-1" style={{ color: colors.subText }}>
            {d}
          </div>
        ))}
      </div>

      {/* Day grid */}
      <div className="grid grid-cols-7" onMouseLeave={() => setHover('')}>
        {cells.map((day, idx) => {
          if (!day) return <div key={`pad-${idx}`} className="h-8" />;

          const dateStr  = makeDateStr(viewYear, viewMonth, day);
          const isPast   = dateStr < today;
          const isToday  = dateStr === today;

          // Is this cell the selected start or end?
          const hasRange    = !!(start && effectiveEnd && effectiveEnd > start);
          const isStart     = hasRange && dateStr === start;
          const isEnd       = hasRange && dateStr === effectiveEnd;
          const isAlone     = !!(start && dateStr === start && !hasRange);
          const inRange     = hasRange && dateStr > start && dateStr < effectiveEnd;
          const isHighlight = isStart || isEnd || isAlone;

          // Half-band CSS background
          const showBandLeft  = inRange || isEnd;
          const showBandRight = inRange || isStart;
          let bandBg = 'transparent';
          if (showBandLeft && showBandRight) {
            bandBg = BAND;
          } else if (showBandLeft) {
            bandBg = `linear-gradient(to right, ${BAND} 50%, transparent 50%)`;
          } else if (showBandRight) {
            bandBg = `linear-gradient(to right, transparent 50%, ${BAND} 50%)`;
          }

          return (
            <div
              key={day}
              className="h-8 flex items-center justify-center"
              style={{ background: bandBg }}
            >
              <button
                type="button"
                disabled={isPast}
                onClick={() => handleDayClick(dateStr)}
                onMouseEnter={() => !isPast && setHover(dateStr)}
                className="w-7 h-7 rounded-full text-[12px] relative z-10 transition-colors"
                style={{
                  cursor: isPast ? 'not-allowed' : 'pointer',
                  color: isPast ? colors.dividerColor : isHighlight ? '#ffffff' : colors.text,
                  backgroundColor: isHighlight ? '#FF2D55' : 'transparent',
                  fontWeight: isHighlight ? 700 : isToday ? 700 : 400,
                  boxShadow: isHighlight ? '0 3px 10px rgba(255,45,85,0.35)' : 'none',
                }}
              >
                {day}
                {/* Today indicator — navy, distinct from the pink selection state */}
                {isToday && !isHighlight && (
                  <span
                    className="absolute bottom-[3px] left-1/2 -translate-x-1/2 w-[3px] h-[3px] rounded-full"
                    style={{ backgroundColor: colors.secondaryBg }}
                  />
                )}
              </button>
            </div>
          );
        })}
      </div>

      {/* Status bar */}
      <div className="mt-3 py-2 px-3 rounded-2xl text-center min-h-[32px] flex items-center justify-center" style={{ backgroundColor: colors.sectionCardBg }}>
        {!start ? (
          <p className="text-[11px]" style={{ color: colors.subText }}>Click to select a start date</p>
        ) : !end ? (
          <p className="text-[11px]" style={{ color: '#FF2D55' }}>
            <span className="font-bold">
              {new Date(start + 'T00:00:00').toLocaleDateString('en-ZA', { day: 'numeric', month: 'short' })}
            </span>
            <span className="mx-1.5" style={{ color: colors.subText }}>→</span>
            <span style={{ color: colors.subText }}>pick an end date</span>
          </p>
        ) : (
          <p className="text-[11px] font-semibold" style={{ color: colors.text }}>
            {new Date(start + 'T00:00:00').toLocaleDateString('en-ZA', { day: 'numeric', month: 'short' })}
            <span className="mx-2" style={{ color: colors.subText }}>–</span>
            {new Date(end + 'T00:00:00').toLocaleDateString('en-ZA', { day: 'numeric', month: 'short' })}
          </p>
        )}
      </div>
    </div>
  );
}

// ── Constants ─────────────────────────────────────────────────────────────────
const PROVINCES = [
  'Any', 'Gauteng', 'Western Cape', 'Eastern Cape',
  'KwaZulu-Natal', 'Limpopo', 'Mpumalanga',
  'North West', 'Free State', 'Northern Cape',
];

const DATE_PRESETS: { value: DatePreset; label: string }[] = [
  { value: 'any',      label: 'Any time'     },
  { value: 'today',    label: 'Today'        },
  { value: 'tomorrow', label: 'Tomorrow'     },
  { value: 'weekend',  label: 'This Weekend' },
  { value: 'custom',   label: 'Custom range' },
];

// ── Main FilterModal ──────────────────────────────────────────────────────────
export default function FilterModal({
  visible,
  onClose,
  onApply,
  initialParams,
}: FilterModalProps) {
  const { colors } = useAppTheme();

  const [city,       setCity]       = useState(initialParams?.city       ?? '');
  const [province,   setProvince]   = useState(initialParams?.province   ?? '');
  const [date,       setDate]       = useState<DatePreset>(initialParams?.date ?? 'any');
  const [dateFrom,   setDateFrom]   = useState(initialParams?.date_from  ?? '');
  const [dateTo,     setDateTo]     = useState(initialParams?.date_to    ?? '');
  const [nearbyMode, setNearbyMode] = useState(false);
  const [radiusKm,   setRadiusKm]   = useState(initialParams?.radius_km  ?? 25);
  const [gpsError,   setGpsError]   = useState('');
  const [gpsLoading, setGpsLoading] = useState(false);
  const [calKey,     setCalKey]     = useState(0); // force RangePicker remount on reset
  const latRef = useRef<number | undefined>(initialParams?.lat);
  const lngRef = useRef<number | undefined>(initialParams?.lng);

  // Escape to close
  useEffect(() => {
    if (!visible) return;
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', h);
    return () => document.removeEventListener('keydown', h);
  }, [visible, onClose]);

  // Lock body scroll while open
  useEffect(() => {
    document.body.style.overflow = visible ? 'hidden' : '';
    return () => { document.body.style.overflow = ''; };
  }, [visible]);

  if (!visible) return null;

  const handleApply = () => {
    onApply({
      city:      nearbyMode ? undefined : (city.trim() || undefined),
      province:  nearbyMode ? undefined : (province.trim() || undefined),
      lat:       nearbyMode ? latRef.current : undefined,
      lng:       nearbyMode ? lngRef.current : undefined,
      radius_km: nearbyMode ? radiusKm       : undefined,
      date,
      date_from: date === 'custom' ? dateFrom : undefined,
      date_to:   date === 'custom' ? dateTo   : undefined,
    });
  };

  const handleNearbyToggle = () => {
    // Turn off (or cancel an in-progress GPS request)
    if (nearbyMode || gpsLoading) {
      setNearbyMode(false);
      setGpsLoading(false);
      setGpsError('');
      return;
    }
    setGpsError('');
    setGpsLoading(true);
    if (!navigator.geolocation) {
      setGpsError('Geolocation is not supported by your browser.');
      setGpsLoading(false);
      return;
    }
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        latRef.current = pos.coords.latitude;
        lngRef.current = pos.coords.longitude;
        setNearbyMode(true);
        setGpsLoading(false);
      },
      () => {
        setGpsError('Location permission denied. Please allow access in your browser settings.');
        setGpsLoading(false);
      },
    );
  };

  const handleReset = () => {
    setCity(''); setProvince(''); setDate('any');
    setDateFrom(''); setDateTo('');
    setNearbyMode(false); setRadiusKm(25); setGpsError('');
    latRef.current = undefined; lngRef.current = undefined;
    setCalKey(k => k + 1); // remount RangePicker fresh
  };

  return (
    /* Backdrop */
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
      onClick={onClose}
    >
      {/* Dialog — no overflow-hidden so city autocomplete can escape */}
      <div
        className="relative w-full max-w-2xl rounded-2xl shadow-2xl"
        style={{ backgroundColor: colors.cardBg }}
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="sr-only">Filter events</h2>
        {/* Close button floats over the corner — no header row, no dead space */}
        <button
          onClick={onClose}
          className="absolute top-4 right-4 z-10 w-8 h-8 flex items-center justify-center rounded-full transition-colors"
          style={{ backgroundColor: colors.sectionCardBg }}
          aria-label="Close"
        >
          <X size={15} color={colors.text} />
        </button>

        {/* ── Body (two columns) ──────────────────────────────────────────────── */}
        <div className="grid grid-cols-2 rounded-t-2xl overflow-hidden" style={{ borderTop: 'none' }}>

          {/* LEFT: Location */}
          <div className="px-8 pt-6 pb-6 space-y-5" style={{ borderRight: `1px solid ${colors.dividerColor}` }}>
            <div className="flex items-center gap-2">
              <MapPin size={13} color="#FF2D55" />
              <span className="text-[10px] font-black tracking-[1.8px]" style={{ color: colors.subText }}>LOCATION</span>

            </div>

            {/* Nearby toggle */}
            <div className="flex items-center justify-between py-1">
              <div>
                <p className="text-sm font-bold leading-tight" style={{ color: colors.text }}>
                  {gpsLoading ? 'Detecting location…' : 'Use my location'}
                </p>
                <p className="text-[11px] mt-0.5" style={{ color: colors.subText }}>Find events near you</p>
              </div>
              <button
                type="button"
                onClick={handleNearbyToggle}
                aria-pressed={nearbyMode}
                className="relative w-12 h-6 rounded-full shrink-0 transition-colors duration-200 focus:outline-none"
                style={{
                  backgroundColor: nearbyMode ? '#FF2D55' : colors.sectionCardBg,
                  cursor: gpsLoading ? 'wait' : 'pointer',
                  boxShadow: nearbyMode ? '0 4px 14px rgba(255,45,85,0.35)' : 'none',
                }}
              >
                {/* Thumb — left-[2px] anchors it; translate shifts it on/off */}
                <span
                  className="absolute top-[2px] left-[2px] w-5 h-5 rounded-full bg-white shadow-md transition-transform duration-200"
                  style={{ transform: nearbyMode ? 'translateX(22px)' : 'translateX(0)' }}
                />
                {/* Spinner overlay while GPS is loading */}
                {gpsLoading && (
                  <span className="absolute inset-0 flex items-center justify-center">
                    <span className="w-3 h-3 border-2 border-white/40 border-t-white rounded-full animate-spin" />
                  </span>
                )}
              </button>
            </div>

            {gpsError && (
              <p className="text-[11px] text-red-500 leading-snug">{gpsError}</p>
            )}

            {nearbyMode && (
              <div className="space-y-2 pt-1">
                <div className="flex items-center justify-between">
                  <label className="text-[10px] font-black tracking-[1.5px]" style={{ color: colors.subText }}>RADIUS</label>
                  <span className="text-sm font-bold" style={{ color: colors.text }}>{radiusKm} km</span>
                </div>
                <input
                  type="range" min={5} max={100} step={5}
                  value={radiusKm}
                  onChange={(e) => setRadiusKm(Number(e.target.value))}
                  className="w-full accent-[#FF2D55]"
                />
                <div className="flex justify-between text-[10px]" style={{ color: colors.subText }}>
                  <span>5 km</span><span>100 km</span>
                </div>
              </div>
            )}

            {!nearbyMode && (
              <div className="space-y-4">
                {/* City with autocomplete */}
                <div className="space-y-1.5">
                  <label className="text-[10px] font-black tracking-[1.5px]" style={{ color: colors.subText }}>CITY</label>
                  <CityAutocomplete
                    value={city}
                    onChange={(v) => {
                      setCity(v);
                      if (v.length > 0) setProvince(''); // city takes priority
                    }}
                  />
                  {city.length > 0 && city.length < 2 && (
                    <p className="text-[10px] pl-1" style={{ color: colors.subText }}>Type at least 2 letters to search…</p>
                  )}
                </div>

                {/* Province chips */}
                <div className="space-y-2">
                  <label className="text-[10px] font-black tracking-[1.5px]" style={{ color: colors.subText }}>PROVINCE</label>
                  <div className="flex flex-wrap gap-1.5">
                    {PROVINCES.map((p) => {
                      const val    = p === 'Any' ? '' : p;
                      const active = province === val;
                      return (
                        <button
                          key={p}
                          type="button"
                          onClick={() => {
                            setProvince(val);
                            if (val !== '') setCity(''); // province takes priority
                          }}
                          className="px-3 py-1.5 rounded-full text-[11px] font-bold tracking-wide transition-all duration-200 ease-out"
                          style={{
                            backgroundColor: active ? '#FF2D55' : 'transparent',
                            color: active ? '#ffffff' : colors.subText,
                            border: `1px solid ${active ? '#FF2D55' : colors.dividerColor}`,
                            boxShadow: active ? '0 4px 14px rgba(255,45,85,0.35)' : 'none',
                          }}
                        >
                          {p}
                        </button>
                      );
                    })}
                  </div>
                </div>
              </div>
            )}
          </div>

          {/* RIGHT: Date */}
          <div className="px-8 pt-6 pb-6 space-y-4">
            <div className="flex items-center gap-2">
              <Calendar size={13} color="#FF2D55" />
              <span className="text-[10px] font-black tracking-[1.8px]" style={{ color: colors.subText }}>DATE</span>
            </div>

            {/* Preset buttons */}
            <div className="space-y-2">
              {DATE_PRESETS.map(({ value, label }) => {
                const active = date === value;
                return (
                  <button
                    key={value}
                    type="button"
                    onClick={() => setDate(value)}
                    className="w-full flex items-center justify-between px-4 py-3 rounded-2xl text-sm font-semibold transition-all duration-200 ease-out"
                    style={{
                      backgroundColor: active ? '#FF2D55' : 'transparent',
                      color: active ? '#ffffff' : colors.text,
                      border: `1px solid ${active ? '#FF2D55' : colors.dividerColor}`,
                      boxShadow: active ? '0 4px 14px rgba(255,45,85,0.35)' : 'none',
                    }}
                  >
                    <span>{label}</span>
                  </button>
                );
              })}
            </div>

            {/* Calendar — shown only for custom range */}
            {date === 'custom' && (
              <div className="pt-1" style={{ borderTop: `1px solid ${colors.dividerColor}` }}>
                <RangePicker
                  key={calKey}
                  from={dateFrom}
                  to={dateTo}
                  onChange={(f, t) => { setDateFrom(f); setDateTo(t); }}
                />
              </div>
            )}
          </div>
        </div>

        {/* ── Footer ─────────────────────────────────────────────────────────── */}
        <div
          className="flex items-center gap-3 px-8 py-5 rounded-b-2xl"
          style={{ borderTop: `1px solid ${colors.dividerColor}`, backgroundColor: colors.cardBg }}
        >
          <button
            type="button"
            onClick={handleReset}
            className="px-5 py-2.5 rounded-full text-[12px] font-extrabold tracking-[1px] transition-all duration-150 hover:opacity-80"
            style={{ backgroundColor: colors.sectionCardBg, color: colors.text }}
          >
            RESET
          </button>
          <button
            type="button"
            onClick={handleApply}
            className="flex-1 font-extrabold text-[12px] tracking-[1.5px] py-2.5 rounded-full transition-all duration-150 hover:opacity-90 hover:scale-[1.01]"
            style={{ backgroundColor: '#FF2D55', color: '#ffffff', boxShadow: '0 4px 14px rgba(255,45,85,0.35)' }}
          >
            APPLY FILTERS
          </button>
        </div>
      </div>
    </div>
  );
}