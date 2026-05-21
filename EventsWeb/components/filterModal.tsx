'use client';

import { useState } from 'react';

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
  const [city,     setCity]     = useState(initialParams?.city ?? '');
  const [province, setProvince] = useState(initialParams?.province ?? '');
  const [date,     setDate]     = useState<DatePreset>(initialParams?.date ?? 'any');
  const [dateFrom, setDateFrom] = useState(initialParams?.date_from ?? '');
  const [dateTo,   setDateTo]   = useState(initialParams?.date_to ?? '');

  if (!visible) return null;

  const handleApply = () => {
    onApply({
      city:      city.trim() || undefined,
      province:  province.trim() || undefined,
      date,
      date_from: date === 'custom' ? dateFrom : undefined,
      date_to:   date === 'custom' ? dateTo   : undefined,
    });
  };

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/60 backdrop-blur-sm">
      <div className="w-full max-w-lg bg-white dark:bg-black rounded-t-2xl p-6 pb-10 space-y-5">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-black tracking-wide">FILTER</h2>
          <button onClick={onClose} className="text-xl font-bold">✕</button>
        </div>

        <div className="space-y-2">
          <label className="text-xs font-bold tracking-widest text-gray-400">CITY</label>
          <input
            className="w-full border border-gray-200 rounded-md px-4 py-2 text-sm"
            placeholder="e.g. Cape Town"
            value={city}
            onChange={(e) => setCity(e.target.value)}
          />
        </div>

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