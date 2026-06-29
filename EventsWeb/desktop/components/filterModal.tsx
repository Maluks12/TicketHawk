'use client';

import { useState, useEffect, useRef } from 'react';
import { MapPin, Calendar, X, ChevronLeft, ChevronRight } from 'lucide-react';

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
        className="w-full border border-black/10 dark:border-white/10 dark:bg-transparent dark:text-white rounded-lg px-4 py-2.5 text-sm placeholder:text-gray-300 dark:placeholder:text-gray-600 focus:outline-none focus:ring-2 focus:ring-[#FF2D55]/30"
        placeholder="e.g. Cape Town"
        value={value}
        autoComplete="off"
        spellCheck={false}
        onChange={(e) => onChange(e.target.value)}
        onFocus={() => setIsFocused(true)}
        onBlur={() => setIsFocused(false)}
      />

      {open && (
        <div className="absolute top-full left-0 right-0 mt-1 bg-white dark:bg-[#1c1c1c] rounded-xl border border-black/10 dark:border-white/10 shadow-2xl z-30 overflow-hidden">
          {suggestions.map((city) => (
            <button
              key={city}
              type="button"
              className="w-full text-left px-4 py-2.5 text-sm dark:text-gray-200 hover:bg-[#FF2D55]/10 transition-colors flex items-center gap-2.5"
              onMouseDown={(e) => e.preventDefault()} /* keep input focused */
              onClick={() => {
                justSelected.current = true;
                onChange(city);
                setSuggestions([]);
                setIsFocused(false);
              }}
            >
              <MapPin size={11} className="shrink-0 text-gray-400" />
              <span className="dark:text-gray-200">{highlightMatch(city, value)}</span>
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
          className="w-7 h-7 flex items-center justify-center rounded-full text-gray-400 hover:bg-black/5 dark:hover:bg-white/10 hover:text-black dark:hover:text-white transition-colors"
        >
          <ChevronLeft size={14} />
        </button>
        <span className="text-[13px] font-bold dark:text-white">
          {MONTH_NAMES[viewMonth]} {viewYear}
        </span>
        <button
          type="button"
          onClick={goNext}
          className="w-7 h-7 flex items-center justify-center rounded-full text-gray-400 hover:bg-black/5 dark:hover:bg-white/10 hover:text-black dark:hover:text-white transition-colors"
        >
          <ChevronRight size={14} />
        </button>
      </div>

      {/* Weekday headers */}
      <div className="grid grid-cols-7 mb-1">
        {WEEKDAY_LABELS.map(d => (
          <div key={d} className="text-center text-[9px] font-black tracking-wider text-gray-400 py-1">
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
                className={[
                  'w-7 h-7 rounded-full text-[12px] relative z-10 transition-colors',
                  isPast
                    ? 'text-gray-600 dark:text-gray-700 cursor-not-allowed'
                    : isHighlight
                      ? 'bg-[#FF2D55] text-white font-bold cursor-pointer'
                      : 'dark:text-white cursor-pointer hover:bg-black/8 dark:hover:bg-white/10',
                  isToday && !isHighlight ? 'font-bold' : '',
                ].join(' ')}
              >
                {day}
                {/* Today dot */}
                {isToday && !isHighlight && (
                  <span className="absolute bottom-[3px] left-1/2 -translate-x-1/2 w-[3px] h-[3px] rounded-full bg-[#FF2D55]" />
                )}
              </button>
            </div>
          );
        })}
      </div>

      {/* Status bar */}
      <div className="mt-3 py-2 px-3 rounded-lg bg-black/[0.03] dark:bg-white/[0.04] text-center min-h-[32px] flex items-center justify-center">
        {!start ? (
          <p className="text-[11px] text-gray-400">Click to select a start date</p>
        ) : !end ? (
          <p className="text-[11px] text-[#FF2D55]">
            <span className="font-bold">
              {new Date(start + 'T00:00:00').toLocaleDateString('en-ZA', { day: 'numeric', month: 'short' })}
            </span>
            <span className="text-gray-400 mx-1.5">→</span>
            <span className="text-gray-400">pick an end date</span>
          </p>
        ) : (
          <p className="text-[11px] font-semibold dark:text-white">
            {new Date(start + 'T00:00:00').toLocaleDateString('en-ZA', { day: 'numeric', month: 'short' })}
            <span className="mx-2 text-gray-400">–</span>
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
        className="w-full max-w-2xl bg-white dark:bg-[#0d0d0d] rounded-2xl shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* ── Header ─────────────────────────────────────────────────────────── */}
        <div className="flex items-center justify-between px-8 pt-7 pb-5 border-b border-black/[0.06] dark:border-white/[0.06] rounded-t-2xl bg-white dark:bg-[#0d0d0d]">
          <h2 className="text-base font-black tracking-[1.5px] dark:text-white">
            FILTER EVENTS
          </h2>
          <button
            onClick={onClose}
            className="w-8 h-8 flex items-center justify-center rounded-full bg-black/[0.05] dark:bg-white/[0.08] hover:bg-black/10 dark:hover:bg-white/15 transition-colors"
            aria-label="Close"
          >
            <X size={15} className="text-black dark:text-white" />
          </button>
        </div>

        {/* ── Body (two columns) ──────────────────────────────────────────────── */}
        <div className="grid grid-cols-2 divide-x divide-black/[0.06] dark:divide-white/[0.06]">

          {/* LEFT: Location */}
          <div className="px-8 py-6 space-y-5">
            <div className="flex items-center gap-2">
              <MapPin size={13} className="text-[#FF2D55]" />
              <span className="text-[10px] font-black tracking-[1.8px] text-gray-400">LOCATION</span>
            </div>

            {/* Nearby toggle */}
            <div className="flex items-center justify-between py-1">
              <div>
                <p className="text-sm font-bold dark:text-white leading-tight">
                  {gpsLoading ? 'Detecting location…' : 'Use my location'}
                </p>
                <p className="text-[11px] text-gray-400 mt-0.5">Find events near you</p>
              </div>
              <button
                type="button"
                onClick={handleNearbyToggle}
                aria-pressed={nearbyMode}
                className={`relative w-12 h-6 rounded-full shrink-0 transition-colors duration-200 focus:outline-none ${
                  nearbyMode    ? 'bg-[#FF2D55]'              : 'bg-gray-200 dark:bg-gray-700'
                } ${gpsLoading ? 'cursor-wait'               : 'cursor-pointer'}`}
              >
                {/* Thumb — left-[2px] anchors it; translate shifts it on/off */}
                <span
                  className={`absolute top-[2px] left-[2px] w-5 h-5 rounded-full bg-white shadow-md transition-transform duration-200 ${
                    nearbyMode ? 'translate-x-[22px]' : 'translate-x-0'
                  }`}
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
                  <label className="text-[10px] font-black tracking-[1.5px] text-gray-400">RADIUS</label>
                  <span className="text-sm font-bold dark:text-white">{radiusKm} km</span>
                </div>
                <input
                  type="range" min={5} max={100} step={5}
                  value={radiusKm}
                  onChange={(e) => setRadiusKm(Number(e.target.value))}
                  className="w-full accent-[#FF2D55]"
                />
                <div className="flex justify-between text-[10px] text-gray-400">
                  <span>5 km</span><span>100 km</span>
                </div>
              </div>
            )}

            {!nearbyMode && (
              <div className="space-y-4">
                {/* City with autocomplete */}
                <div className="space-y-1.5">
                  <label className="text-[10px] font-black tracking-[1.5px] text-gray-400">CITY</label>
                  <CityAutocomplete
                    value={city}
                    onChange={(v) => {
                      setCity(v);
                      if (v.length > 0) setProvince(''); // city takes priority
                    }}
                  />
                  {city.length > 0 && city.length < 2 && (
                    <p className="text-[10px] text-gray-400 pl-1">Type at least 2 letters to search…</p>
                  )}
                </div>

                {/* Province chips */}
                <div className="space-y-2">
                  <label className="text-[10px] font-black tracking-[1.5px] text-gray-400">PROVINCE</label>
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
                          className={`px-3 py-1.5 rounded-full border text-[11px] font-bold tracking-wide transition-colors ${
                            active
                              ? 'bg-black dark:bg-white text-white dark:text-black border-black dark:border-white'
                              : 'border-black/10 dark:border-white/15 text-gray-500 dark:text-gray-400 hover:border-black/25 dark:hover:border-white/30'
                          }`}
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
          <div className="px-8 py-6 space-y-4">
            <div className="flex items-center gap-2">
              <Calendar size={13} className="text-[#FF2D55]" />
              <span className="text-[10px] font-black tracking-[1.8px] text-gray-400">DATE</span>
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
                    className={`w-full flex items-center justify-between px-4 py-3 rounded-xl border text-sm font-semibold transition-all ${
                      active
                        ? 'bg-black dark:bg-white text-white dark:text-black border-black dark:border-white'
                        : 'border-black/08 dark:border-white/08 text-gray-700 dark:text-gray-300 hover:border-black/20 dark:hover:border-white/20 hover:bg-black/[0.02] dark:hover:bg-white/[0.02]'
                    }`}
                  >
                    <span>{label}</span>
                    {active && (
                      <span className="text-[#FF2D55] text-xs font-black">✓</span>
                    )}
                  </button>
                );
              })}
            </div>

            {/* Calendar — shown only for custom range */}
            {date === 'custom' && (
              <div className="pt-1 border-t border-black/[0.06] dark:border-white/[0.06]">
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
        <div className="flex items-center gap-3 px-8 py-5 border-t border-black/[0.06] dark:border-white/[0.06] rounded-b-2xl bg-white dark:bg-[#0d0d0d]">
          <button
            type="button"
            onClick={handleReset}
            className="px-5 py-2.5 rounded-lg text-[12px] font-extrabold tracking-[1px] border border-black/10 dark:border-white/15 text-gray-500 dark:text-gray-400 hover:border-black/25 dark:hover:border-white/30 transition-colors"
          >
            RESET
          </button>
          <button
            type="button"
            onClick={handleApply}
            className="flex-1 bg-[#FF2D55] text-white font-extrabold text-[12px] tracking-[1.5px] py-2.5 rounded-lg hover:bg-[#e0263f] transition-colors"
          >
            APPLY FILTERS
          </button>
        </div>
      </div>
    </div>
  );
}