import {
  View,
  Text,
  TouchableOpacity,
  StatusBar,
  ActivityIndicator,
  Share,
  Linking,
  Platform,
  Animated,
  Dimensions,
  InteractionManager,
  Alert,
} from 'react-native';
import { useRouter, useLocalSearchParams } from 'expo-router';
import { LinearGradient } from 'expo-linear-gradient';
import { Image as ExpoImage } from 'expo-image';
// UsersThree and Hash are gone with the Capacity row and the Tags section.
// Storefront/CheckCircle are for the new Organiser row. Both are already used
// elsewhere in this app (see components/), so they are known to exist in
// phosphor-react-native 3.0.6 — unlike Buildings/SealCheck, which are not.
// Export stays: the SHARE button was removed from the action card, but the
// hero still has one at the top right — that is the one being kept.
import { CalendarDots, Clock, MapPin, Heart, Export, Ticket, Storefront, CheckCircle, type Icon as PhosphorIcon } from 'phosphor-react-native';
import { useState, useRef, useEffect, useCallback } from 'react';
import { useEventStore } from '../../store/eventStore';
import { useAuthStore } from '../../store/authStore';
import { s, sheet, HERO_H } from '../../styles/eventDetail';
import { useAppTheme } from '../../theme/colors';
import { getBaseUrl } from '../../components/FilterModal';
import { toggleFollow, getFollowStatus } from '../../lib/api';
import { buildEventShare } from '../../lib/shareLinks';
import { PolicySheet } from '../../components/PolicySheet';
import { IMG_PLACEHOLDER } from '../../lib/img';

// ─── Hero frame ───────────────────────────────────────────────────────────────
// The hero no longer forces every image into one tall portrait box. The frame
// TRACKS THE ARTWORK's own aspect ratio, clamped between the two bounds below,
// which is what lets the wide banner render here the way it does on web.
//
// Why this matters: the old frame was a fixed 58vh portrait box, so a 16:9
// banner had only bad options — 'contain' letterboxed it into a thin strip
// with ~270px of blur bars, and 'cover' cropped ~55% of its width. That's why
// the screen used to quietly prefer the (portrait) cover image instead, which
// contradicted the organiser dashboard's own promise that the banner is the
// "wide hero image at the top of the event page".
//
// Once the frame matches the image, 'cover' becomes the right fit rather than a
// destructive one: inside the clamp range it crops NOTHING (frame aspect ==
// image aspect) while still filling edge-to-edge, and only at the clamp limits
// does it trim a little. Hence HERO_FIT is now 'cover'.
const HERO_FIT: 'contain' | 'cover' = 'cover';

const { width: SCREEN_W, height: SCREEN_H_LOCAL } = Dimensions.get('window');

// Upper bound: the original 58vh portrait frame — unchanged, so portrait cover
// artwork looks exactly as it does today.
const HERO_MAX_H = HERO_H;
// Lower bound: how short the frame may get for very wide artwork. Keeps enough
// vertical room for the title block (absolutely positioned at bottom: 36) and
// stops an ultra-wide banner collapsing the hero into a letterbox slot.
const HERO_MIN_H = Math.round(SCREEN_H_LOCAL * 0.34);

