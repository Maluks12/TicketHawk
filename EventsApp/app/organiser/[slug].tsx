import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  ActivityIndicator,
  StatusBar,
  StyleSheet,
  Dimensions,
  Linking,
  Alert,
} from 'react-native';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useState, useEffect, useCallback } from 'react';
import { LinearGradient } from 'expo-linear-gradient';
import { Image as ExpoImage } from 'expo-image';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { CheckCircle, Storefront } from 'phosphor-react-native';
import { SocialIcon, SOCIAL_LABEL } from '../../components/SocialIcon';
import { IMG_PLACEHOLDER } from '../../lib/img';
import {
  getOrganizer,
  getOrganizerEvents,
  toggleOrganizerFollow,
  type Event,
  type PublicOrganizer,
} from '../../lib/api';
import { useEventStore } from '../../store/eventStore';
import { useAuthStore } from '../../store/authStore';
import { useAppTheme } from '../../theme/colors';
import { CoverImage } from '../../components/CoverImage';

/**
 * Organiser profile.
 *
 * Reached from the "ORGANISED BY" row on an event, which replaced the Capacity
 * row. Lists everything they have on sale plus everything they have run.
 *
 * The socials live HERE and not on the event page. An event page is about one
 * event, and a row of outbound links on it competes with the only action that
 * matters there — buying a ticket. Someone who wants to know who is behind the
 * event has already tapped through to this screen, which is where a link off
 * the platform costs nothing.
 */

const { width: SCREEN_W } = Dimensions.get('window');
const CARD_IMAGE_H = Math.round(SCREEN_W * 0.52);
const CARD_RADIUS  = 14;

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatPrice(price: number) {
  if (!price || price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

function formatCardDate(iso?: string | null) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-ZA', {
    weekday: 'short', day: 'numeric', month: 'short', year: 'numeric',
  });
}

function formatTime(iso?: string | null) {
  if (!iso) return '';
  return new Date(iso).toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
}

function compactCount(n: number) {
  if (n >= 1000) return `${(n / 1000).toFixed(n >= 10000 ? 0 : 1)}k`;
  return String(n);
}

// ─── Card ─────────────────────────────────────────────────────────────────────
// Same shape as CategoryEventCard so an organiser's events look identical to
// the same events anywhere else in the app.

