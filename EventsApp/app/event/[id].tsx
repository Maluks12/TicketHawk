import {
  View,
  Text,
  TouchableOpacity,
  Image,
  StatusBar,
  Share,
  Linking,
  Platform,
  Animated,
  InteractionManager,
  Alert,
} from 'react-native';
import { useRouter, useLocalSearchParams } from 'expo-router';
import { LinearGradient } from 'expo-linear-gradient';
import { useState, useRef, useEffect, useCallback } from 'react';
import { useEventStore } from '../../store/eventStore';
import { useAuthStore } from '../../store/authStore';
import { s, sheet, HERO_H } from '../../styles/eventDetail';
import { useAppTheme } from '../../theme/colors';
import { getBaseUrl } from '../../components/FilterModal';
import { toggleFollow, getFollowStatus } from '../../lib/api';

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

// ─── MetaItem ─────────────────────────────────────────────────────────────────

function MetaItem({ icon, label, value }: { icon: string; label: string; value: string }) {
  const { colors, isDark } = useAppTheme();
  return (
    <View className={s.metaItemRow}>
      <View className={s.metaIconBox} style={{ backgroundColor: colors.sectionCardBg }}>
        <Text className={s.metaIconText}>{icon}</Text>
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
  const scrollY = useRef(new Animated.Value(0)).current;

  // ── View tracking + fresh data fetch ──────────────────────────────────────
  // Fires once when the user opens this screen (after the nav animation).
  // 1. POST /view  — tells the backend a real user opened the screen.
  // 2. GET event   — fetches fresh data so the view count shown is up-to-date.
  useEffect(() => {
    const eventId = id ?? event?.id ?? event?.slug;
    if (!eventId) return;

    const base = getBaseUrl();

    const task = InteractionManager.runAfterInteractions(() => {
      // 1. Track the view — fire-and-forget, never blocks the UI.
      fetch(`${base}/v1/events/${eventId}/view`, { method: 'POST' }).catch(() => {});

      // 2. Fetch fresh event data so views (and anything else) reflect reality.
      fetch(`${base}/v1/events/${eventId}`)
        .then((res) => { if (!res.ok) return; return res.json(); })
        .then((fresh) => { if (fresh) setSelectedEvent(fresh); })
        .catch(() => {});
    });

    return () => task.cancel();
  }, [id, event?.id]);

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
  }, [event?.id, authUser]);

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

  if (!event) {
    return (
      <View className="flex-1 items-center justify-center" style={{ backgroundColor: colors.background }}>
        <Text style={{ color: colors.subText, fontWeight: '800', fontSize: 12, letterSpacing: 4 }}>
          EVENT NOT FOUND
        </Text>
        <TouchableOpacity
          onPress={() => router.back()}
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
  const heroImage    = event.banner_image_url ?? event.cover_image_url;
  const maxCap       = ev.max_capacity ?? 0;
  const isSoldOut    = event.is_sold_out ?? false;
  const isASport     = event.category?.toLowerCase() === 'sports';
  const teams:      string[] = ev.teams      ?? [];
  const performers: string[] = ev.performers ?? [];

  // Routing unchanged — matches index.tsx push to /event/${event.id}
  const handleGetTickets = () => router.push('/event/purchase');

  const handleShare = async () => {
    try {
      await Share.share({
        message: `Check out ${event.title} at ${event.venue_name} on ${formatDate(event.start_time)}`,
      });
    } catch (_) {}
  };

  const handleDirections = async () => {
    const query     = encodeURIComponent(`${event.venue_name}, ${event.city}`);
    const nativeUrl = Platform.OS === 'ios' ? `maps://?q=${query}` : `geo:0,0?q=${query}`;
    const webUrl    = `https://www.google.com/maps/search/?api=1&query=${query}`;
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

  const organizerName    = ev.organizer_name ?? 'EVENT ORGANIZER';
  const organizerInitial = organizerName.charAt(0).toUpperCase();

  const PARALLAX_EXTRA  = Math.round(HERO_H * 0.35);
  const imageTranslateY = scrollY.interpolate({
    inputRange:  [-HERO_H, 0, HERO_H],
    outputRange: [-PARALLAX_EXTRA / 2, 0, PARALLAX_EXTRA * 0.6],
    extrapolate: 'clamp',
  });

  return (
    <View style={{ flex: 1, backgroundColor: colors.background }}>
      <StatusBar barStyle="light-content" />

      {/* ── Nav overlay ─────────────────────────────────────────────── */}
      <View style={sheet.navOverlay} pointerEvents="box-none">
        <TouchableOpacity style={sheet.navBtn} onPress={() => router.back()} activeOpacity={0.8}>
          <Text className={s.backIcon}>‹</Text>
        </TouchableOpacity>
        <TouchableOpacity style={sheet.navBtn} onPress={handleShare} activeOpacity={0.8}>
          <Text className={s.shareIcon}>↑</Text>
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
        <View style={sheet.heroContainer}>
          {heroImage ? (
            <Animated.Image
              source={{ uri: heroImage }}
              style={[sheet.heroImage, { transform: [{ translateY: imageTranslateY }] }]}
              resizeMode="cover"
            />
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
            <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: 12, alignItems: 'center', marginBottom: 10 }}>
              {/* TRENDING — #FF2D55 badge retained */}
              {event.is_featured && (
                <View style={sheet.trendingBadge}>
                  <Text style={sheet.trendingBadgeText}>TRENDING #1</Text>
                </View>
              )}
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4 }}>
                <Text style={{ color: 'rgba(255,255,255,0.85)', fontSize: 13 }}>👁</Text>
                <Text style={sheet.viewsText}>
                  {event.views?.toLocaleString('en-ZA') ?? '0'} VIEWS
                </Text>
              </View>
            </View>
            {/* Category — #FF2D55 text on dark, not a fill */}
            <Text style={sheet.heroCategory}>✦ {event.category}</Text>
            <Text style={sheet.heroTitle} numberOfLines={3}>
              {event.title.toUpperCase()}
            </Text>
          </View>
        </View>

        {/* ── Floating action card ─────────────────────────────────── */}
        <View style={[sheet.actionCard, { backgroundColor: colors.cardBg }]}>
          <MetaItem icon="📅" label="DATE"     value={formatDateShort(event.start_time)} />
          <MetaItem icon="🕐" label="TIME"     value={formatTime(event.start_time)} />
          <MetaItem icon="📍" label="VENUE"    value={`${event.venue_name}, ${event.city}`} />
          {maxCap > 0 && (
            <MetaItem icon="👥" label="CAPACITY" value={`${maxCap.toLocaleString('en-ZA')} Seats`} />
          )}
          <View style={[sheet.metaDivider, { backgroundColor: colors.dividerColor }]} />
          <View className={s.ctaWrap}>
            {isSoldOut ? (
              <TouchableOpacity className={s.btnSoldOut} disabled>
                <Text className={s.btnSoldOutText}>🎟  SOLD OUT</Text>
              </TouchableOpacity>
            ) : (
              /* GET TICKETS — only full #FF2D55 fill in the whole screen */
              <TouchableOpacity className={s.btnPrimary} onPress={handleGetTickets} activeOpacity={0.88}>
                <Text className={s.btnPrimaryText}>GET TICKETS</Text>
              </TouchableOpacity>
            )}
            <View className={s.actionRow}>

              {/* ── Follow button ──────────────────────────────────────────── */}
              <TouchableOpacity
                className={s.btnOutline}
                onPress={handleFollow}
                activeOpacity={0.85}
                disabled={followLoading}
                style={isFollowing
                  ? { backgroundColor: '#2d1c3a', borderColor: '#b57bee' }
                  : undefined}
              >
                <Text className={s.btnOutlineIcon}>
                  {isFollowing ? '💜' : '🤍'}
                </Text>
                <Text
                  className={s.btnOutlineText}
                  style={isFollowing ? { color: '#b57bee' } : undefined}
                >
                  {followLoading ? '...' : isFollowing ? 'FOLLOWING' : 'FOLLOW'}
                </Text>
              </TouchableOpacity>

              <TouchableOpacity className={s.btnOutline} onPress={handleShare} activeOpacity={0.85}>
                <Text className={s.btnOutlineIcon}>↑</Text>
                <Text className={s.btnOutlineText}>SHARE</Text>
              </TouchableOpacity>
            </View>
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

          {/* Bento */}
          <View style={{ marginBottom: 8 }}>
            <View className={s.bentoGrid}>
              <View className={s.bentoCard}
                style={{ backgroundColor: colors.sectionCardBg, borderColor: colors.dividerColor }}>
                <Text className={s.bentoIcon}>🎟</Text>
                <Text className={s.bentoLabel} style={{ color: colors.subText }}>STATUS</Text>
                <Text className={s.bentoValue} style={{ color: colors.text }}>...</Text>
              </View>
              <View className={s.bentoCard}
                style={{ backgroundColor: colors.sectionCardBg, borderColor: colors.dividerColor }}>
                <Text className={s.bentoIcon}>✅</Text>
                <Text className={s.bentoLabel} style={{ color: colors.subText }}>TYPE</Text>
                <Text className={s.bentoValue} style={{ color: colors.text }}>{event.category}</Text>
              </View>
            </View>
          </View>

          <View className={s.divider} style={{ backgroundColor: 'transparent' }} />

          {/* Teams */}
          {isASport && teams.length > 0 && (
            <>
              <View style={{ marginBottom: 8 }}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle} style={{ color: colors.text }}>TEAMS</Text>
                </View>
                {teams.map((team, i) => (
                  <View key={i} className={i < teams.length - 1 ? s.participantRow : s.participantRowLast}
                    style={i < teams.length - 1 ? { borderBottomColor: colors.dividerColor } : undefined}>
                    <View className={s.participantAvatar}
                      style={{ backgroundColor: colors.sectionCardBg, borderColor: colors.dividerColor }}>
                      <Text className={s.participantAvatarText} style={{ color: colors.text }}>
                        {team.trim().charAt(0).toUpperCase()}
                      </Text>
                    </View>
                    <Text className={s.participantName} style={{ color: colors.text }}>{team}</Text>
                    <Text className={s.participantArrow} style={{ color: colors.subText }}>›</Text>
                  </View>
                ))}
              </View>
              <View className={s.divider} style={{ backgroundColor: 'transparent' }} />
            </>
          )}

          <View className={s.divider} style={{ backgroundColor: 'transparent' }} />

          {/* Performers */}
          {!isASport && performers.length > 0 && (
            <>
              <View style={{ marginBottom: 8 }}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle} style={{ color: colors.text }}>PERFORMERS</Text>
                </View>
                {performers.map((performer, i) => (
                  <View key={i} className={i < performers.length - 1 ? s.participantRow : s.participantRowLast}
                    style={i < performers.length - 1 ? { borderBottomColor: colors.dividerColor } : undefined}>
                    <View className={s.participantAvatar}
                      style={{ backgroundColor: colors.sectionCardBg, borderColor: colors.dividerColor }}>
                      <Text className={s.participantAvatarText} style={{ color: colors.text }}>
                        {performer.trim().charAt(0).toUpperCase()}
                      </Text>
                    </View>
                    <Text className={s.participantName} style={{ color: colors.text }}>{performer}</Text>
                    <Text className={s.participantArrow} style={{ color: colors.subText }}>›</Text>
                  </View>
                ))}
              </View>
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
                  <Text style={{ fontSize: 22 }}>📍</Text>
                </View>
              </View>
              <View className={s.locationInfoRow}>
                <View className={s.locationInfoLeft}>
                  <Text className={s.locationName} style={{ color: colors.text }}>{event.venue_name}</Text>
                  {ev.address ? <Text className={s.locationAddr} style={{ color: colors.subText }}>{ev.address}</Text> : null}
                  <Text className={s.locationAddr} style={{ color: colors.subText }}>{event.city}</Text>
                </View>
                <View className={s.locationBtnRow}>
                  <TouchableOpacity className={s.locationBtnFilled} onPress={handleDirections} activeOpacity={0.8}>
                    <Text className={s.locationBtnFilledText}>GET DIRECTIONS</Text>
                  </TouchableOpacity>
                  <TouchableOpacity className={s.locationBtnOutline} onPress={handleDirections} activeOpacity={0.8}>
                    <Text className={s.locationBtnOutlineText}>OPEN IN MAPS</Text>
                  </TouchableOpacity>
                </View>
              </View>
            </View>
          </View>

          <View className={s.divider} style={{ backgroundColor: 'transparent' }} />

          {/* Organizer */}
          <View className={s.organizerCard}
            style={{ backgroundColor: colors.cardBg, borderColor: colors.dividerColor, marginBottom: 8 }}>
            <View className={s.organizerLeft}>
              <View className={s.organizerAvatar}
                style={{ backgroundColor: colors.sectionCardBg, borderColor: colors.dividerColor }}>
                {ev.organizer_avatar ? (
                  <Image source={{ uri: ev.organizer_avatar }} style={{ width: 56, height: 56, borderRadius: 28 }} resizeMode="cover" />
                ) : (
                  <Text className={s.organizerAvatarText} style={{ color: colors.text }}>{organizerInitial}</Text>
                )}
              </View>
              <View style={{ flex: 1 }}>
                <Text className={s.organizerByLabel} style={{ color: colors.subText }}>Organized by</Text>
                <Text className={s.organizerName} style={{ color: colors.text }} numberOfLines={2}>{organizerName}</Text>
              </View>
            </View>
            <TouchableOpacity style={{ width: 40, height: 40, alignItems: 'center', justifyContent: 'center' }} activeOpacity={0.7}>
              <Text style={{ fontSize: 22 }}>💬</Text>
            </TouchableOpacity>
          </View>

          <View className={s.bottomPad} />
        </View>
      </Animated.ScrollView>
    </View>
  );
}