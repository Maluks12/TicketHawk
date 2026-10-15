'use client';

import { useState, useEffect, useCallback } from 'react';
// Only icons already proven present in this codebase's lucide-react (^0.400.0).
import { BadgeCheck } from 'lucide-react';
import { useParams, useRouter } from 'next/navigation';
import { useEventStore } from '@/store/eventsStore';
import { useAppTheme } from '@/themes/colors';
import {
  getOrganizer, getOrganizerEvents, toggleOrganizerFollow,
  type Event, type PublicOrganizer, type OrganizerLink,
} from '@/lib/api';
import { SocialIcon, SOCIAL_LABEL } from '@/components/socialIcon';

/**
 * Public organiser profile.
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

  // Fires on mount alongside the profile request rather than after it. The
  // events endpoint takes the slug, not anything from the profile, so gating it
  // on `organiser` serialised two independent round trips for no reason.
  useEffect(() => { loadEvents(scope); }, [scope, loadEvents]);

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
    // Skeleton in the shape of the real page, not a lone spinner in the middle
    // of an empty screen. The layout appearing instantly is most of what makes
    // a wait feel short — the content then fills in where it was expected.
    const bone = 'rgba(255,255,255,0.06)';
    return (
      <div className="min-h-screen" style={{ backgroundColor: colors.background }}>
        <div className="mx-auto px-8 pt-6" style={{ maxWidth: 1200 }}>
          <div className="w-full rounded-2xl animate-pulse" style={{ aspectRatio: '3 / 1', backgroundColor: bone }} />
        </div>
        <div className="mx-auto px-8" style={{ maxWidth: 1200 }}>
          <div className="flex flex-col items-center" style={{ marginTop: -52 }}>
            <div className="rounded-2xl animate-pulse" style={{ width: 104, height: 104, backgroundColor: bone, border: `3px solid ${colors.background}` }} />
            <div className="rounded-lg animate-pulse mt-4" style={{ width: 220, height: 26, backgroundColor: bone }} />
            <div className="rounded animate-pulse mt-3" style={{ width: 160, height: 13, backgroundColor: bone }} />
            <div className="rounded-xl animate-pulse mt-6" style={{ width: 150, height: 44, backgroundColor: bone }} />
          </div>
          <div className="grid gap-5 mt-12" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))' }}>
            {[0, 1, 2, 3].map(i => (
              <div key={i} className="animate-pulse rounded-xl" style={{ aspectRatio: '16 / 9', backgroundColor: bone }} />
            ))}
          </div>
        </div>
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
  const label  = (p: string) => SOCIAL_LABEL[p as keyof typeof SOCIAL_LABEL] ?? p;

  return (
    <div className="min-h-screen" style={{ backgroundColor: colors.background }}>
      {/* Cover.
          The dashboard crops covers to 3:1 (CROP_PROFILE_COVER in
          events-organizer/app/(dashboard)/organiser-page/page.tsx), but this
          used to be a fixed 220px band across the whole viewport — ~8.6:1 on a
          1900px screen — so object-cover threw away most of the crop. The
          cover now renders at exactly 3:1 inside the content column, so what
          the organiser cropped is what the visitor sees. The full-width band
          behind it is a blurred copy, so the edges of a wide screen are not
          left empty. */}
      <div className="relative w-full overflow-hidden">
        {organiser.cover_image_url && (
          <img
            src={organiser.cover_image_url}
            alt=""
            aria-hidden
            className="absolute inset-0 w-full h-full object-cover"
            style={{ filter: 'blur(48px)', transform: 'scale(1.2)', opacity: 0.45 }}
          />
        )}
        <div
          className="absolute inset-0"
          style={{ background: `linear-gradient(to bottom, rgba(0,0,0,0.10), ${colors.background})` }}
        />
        <div className="relative mx-auto px-8 pt-6" style={{ maxWidth: 1200 }}>
          <div
            className="relative w-full rounded-2xl overflow-hidden"
            style={
              organiser.cover_image_url
                ? { aspectRatio: '3 / 1', backgroundColor: subtle }
                : { height: 160, backgroundColor: subtle }
            }
          >
            {organiser.cover_image_url && (
              <img src={organiser.cover_image_url} alt="" className="absolute inset-0 w-full h-full object-cover" />
            )}
          </div>
        </div>
      </div>

      <div className="mx-auto px-8" style={{ maxWidth: 1200 }}>
        {/* Identity — pulled up over the cover. `relative z-10` is what puts
            it on top: the cover above is a positioned element and this block
            was not, so the cover (and its fade) painted OVER the top half of
            the logo — only the bottom of it was visible. */}
        <div className="relative z-10 flex flex-col items-center text-center" style={{ marginTop: -52 }}>
          {/* The organiser's own picture. Omitted entirely when they have not
              set one, rather than filled with a placeholder glyph — the slot is
              for a photo, and an icon is not a photo. */}
          {organiser.logo_url && (
            <div
              className="rounded-2xl overflow-hidden"
              style={{ width: 104, height: 104, backgroundColor: colors.cardBg, border: `3px solid ${colors.background}` }}
            >
              {/* contain, not cover: logos are uploaded without a crop, so
                  a wide wordmark in a square box lost its sides. */}
              <img src={organiser.logo_url} alt="" className="w-full h-full object-contain" />
            </div>
          )}

          <div className="flex items-center gap-2 mt-4">
            <h1 className="font-black" style={{ color: colors.text, fontSize: 26, letterSpacing: '-0.4px', margin: 0 }}>
              {organiser.business_name}
            </h1>
            {organiser.verified && (
              <BadgeCheck size={20} color="#2f7cf6" aria-label="Verified organiser" />
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
          </button>

          {links.length > 0 && (
            // Bare marks, matching the app (EventsApp/app/organiser/[slug].tsx)
            // rather than the grey text chips this used to be. The padding keeps
            // the click/tap target at ~44px around the glyph.
            <div className="flex flex-wrap items-center justify-center gap-4 mt-5">
              {links.map(l => (
                <a
                  key={l.platform}
                  href={l.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  aria-label={`${organiser.business_name} on ${label(l.platform)}`}
                  title={label(l.platform)}
                  className="flex items-center justify-center rounded-lg p-2 opacity-90 transition-opacity duration-150 hover:opacity-60"
                  style={{ textDecoration: 'none' }}
                >
                  <SocialIcon platform={l.platform} size={26} color={colors.text} />
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
            <div className="grid gap-5" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))' }}>
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
            <div className="grid gap-5" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))' }}>
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