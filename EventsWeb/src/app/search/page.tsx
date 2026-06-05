'use client';

import { useState, useCallback, useRef } from 'react';
import { useRouter } from 'next/navigation';
import { Search, X, Star, Megaphone, ChevronRight } from 'lucide-react';
import { searchEvents, getSuggestionEvents, type Event } from '@/lib/api';
import { useEventStore } from '@/store/eventsStore';
import { useAppTheme } from '@/themes/colors';

const ACCENT = '#ff3c6e';

const SUGGESTIONS = [
  {
    key: 'popular'        as const,
    label: 'Popular Events',
    Icon: Star,
    iconColor: '#3b82f6',
    bgColor: 'rgba(59,130,246,0.12)',
  },
  {
    key: 'just_announced' as const,
    label: 'Just Announced',
    Icon: Megaphone,
    iconColor: '#a855f7',
    bgColor: 'rgba(168,85,247,0.12)',
  },
];

const fmt      = (iso: string) =>
  new Date(iso).toLocaleDateString('en-ZA', { day: 'numeric', month: 'short', year: 'numeric' });
const fmtPrice = (p: number) =>
  p === 0 ? 'Free' : `R${Math.round(p).toLocaleString('en-ZA')}`;

// ── Skeleton ──────────────────────────────────────────────────────────────────
function SkeletonResult() {
  return (
    <div className="flex items-center gap-3.5 px-4 py-3.5 animate-pulse border-b border-white/5">
      <div className="w-20 h-20 rounded-[10px] shrink-0 bg-white/10" />
      <div className="flex-1 space-y-2">
        <div className="h-2 w-16 rounded bg-white/10" />
        <div className="h-3.5 w-48 rounded bg-white/10" />
        <div className="h-2 w-32 rounded bg-white/10" />
      </div>
      <div className="h-3.5 w-10 rounded bg-white/10 shrink-0" />
    </div>
  );
}