const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), hi);

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatPrice(price: number, currency: string) {
  if (price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

function formatDate(iso: string) {
  const d = new Date(iso);
  return d.toLocaleDateString('en-ZA', {
    weekday: 'short', day: 'numeric', month: 'long', year: 'numeric',
  });
}

function formatDateShort(iso: string) {
  const d = new Date(iso);
  return d.toLocaleDateString('en-ZA', {
    weekday: 'short', day: 'numeric', month: 'short', year: 'numeric',
  });
}

function formatTime(iso: string) {
  const d = new Date(iso);
  return d.toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
}

// ─── LineupRow ────────────────────────────────────────────────────────────────
//
// One act or team. Handles both shapes: a structured `lineup` row with an
// image, a stage and a set time, and a bare {name} object built from the legacy
// performers/teams arrays.
//
// The chevron only renders when there is somewhere to go. The old rows always
// showed one, which promised a screen that has never existed — now it appears
// only for an entry carrying an external_url, and tapping opens that link.
function LineupRow({
  entry,
  isLast,
  colors,
}: {
  entry: { id?: string; name: string; image_url?: string; subtitle?: string; stage?: string; set_time?: string; external_url?: string };
  isLast: boolean;
  colors: any;
}) {
  const hasLink = typeof entry.external_url === 'string' && entry.external_url.length > 0;

  // "Main stage · 21:30" — whichever of the two exist. Set times are full
  // timestamps because festivals cross midnight, so only the clock part is
  // shown here; the date is the event's own.
  const metaBits: string[] = [];
  if (entry.subtitle) metaBits.push(entry.subtitle);
  if (entry.stage && entry.stage !== entry.subtitle) metaBits.push(entry.stage);
  if (entry.set_time) {
    const d = new Date(entry.set_time);
    if (!Number.isNaN(d.getTime())) {
      metaBits.push(d.toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit', hour12: false }));
    }
  }
  const meta = metaBits.join(' · ');

  const body = (
    <>
      <View
        className={s.participantAvatar}
        style={{ backgroundColor: colors.sectionCardBg, borderColor: colors.dividerColor, overflow: 'hidden' }}
      >
        {entry.image_url ? (
          <ExpoImage
            source={{ uri: entry.image_url }}
            style={{ width: '100%', height: '100%' }}
            contentFit="cover"
            cachePolicy="memory-disk"
            placeholder={IMG_PLACEHOLDER}
            placeholderContentFit="cover"
            recyclingKey={entry.id ?? entry.name}
            allowDownscaling
            transition={120}
          />
        ) : (
          <Text className={s.participantAvatarText} style={{ color: colors.text }}>
            {entry.name.trim().charAt(0).toUpperCase()}
          </Text>
        )}
      </View>
      <View style={{ flex: 1 }}>
        <Text className={s.participantName} style={{ color: colors.text }} numberOfLines={1}>
          {entry.name}
        </Text>
        {meta.length > 0 && (
          <Text className={s.participantMeta} style={{ color: colors.subText }} numberOfLines={1}>
            {meta}
          </Text>
        )}
      </View>
      {hasLink && <Text className={s.participantArrow} style={{ color: colors.subText }}>›</Text>}
    </>
  );

  const rowClass = isLast ? s.participantRowLast : s.participantRow;
  const rowStyle = isLast ? undefined : { borderBottomColor: colors.dividerColor };

  if (!hasLink) {
    return <View className={rowClass} style={rowStyle}>{body}</View>;
  }

  return (
    <TouchableOpacity
      className={rowClass}
      style={rowStyle}
      activeOpacity={0.75}
      onPress={() => { Linking.openURL(entry.external_url!).catch(() => {}); }}
    >
      {body}
    </TouchableOpacity>
  );
}

// ─── MetaItem ─────────────────────────────────────────────────────────────────

function MetaItem({ icon: IconCmp, label, value }: { icon: PhosphorIcon; label: string; value: string }) {
  const { colors, isDark } = useAppTheme();
  return (
    <View className={s.metaItemRow}>
      <View className={s.metaIconBox} style={{ backgroundColor: colors.sectionCardBg }}>
        <IconCmp size={22} color={colors.text} weight="regular" />
      </View>
      <View>
        <Text className={s.metaLabelText} style={{ color: colors.subText }}>{label}</Text>
        <Text className={s.metaValueText} style={{ color: colors.text }}>{value}</Text>
      </View>
    </View>
  );
}

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function EventDetail() {
  const router             = useRouter();
  // goBack, not router.back().
  //
  // A deep link (a private event link from an email or WhatsApp) launches the
  // app straight onto this screen with an EMPTY navigation stack. router.back()
  // then dispatches GO_BACK with nothing to pop, which expo-router reports as
  // "The action 'GO_BACK' was not handled by any navigator" and, in release,
  // simply does nothing — the button looks broken.
  //
  // canGoBack() is the documented way to ask. When there is no history we
  // replace with the home tab instead, so a deep-linked visitor lands somewhere
  // real rather than on a dead end.
  const goBack = useCallback(() => {
    if (router.canGoBack()) router.back();
    else router.replace('/(tabs)');
  }, [router]);

  const { id }             = useLocalSearchParams<{ id: string }>();
  const event              = useEventStore((state) => state.selectedEvent);
  const setSelectedEvent   = useEventStore((state) => state.setSelectedEvent);
  const addRecentlyViewed  = useEventStore((state) => state.addRecentlyViewed);
  const authUser           = useAuthStore((state) => state.user);

  // ── Follow state ──────────────────────────────────────────────────────────
  const [isFollowing,   setIsFollowing]   = useState(false);
  const [followLoading, setFollowLoading] = useState(false);

  const { colors, isDark } = useAppTheme();
  const [expanded, setExpanded] = useState(false);

  // Which policy the sheet is showing, or null. One piece of state for both
  // rows rather than two booleans — they are mutually exclusive by nature and
  // two flags would allow a state that cannot be rendered.
  const [policySheet, setPolicySheet] = useState<null | 'refund' | 'terms'>(null);
  const scrollY = useRef(new Animated.Value(0)).current;

  // Natural aspect ratio (w/h) of the hero artwork, learned from the image
  // itself via onLoad. Null until the first layer reports back — until then we
  // render at the max (portrait) height, which is the safe default and matches
  // the pre-change look for the one frame before load completes.
  const [heroAspect, setHeroAspect] = useState<number | null>(null);

  // ── View tracking + fresh data fetch ──────────────────────────────────────
  // Fires once when the user opens this screen (after the nav animation).
  // 1. POST /view  — tells the backend a real user opened the screen.
  // 2. GET event   — fetches fresh data so the view count shown is up-to-date.
  //
  // The fetch outcome is now TRACKED rather than swallowed. It used to read:
  //
  //     .then(res => { if (!res.ok) return; return res.json(); })
  //     .then(fresh => { if (fresh) setSelectedEvent(fresh); })
  //     .catch(() => {});
  //
  // …which meant every failure path set nothing at all. Combined with the
  // "store doesn't match the route yet ⇒ show a spinner" guard further down,
  // any failure left the screen spinning forever with no way out but the OS
  // back gesture.
  //
  // That is not a rare edge case, it is the NORMAL path for an old
  // notification. GetEventByID filters on status = ANY(published,
  // published_private), so once an event is marked `completed` (or cancelled)
  // the endpoint 404s. Tapping a months-old "event reminder" push therefore
  // routes to /event/<id>, finds nothing in the store, gets a 404, and hangs.
  const [resolveState, setResolveState] = useState<'loading' | 'ok' | 'gone' | 'error'>(
    // If the store already holds this event we have something to draw
    // immediately; the fetch below is then only a freshness update and must
    // never be able to take the screen down.
    'loading',
  );
  const [retryTick, setRetryTick] = useState(0);

  useEffect(() => {
    const eventId = id ?? event?.id ?? event?.slug;
    if (!eventId) return;

    const base = getBaseUrl();
    const controller = new AbortController();
    // A hung socket is indistinguishable from a slow one from in here, and RN's
    // fetch has no default timeout — so a dead tunnel spins indefinitely.
    // Bound it and fall through to the retry screen.
    const timeout = setTimeout(() => controller.abort(), 12000);
    let cancelled = false;

    setResolveState('loading');

    const task = InteractionManager.runAfterInteractions(() => {
      // 1. Track the view — fire-and-forget, never blocks the UI.
      fetch(`${base}/v1/events/${eventId}/view`, { method: 'POST' }).catch(() => {});

      // 2. Fetch fresh event data so views (and anything else) reflect reality.
      fetch(`${base}/v1/events/${eventId}`, { signal: controller.signal })
        .then(async (res) => {
          if (cancelled) return;
          // 404 = filtered out by status (completed / cancelled / unpublished)
          // or genuinely deleted. 410 is included for future-proofing in case
          // the backend ever starts distinguishing "gone" from "never existed".
          // Either way there is nothing to show and the user needs to be told.
          if (res.status === 404 || res.status === 410) { setResolveState('gone'); return; }
          if (!res.ok) { setResolveState('error'); return; }

          const fresh = await res.json().catch(() => null);
          if (cancelled) return;
          if (!fresh || !fresh.id) { setResolveState('error'); return; }

          setSelectedEvent(fresh);
          setResolveState('ok');
        })
        .catch(() => {
          if (!cancelled) setResolveState('error');
        })
        .finally(() => clearTimeout(timeout));
    });

    return () => {
      cancelled = true;
      clearTimeout(timeout);
      controller.abort();
      task.cancel();
    };
  }, [id, retryTick]);

  // ── Recently viewed (deferred, same pattern as before) ────────────────────
  useEffect(() => {
    if (event?.id && authUser?.id) {
      const task = InteractionManager.runAfterInteractions(() => {
        addRecentlyViewed(event!, authUser!.id);
      });
      return () => task.cancel();
    }
  }, [event?.id, authUser?.id]);

  // ── Fetch current follow status on mount ──────────────────────────────────
  // Only runs when a logged-in user opens the screen so the button always
  // reflects reality (e.g. if they followed on another device).
  useEffect(() => {
    if (!event?.id || !authUser) return;
    let cancelled = false;
    (async () => {
      try {
        const res = await getFollowStatus(event.id);
        if (!cancelled) setIsFollowing(res.following);
      } catch { /* default to not-following — no disruption to the UI */ }
    })();
    return () => { cancelled = true; };
  }, [event?.id, authUser?.id]);

  // ── Toggle follow ─────────────────────────────────────────────────────────
  const handleFollow = useCallback(async () => {
    if (!authUser) {
      Alert.alert(
        'Sign in required',
        'Log in or sign up to follow events and get reminders.',
        [
          { text: 'Cancel', style: 'cancel' },
          { text: 'Log in / Sign up', onPress: () => router.push('/auth/login' as any) },
        ],
      );
      return;
    }
    if (!event?.id || followLoading) return;

    // Optimistic update so the button snaps immediately
    setIsFollowing(prev => !prev);
    setFollowLoading(true);

    try {
      const res = await toggleFollow(event.id);
      // Reconcile with the authoritative server response
      setIsFollowing(res.following);
    } catch {
      // Roll back the optimistic update on failure
      setIsFollowing(prev => !prev);
      Alert.alert('Error', 'Could not update follow status. Please try again.');
    } finally {
      setFollowLoading(false);
    }
  }, [authUser, event?.id, followLoading, router]);

  // ── Cold-start / deep-link guard ──────────────────────────────────────────
  // Reaching this screen from inside the app always pre-seeds selectedEvent, so
  // `event` is already there. Reaching it from a SHARED LINK does not: the
  // process starts on this route with nothing in the store, and the fetch in the
  // effect above is what fills it in.
  //
  // Two things that then go wrong without this guard:
  //   1. an empty store rendered "EVENT NOT FOUND" for the length of the fetch,
  //      which is the worst possible first impression of a shared ticket link;
  //   2. a store still holding the LAST event viewed rendered that event's
  //      artwork, price and Get Tickets button under the new event's URL.
  // Both are fixed by treating "store doesn't match the route yet" as loading.
  const routeKey     = id ? String(id) : null;
  const storeMatches = !routeKey || !event
    ? !!event
    : (event.id === routeKey || (event as any).slug === routeKey);
  const resolving    = !!routeKey && !storeMatches;

  // The spinner is now only shown while a fetch is genuinely IN FLIGHT. Once
  // it settles we always land on something the user can act on.
  if (resolving && resolveState === 'loading') {
    return (
      <View className="flex-1 items-center justify-center" style={{ backgroundColor: colors.background }}>
        <ActivityIndicator color={colors.subText} />
      </View>
    );
  }

  // Terminal: the event is no longer available. Overwhelmingly this is a stale
  // notification or an old shared link for an event that has since finished,
  // so the copy leads with that rather than with a bare "not found", which
  // reads as a bug to the user.
  if (resolving && resolveState === 'gone') {
    return (
      <View className="flex-1 items-center justify-center" style={{ backgroundColor: colors.background, paddingHorizontal: 40 }}>
        <CalendarDots size={44} color={colors.subText} weight="regular" />
        <Text style={{ color: colors.text, fontWeight: '800', fontSize: 17, marginTop: 18, textAlign: 'center' }}>
          This event has ended
        </Text>
        <Text style={{ color: colors.subText, fontSize: 13.5, lineHeight: 20, marginTop: 8, textAlign: 'center' }}>
          It's no longer on sale. If you bought tickets, they're still in your
          Tickets tab.
        </Text>
        <TouchableOpacity
          onPress={() => router.replace('/(tabs)' as any)}
          style={{ marginTop: 22, backgroundColor: '#ff2d55', paddingHorizontal: 26, paddingVertical: 12, borderRadius: 22 }}
          activeOpacity={0.85}
        >
          <Text style={{ color: '#fff', fontSize: 14, fontWeight: '700' }}>Browse events</Text>
        </TouchableOpacity>
        <TouchableOpacity onPress={goBack} style={{ marginTop: 14 }} activeOpacity={0.7}>
          <Text style={{ color: colors.subText, fontSize: 13, fontWeight: '600' }}>Go back</Text>
        </TouchableOpacity>
      </View>
    );
  }

  // Terminal: something went wrong that a retry might fix (offline, timeout,
  // 5xx). Distinct from "gone" because the action the user wants is different.
  if (resolving && resolveState === 'error') {
    return (
      <View className="flex-1 items-center justify-center" style={{ backgroundColor: colors.background, paddingHorizontal: 40 }}>
        <Text style={{ color: colors.text, fontWeight: '800', fontSize: 17, textAlign: 'center' }}>
          Couldn't load this event
        </Text>
        <Text style={{ color: colors.subText, fontSize: 13.5, lineHeight: 20, marginTop: 8, textAlign: 'center' }}>
          Check your connection and try again.
        </Text>
        <TouchableOpacity
          onPress={() => setRetryTick((n) => n + 1)}
          style={{ marginTop: 22, backgroundColor: '#ff2d55', paddingHorizontal: 26, paddingVertical: 12, borderRadius: 22 }}
          activeOpacity={0.85}
        >
          <Text style={{ color: '#fff', fontSize: 14, fontWeight: '700' }}>Try again</Text>
        </TouchableOpacity>
        <TouchableOpacity onPress={goBack} style={{ marginTop: 14 }} activeOpacity={0.7}>
          <Text style={{ color: colors.subText, fontSize: 13, fontWeight: '600' }}>Go back</Text>
        </TouchableOpacity>
      </View>
    );
  }

  if (!event) {
    return (
      <View className="flex-1 items-center justify-center" style={{ backgroundColor: colors.background }}>
        <Text style={{ color: colors.subText, fontWeight: '800', fontSize: 12, letterSpacing: 4 }}>
          EVENT NOT FOUND
        </Text>
        <TouchableOpacity
          onPress={goBack}
          style={{ marginTop: 16, borderWidth: 1, borderColor: colors.dividerColor,
            paddingHorizontal: 20, paddingVertical: 10, borderRadius: 12 }}
        >
          <Text style={{ color: colors.text, fontSize: 10, fontWeight: '800', letterSpacing: 1.5 }}>
            ← GO BACK
          </Text>
        </TouchableOpacity>
      </View>
    );
  }

  const ev           = event as any;
  // Banner first — matching web-desktop, web-mobile, the app's own home hero
  // and its purchase screen, all of which resolve `banner_image_url ?? cover`.
  // This screen was the single outlier in the codebase, so an organiser who
  // uploaded a banner saw it everywhere EXCEPT here, and the image changed
  // under them when they tapped through to Get Tickets. The frame below adapts
  // to the artwork, so preferring the wide banner no longer costs anything.
  const heroImage    = event.banner_image_url ?? event.cover_image_url;
  const isSoldOut    = event.is_sold_out ?? false;

  // An event can still resolve from the API while being over: status is only
  // flipped to `completed` by an operator/worker pass, so there's a window
  // (and, for events that never get swept, indefinitely) where a past event is
  // still `published` and fetches fine. Selling a ticket to it would be a real
  // problem, so the CTA is gated on wall-clock time, not on status.
  // Matches isEventLive() on the home feed: no end_time ⇒ assume 4h runtime.
  const endedAtMs = ev.end_time
    ? new Date(ev.end_time).getTime()
    : (event.start_time ? new Date(event.start_time).getTime() + 4 * 60 * 60 * 1000 : NaN);
  const hasEnded     = Number.isFinite(endedAtMs) && endedAtMs < Date.now();
  const isCancelled  = String(ev.status ?? '').toLowerCase() === 'cancelled';
  const isASport     = event.category?.toLowerCase() === 'sports';

  // ── Line-up ───────────────────────────────────────────────────────────────
  //
  // Prefer the structured `lineup` rows (names + images + stages + set times)
  // and fall back to the legacy flat arrays.
  //
  // The fallback is not dead code. The backend still writes teams/performers
  // on every line-up save, and an event that has not been re-saved since the
  // migration has rows in both places — but a CACHED event object held by this
  // store from before the app updated will only have the arrays. Reading both
  // means the section never empties during a rollout.
  const lineupRows: any[] = Array.isArray(ev.lineup) ? ev.lineup : [];
  const lineupTeams      = lineupRows.filter((l: any) => l.kind === 'team');
  const lineupPerformers = lineupRows.filter((l: any) => l.kind !== 'team');

  const teams:      string[] = ev.teams      ?? [];
  const performers: string[] = ev.performers ?? [];

  // Information block — only fields the organiser actually filled in have
  // rows, so there is nothing here to guard against rendering empty.
  const infoItems: any[] = Array.isArray(ev.info_items) ? ev.info_items : [];

  // Organiser. Absent when they have no public page (KYC below level 2, page
  // switched off, account suspended) — in which case nothing renders rather
  // than falling back to the venue name, which is what this screen used to
  // imply by having no organiser at all.
  const organiser: any = ev.organizer ?? null;

  // Refund policy and T&Cs are columns on the event, not catalogue items, and
  // they render at the bottom of the Information block.
  const refundPolicy = typeof ev.refund_policy === 'string' ? ev.refund_policy.trim() : '';
  const termsAndConditions = typeof ev.terms_and_conditions === 'string' ? ev.terms_and_conditions.trim() : '';

  // Routing unchanged — matches index.tsx push to /event/${event.id}
  const handleGetTickets = () => router.push('/event/purchase');

  // ── Share ─────────────────────────────────────────────────────────────────
  // Shares the canonical WEB url for the event, not a bare sentence and not the
  // eventsapp:// scheme. A recipient without the app gets a page that opens; a
  // recipient with the app gets handed straight to this screen by the OS via
  // Universal Links / App Links (see app.json + lib/shareLinks.ts).
  //
  // iOS and Android disagree about where the URL belongs — iOS renders `url` as
  // a rich link preview and would double-post it if it were also in `message`,
  // while Android drops `url` entirely and only sends `message`. buildEventShare
  // resolves that per platform rather than us shipping a URL that silently
  // vanishes on one of them.
  const handleShare = async () => {
    try {
      const payload = buildEventShare(ev, Platform.OS === 'ios', id);
      const result  = await Share.share(payload, {
        dialogTitle:  'Share this event',
        subject:      payload.title,          // iOS: pre-fills the email subject
        // Nothing useful can be posted to the clipboard-only targets, and the
        // print/assign-to-contact actions make no sense for a link.
        excludedActivityTypes: [
          'com.apple.UIKit.activity.Print',
          'com.apple.UIKit.activity.AssignToContact',
        ],
      });
      // Not an error path — just nothing to do. Kept explicit so a future
      // analytics call has an obvious home.
      if (result.action === Share.dismissedAction) return;
    } catch (_) {
      // The sheet failing to open is not worth an alert; the buyer can retry.
    }
  };

  const handleDirections = async () => {
    // Prefer EXACT coordinates / place_id over a text search. The old version
    // searched for "<venue>, <city>" and let Google guess, which is why buyers
    // were sent to a completely different place — a mismatched pair like
    // "FNB Stadium, Sandton" (FNB is in Nasrec) can resolve anywhere. It also
    // dropped the street address entirely.
    const ev        = event as any;
    const label     = [event.venue_name, ev.address, event.city].filter(Boolean).join(', ');
    const query     = encodeURIComponent(label);
    const hasCoords = typeof ev.latitude === 'number' && typeof ev.longitude === 'number';
    const coords    = hasCoords ? `${ev.latitude},${ev.longitude}` : '';

    // Native apps take coordinates directly, which removes all ambiguity.
    const nativeUrl = hasCoords
      ? (Platform.OS === 'ios' ? `maps://?ll=${coords}&q=${query}` : `geo:${coords}?q=${coords}(${query})`)
      : (Platform.OS === 'ios' ? `maps://?q=${query}` : `geo:0,0?q=${query}`);

    const webUrl = ev.place_id
      ? `https://www.google.com/maps/search/?api=1&query=${query}&query_place_id=${encodeURIComponent(ev.place_id)}`
      : hasCoords
        ? `https://www.google.com/maps/search/?api=1&query=${coords}`
        : `https://www.google.com/maps/search/?api=1&query=${query}`;
    try {
      const ok = await Linking.canOpenURL(nativeUrl);
      await Linking.openURL(ok ? nativeUrl : webUrl);
    } catch {
      await Linking.openURL(webUrl);
    }
  };

  const shortDesc   = event.description?.slice(0, 200) ?? '';
  const longDesc    = event.description ?? '';
  const hasMore     = longDesc.length > 200;
  const displayDesc = expanded ? longDesc : (shortDesc + (hasMore ? '…' : ''));

  // Frame height that would display the artwork edge-to-edge with no crop,
  // then clamped to [HERO_MIN_H, HERO_MAX_H]. A tall/portrait cover lands at
  // the max (unchanged from before); a 16:9 banner lands around 0.56·screen-W,
  // i.e. a proper wide hero instead of a letterboxed strip. Falls back to the
  // max height until the image has reported its dimensions.
  // Plain const, NOT useMemo: this sits below the `if (!event)` early return,
  // so a hook here would violate the rules of hooks. The maths is trivial.
  const heroHeight = (!heroAspect || heroAspect <= 0)
    ? HERO_MAX_H
    : Math.round(clamp(SCREEN_W / heroAspect, HERO_MIN_H, HERO_MAX_H));

  // Gentle parallax on the sharp foreground only. The image is no longer
  // oversized (the old `bottom: -35%` overscan forced `cover` to upscale the
  // source ~35% beyond full-screen). As it drifts, the blurred backdrop shows
  // through — no zoom, no crop, no resolution loss. Driven by the ACTUAL frame
  // height so the drift stays proportional whatever aspect the artwork is.
  const PARALLAX_SHIFT  = Math.round(heroHeight * 0.12);
  const imageTranslateY = scrollY.interpolate({
    inputRange:  [-heroHeight, 0, heroHeight],
    outputRange: [-PARALLAX_SHIFT / 2, 0, PARALLAX_SHIFT],
    extrapolate: 'clamp',
  });

  // expo-image reports the decoded source size on load; derive w/h aspect from
  // whichever layer resolves first. Guarded so a re-report of the same ratio
  // doesn't thrash state. Plain function (not useCallback) for the same
  // rules-of-hooks reason as heroHeight above.
  const onHeroLoad = (e: { source?: { width?: number; height?: number } }) => {
    const w = e?.source?.width;
    const h = e?.source?.height;
    if (!w || !h) return;
    const ratio = w / h;
    setHeroAspect((prev) => (prev && Math.abs(prev - ratio) < 0.001 ? prev : ratio));
  };

  return (
    <View style={{ flex: 1, backgroundColor: colors.background }}>
      <StatusBar barStyle="light-content" />

      {/* ── Nav overlay ─────────────────────────────────────────────── */}
      <View style={sheet.navOverlay} pointerEvents="box-none">
        <TouchableOpacity style={sheet.navBtn} onPress={goBack} activeOpacity={0.8}>
          <Text className={s.backIcon}>‹</Text>
        </TouchableOpacity>
        <TouchableOpacity style={sheet.navBtn} onPress={handleShare} activeOpacity={0.8}>
          <Export size={18} color="#FFFFFF" weight="regular" />
        </TouchableOpacity>
      </View>

      <Animated.ScrollView
        showsVerticalScrollIndicator={false}
        contentContainerStyle={{ paddingBottom: 40 }}
        scrollEventThrottle={16}
        onScroll={Animated.event(
          [{ nativeEvent: { contentOffset: { y: scrollY } } }],
          { useNativeDriver: true },
        )}
      >
        {/* ── Hero ─────────────────────────────────────────────────── */}
        {/* Height tracks the artwork's aspect (clamped): portrait covers stay
            at the tall max, wide banners get a proportionate shorter frame. */}
        <View style={[sheet.heroContainer, { height: heroHeight }]}>
          {heroImage ? (
            <>
              {/* Blurred backdrop fills the frame edge-to-edge, so the sharp
                  layer never needs to be zoomed/cropped to cover it. */}
              <ExpoImage
                source={{ uri: heroImage }}
                style={sheet.heroBackdrop}
                contentFit="cover"
                blurRadius={24}
                cachePolicy="memory-disk"
                transition={150}
              />
              {/* Sharp foreground. HERO_FIT='cover' now fills edge-to-edge, and
                  because the frame above matches the artwork's aspect ratio it
                  crops nothing within the clamp range — only at the min/max
                  bounds does it trim slightly. onLoad reports the natural size
                  that drives that frame height. */}
              <Animated.View
                style={[sheet.heroImage, { transform: [{ translateY: imageTranslateY }] }]}
                pointerEvents="none"
              >
                <ExpoImage
                  source={{ uri: heroImage }}
                  style={{ flex: 1 }}
                  contentFit={HERO_FIT}
                  onLoad={onHeroLoad}
                  cachePolicy="memory-disk"
                  priority="high"
                  transition={150}
                />
              </Animated.View>
            </>
          ) : (
            <View style={[sheet.heroImage, { backgroundColor: '#222222' }]} />
          )}

          <LinearGradient
            colors={['transparent', 'rgba(0,0,0,0.50)', 'rgba(0,0,0,0.90)']}
            locations={[0.3, 0.6, 1]}
            style={sheet.absoluteFill}
            pointerEvents="none"
          />
          <LinearGradient
            colors={['rgba(0,0,0,0.55)', 'transparent']}
            locations={[0, 1]}
            style={[sheet.absoluteFill, { bottom: undefined, height: 130 }]}
            pointerEvents="none"
          />

          {isSoldOut && (
            <View style={sheet.soldOutBadge}>
              <Text style={sheet.soldOutText}>SOLD OUT</Text>
            </View>
          )}

          <View style={sheet.heroTitleBlock}>
            {/* TRENDING — #FF2D55 badge retained. View count is intentionally
                not shown to users (tracked backend-side only). */}
            {event.is_featured && (
              <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: 12, alignItems: 'center', marginBottom: 10 }}>
                <View style={sheet.trendingBadge}>
                  <Text style={sheet.trendingBadgeText}>TRENDING #1</Text>
                </View>
              </View>
            )}
            {/* Category — #FF2D55 text on dark, not a fill */}
            <Text style={sheet.heroCategory}>✦ {event.category}</Text>
            <Text style={sheet.heroTitle} numberOfLines={3}>
              {event.title.toUpperCase()}
            </Text>
          </View>
        </View>

        {/* ── Floating action card ─────────────────────────────────── */}
        <View style={[sheet.actionCard, { backgroundColor: colors.cardBg }]}>
          <MetaItem icon={CalendarDots} label="DATE"     value={formatDateShort(event.start_time)} />
          <MetaItem icon={Clock}        label="TIME"     value={formatTime(event.start_time)} />
          <MetaItem icon={MapPin}       label="VENUE"    value={`${event.venue_name}, ${event.city}`} />
          {/* Organiser replaces Capacity here.
              Capacity told the buyer nothing actionable — this screen never
              showed remaining tickets beside it, so "800 Seats" on a sold-out
              event read as availability. max_capacity is untouched in the
              model and still drives the sold-out calculation and the organiser
              dashboard; it is only this row that goes.

              Tappable, unlike the three rows above it, so it carries a chevron
              and its own press target rather than being a fourth static
              MetaItem. */}
          {organiser && (
            <TouchableOpacity
              className={s.metaItemRow}
              onPress={() => router.push(`/organiser/${organiser.slug ?? organiser.id}`)}
              activeOpacity={0.75}
            >
              <View className={s.metaIconBox} style={{ backgroundColor: colors.sectionCardBg }}>
                {organiser.logo_url ? (
                  /* cachePolicy + placeholder + recyclingKey, matching
                     CoverImage and the hero. Without cachePolicy expo-image
                     re-downloads on every mount, which on this screen means
                     every time someone opens an event by this organiser. */
                  <ExpoImage
                    source={{ uri: organiser.logo_url }}
                    style={{ width: '100%', height: '100%', borderRadius: 12 }}
                    contentFit="cover"
                    cachePolicy="memory-disk"
                    placeholder={IMG_PLACEHOLDER}
                    placeholderContentFit="cover"
                    recyclingKey={organiser.id}
                    allowDownscaling
                    transition={120}
                  />
                ) : (
                  <Storefront size={20} color={colors.text} weight="regular" />
                )}
              </View>
              <View style={{ flex: 1 }}>
                <Text className={s.metaLabelText} style={{ color: colors.subText }}>ORGANISED BY</Text>
                <View style={{ flexDirection: 'row', alignItems: 'center', gap: 5 }}>
                  <Text className={s.metaValueText} style={{ color: colors.text }} numberOfLines={1}>
                    {organiser.business_name}
                  </Text>
                  {organiser.verified && (
                    <CheckCircle size={15} color="#2f7cf6" weight="fill" />
                  )}
                </View>
              </View>
              <Text className={s.participantArrow} style={{ color: colors.subText }}>›</Text>
            </TouchableOpacity>
          )}
          <View style={[sheet.metaDivider, { backgroundColor: colors.dividerColor }]} />
          <View className={s.ctaWrap}>
            {(hasEnded || isCancelled) ? (
              /* Past or cancelled: no purchase path. Reusing the sold-out
                 button style keeps this visually consistent with the other
                 "can't buy" state rather than inventing a third treatment. */
              <TouchableOpacity className={s.btnSoldOut} style={{ flexDirection: 'row', justifyContent: 'center', gap: 8 }} disabled>
                <CalendarDots size={18} color="rgba(0,0,0,0.3)" weight="regular" />
                <Text className={s.btnSoldOutText}>
                  {isCancelled ? 'EVENT CANCELLED' : 'EVENT HAS ENDED'}
                </Text>
              </TouchableOpacity>
            ) : isSoldOut ? (
              <TouchableOpacity className={s.btnSoldOut} style={{ flexDirection: 'row', justifyContent: 'center', gap: 8 }} disabled>
                <Ticket size={18} color="rgba(0,0,0,0.3)" weight="regular" />
                <Text className={s.btnSoldOutText}>SOLD OUT</Text>
              </TouchableOpacity>
            ) : (
              /* GET TICKETS — only full #FF2D55 fill in the whole screen */
              <TouchableOpacity className={s.btnPrimary} onPress={handleGetTickets} activeOpacity={0.88}>
                <Text className={s.btnPrimaryText}>GET TICKETS</Text>
              </TouchableOpacity>
            )}
            {/* ── Follow ──────────────────────────────────────────────────
                Full width, matching GET TICKETS above it.

                SHARE was removed from here rather than relocated: the header
                already carries a share control at the top right of the hero,
                so this was the second one on the same screen. Sharing it
                twice does not make it more likely, and the pair of half-width
                buttons made FOLLOW read as the lesser of two secondary
                actions when it is the only one.

                Labelled FOLLOW EVENT, not FOLLOW, because the organiser row
                immediately above now leads to a page with its own follow
                button. Two unlabelled follows on adjacent surfaces would be
                ambiguous about what is being followed. */}
            <TouchableOpacity
              className={s.btnOutlineWide}
              onPress={handleFollow}
              activeOpacity={0.85}
              disabled={followLoading}
              style={isFollowing
                ? { backgroundColor: '#2d1c3a', borderColor: '#b57bee' }
                : undefined}
            >
              <Heart
                size={17}
                color={isFollowing ? '#b57bee' : '#FFFFFF'}
                weight={isFollowing ? 'fill' : 'regular'}
              />
              <Text
                className={s.btnOutlineText}
                style={isFollowing ? { color: '#b57bee' } : undefined}
              >
                {followLoading ? '...' : isFollowing ? 'FOLLOWING' : 'FOLLOW EVENT'}
              </Text>
            </TouchableOpacity>
          </View>
        </View>

        {/* ── Content ──────────────────────────────────────────────── */}
        <View className={s.content} style={{ marginTop: 28 }}>

          {/* About */}
          <View style={{ marginBottom: 8 }}>
            <View className={s.sectionHeader}>
              <Text className={s.sectionTitle} style={{ color: colors.text }}>About This Event</Text>
            </View>
            <Text className={s.descText} style={{ color: colors.subText }}>{displayDesc}</Text>
            {hasMore && (
              <TouchableOpacity onPress={() => setExpanded(v => !v)} activeOpacity={0.7}>
                <Text className={s.descReadMore} style={{ color: colors.text, borderBottomColor: colors.text }}>
                  {expanded ? 'SHOW LESS ▴' : 'READ MORE'}
                </Text>
              </TouchableOpacity>
            )}
          </View>

          <View className={s.divider} style={{ backgroundColor: 'transparent' }} />

          {/* Teams */}
          {isASport && (lineupTeams.length > 0 || teams.length > 0) && (
            <>
              <View style={{ marginBottom: 8 }}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle} style={{ color: colors.text }}>TEAMS</Text>
                </View>
                {(lineupTeams.length > 0
                  ? lineupTeams
                  : teams.map((name: string) => ({ id: name, name }))
                ).map((entry: any, i: number, arr: any[]) => (
                  <LineupRow
                    key={entry.id ?? i}
                    entry={entry}
                    isLast={i === arr.length - 1}
                    colors={colors}
                  />
                ))}
              </View>
              <View className={s.divider} style={{ backgroundColor: 'transparent' }} />
            </>
          )}

          <View className={s.divider} style={{ backgroundColor: 'transparent' }} />

          {/* Performers */}
          {!isASport && (lineupPerformers.length > 0 || performers.length > 0) && (
            <>
              <View style={{ marginBottom: 8 }}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle} style={{ color: colors.text }}>PERFORMERS</Text>
                </View>
                {(lineupPerformers.length > 0
                  ? lineupPerformers
                  : performers.map((name: string) => ({ id: name, name }))
                ).map((entry: any, i: number, arr: any[]) => (
                  <LineupRow
                    key={entry.id ?? i}
                    entry={entry}
                    isLast={i === arr.length - 1}
                    colors={colors}
                  />
                ))}
              </View>
            </>
          )}

          {/* TAGS REMOVED.
              Tags are not decorative metadata — the events_search_trigger in
              migration 000004 folds them into search_vector at weight B, the
              same weight as the description. They are invisible SEARCH
              KEYWORDS that were mistakenly being rendered as a visible list of
              untappable rows.
              events.tags is untouched and still feeds search. Only the display
              is gone. */}

          {/* Information */}
          {infoItems.length > 0 && (
            <>
              <View style={{ marginBottom: 8 }}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle} style={{ color: colors.text }}>INFORMATION</Text>
                </View>
                {infoItems.map((item: any, i: number) => (
                  <View
                    key={item.key ?? i}
                    className={i < infoItems.length - 1 ? s.infoRow : s.infoRowLast}
                    style={i < infoItems.length - 1 ? { borderBottomColor: colors.dividerColor } : undefined}
                  >
                    <Text className={s.infoLabel} style={{ color: colors.subText }}>
                      {String(item.label ?? '').toUpperCase()}
                    </Text>
                    <Text className={s.infoValue} style={{ color: colors.text }}>{item.value}</Text>
                  </View>
                ))}
              </View>
              <View className={s.divider} style={{ backgroundColor: 'transparent' }} />
            </>
          )}

          {/* Location */}
          <View style={{ marginBottom: 8 }}>
            <View className={s.sectionHeader}>
              <Text className={s.sectionTitle} style={{ color: colors.text }}>LOCATION</Text>
            </View>
            <View className={s.locationCard}
              style={{ backgroundColor: colors.cardBg, borderColor: colors.dividerColor }}>
              <View className={s.locationMapPlaceholder}
                style={{ backgroundColor: isDark ? '#1a1a1a' : '#F0F0F0' }}>
                {/* Grid lines — neutral grey */}
                <View style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, opacity: 0.18 }}>
                  {[1,2,3,4,5,6].map(i => (
                    <View key={`h${i}`} style={{ position: 'absolute', left: 0, right: 0, top: `${i * 14}%` as any, height: 1, backgroundColor: '#999999' }} />
                  ))}
                  {[1,2,3,4,5,6].map(i => (
                    <View key={`v${i}`} style={{ position: 'absolute', top: 0, bottom: 0, left: `${i * 14}%` as any, width: 1, backgroundColor: '#999999' }} />
                  ))}
                </View>
                {/* Pin — dark, not red */}
                <View style={{
                  width: 44, height: 44, borderRadius: 22,
                  backgroundColor: isDark ? '#ffffff' : '#1A1A1A',
                  alignItems: 'center', justifyContent: 'center',
                  shadowColor: '#000', shadowOffset: { width: 0, height: 4 },
                  shadowOpacity: 0.25, shadowRadius: 8, elevation: 6,
                }}>
                  <MapPin size={24} color="#FF2D55" weight="fill" />
                </View>
              </View>
              <View className={s.locationInfoRow}>
                <View className={s.locationInfoLeft}>
                  <Text className={s.locationName} style={{ color: colors.text }}>{event.venue_name}</Text>
                  {ev.address ? <Text className={s.locationAddr} style={{ color: colors.subText }}>{ev.address}</Text> : null}
                  <Text className={s.locationAddr} style={{ color: colors.subText }}>{event.city}</Text>
                </View>
                <View className={s.locationBtnRow}>
                  {/* GET DIRECTIONS removed — it was a duplicate of OPEN IN MAPS
                      (both called handleDirections). */}
                  <TouchableOpacity className={s.locationBtnOutline} onPress={handleDirections} activeOpacity={0.8}>
                    <Text className={s.locationBtnOutlineText}>OPEN IN MAPS</Text>
                  </TouchableOpacity>
                </View>
              </View>
            </View>
          </View>

          {/* ── Policies ───────────────────────────────────────────────
              Below LOCATION on purpose. These are read at the point of
              deciding to buy or after something has gone wrong, not while
              browsing — putting them above the map would push the venue, which
              people check constantly, further down the scroll.

              refund_policy and terms_and_conditions are COLUMNS on the event,
              not catalogue items. They stay that way because the booking and
              refund flows reference them and they are the two fields most
              likely to be produced in a dispute — findable by name, not by
              string key. */}
          {(refundPolicy.length > 0 || termsAndConditions.length > 0) && (
            <>
              <View className={s.divider} style={{ backgroundColor: 'transparent' }} />
              <View style={{ marginBottom: 8 }}>
                {refundPolicy.length > 0 && (
                  <TouchableOpacity
                    className={termsAndConditions.length > 0 ? s.infoRow : s.infoRowLast}
                    style={termsAndConditions.length > 0 ? { borderBottomColor: colors.dividerColor } : undefined}
                    onPress={() => setPolicySheet('refund')}
                    activeOpacity={0.7}
                  >
                    <Text className={s.infoLabel} style={{ color: colors.subText }}>TICKET REFUND POLICY</Text>
                    <Text className={s.policyLink} style={{ color: '#FF2D55' }}>View policy ↗</Text>
                  </TouchableOpacity>
                )}
                {termsAndConditions.length > 0 && (
                  <TouchableOpacity
                    className={s.infoRowLast}
                    onPress={() => setPolicySheet('terms')}
                    activeOpacity={0.7}
                  >
                    <Text className={s.infoLabel} style={{ color: colors.subText }}>TERMS &amp; CONDITIONS</Text>
                    <Text className={s.policyLink} style={{ color: '#FF2D55' }}>View terms ↗</Text>
                  </TouchableOpacity>
                )}
              </View>
            </>
          )}

          <View className={s.bottomPad} />
        </View>
      </Animated.ScrollView>

      <PolicySheet
        visible={policySheet !== null}
        title={policySheet === 'terms' ? 'Terms & Conditions' : 'Ticket Refund Policy'}
        body={policySheet === 'terms' ? termsAndConditions : refundPolicy}
        onClose={() => setPolicySheet(null)}
      />
    </View>
  );
}