function OrganiserEventCard({ item, onPress, dimmed }: { item: Event; onPress: () => void; dimmed?: boolean }) {
  return (
    <TouchableOpacity onPress={onPress} style={[styles.card, dimmed && { opacity: 0.62 }]} activeOpacity={0.88}>
      <View style={styles.cardImageWrap}>
        {item.cover_image_url ? (
          <CoverImage uri={item.cover_image_url} style={StyleSheet.absoluteFillObject} />
        ) : (
          <View style={[StyleSheet.absoluteFillObject, { backgroundColor: '#2a2a2a' }]} />
        )}
        <LinearGradient
          colors={['transparent', 'rgba(0,0,0,0.35)', 'rgba(0,0,0,0.86)']}
          locations={[0.3, 0.62, 1]}
          style={StyleSheet.absoluteFillObject}
        />
        <View style={styles.priceBadge}>
          <Text style={styles.priceBadgeText}>{formatPrice(item.price)}</Text>
        </View>
        <View style={styles.cardBody}>
          <Text style={styles.cardTitle} numberOfLines={2}>{item.title}</Text>
          <Text style={styles.cardMeta} numberOfLines={1}>
            {formatCardDate(item.start_time)}{item.start_time ? `  ·  ${formatTime(item.start_time)}` : ''}
          </Text>
          {item.venue_name ? (
            <Text style={styles.cardVenue} numberOfLines={1}>
              {item.venue_name}{item.city ? `, ${item.city}` : ''}
            </Text>
          ) : null}
        </View>
      </View>
    </TouchableOpacity>
  );
}

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function OrganiserScreen() {
  const { slug }  = useLocalSearchParams<{ slug: string }>();
  const router    = useRouter();
  const insets    = useSafeAreaInsets();
  const { colors, isDark } = useAppTheme();
  const { setSelectedEvent } = useEventStore();
  const authUser = useAuthStore((st) => st.user);

  const [organiser, setOrganiser] = useState<PublicOrganizer | null>(null);
  const [events,    setEvents]    = useState<Event[]>([]);
  const [scope,     setScope]     = useState<'upcoming' | 'past'>('upcoming');
  const [loading,   setLoading]   = useState(true);
  const [eventsLoading, setEventsLoading] = useState(false);
  const [error,     setError]     = useState<string | null>(null);

  const [following,     setFollowing]     = useState(false);
  const [followerCount, setFollowerCount] = useState(0);
  const [followBusy,    setFollowBusy]    = useState(false);

  const load = useCallback(async () => {
    if (!slug) return;
    setError(null);
    try {
      const { data } = await getOrganizer(slug);
      setOrganiser(data);
      setFollowerCount(data.follower_count ?? 0);
      // `is_following` is ABSENT (not false) when the request carried no valid
      // token, so `=== true` keeps a logged-out visitor at false without
      // treating "unknown" as "not following".
      setFollowing(data.is_following === true);
    } catch {
      // A 404 here is the public gate doing its job — the organiser is below
      // KYC level 2, has switched their page off, or is suspended. The screen
      // says the same thing either way, because which of those it is is not
      // the visitor's business.
      setError('This organiser page is not available.');
    } finally {
      setLoading(false);
    }
  }, [slug]);

  const loadEvents = useCallback(async (which: 'upcoming' | 'past') => {
    if (!slug) return;
    setEventsLoading(true);
    try {
      const { data } = await getOrganizerEvents(slug, which);
      setEvents(data.data ?? []);
    } catch {
      setEvents([]);
    } finally {
      setEventsLoading(false);
    }
  }, [slug]);

  useEffect(() => { load(); }, [load]);
  useEffect(() => { if (organiser) loadEvents(scope); }, [organiser, scope, loadEvents]);

  async function handleFollow() {
    if (!organiser) return;

    // ── This used to router.push('/auth/login') and that was the bug ────────
    //
    // Tapping FOLLOW navigated away, and if the auth store had not finished
    // hydrating yet it navigated a logged-IN user away too — which is what
    // produced the unmatched-route screen.
    //
    // Now it never navigates on its own. The same Alert the event follow
    // button uses offers login as a CHOICE, so the tap does what it says on
    // the button and nothing else. Reading authUser (the object) rather than a
    // derived boolean also means a null during hydration reads as "not loaded"
    // consistently with every other screen.
    if (!authUser) {
      Alert.alert(
        'Sign in required',
        'Log in or sign up to follow organisers and hear about new events first.',
        [
          { text: 'Cancel', style: 'cancel' },
          { text: 'Log in / Sign up', onPress: () => router.push('/auth/login' as any) },
        ],
      );
      return;
    }
    if (followBusy) return;

    // Optimistic. A follow toggle that waits on the network feels broken on a
    // slow connection, and the reconciliation below is cheap.
    const wasFollowing = following;
    setFollowing(!wasFollowing);
    setFollowerCount((c) => c + (wasFollowing ? -1 : 1));
    setFollowBusy(true);
    try {
      const res = await toggleOrganizerFollow(slug);
      setFollowing(res.following);
      setFollowerCount(res.follower_count);
    } catch {
      setFollowing(wasFollowing);
      setFollowerCount((c) => c + (wasFollowing ? 1 : -1));
    } finally {
      setFollowBusy(false);
    }
  }

  function openEvent(e: Event) {
    setSelectedEvent(e);
    router.push(`/event/${e.id}` as any);
  }

  if (loading) {
    return (
      <View style={[styles.centreWrap, { flex: 1, backgroundColor: colors.background }]}>
        <ActivityIndicator color={colors.text} />
      </View>
    );
  }

  if (error || !organiser) {
    return (
      <View style={{ flex: 1, backgroundColor: colors.background, paddingTop: insets.top }}>
        <View style={styles.header}>
          <TouchableOpacity style={styles.backBtn} onPress={() => router.back()} hitSlop={12}>
            <Text style={[styles.backIcon, { color: colors.text }]}>‹</Text>
          </TouchableOpacity>
        </View>
        <View style={styles.centreWrap}>
          <Text style={[styles.emptyTitle, { color: colors.text }]}>Not available</Text>
          <Text style={[styles.emptySub, { color: colors.subText }]}>{error}</Text>
        </View>
      </View>
    );
  }

  const links = organiser.links ?? [];

  const ListHeader = (
    <View>
      {/* Cover. Falls back to a flat block rather than collapsing, so the logo
          below always has something to sit against. */}
      <View style={styles.coverWrap}>
        {organiser.cover_image_url ? (
          <ExpoImage
            source={{ uri: organiser.cover_image_url }}
            style={StyleSheet.absoluteFillObject}
            contentFit="cover"
            cachePolicy="memory-disk"
            placeholder={IMG_PLACEHOLDER}
            placeholderContentFit="cover"
            recyclingKey={organiser.id}
            allowDownscaling
            transition={160}
          />
        ) : (
          <View style={[StyleSheet.absoluteFillObject, { backgroundColor: isDark ? '#1a1a1a' : '#E8E8E8' }]} />
        )}
        <LinearGradient
          colors={['transparent', 'rgba(0,0,0,0.25)', colors.background]}
          locations={[0.2, 0.6, 1]}
          style={StyleSheet.absoluteFillObject}
        />
      </View>

      <View style={styles.identityWrap}>
        <View style={[styles.logoBox, { backgroundColor: colors.cardBg, borderColor: colors.background }]}>
          {organiser.logo_url ? (
            /* Same cache policy and blur-up placeholder as CoverImage uses on
               every card in the app. Without cachePolicy expo-image re-fetches
               on each mount — for a logo that also renders in the ORGANISED BY
               row of every one of their events, that is a download per screen
               open. */
            <ExpoImage
              source={{ uri: organiser.logo_url }}
              style={{ width: '100%', height: '100%', borderRadius: 16 }}
              contentFit="cover"
              cachePolicy="memory-disk"
              placeholder={IMG_PLACEHOLDER}
              placeholderContentFit="cover"
              recyclingKey={organiser.id}
              allowDownscaling
              transition={140}
            />
          ) : (
            <Storefront size={30} color={colors.text} weight="regular" />
          )}
        </View>

        <View style={styles.nameRow}>
          <Text style={[styles.orgName, { color: colors.text }]} numberOfLines={2}>
            {organiser.business_name}
          </Text>
          {organiser.verified && <CheckCircle size={20} color="#2f7cf6" weight="fill" />}
        </View>

        <Text style={[styles.orgMeta, { color: colors.subText }]}>
          {organiser.verified ? 'Verified organiser · ' : ''}
          {organiser.events_hosted} event{organiser.events_hosted === 1 ? '' : 's'} hosted
          {organiser.member_since ? ` · since ${organiser.member_since}` : ''}
        </Text>

        {organiser.description ? (
          <Text style={[styles.orgBio, { color: colors.subText }]}>{organiser.description}</Text>
        ) : null}

        {/* Follow. Follower count sits inside the button rather than beside it
            so the number and the action that changes it are one target. */}
        <TouchableOpacity
          onPress={handleFollow}
          disabled={followBusy}
          activeOpacity={0.85}
          style={[
            styles.followBtn,
            following
              ? { backgroundColor: '#2d1c3a', borderColor: '#b57bee' }
              : { backgroundColor: 'transparent', borderColor: colors.dividerColor },
          ]}
        >
          <Text style={[styles.followText, { color: following ? '#b57bee' : colors.text }]}>
            {following ? 'FOLLOWING' : 'FOLLOW'}
            {followerCount > 0 ? `  ·  ${compactCount(followerCount)}` : ''}
          </Text>
        </TouchableOpacity>

        {/* Socials — on THIS screen, not the event page. */}
        {links.length > 0 && (
          <View style={styles.linkWrap}>
            {links.map((l) => (
              <TouchableOpacity
                key={l.platform}
                style={[styles.linkBtn, { backgroundColor: colors.sectionCardBg, borderColor: colors.dividerColor }]}
                onPress={() => { Linking.openURL(l.url).catch(() => {}); }}
                activeOpacity={0.75}
                // The icon carries no text, so it needs a label for anyone
                // using a screen reader.
                accessibilityRole="link"
                accessibilityLabel={`${organiser.business_name} on ${SOCIAL_LABEL[l.platform] ?? l.platform}`}
              >
                <SocialIcon platform={l.platform} size={19} color={colors.text} />
              </TouchableOpacity>
            ))}
          </View>
        )}
      </View>

      {/* Upcoming / Past */}
      <View style={[styles.scopeRow, { borderBottomColor: colors.dividerColor }]}>
        {(['upcoming', 'past'] as const).map((sc) => (
          <TouchableOpacity
            key={sc}
            onPress={() => setScope(sc)}
            style={[styles.scopeTab, scope === sc && { borderBottomColor: '#FF2D55' }]}
            activeOpacity={0.8}
          >
            <Text style={[styles.scopeText, { color: scope === sc ? colors.text : colors.subText }]}>
              {sc === 'upcoming' ? 'UPCOMING' : 'PAST'}
            </Text>
          </TouchableOpacity>
        ))}
      </View>
    </View>
  );

  return (
    <View style={{ flex: 1, backgroundColor: colors.background }}>
      <StatusBar barStyle={isDark ? 'light-content' : 'dark-content'} />

      {/* Back button floats over the cover rather than sitting in a bar, so the
          cover reaches the top of the screen. */}
      <TouchableOpacity
        style={[styles.floatingBack, { top: insets.top + 8 }]}
        onPress={() => router.back()}
        hitSlop={12}
      >
        <Text style={styles.floatingBackIcon}>‹</Text>
      </TouchableOpacity>

      <FlatList
        data={events}
        keyExtractor={(e) => e.id}
        ListHeaderComponent={ListHeader}
        renderItem={({ item }) => (
          <OrganiserEventCard
            item={item}
            dimmed={scope === 'past'}
            onPress={() => openEvent(item)}
          />
        )}
        contentContainerStyle={{ paddingBottom: insets.bottom + 40 }}
        ListEmptyComponent={
          eventsLoading ? (
            <View style={styles.centreWrap}><ActivityIndicator color={colors.text} /></View>
          ) : (
            <View style={styles.centreWrap}>
              <Text style={[styles.emptySub, { color: colors.subText }]}>
                {scope === 'upcoming'
                  ? 'Nothing on sale right now.'
                  : 'No past events to show.'}
              </Text>
            </View>
          )
        }
        showsVerticalScrollIndicator={false}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 16,
    paddingBottom: 12,
  },
  backBtn:  { width: 40, alignItems: 'center', justifyContent: 'center' },
  backIcon: { fontSize: 32, fontWeight: '300', lineHeight: 36, marginTop: -2 },

  floatingBack: {
    position: 'absolute',
    left: 12,
    zIndex: 20,
    width: 38,
    height: 38,
    borderRadius: 19,
    backgroundColor: 'rgba(0,0,0,0.45)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  floatingBackIcon: { color: '#fff', fontSize: 28, fontWeight: '300', lineHeight: 30, marginTop: -2 },

  coverWrap: { width: '100%', height: Math.round(SCREEN_W * 0.42) },

  identityWrap: { paddingHorizontal: 16, marginTop: -36 },
  logoBox: {
    width: 72, height: 72, borderRadius: 16, borderWidth: 3,
    alignItems: 'center', justifyContent: 'center', overflow: 'hidden',
    marginBottom: 12,
  },
  nameRow:  { flexDirection: 'row', alignItems: 'center', gap: 7 },
  orgName:  { fontSize: 22, fontWeight: '800', letterSpacing: -0.3, flexShrink: 1 },
  orgMeta:  { fontSize: 12.5, fontWeight: '500', marginTop: 5 },
  orgBio:   { fontSize: 14, lineHeight: 21, marginTop: 12 },

  followBtn: {
    marginTop: 16, borderWidth: 1, borderRadius: 12,
    paddingVertical: 13, alignItems: 'center',
  },
  followText: { fontSize: 12, fontWeight: '800', letterSpacing: 1.2 },

  linkWrap:  { flexDirection: 'row', flexWrap: 'wrap', gap: 10, marginTop: 16 },
  // Square-ish icon buttons rather than text pills. A row of nine word-chips
  // read as navigation; the marks read as what they are.
  linkBtn:   { width: 44, height: 44, borderRadius: 13, borderWidth: 1, alignItems: 'center', justifyContent: 'center' },

  scopeRow:  { flexDirection: 'row', gap: 24, paddingHorizontal: 16, marginTop: 24, borderBottomWidth: 1 },
  scopeTab:  { paddingVertical: 12, borderBottomWidth: 2, borderBottomColor: 'transparent' },
  scopeText: { fontSize: 12, fontWeight: '800', letterSpacing: 1.1 },

  card: {
    marginHorizontal: 16,
    marginTop: 16,
    borderRadius: CARD_RADIUS,
    overflow: 'hidden',
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 3 },
    shadowOpacity: 0.18,
    shadowRadius: 10,
    elevation: 5,
  },
  cardImageWrap: {
    width: '100%',
    height: CARD_IMAGE_H,
    backgroundColor: '#2a2a2a',
    borderRadius: CARD_RADIUS,
    overflow: 'hidden',
  },
  priceBadge: {
    position: 'absolute', top: 12, right: 12,
    backgroundColor: 'rgba(0,0,0,0.60)',
    borderRadius: 6, paddingHorizontal: 10, paddingVertical: 5,
  },
  priceBadgeText: { color: '#fff', fontSize: 12, fontWeight: '700', letterSpacing: 0.3 },

  cardBody: { position: 'absolute', bottom: 0, left: 0, right: 0, padding: 14, paddingBottom: 16 },
  cardTitle: {
    fontSize: 16, fontWeight: '800', marginBottom: 4, lineHeight: 21, color: '#ffffff',
    textShadowColor: 'rgba(0,0,0,0.55)', textShadowOffset: { width: 0, height: 1 }, textShadowRadius: 4,
  },
  cardMeta: {
    fontSize: 12, fontWeight: '500', marginBottom: 2, color: 'rgba(255,255,255,0.82)',
    textShadowColor: 'rgba(0,0,0,0.55)', textShadowOffset: { width: 0, height: 1 }, textShadowRadius: 3,
  },
  cardVenue: {
    fontSize: 12, fontWeight: '400', color: 'rgba(255,255,255,0.68)',
    textShadowColor: 'rgba(0,0,0,0.55)', textShadowOffset: { width: 0, height: 1 }, textShadowRadius: 3,
  },

  centreWrap: { alignItems: 'center', justifyContent: 'center', padding: 32 },
  emptyTitle: { fontSize: 18, fontWeight: '700', marginBottom: 8, textAlign: 'center' },
  emptySub:   { fontSize: 13, textAlign: 'center', lineHeight: 19 },
});