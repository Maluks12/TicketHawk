'use client';

import { useState, useEffect, useRef } from 'react';
import { MapPin, Calendar, X, Navigation } from 'lucide-react';

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
  if (params.city)          qs.set('city',      params.city);
  if (params.province)      qs.set('province',  params.province);
  if (params.lat  != null)  qs.set('lat',       String(params.lat));
  if (params.lng  != null)  qs.set('lng',       String(params.lng));
  if (params.radius_km)     qs.set('radius_km', String(params.radius_km));
  if (params.date && params.date !== 'any') qs.set('date', params.date);
  if (params.date === 'custom' && params.date_from) qs.set('date_from', params.date_from);
  if (params.date === 'custom' && params.date_to)   qs.set('date_to',   params.date_to);
  return qs.toString();
}

const PROVINCES = [
  'Any', 'Gauteng', 'Western Cape', 'Eastern Cape',
  'KwaZulu-Natal', 'Limpopo', 'Mpumalanga',
  'North West', 'Free State', 'Northern Cape',
];

const DATE_PRESETS: { value: DatePreset; label: string }[] = [
  { value: 'any',      label: 'Any time'    },
  { value: 'today',    label: 'Today'       },
  { value: 'tomorrow', label: 'Tomorrow'    },
  { value: 'weekend',  label: 'This Weekend'},
  { value: 'custom',   label: 'Custom range'},
];

