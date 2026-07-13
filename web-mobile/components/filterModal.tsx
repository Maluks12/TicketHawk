'use client';

import { useState, useEffect, useRef } from 'react';

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

export default function FilterModal({ visible, onClose, onApply, initialParams }: FilterModalProps) {
  const [city,       setCity]       = useState(initialParams?.city ?? '');
  const [province,   setProvince]   = useState(initialParams?.province ?? '');
  const [date,       setDate]       = useState<DatePreset>(initialParams?.date ?? 'any');
  const [dateFrom,   setDateFrom]   = useState(initialParams?.date_from ?? '');
  const [dateTo,     setDateTo]     = useState(initialParams?.date_to ?? '');
  const [nearbyMode, setNearbyMode] = useState(false);
  const [radiusKm,   setRadiusKm]   = useState(initialParams?.radius_km ?? 25);
  const [gpsError,   setGpsError]   = useState('');
  const latRef = useRef<number | undefined>(initialParams?.lat);
  const lngRef = useRef<number | undefined>(initialParams?.lng);

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
    if (!navigator.geolocation) { setGpsError('Geolocation is not supported by your browser.'); return; }
    navigator.geolocation.getCurrentPosition(
      (pos) => { latRef.current = pos.coords.latitude; lngRef.current = pos.coords.longitude; setNearbyMode(true); },
      () => setGpsError('Location permission denied. Please allow location access and try again.'),
    );
  };

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/60 backdrop-blur-sm">
      <div className="w-full max-w-lg bg-white dark:bg-black rounded-t-2xl p-6 pb-10 space-y-5">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-black tracking-wide">FILTER</h2>
          <button onClick={onClose} className="text-xl font-bold">✕</button>
        </div>

        {/* ── Nearby toggle ────────────────────────────────────────── */}
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs font-bold tracking-widest text-gray-400">NEARBY</p>
            <p className="text-[11px] text-gray-400 mt-0.5">Use my current location</p>
          </div>
          <button
            onClick={handleNearbyToggle}
            className={`w-12 h-6 rounded-full transition-colors relative ${nearbyMode ? 'bg-[#FF2D55]' : 'bg-gray-200 dark:bg-gray-700'}`}
          >
            <span className={`absolute top-0.5 w-5 h-5 rounded-full bg-white shadow transition-transform ${nearbyMode ? 'translate-x-6' : 'translate-x-0.5'}`} />
          </button>
        </div>
        {gpsError && <p className="text-xs text-red-500 -mt-2">{gpsError}</p>}

        {nearbyMode && (
          <div className="space-y-2">
            <label className="text-xs font-bold tracking-widest text-gray-400">RADIUS</label>
            <div className="flex items-center gap-3">
              <input type="range" min={5} max={100} step={5} value={radiusKm} onChange={(e) => setRadiusKm(Number(e.target.value))} className="flex-1" />
              <span className="text-sm font-bold w-16 text-right dark:text-white">{radiusKm} km</span>
            </div>
          </div>
        )}

        {!nearbyMode && (
          <>
            {/* City */}
            <div className="space-y-2">
              <label className="text-xs font-bold tracking-widest text-gray-400">CITY</label>
              <input
                className="w-full border border-gray-200 dark:border-gray-700 dark:bg-transparent dark:text-white rounded-md px-4 py-2 text-sm"
                placeholder="e.g. Cape Town"
                value={city}
                onChange={(e) => setCity(e.target.value)}
              />
            </div>

            {/* Province */}
            <div className="space-y-2">
              <label className="text-xs font-bold tracking-widest text-gray-400">PROVINCE</label>
              <div className="flex flex-wrap gap-2">
                {['Any', 'Gauteng', 'Western Cape', 'Eastern Cape', 'KwaZulu-Natal', 'Limpopo', 'Mpumalanga', 'North West', 'Free State', 'Northern Cape'].map((p) => {
                  const val = p === 'Any' ? '' : p;
                  const active = province === val;
                  return (
                    <button
                      key={p}
                      onClick={() => setProvince(val)}
                      className={`px-3 py-1.5 rounded-full border text-xs font-bold tracking-wide transition-colors ${
                        active ? 'bg-black dark:bg-white text-white dark:text-black border-black dark:border-white' : 'border-gray-200 dark:border-gray-600 text-gray-500 dark:text-gray-400'
                      }`}
                    >
                      {p}
                    </button>
                  );
                })}
              </div>
            </div>
          </>
        )}

        <div className="space-y-2">
          <label className="text-xs font-bold tracking-widest text-gray-400">DATE</label>
          <div className="flex flex-wrap gap-2">
            {(['any', 'today', 'tomorrow', 'weekend', 'custom'] as DatePreset[]).map((d) => (
              <button
                key={d}
                onClick={() => setDate(d)}
                className={`px-4 py-1.5 rounded-full border text-xs font-bold tracking-wider ${
                  date === d ? 'bg-black text-white border-black' : 'border-gray-200 text-gray-500'
                }`}
              >
                {d.toUpperCase()}
              </button>
            ))}
          </div>
          {date === 'custom' && (
            <div className="flex gap-2 mt-2">
              <input type="date" className="flex-1 border rounded-md px-3 py-1.5 text-sm" value={dateFrom} onChange={(e) => setDateFrom(e.target.value)} />
              <input type="date" className="flex-1 border rounded-md px-3 py-1.5 text-sm" value={dateTo}   onChange={(e) => setDateTo(e.target.value)} />
            </div>
          )}
        </div>

        <button
          onClick={handleApply}
          className="w-full bg-[#FF2D55] text-white font-extrabold text-xs tracking-[1.5px] py-3 rounded-md"
        >
          APPLY FILTERS
        </button>
      </div>
    </div>
  );
}