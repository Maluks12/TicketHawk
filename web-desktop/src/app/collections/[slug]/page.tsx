'use client';

// ─────────────────────────────────────────────────────────────────────────────
// Event collection page — /collections/[slug]
//
// Where a collection card on the home feed leads: the collection's cover,
// title and description, the organiser, and every upcoming public date as a
// list ordered by date — the shape of a tour poster's calendar, which is what
// an organiser grouping a tour is reproducing.
//
// Each date opens that event's own page, where tickets are bought: a
// collection is navigation, never a checkout of its own.
//
// Kept identical between web-desktop and web-mobile; the layout adapts at the
// md breakpoint rather than being forked.
// ─────────────────────────────────────────────────────────────────────────────
import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
// Only icons already used elsewhere in this codebase's lucide-react (^0.400.0).
import { BadgeCheck, ChevronLeft, ChevronRight, Layers } from 'lucide-react';
import { useEventStore } from '@/store/eventsStore';
import { useAppTheme } from '@/themes/colors';
import { getEventCollection, type Event, type PublicEventCollection } from '@/lib/api';
import { collectionDateRange } from '@/lib/eventCollections';

function formatPrice(price: number) {
  if (!price || price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}
const day = (iso: string) => new Date(iso).toLocaleDateString('en-ZA', { day: 'numeric' });
const month = (iso: string) => new Date(iso).toLocaleDateString('en-ZA', { month: 'short' }).toUpperCase();
const weekday = (iso: string) => new Date(iso).toLocaleDateString('en-ZA', { weekday: 'short' });
const time = (iso: string) => new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });

export default function CollectionPage() {
  const params = useParams<{ slug: string }>();
  const router = useRouter();
  const { colors, isDark } = useAppTheme();
  const { setSelectedEvent } = useEventStore();
  const slug = params.slug ? decodeURIComponent(params.slug) : '';

  const [collection, setCollection] = useState<PublicEventCollection | null>(null);
  const [loading, setLoading] = useState(true);
  const [notFound, setNotFound] = useState(false);
  const [city, setCity] = useState<string>('all');

  useEffect(() => { window.scrollTo({ top: 0, behavior: 'instant' }); }, []);

  useEffect(() => {
    if (!slug) return;
    let cancelled = false;
    (async () => {
      try {
        const { data } = await getEventCollection(slug);
        if (!cancelled) setCollection(data);
      } catch {
        if (!cancelled) setNotFound(true);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [slug]);

  const events = useMemo(() => collection?.events ?? [], [collection]);

  // Cities in date order. The filter only appears when there are enough to be
  // worth filtering — two cities are readable at a glance.
  const cities = useMemo(() => {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const e of events) {
      const c = (e.city ?? '').trim();
      if (c && !seen.has(c.toLowerCase())) { seen.add(c.toLowerCase()); out.push(c); }
    }
    return out;
  }, [events]);

  const shown = city === 'all' ? events : events.filter(e => (e.city ?? '').trim() === city);
  const prices = events.map(e => e.price ?? 0);
  const minPrice = prices.length ? Math.min(...prices) : 0;
  const maxPrice = prices.length ? Math.max(...prices) : 0;
  // "R80", "Free", or a range: "Free – R150", "R80 – R150".
  const priceLabel = minPrice === maxPrice
    ? formatPrice(minPrice)
    : `${formatPrice(minPrice)} – ${formatPrice(maxPrice)}`;

  const border = isDark ? 'rgba(255,255,255,0.10)' : 'rgba(0,0,0,0.10)';
  const subtle = isDark ? '#1c1c1c' : '#f0f0f4';
  const bone = isDark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.06)';

  const openEvent = (e: Event) => { setSelectedEvent(e); router.push(`/events/${e.id}`); };

  if (loading) {
    return (
      <div className="min-h-screen" style={{ backgroundColor: colors.background }}>
        <div className="w-full animate-pulse" style={{ aspectRatio: '16 / 7', maxHeight: 420, backgroundColor: bone }} />
        <div className="mx-auto px-5 md:px-8 py-8" style={{ maxWidth: 920 }}>
          <div className="rounded animate-pulse" style={{ width: 280, height: 28, backgroundColor: bone }} />
          <div className="rounded animate-pulse mt-3" style={{ width: 200, height: 14, backgroundColor: bone }} />
          {[0, 1, 2].map(i => (
            <div key={i} className="rounded-xl animate-pulse mt-4" style={{ height: 76, backgroundColor: bone }} />
          ))}
        </div>
      </div>
    );
  }

  if (notFound || !collection) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center gap-4 px-6 text-center" style={{ backgroundColor: colors.background }}>
        <h2 style={{ color: colors.text, margin: 0, fontSize: 17, fontWeight: 600 }}>Collection not found</h2>
        <button
          onClick={() => router.push('/')}
          className="rounded-lg px-5 py-2.5 text-[12px] font-bold tracking-wider uppercase"
          style={{ backgroundColor: '#ff2d55', color: '#fff' }}
        >
          Browse events
        </button>
      </div>
    );
  }

  const cover = collection.cover_image_url || events[0]?.cover_image_url || null;
  const range = events.length
    ? collectionDateRange({ first_start: events[0].start_time, last_start: events[events.length - 1].start_time })
    : '';
  const org = collection.organizer;

  return (
    <div className="min-h-screen" style={{ backgroundColor: colors.background }}>
      {/* Cover */}
      <div className="relative w-full overflow-hidden" style={{ aspectRatio: '16 / 7', maxHeight: 440, minHeight: 200, backgroundColor: subtle }}>
        {cover && <img src={cover} alt="" className="absolute inset-0 w-full h-full object-cover" />}
        <div className="absolute inset-0" style={{ background: `linear-gradient(to bottom, rgba(0,0,0,0.25) 0%, transparent 35%, ${colors.background} 100%)` }} />
        <button
          onClick={() => (window.history.length > 1 ? router.back() : router.push('/'))}
          aria-label="Back"
          className="absolute top-4 left-4 w-10 h-10 rounded-full flex items-center justify-center"
          style={{ backgroundColor: 'rgba(0,0,0,0.55)' }}
        >
          <ChevronLeft size={22} color="#fff" />
        </button>
      </div>

      <div className="mx-auto px-5 md:px-8" style={{ maxWidth: 920, marginTop: -36, position: 'relative' }}>
        {/* Identity */}
        <div
          className="inline-flex items-center gap-1.5 rounded-[6px] px-2.5"
          style={{ height: 24, backgroundColor: '#ff2d55' }}
        >
          <Layers size={13} color="#fff" strokeWidth={2.5} />
          <span className="text-white text-[10px] font-extrabold tracking-[1.2px]">
            COLLECTION · {events.length} {events.length === 1 ? 'DATE' : 'DATES'}
          </span>
        </div>

        <h1 className="font-black mt-3" style={{ color: colors.text, fontSize: 'clamp(24px, 4vw, 34px)', letterSpacing: '-0.6px', lineHeight: 1.1, margin: '12px 0 0' }}>
          {collection.title}
        </h1>

        {events.length > 0 && (
          <p className="mt-2 text-[14px]" style={{ color: colors.subText }}>
            {range}
            {cities.length > 0 && ` · ${cities.length} ${cities.length === 1 ? 'city' : 'cities'}`}
            {` · ${priceLabel}`}
          </p>
        )}

        {org && (
          <Link
            href={`/organiser/${encodeURIComponent(org.slug || org.id)}`}
            className="mt-4 inline-flex items-center gap-2.5 rounded-xl px-3 py-2"
            style={{ backgroundColor: subtle, textDecoration: 'none' }}
          >
            {org.logo_url && (
              <img src={org.logo_url} alt="" className="rounded-lg object-cover" style={{ width: 30, height: 30 }} />
            )}
            <span className="text-[13px]" style={{ color: colors.subText }}>
              By <strong style={{ color: colors.text }}>{org.business_name}</strong>
            </span>
            {org.verified && <BadgeCheck size={15} color="#2f7cf6" aria-label="Verified organiser" />}
            <ChevronRight size={15} color={colors.subText} />
          </Link>
        )}

        {collection.description && (
          <p className="mt-5 text-[14.5px] leading-[24px] whitespace-pre-line" style={{ color: colors.subText, maxWidth: 680 }}>
            {collection.description}
          </p>
        )}

        {/* Dates */}
        <div className="mt-8 pb-3 flex items-end justify-between gap-4 flex-wrap" style={{ borderBottom: `1px solid ${border}` }}>
          <h2 className="text-[13px] font-extrabold tracking-[1.4px] uppercase" style={{ color: colors.text, margin: 0 }}>
            Dates
          </h2>
          {cities.length >= 3 && (
            <div className="flex gap-2 overflow-x-auto scrollbar-hide" style={{ maxWidth: '100%' }}>
              {['all', ...cities].map(c => {
                const on = city === c;
                return (
                  <button
                    key={c}
                    onClick={() => setCity(c)}
                    className="shrink-0 rounded-full px-3.5 h-8 text-[12px] font-bold whitespace-nowrap"
                    style={{
                      backgroundColor: on ? '#ff2d55' : 'transparent',
                      color: on ? '#fff' : colors.text,
                      border: on ? 'none' : `1px solid ${border}`,
                    }}
                  >
                    {c === 'all' ? 'All cities' : c}
                  </button>
                );
              })}
            </div>
          )}
        </div>

        {events.length === 0 ? (
          <div className="text-center py-16">
            <p className="text-[15px] font-semibold" style={{ color: colors.text }}>All dates have passed</p>
            <p className="text-[13px] mt-1.5" style={{ color: colors.subText }}>
              {org ? 'Follow the organiser to hear about the next one.' : 'Check back for new dates.'}
            </p>
          </div>
        ) : (
          <div className="pb-24">
            {shown.map(e => (
              <button
                key={e.id}
                onClick={() => openEvent(e)}
                className="w-full text-left flex items-center gap-4 py-4"
                style={{ borderBottom: `1px solid ${border}` }}
              >
                {/* Date tile */}
                <div
                  className="shrink-0 flex flex-col items-center justify-center rounded-xl"
                  style={{ width: 58, height: 62, backgroundColor: subtle }}
                >
                  <span className="text-[10px] font-extrabold tracking-[1px]" style={{ color: '#ff2d55' }}>{month(e.start_time)}</span>
                  <span className="text-[22px] font-black leading-none mt-0.5" style={{ color: colors.text }}>{day(e.start_time)}</span>
                </div>

                {/* What + where */}
                <div className="min-w-0 flex-1">
                  <p className="text-[15px] font-bold truncate" style={{ color: colors.text, margin: 0 }}>
                    {e.city || e.title}
                  </p>
                  <p className="text-[13px] truncate mt-0.5" style={{ color: colors.subText, margin: 0 }}>
                    {/* The date's own title only when it says something the
                        collection title does not (a named night, a partner). */}
                    {e.city && e.title && e.title.trim() !== collection.title.trim()
                      ? `${e.title} · ${e.venue_name}`
                      : e.venue_name}
                  </p>
                  <p className="text-[12px] mt-0.5" style={{ color: colors.subText, margin: 0 }}>
                    {weekday(e.start_time)} · {time(e.start_time)}
                    {e.is_sold_out ? ' · Sold out' : ''}
                  </p>
                </div>

                {/* Price + go */}
                <div className="shrink-0 flex items-center gap-2">
                  <span
                    className="rounded-md px-2.5 py-1 text-[12px] font-black"
                    style={{
                      backgroundColor: e.price === 0 ? '#22c55e' : (isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)'),
                      color: e.price === 0 ? '#fff' : colors.text,
                    }}
                  >
                    {formatPrice(e.price)}
                  </span>
                  <ChevronRight size={18} color={colors.subText} />
                </div>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}