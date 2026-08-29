'use client';

import { useState, useEffect, useCallback } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { useEventStore } from '@/store/eventsStore';
import { useAppTheme } from '@/themes/colors';
import {
  getOrganizer, getOrganizerEvents, toggleOrganizerFollow,
  type Event, type PublicOrganizer, type OrganizerLink,
} from '@/lib/api';

/**
 * Public organiser profile — mobile-web variant of the desktop page.
 *
 * The mobile app has had this screen for a while; the web never did, so an
 * organiser's name on an event page was dead text and a web visitor had no way
 * to follow one at all. Every endpoint used here already existed.
 *
 * The socials live on THIS page and not on the event page, matching the app: an
 * event page is about one event, and an organiser's Instagram belongs to the
 * organiser.
 */

function formatPrice(price: number) {
  if (!price || price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}
function formatCardDate(iso?: string | null) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-ZA', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
}
function formatTime(iso?: string | null) {
  if (!iso) return '';
  return new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
}

const SOCIAL_LABEL: Record<string, string> = {
  instagram: 'Instagram', x: 'X', facebook: 'Facebook', tiktok: 'TikTok',
  youtube: 'YouTube', linkedin: 'LinkedIn', whatsapp: 'WhatsApp',
  spotify: 'Spotify', website: 'Website',
};

// Same card as the category page, so an organiser's events look identical to
// the same events anywhere else on the site.
function GridCard({ item, onPress, colors }: { item: Event; onPress: (e: Event) => void; colors: any }) {
  const [hovered, setHovered] = useState(false);
  return (
    <button
      onClick={() => onPress(item)}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      className="text-left w-full"
    >
      <div className="relative w-full aspect-video rounded-xl overflow-hidden mb-2.5" style={{ backgroundColor: 'rgba(255,255,255,0.05)' }}>
        {item.cover_image_url ? (
          <img
            src={item.cover_image_url}
            alt={item.title}
            className="absolute inset-0 w-full h-full object-cover transition-transform duration-300"
            style={{ transform: hovered ? 'scale(1.06)' : 'scale(1)' }}
          />
        ) : (
          <div className="absolute inset-0" style={{ backgroundColor: 'rgba(255,255,255,0.05)' }} />
        )}
        <div
          className="absolute inset-0 transition-opacity duration-200"
          style={{ backgroundColor: 'rgba(0,0,0,0.28)', opacity: hovered ? 1 : 0 }}
        />
        <div
          className="absolute bottom-2 left-2 rounded-md px-2 py-0.5"
          style={{ backgroundColor: formatPrice(item.price) === 'Free' ? '#22c55e' : 'rgba(0,0,0,0.82)', backdropFilter: 'blur(4px)' }}
        >
          <span className="text-white text-[10px] font-black tracking-wide">{formatPrice(item.price)}</span>
        </div>
      </div>
      <p
        className="font-semibold text-[14px] leading-[20px] line-clamp-2 mb-1 transition-colors duration-150"
        style={{ color: hovered ? '#ff2d55' : colors.text }}
      >
        {item.title}
      </p>
      <p className="text-[12px] leading-[18px]" style={{ color: colors.subText }}>
        {formatCardDate(item.start_time)}{item.start_time ? ` · ${formatTime(item.start_time)}` : ''}
      </p>
      {item.venue_name && (
        <p className="text-[12px] leading-[18px] truncate" style={{ color: colors.subText }}>
          {item.venue_name}{item.city ? `, ${item.city}` : ''}
        </p>
      )}
    </button>
  );
}

