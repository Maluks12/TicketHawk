import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  Image,
  StatusBar,
  Share,
  Linking,
} from 'react-native';
import { useRouter } from 'expo-router';
import { LinearGradient } from 'expo-linear-gradient';
import { useState } from 'react';
import { useEventStore } from '../../store/eventStore';
import { s, sheet } from '../../styles/eventDetail';

// ─── Helpers ─────────────────────────────────────────────────────────────────

function formatPrice(price: number, currency: string) {
  if (price === 0) return 'Free';
  return `R${Math.round(price).toLocaleString('en-ZA')}`;
}

function formatDate(iso: string) {
  const d = new Date(iso);
  return d.toLocaleDateString('en-ZA', {
    weekday: 'short',
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  });
}

function formatTime(iso: string) {
  const d = new Date(iso);
  return d.toLocaleTimeString('en-ZA', { hour: '2-digit', minute: '2-digit' });
}

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function EventDetail() {
  const router = useRouter();
  const event  = useEventStore((state) => state.selectedEvent);
  const [expanded, setExpanded] = useState(false);

  if (!event) {
    return (
      <View className="flex-1 bg-white items-center justify-center">
        <Text className="text-black/30 font-bold text-sm tracking-widest">
          EVENT NOT FOUND
        </Text>
        <TouchableOpacity
          onPress={() => router.back()}
          className="mt-4 border border-black/15 px-5 py-2.5 rounded-[2px]"
        >
          <Text className="text-black text-[10px] font-extrabold tracking-[1.5px]">
            ← GO BACK
          </Text>
        </TouchableOpacity>
      </View>
    );
  }

  // Access extra API fields not in the base Event type
  const ev           = event as any;
  const heroImage    = event.banner_image_url ?? event.cover_image_url;
  const maxCap       = ev.max_capacity ?? 0;
  const isSoldOut    = event.is_sold_out ?? false;
  const isASport     = event.category?.toLowerCase() === 'sports';

  // Teams shown for sports events, performers for everything else
  const teams:      string[] = ev.teams      ?? [];
  const performers: string[] = ev.performers ?? [];

  const handleShare = async () => {
    try {
      await Share.share({
        message: `Check out ${event.title} at ${event.venue_name} on ${formatDate(event.start_time)}`,
      });
    } catch (_) {}
  };

  const handleDirections = async () => {
    const query     = encodeURIComponent(`${event.venue_name}, ${event.city}`);
    const url       = `https://maps.google.com/?q=${query}`;
    const supported = await Linking.canOpenURL(url);
    if (supported) {
      await Linking.openURL(url);
    } else {
      await Linking.openURL(`https://www.google.com/search?q=${query}`);
    }
  };

  const shortDesc   = event.description?.slice(0, 160);
  const longDesc    = event.description ?? '';
  const hasMore     = longDesc.length > 160;
  const displayDesc = expanded ? longDesc : (shortDesc + (hasMore ? '…' : ''));

  return (
    <View className={s.screen}>
      <StatusBar barStyle="light-content" />

      {/* ── Hero ────────────────────────────────────────────────────── */}
      <View className={s.hero}>
        {heroImage ? (
          <Image source={{ uri: heroImage }} style={sheet.absoluteFill} resizeMode="cover" />
        ) : (
          <View className={s.heroImagePlaceholder} />
        )}

        <LinearGradient
          colors={['transparent', 'rgba(0,0,0,0.55)', 'rgba(0,0,0,0.75)']}
          locations={[0.3, 0.65, 1]}
          style={sheet.absoluteFill}
        />
        <LinearGradient
          colors={['rgba(0,0,0,0.45)', 'transparent']}
          locations={[0, 1]}
          style={[sheet.absoluteFill, { bottom: undefined, height: 130 }]}
        />

        {/* Back */}
        <TouchableOpacity className={s.backBtn} onPress={() => router.back()}>
          <Text className={s.backIcon}>‹</Text>
        </TouchableOpacity>

        {/* Share */}
        <TouchableOpacity className={s.shareBtn} onPress={handleShare}>
          <Text className={s.shareIcon}>↑</Text>
        </TouchableOpacity>

        {/* Category + title */}
        <View className="absolute bottom-5 left-5 right-5">
          <Text className="text-[10px] font-extrabold text-[#ff3c6e] tracking-[2px] uppercase mb-1.5">
            ✦ {event.category}
          </Text>
          <Text
            className="text-[28px] font-black text-white tracking-[1px] leading-[30px]"
            numberOfLines={2}
          >
            {event.title.toUpperCase()}
          </Text>
        </View>

        {/* Sold-out badge only — price badge and low-stock removed */}
        {isSoldOut && (
          <View className={s.soldOutBadge}>
            <Text className={s.soldOutText}>SOLD OUT</Text>
          </View>
        )}
      </View>

      {/* ── Scrollable content ──────────────────────────────────────── */}
      <ScrollView className={s.scroll} showsVerticalScrollIndicator={false}>

        {/* Gallery */}
        {event.cover_image_url && (
          <View className={s.galleryWrap}>
            <ScrollView
              horizontal
              showsHorizontalScrollIndicator={false}
              contentContainerClassName={s.galleryScroll}
            >
              {[event.cover_image_url, event.banner_image_url]
                .filter(Boolean)
                .map((uri, i) => (
                  <View key={i} className={s.galleryThumb}>
                    <Image source={{ uri: uri! }} style={sheet.absoluteFill} resizeMode="cover" />
                  </View>
                ))}
            </ScrollView>
          </View>
        )}

        <View className={s.content}>

          {/* ── Meta ──────────────────────────────────────────────────── */}
          <View className={s.metaRow}>
            <Text className={s.metaIcon}>📅</Text>
            <Text className={s.metaTextBold}>{formatDate(event.start_time)}</Text>
            <Text className={s.metaText}>at {formatTime(event.start_time)}</Text>
          </View>
          <View className={s.metaRow}>
            <Text className={s.metaIcon}>📍</Text>
            <Text className={s.metaTextBold}>{event.venue_name}</Text>
            <Text className={s.metaText}>· {event.city}</Text>
          </View>
          <View className={s.metaRow}>
            <Text className={s.metaIcon}>👁</Text>
            <Text className={s.metaText}>{event.views?.toLocaleString('en-ZA') ?? '0'} views</Text>
          </View>

          <View className={s.divider} />

          {/* ── CTA ───────────────────────────────────────────────────── */}
          <View className={s.ctaWrap}>
            {isSoldOut ? (
              <TouchableOpacity className={s.btnSoldOut} disabled>
                <Text className={s.btnSoldOutText}>🎟 SOLD OUT</Text>
              </TouchableOpacity>
            ) : (
              <TouchableOpacity className={s.btnPrimary}>
                <Text className={s.btnPrimaryText}>
                  🎟 GET TICKETS · {formatPrice(event.price, event.currency)}
                </Text>
              </TouchableOpacity>
            )}
          </View>

          <View className={s.actionRow}>
            <TouchableOpacity className={s.btnOutline}>
              <Text className={s.btnOutlineIcon}>🔔</Text>
              <Text className={s.btnOutlineText}>TRACK EVENT</Text>
            </TouchableOpacity>
            <TouchableOpacity className={s.btnOutline} onPress={handleShare}>
              <Text className={s.btnOutlineIcon}>↑</Text>
              <Text className={s.btnOutlineText}>SHARE</Text>
            </TouchableOpacity>
          </View>

          <View className={s.divider} />

          {/* ── About ─────────────────────────────────────────────────── */}
          <View className="mb-5">
            <View className={s.sectionHeader}>
              <Text className={s.sectionTitle}>ABOUT THIS EVENT</Text>
            </View>
            <Text className={s.descText}>{displayDesc}</Text>
            {hasMore && (
              <TouchableOpacity onPress={() => setExpanded((v) => !v)}>
                <Text className={s.descReadMore}>
                  {expanded ? 'SHOW LESS ▴' : 'READ MORE ▾'}
                </Text>
              </TouchableOpacity>
            )}
          </View>

          {/* ── Teams (sports) ────────────────────────────────────────── */}
          {isASport && teams.length > 0 && (
            <>
              <View className={s.divider} />
              <View className="mb-5">
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle}>TEAMS</Text>
                </View>
                {teams.map((team, i) => (
                  <View
                    key={i}
                    className={i < teams.length - 1 ? s.participantRow : s.participantRowLast}
                  >
                    <View className={s.participantAvatar}>
                      <Text className={s.participantAvatarText}>
                        {team.trim().charAt(0).toUpperCase()}
                      </Text>
                    </View>
                    <Text className={s.participantName}>{team}</Text>
                    <Text className={s.participantArrow}>›</Text>
                  </View>
                ))}
              </View>
            </>
          )}

          {/* ── Performers (non-sports) ───────────────────────────────── */}
          {!isASport && performers.length > 0 && (
            <>
              <View className={s.divider} />
              <View className="mb-5">
                <View className={s.sectionHeader}>
                  <Text className={s.sectionTitle}>PERFORMERS</Text>
                </View>
                {performers.map((performer, i) => (
                  <View
                    key={i}
                    className={i < performers.length - 1 ? s.participantRow : s.participantRowLast}
                  >
                    <View className={s.participantAvatar}>
                      <Text className={s.participantAvatarText}>
                        {performer.trim().charAt(0).toUpperCase()}
                      </Text>
                    </View>
                    <Text className={s.participantName}>{performer}</Text>
                    <Text className={s.participantArrow}>›</Text>
                  </View>
                ))}
              </View>
            </>
          )}

          <View className={s.divider} />

          {/* ── Event info — capacity + start only ────────────────────── */}
          <View className="mb-5">
            <View className={s.sectionHeader}>
              <Text className={s.sectionTitle}>EVENT INFO</Text>
            </View>

            {maxCap > 0 && (
              <View className={[s.bentoCard, 'mb-3'].join(' ')}>
                <Text className={s.bentoIcon}>🏟</Text>
                <Text className={s.bentoLabel}>CAPACITY</Text>
                <Text className={s.bentoValue}>{maxCap.toLocaleString('en-ZA')}</Text>
                <Text className={s.bentoSub}>{event.venue_name}</Text>
              </View>
            )}

            <View className={s.bentoCard}>
              <Text className={s.bentoIcon}>⏰</Text>
              <Text className={s.bentoLabel}>START</Text>
              <Text className={s.bentoValue}>{formatTime(event.start_time)}</Text>
              <Text className={s.bentoSub}>{formatDate(event.start_time)}</Text>
            </View>
          </View>

          <View className={s.divider} />

          {/* ── Location ──────────────────────────────────────────────── */}
          <View className="mb-5">
            <View className={s.sectionHeader}>
              <Text className={s.sectionTitle}>LOCATION</Text>
              <TouchableOpacity onPress={handleDirections}>
                <Text className={s.sectionLink}>GET DIRECTIONS ↗</Text>
              </TouchableOpacity>
            </View>
            <View className={s.locationCard}>
              <Text className={s.locationName}>{event.venue_name}</Text>
              {ev.address ? (
                <Text className={s.locationAddr}>{ev.address}</Text>
              ) : null}
              <Text className={s.locationAddr}>{event.city}</Text>
              <TouchableOpacity className={s.locationDirections} onPress={handleDirections}>
                <Text className={s.locationDirIcon}>↗</Text>
                <Text className={s.locationDirText}>OPEN IN MAPS</Text>
              </TouchableOpacity>
            </View>
          </View>

          {/* ── Status ────────────────────────────────────────────────── */}
          <View className="flex-row items-center gap-2 mb-6">
            <View
              style={{
                width: 6,
                height: 6,
                borderRadius: 3,
                backgroundColor: event.status === 'published' ? '#22c55e' : '#ff3c6e',
              }}
            />
            <Text className="text-[10px] font-extrabold text-black/30 tracking-[1.5px] uppercase">
              {event.status === 'published' ? 'CONFIRMED' : event.status?.toUpperCase()}
            </Text>
          </View>

        </View>

        <View className={s.bottomPad} />
      </ScrollView>
    </View>
  );
}