export default function FilterModal({ visible, onClose, onApply, initialParams }: FilterModalProps) {
  const [city,       setCity]       = useState(initialParams?.city ?? '');
  const [province,   setProvince]   = useState(initialParams?.province ?? '');
  const [date,       setDate]       = useState<DatePreset>(initialParams?.date ?? 'any');
  const [dateFrom,   setDateFrom]   = useState(initialParams?.date_from ?? '');
  const [dateTo,     setDateTo]     = useState(initialParams?.date_to ?? '');
  const [nearbyMode, setNearbyMode] = useState(false);
  const [radiusKm,   setRadiusKm]   = useState(initialParams?.radius_km ?? 25);
  const [gpsError,   setGpsError]   = useState('');
  const [gpsLoading, setGpsLoading] = useState(false);
  const latRef = useRef<number | undefined>(initialParams?.lat);
  const lngRef = useRef<number | undefined>(initialParams?.lng);

  // Close on Escape key
  useEffect(() => {
    if (!visible) return;
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
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
      radius_km: nearbyMode ? radiusKm      : undefined,
      date,
      date_from: date === 'custom' ? dateFrom : undefined,
      date_to:   date === 'custom' ? dateTo   : undefined,
    });
  };

  const handleNearbyToggle = () => {
    if (nearbyMode) { setNearbyMode(false); return; }
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
        setGpsError('Location permission denied. Please allow location access and try again.');
        setGpsLoading(false);
      },
    );
  };

  const handleReset = () => {
    setCity('');
    setProvince('');
    setDate('any');
    setDateFrom('');
    setDateTo('');
    setNearbyMode(false);
    setRadiusKm(25);
    setGpsError('');
    latRef.current = undefined;
    lngRef.current = undefined;
  };

  return (
    /* ── Backdrop ─────────────────────────────────────────────────────────── */
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
      onClick={onClose}
    >
      {/* ── Dialog ──────────────────────────────────────────────────────────── */}
      <div
        className="w-full max-w-2xl bg-white dark:bg-[#0d0d0d] rounded-2xl shadow-2xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header bar */}
        <div className="flex items-center justify-between px-8 pt-7 pb-5 border-b border-black/[0.06] dark:border-white/[0.06]">
          <h2 className="text-base font-black tracking-[1.5px] dark:text-white">FILTER EVENTS</h2>
          <button
            onClick={onClose}
            className="w-8 h-8 flex items-center justify-center rounded-full bg-black/[0.05] dark:bg-white/[0.08] hover:bg-black/10 dark:hover:bg-white/15 transition-colors"
            aria-label="Close"
          >
            <X size={15} className="text-black dark:text-white" />
          </button>
        </div>

        {/* Body — two-column grid */}
        <div className="grid grid-cols-2 gap-0 divide-x divide-black/[0.06] dark:divide-white/[0.06]">

          {/* ── LEFT: Location ─────────────────────────────────────────────── */}
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
                onClick={handleNearbyToggle}
                disabled={gpsLoading}
                className={`w-12 h-6 rounded-full transition-colors relative shrink-0 ${
                  nearbyMode ? 'bg-[#FF2D55]' : 'bg-gray-200 dark:bg-gray-700'
                } ${gpsLoading ? 'opacity-50 cursor-wait' : ''}`}
              >
                <span
                  className={`absolute top-0.5 w-5 h-5 rounded-full bg-white shadow transition-transform ${
                    nearbyMode ? 'translate-x-6' : 'translate-x-0.5'
                  }`}
                />
              </button>
            </div>

            {gpsError && (
              <p className="text-[11px] text-red-500 -mt-2 leading-snug">{gpsError}</p>
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
                {/* City */}
                <div className="space-y-1.5">
                  <label className="text-[10px] font-black tracking-[1.5px] text-gray-400">CITY</label>
                  <input
                    className="w-full border border-black/10 dark:border-white/10 dark:bg-transparent dark:text-white rounded-lg px-4 py-2.5 text-sm placeholder:text-gray-300 dark:placeholder:text-gray-600 focus:outline-none focus:ring-2 focus:ring-[#FF2D55]/30"
                    placeholder="e.g. Cape Town"
                    value={city}
                    onChange={(e) => setCity(e.target.value)}
                  />
                </div>

                {/* Province */}
                <div className="space-y-2">
                  <label className="text-[10px] font-black tracking-[1.5px] text-gray-400">PROVINCE</label>
                  <div className="flex flex-wrap gap-1.5">
                    {PROVINCES.map((p) => {
                      const val    = p === 'Any' ? '' : p;
                      const active = province === val;
                      return (
                        <button
                          key={p}
                          onClick={() => setProvince(val)}
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

          {/* ── RIGHT: Date ────────────────────────────────────────────────── */}
          <div className="px-8 py-6 space-y-5">
            <div className="flex items-center gap-2">
              <Calendar size={13} className="text-[#FF2D55]" />
              <span className="text-[10px] font-black tracking-[1.8px] text-gray-400">DATE</span>
            </div>

            <div className="space-y-2 pt-1">
              {DATE_PRESETS.map(({ value, label }) => {
                const active = date === value;
                return (
                  <button
                    key={value}
                    onClick={() => setDate(value)}
                    className={`w-full flex items-center justify-between px-4 py-3 rounded-xl border text-sm font-semibold transition-all ${
                      active
                        ? 'bg-black dark:bg-white text-white dark:text-black border-black dark:border-white'
                        : 'border-black/08 dark:border-white/08 text-gray-700 dark:text-gray-300 hover:border-black/20 dark:hover:border-white/20 hover:bg-black/[0.02] dark:hover:bg-white/[0.02]'
                    }`}
                  >
                    <span>{label}</span>
                    {active && (
                      <span className={`text-xs font-black tracking-wider ${active ? 'text-[#FF2D55]' : ''}`}>
                        ✓
                      </span>
                    )}
                  </button>
                );
              })}
            </div>

            {date === 'custom' && (
              <div className="space-y-2 pt-1">
                <label className="text-[10px] font-black tracking-[1.5px] text-gray-400">DATE RANGE</label>
                <div className="flex gap-2">
                  <div className="flex-1 space-y-1">
                    <span className="text-[10px] text-gray-400">From</span>
                    <input
                      type="date"
                      className="w-full border border-black/10 dark:border-white/10 rounded-lg px-3 py-2 text-sm dark:bg-transparent dark:text-white focus:outline-none focus:ring-2 focus:ring-[#FF2D55]/30"
                      value={dateFrom}
                      onChange={(e) => setDateFrom(e.target.value)}
                    />
                  </div>
                  <div className="flex-1 space-y-1">
                    <span className="text-[10px] text-gray-400">To</span>
                    <input
                      type="date"
                      className="w-full border border-black/10 dark:border-white/10 rounded-lg px-3 py-2 text-sm dark:bg-transparent dark:text-white focus:outline-none focus:ring-2 focus:ring-[#FF2D55]/30"
                      value={dateTo}
                      onChange={(e) => setDateTo(e.target.value)}
                    />
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* ── Footer actions ───────────────────────────────────────────────── */}
        <div className="flex items-center gap-3 px-8 py-5 border-t border-black/[0.06] dark:border-white/[0.06]">
          <button
            onClick={handleReset}
            className="px-5 py-2.5 rounded-lg text-[12px] font-extrabold tracking-[1px] border border-black/10 dark:border-white/15 text-gray-500 dark:text-gray-400 hover:border-black/25 dark:hover:border-white/30 transition-colors"
          >
            RESET
          </button>
          <button
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