export default function OrganiserPage() {
  const params = useParams<{ slug: string }>();
  const router = useRouter();
  const { colors, isDark } = useAppTheme();
  const { setSelectedEvent } = useEventStore();

  const slug = params.slug ? decodeURIComponent(params.slug) : '';

  const [organiser, setOrganiser] = useState<PublicOrganizer | null>(null);
  const [loading,   setLoading]   = useState(true);
  const [error,     setError]     = useState<string | null>(null);

  const [events,        setEvents]        = useState<Event[]>([]);
  const [scope,         setScope]         = useState<'upcoming' | 'past'>('upcoming');
  const [eventsLoading, setEventsLoading] = useState(false);

  const [following,  setFollowing]  = useState(false);
  const [followBusy, setFollowBusy] = useState(false);
  const [followNote, setFollowNote] = useState<string | null>(null);

  useEffect(() => { window.scrollTo({ top: 0, behavior: 'instant' }); }, []);

  useEffect(() => {
    if (!slug) return;
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const { data } = await getOrganizer(slug);
        if (cancelled) return;
        setOrganiser(data);
        // === true, not truthiness: the key is ABSENT for a logged-out visitor,
        // and undefined must not be read as "not following".
        setFollowing(data.is_following === true);
      } catch {
        if (!cancelled) setError('Organiser not found');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [slug]);

  const loadEvents = useCallback(async (sc: 'upcoming' | 'past') => {
    if (!slug) return;
    setEventsLoading(true);
    try {
      const { data } = await getOrganizerEvents(slug, sc);
      setEvents(data.data ?? []);
    } catch {
      setEvents([]);
    } finally {
      setEventsLoading(false);
    }
  }, [slug]);

  useEffect(() => { if (organiser) loadEvents(scope); }, [organiser, scope, loadEvents]);

  async function handleFollow() {
    if (followBusy || !organiser) return;
    // Optimistic, like the app: a follow toggle that waits on the network reads
    // as broken. Reverted on failure.
    const was = following;
    setFollowing(!was);
    setFollowBusy(true);
    try {
      const res = await toggleOrganizerFollow(slug);
      setFollowing(res.following);
    } catch (e: any) {
      setFollowing(was);
      setFollowNote(
        e?.response?.status === 401
          ? 'Log in to follow organisers and hear about new events first.'
          : 'Could not update follow. Try again.'
      );
      setTimeout(() => setFollowNote(null), 3200);
    } finally {
      setFollowBusy(false);
    }
  }

  const handlePress = (item: Event) => { setSelectedEvent(item); router.push(`/events/${item.id}`); };

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center" style={{ backgroundColor: colors.background }}>
        <div className="animate-pulse rounded-2xl" style={{ width: 96, height: 96, backgroundColor: 'rgba(255,255,255,0.06)' }} />
      </div>
    );
  }

  if (error || !organiser) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center gap-4" style={{ backgroundColor: colors.background }}>
        <h2 style={{ color: colors.text, margin: 0, fontSize: 17, fontWeight: 600 }}>Organiser not found</h2>
        <button
          onClick={() => router.back()}
          className="rounded-lg px-5 py-2.5 text-[12px] font-bold tracking-wider uppercase"
          style={{ backgroundColor: '#ff2d55', color: '#fff' }}
        >
          Go back
        </button>
      </div>
    );
  }

  const links: OrganizerLink[] = [...(organiser.links ?? [])].sort((a, b) => a.sort_order - b.sort_order);
  const border = isDark ? 'rgba(255,255,255,0.10)' : 'rgba(0,0,0,0.10)';
  const subtle = isDark ? '#1c1c1c' : '#f0f0f4';

  return (
    <div className="min-h-screen" style={{ backgroundColor: colors.background }}>
      {/* Cover */}
      <div className="relative w-full" style={{ height: 150, backgroundColor: subtle }}>
        {organiser.cover_image_url && (
          <img src={organiser.cover_image_url} alt="" className="absolute inset-0 w-full h-full object-cover" />
        )}
        <div
          className="absolute inset-0"
          style={{ background: `linear-gradient(to bottom, rgba(0,0,0,0.15), ${colors.background})` }}
        />
      </div>

      <div className="mx-auto px-5" style={{ maxWidth: 640 }}>
        {/* Identity — pulled up over the cover */}
        <div className="flex flex-col items-center text-center" style={{ marginTop: -44 }}>
          <div
            className="rounded-2xl overflow-hidden flex items-center justify-center"
            style={{ width: 88, height: 88, backgroundColor: colors.cardBg, border: `3px solid ${colors.background}` }}
          >
            {organiser.logo_url
              ? <img src={organiser.logo_url} alt="" className="w-full h-full object-cover" />
              : <span style={{ fontSize: 34, color: colors.subText }}>🏢</span>}
          </div>

          <div className="flex items-center gap-2 mt-4">
            <h1 className="font-black" style={{ color: colors.text, fontSize: 22, letterSpacing: '-0.3px', margin: 0 }}>
              {organiser.business_name}
            </h1>
            {organiser.verified && (
              <span title="Verified organiser" style={{ color: '#2f7cf6', fontSize: 20, lineHeight: 1 }}>✓</span>
            )}
          </div>

          <p className="mt-2 text-[13px]" style={{ color: colors.subText }}>
            {organiser.verified ? 'Verified organiser · ' : ''}
            {organiser.events_hosted} event{organiser.events_hosted === 1 ? '' : 's'} hosted
            {organiser.member_since ? ` · since ${organiser.member_since}` : ''}
          </p>

          {organiser.description && (
            <p className="mt-4 text-[14px] leading-[23px]" style={{ color: colors.subText, maxWidth: 560 }}>
              {organiser.description}
            </p>
          )}

          {/* Follower count sits inside the button so the number and the action
              that changes it are one target — same call the app makes. */}
          <button
            onClick={handleFollow}
            disabled={followBusy}
            className="mt-6 rounded-xl px-8 py-3 text-[12px] font-extrabold tracking-[1.4px] uppercase transition-colors duration-150"
            style={
              following
                ? { backgroundColor: '#2d1c3a', border: '1px solid #b57bee', color: '#b57bee' }
                : { backgroundColor: 'transparent', border: `1px solid ${border}`, color: colors.text }
            }
          >
            {following ? 'Following' : 'Follow'}
            {organiser.follower_count > 0 && (
              <span style={{ opacity: 0.65, marginLeft: 8 }}>{organiser.follower_count.toLocaleString('en-ZA')}</span>
            )}
          </button>

          {links.length > 0 && (
            <div className="flex flex-wrap items-center justify-center gap-2 mt-5">
              {links.map(l => (
                <a
                  key={l.platform}
                  href={l.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  aria-label={`${organiser.business_name} on ${SOCIAL_LABEL[l.platform] ?? l.platform}`}
                  className="rounded-lg px-3.5 py-2 text-[12px] font-semibold transition-colors duration-150"
                  style={{ backgroundColor: subtle, color: colors.text, textDecoration: 'none' }}
                >
                  {SOCIAL_LABEL[l.platform] ?? l.platform}
                </a>
              ))}
            </div>
          )}
        </div>

        {/* Upcoming / Past */}
        <div className="flex gap-6 mt-10" style={{ borderBottom: `1px solid ${border}` }}>
          {(['upcoming', 'past'] as const).map(sc => (
            <button
              key={sc}
              onClick={() => setScope(sc)}
              className="pb-3 text-[12px] font-extrabold tracking-[1.4px] uppercase transition-colors duration-150"
              style={{
                color: scope === sc ? colors.text : colors.subText,
                borderBottom: `2px solid ${scope === sc ? '#ff2d55' : 'transparent'}`,
                marginBottom: -1,
              }}
            >
              {sc}
            </button>
          ))}
        </div>

        {/* Events */}
        <div className="py-8">
          {eventsLoading ? (
            <div className="grid gap-5" style={{ gridTemplateColumns: 'repeat(2, minmax(0, 1fr))' }}>
              {[0, 1, 2, 3].map(i => (
                <div key={i} className="animate-pulse rounded-xl" style={{ aspectRatio: '16 / 9', backgroundColor: 'rgba(255,255,255,0.05)' }} />
              ))}
            </div>
          ) : events.length === 0 ? (
            <div className="text-center py-16">
              <p className="text-[15px] font-semibold" style={{ color: colors.text }}>
                {scope === 'upcoming' ? 'No upcoming events' : 'No past events'}
              </p>
              <p className="text-[13px] mt-1.5" style={{ color: colors.subText }}>
                {scope === 'upcoming'
                  ? 'Follow this organiser to hear about new events first.'
                  : 'Nothing in the archive yet.'}
              </p>
            </div>
          ) : (
            <div className="grid gap-5" style={{ gridTemplateColumns: 'repeat(2, minmax(0, 1fr))' }}>
              {events.map(item => (
                <GridCard key={item.id} item={item} onPress={handlePress} colors={colors} />
              ))}
            </div>
          )}
        </div>
      </div>

      {followNote && (
        <div
          role="status"
          style={{
            position: 'fixed', bottom: 24, left: 16, right: 16, zIndex: 200,
            maxWidth: 420, margin: '0 auto',
            background: 'rgba(255,255,255,0.95)', color: '#111',
            borderRadius: 12, padding: '12px 16px', textAlign: 'center',
            fontSize: 13, fontWeight: 600,
            boxShadow: '0 8px 24px rgba(0,0,0,0.35)',
          }}
        >
          {followNote}
        </div>
      )}
    </div>
  );
}