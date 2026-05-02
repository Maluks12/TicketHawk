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
} from 'react-native';
import { useRouter } from 'expo-router';
import { LinearGradient } from 'expo-linear-gradient';
import { useState, useRef } from 'react';
import { useEventStore } from '../../store/eventStore';
import { s, sheet, HERO_H } from '../../styles/eventDetail';

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
  return (
    <View className={s.metaItemRow}>
      <View className={s.metaIconBox}>
        <Text className={s.metaIconText}>{icon}</Text>
      </View>
      <View>
        <Text className={s.metaLabelText}>{label}</Text>
        <Text className={s.metaValueText}>{value}</Text>
      </View>
    </View>
  );
}

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function EventDetail() {
  const router = useRouter();
  const event  = useEventStore((state) => state.selectedEvent);
  const [expanded, setExpanded] = useState(false);

  const scrollY = useRef(new Animated.Value(0)).current;

  if (!event) {
    return (
      <View className="flex-1 bg-white items-center justify-center">
        <Text className="text-[#888888] font-bold text-sm tracking-widest">
          EVENT NOT FOUND
        </Text>
        <TouchableOpacity
          onPress={() => router.back()}
          className="mt-4 border border-[#DDDDDD] px-5 py-2.5 rounded-xl"
        >
          <Text className="text-[#111111] text-[10px] font-extrabold tracking-[1.5px]">
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
    <View style={{ flex: 1, backgroundColor: '#FFFFFF' }}>
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
        <View style={sheet.actionCard}>
          <MetaItem icon="📅" label="DATE"     value={formatDateShort(event.start_time)} />
          <MetaItem icon="🕐" label="TIME"     value={formatTime(event.start_time)} />
          <MetaItem icon="📍" label="VENUE"    value={`${event.venue_name}, ${event.city}`} />
          {maxCap > 0 && (
            <MetaItem icon="👥" label="CAPACITY" value={`${maxCap.toLocaleString('en-ZA')} Seats`} />
          )}
          <View style={sheet.metaDivider} />
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
              <TouchableOpacity className={s.btnOutline} activeOpacity={0.85}>
                <Text className={s.btnOutlineIcon}>🔔</Text>
                <Text className={s.btnOutlineText}>TRACK EVENT</Text>
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
              <Text className={s.sectionTitle}>About This Event</Text>
            </View>
            <Text className={s.descText}>{displayDesc}</Text>
            {hasMore && (
              <TouchableOpacity onPress={() => setExpanded(v => !v)} activeOpacity={0.7}>
                <Text className={s.descReadMore}>{expanded ? 'SHOW LESS ▴' : 'READ MORE'}</Text>
              </TouchableOpacity>
            )}
          </View>

          <View className={s.divider} />

          {/* Bento */}
          <View style={{ marginBottom: 8 }}>
            <View className={s.bentoGrid}>
              <View className={s.bentoCard}>
                <Text className={s.bentoIcon}>🎟</Text>
                <Text className={s.bentoLabel}>STATUS</Text>
                <Text className={s.bentoValue}>
                  {isSoldOut ? 'Sold Out' : (event.available_tickets ?? 0) < 50 ? 'Limited\nAvailable' : 'Available'}
                </Text>
              </View>
              <View className={s.bentoCard}>
                <Text className={s.bentoIcon}>✅</Text>
                <Text className={s.bentoLabel}>TYPE</Text>
                <Text className={s.bentoValue}>{event.category}</Text>
              </View>
            </View>
          </View>

          <View className={s.divider} />

          {/* Teams */}
          {isASport && teams.length > 0 && (
            <>
              <View style={{ marginBottom: 8 }}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle}>TEAMS</Text>
                </View>
                {teams.map((team, i) => (
                  <View key={i} className={i < teams.length - 1 ? s.participantRow : s.participantRowLast}>
                    <View className={s.participantAvatar}>
                      <Text className={s.participantAvatarText}>{team.trim().charAt(0).toUpperCase()}</Text>
                    </View>
                    <Text className={s.participantName}>{team}</Text>
                    <Text className={s.participantArrow}>›</Text>
                  </View>
                ))}
              </View>
              <View className={s.divider} />
            </>
          )}

          {/* Performers */}
          {!isASport && performers.length > 0 && (
            <>
              <View style={{ marginBottom: 8 }}>
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle}>PERFORMERS</Text>
                </View>
                {performers.map((performer, i) => (
                  <View key={i} className={i < performers.length - 1 ? s.participantRow : s.participantRowLast}>
                    <View className={s.participantAvatar}>
                      <Text className={s.participantAvatarText}>{performer.trim().charAt(0).toUpperCase()}</Text>
                    </View>
                    <Text className={s.participantName}>{performer}</Text>
                    <Text className={s.participantArrow}>›</Text>
                  </View>
                ))}
              </View>
              <View className={s.divider} />
            </>
          )}

          {/* Location */}
          <View style={{ marginBottom: 8 }}>
            <View className={s.sectionHeader}>
              <Text className={s.sectionTitle}>LOCATION</Text>
            </View>
            <View className={s.locationCard}>
              <View className={s.locationMapPlaceholder}>
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
                  backgroundColor: '#1A1A1A',
                  alignItems: 'center', justifyContent: 'center',
                  shadowColor: '#000', shadowOffset: { width: 0, height: 4 },
                  shadowOpacity: 0.25, shadowRadius: 8, elevation: 6,
                }}>
                  <Text style={{ fontSize: 22 }}>📍</Text>
                </View>
              </View>
              <View className={s.locationInfoRow}>
                <View className={s.locationInfoLeft}>
                  <Text className={s.locationName}>{event.venue_name}</Text>
                  {ev.address ? <Text className={s.locationAddr}>{ev.address}</Text> : null}
                  <Text className={s.locationAddr}>{event.city}</Text>
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

          <View className={s.divider} />

          {/* Organizer */}
          <View className={s.organizerCard} style={{ marginBottom: 8 }}>
            <View className={s.organizerLeft}>
              <View className={s.organizerAvatar}>
                {ev.organizer_avatar ? (
                  <Image source={{ uri: ev.organizer_avatar }} style={{ width: 56, height: 56, borderRadius: 28 }} resizeMode="cover" />
                ) : (
                  <Text className={s.organizerAvatarText}>{organizerInitial}</Text>
                )}
              </View>
              <View style={{ flex: 1 }}>
                <Text className={s.organizerByLabel}>Organized by</Text>
                <Text className={s.organizerName} numberOfLines={2}>{organizerName}</Text>
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