export default function SearchPage() {
  const router = useRouter();
  const { colors } = useAppTheme();
  const setSelectedEvent = useEventStore((s) => s.setSelectedEvent);

  const [query,   setQuery]   = useState('');
  const [results, setResults] = useState<Event[]>([]);
  const [loading, setLoading] = useState(false);
  const [mode,    setMode]    = useState<'idle' | 'results'>('idle');
  const [activeSuggestion, setActiveSuggestion] = useState<string | null>(null);

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const runSearch = useCallback(async (q: string) => {
    if (!q.trim()) { setMode('idle'); setResults([]); return; }
    setLoading(true);
    setMode('results');
    const data = await searchEvents(q);
    setResults(data);
    setLoading(false);
  }, []);

  const handleChange = (val: string) => {
    setQuery(val);
    setActiveSuggestion(null);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => runSearch(val), 350);
  };

  const handleSuggestion = useCallback(async (key: 'popular' | 'just_announced', label: string) => {
    setActiveSuggestion(key);
    setQuery('');
    setLoading(true);
    setMode('results');
    const data = await getSuggestionEvents(key);
    setResults(data);
    setLoading(false);
  }, []);

  const handleClear = () => {
    setQuery('');
    setMode('idle');
    setResults([]);
    setActiveSuggestion(null);
  };

  const handleEventPress = (event: Event) => {
    setSelectedEvent(event);
    router.push(`/event/${event.id}`);
  };

  return (
    <div style={{ backgroundColor: colors.background, minHeight: '100vh' }}>

      {/* ── Search bar ─────────────────────────────────────────────────────── */}
      <div
        className="sticky top-0 md:top-16 z-40 px-4 py-3"
        style={{ backgroundColor: colors.headerBg, borderBottom: `1px solid ${colors.dividerColor}` }}
      >
        <div
          className="flex items-center gap-3 px-4 py-2.5 rounded-[14px]"
          style={{ backgroundColor: colors.sectionCardBg }}
        >
          <Search size={18} color={ACCENT} />
          <input
            value={query}
            onChange={(e) => handleChange(e.target.value)}
            placeholder="Search events, artists, venues..."
            className="flex-1 bg-transparent outline-none text-sm font-medium placeholder:opacity-40"
            style={{ color: colors.text }}
            autoFocus
          />
          {(query.length > 0 || mode === 'results') && (
            <button onClick={handleClear} className="p-0.5">
              <X size={16} color={colors.subText} />
            </button>
          )}
        </div>

        {/* Active suggestion label */}
        {activeSuggestion && mode === 'results' && !query && (
          <p className="mt-2 text-[11px] font-black tracking-[1.5px] px-1" style={{ color: colors.subText }}>
            {SUGGESTIONS.find((s) => s.key === activeSuggestion)?.label.toUpperCase()}
          </p>
        )}
      </div>

      {/* ── Idle state — suggestions ────────────────────────────────────────── */}
      {mode === 'idle' && (
        <div className="pt-6">
          {/* Browse heading */}
          <p
            className="px-5 text-[11px] font-black tracking-[1.5px] mb-3"
            style={{ color: colors.subText }}
          >
            BROWSE
          </p>

          {SUGGESTIONS.map(({ key, label, Icon, iconColor, bgColor }) => (
            <button
              key={key}
              onClick={() => handleSuggestion(key, label)}
              className="flex items-center w-full px-5 py-4 gap-3.5 text-left transition-opacity hover:opacity-75"
              style={{ borderBottom: `1px solid ${colors.dividerColor}` }}
            >
              <div
                className="w-9 h-9 rounded-[10px] flex items-center justify-center shrink-0"
                style={{ backgroundColor: bgColor }}
              >
                <Icon size={18} color={iconColor} />
              </div>
              <span className="flex-1 text-base font-medium" style={{ color: colors.text }}>{label}</span>
              <ChevronRight size={18} color={colors.iconColor} />
            </button>
          ))}

          {/* Empty-state hint */}
          <div className="flex flex-col items-center justify-center pt-20 pb-10 px-8 text-center">
            <Search size={52} color={colors.iconColor} strokeWidth={1.2} />
            <p className="mt-5 text-[22px] font-black tracking-wide" style={{ color: colors.text }}>
              FIND YOUR NEXT EVENT
            </p>
            <p className="mt-2 text-sm leading-relaxed" style={{ color: colors.subText }}>
              Search by name, artist, venue, or city
            </p>
          </div>
        </div>
      )}

      {/* ── Loading skeletons ───────────────────────────────────────────────── */}
      {loading && (
        <div>
          {[0, 1, 2, 3, 4, 5].map((i) => <SkeletonResult key={i} />)}
        </div>
      )}

      {/* ── Results ─────────────────────────────────────────────────────────── */}
      {mode === 'results' && !loading && (
        <div>
          {results.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-24 px-8 text-center">
              <Search size={48} color={colors.iconColor} strokeWidth={1.2} />
              <p className="mt-4 text-lg font-black tracking-wide" style={{ color: colors.text }}>
                NO RESULTS
              </p>
              <p className="mt-1 text-sm" style={{ color: colors.subText }}>
                Try a different search term or browse suggestions above
              </p>
            </div>
          ) : (
            <>
              <p className="px-5 pt-4 pb-2 text-[11px] font-black tracking-[1.5px]" style={{ color: colors.subText }}>
                {results.length} RESULT{results.length !== 1 ? 'S' : ''}
              </p>
              {results.map((item) => (
                <button
                  key={item.id}
                  onClick={() => handleEventPress(item)}
                  className="flex items-center w-full px-4 py-3.5 gap-3.5 text-left transition-opacity hover:opacity-80"
                  style={{ borderBottom: `1px solid ${colors.dividerColor}` }}
                >
                  {/* Thumbnail */}
                  <div
                    className="relative w-20 h-20 rounded-[10px] shrink-0 overflow-hidden"
                    style={{ backgroundColor: colors.sectionCardBg }}
                  >
                    {item.cover_image_url && (
                      <img
                        src={item.cover_image_url}
                        alt={item.title}
                        className="absolute inset-0 w-full h-full object-cover"
                      />
                    )}
                  </div>

                  {/* Info */}
                  <div className="flex-1 min-w-0">
                    <p
                      className="text-[9px] font-black tracking-[1.4px] mb-0.5 uppercase"
                      style={{ color: ACCENT }}
                    >
                      {item.category}
                    </p>
                    <p
                      className="text-sm font-bold leading-snug mb-1 line-clamp-2"
                      style={{ color: colors.text }}
                    >
                      {item.title}
                    </p>
                    <p className="text-[11px] truncate mb-0.5" style={{ color: colors.subText }}>
                      📍 {item.venue_name}{item.city ? `, ${item.city}` : ''}
                    </p>
                    {item.start_time && (
                      <p className="text-[11px]" style={{ color: colors.subText }}>
                        {fmt(item.start_time)}
                      </p>
                    )}
                  </div>

                  {/* Price */}
                  <span className="text-sm font-black shrink-0" style={{ color: colors.text }}>
                    {fmtPrice(item.price)}
                  </span>
                </button>
              ))}
            </>
          )}
        </div>
      )}
    </div>
  );
}