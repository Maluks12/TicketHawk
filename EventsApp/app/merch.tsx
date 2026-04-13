import {
  View,
  Text,
  FlatList,
  Image,
  TouchableOpacity,
  Modal,
  ScrollView,
  StatusBar,
  StyleSheet,
  Dimensions,
  SafeAreaView,
  Animated,
  PanResponder,
} from 'react-native';
import { useState, useRef, useEffect } from 'react';

const { width: W, height: H } = Dimensions.get('window');

// ─── Types ────────────────────────────────────────────────────────────────────

type Drop = {
  id: string;
  brand: string;
  brandLogo: string;
  type: 'available' | 'upcoming';
  timeLabel: string;           // "12 HOURS AGO" | "DROPS IN"
  title: string;
  category: string;
  color: string;
  textColor: string;
  heat: number;
  retail: string;
  releaseDate: string;
  expiresInHours?: number;     // available items expire
  dropsInSeconds?: number;     // upcoming items count down
  description: string;
  images: string[];
};

// ─── Hardcoded data ───────────────────────────────────────────────────────────

const now = Date.now();

const DROPS: Drop[] = [
  {
    id: '1',
    brand: 'NIKE',
    brandLogo: '✔',
    type: 'available',
    timeLabel: '12 HOURS AGO',
    title: 'Rory McIlroy Masters Champion Tribute Pack',
    category: 'CULTURE',
    color: '#1a6b3c',
    textColor: '#ffffff',
    heat: 94,
    retail: 'R2 200',
    releaseDate: 'NOW',
    expiresInHours: 36,
    description:
      'Nike honors Rory McIlroy\'s historic back-to-back Masters win with a limited tribute pack. Once this window closes, it\'s gone for good.',
    images: [
      'https://images.unsplash.com/photo-1530549387789-4c1017266635?w=700&q=80',
      'https://images.unsplash.com/photo-1535131749006-b7f58c99034b?w=400&q=80',
    ],
  },
  {
    id: '2',
    brand: 'NIKE',
    brandLogo: '✔',
    type: 'available',
    timeLabel: '5 HOURS AGO',
    title: 'Nike WNBA 30th Anniversary Collection',
    category: 'DROPS',
    color: '#1a8fe8',
    textColor: '#000000',
    heat: 38,
    retail: 'R3 800',
    releaseDate: 'NOW',
    expiresInHours: 71,
    description:
      'Thirty years of the WNBA, celebrated in an expansive red, white, and blue collection. Available for 3 days only.',
    images: [
      'https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=600&q=80',
      'https://images.unsplash.com/photo-1600185365926-3a2ce3cdb9eb?w=400&q=80',
      'https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=400&q=80',
    ],
  },
  {
    id: '3',
    brand: 'ADIDAS',
    brandLogo: '◈',
    type: 'available',
    timeLabel: '1 DAY AGO',
    title: 'Adidas AE 2 "Glow" — Anthony Edwards PE',
    category: 'SNEAKERS',
    color: '#0d0d0d',
    textColor: '#39ff14',
    heat: 91,
    retail: 'R2 800',
    releaseDate: 'NOW',
    expiresInHours: 18,
    description:
      'Anthony Edwards\' player-exclusive in electric "Glow" green. One of the most hyped basketball shoes of the year — limited 18-hour window.',
    images: [
      'https://images.unsplash.com/photo-1608231387042-66d1773070a5?w=700&q=80',
      'https://images.unsplash.com/photo-1595950653106-6c9ebd614d3a?w=400&q=80',
    ],
  },
  {
    id: '4',
    brand: 'NIKE',
    brandLogo: '✔',
    type: 'upcoming',
    timeLabel: 'DROPS IN',
    title: 'Nike Air Force 1 "Cardinal" — LeBron PE',
    category: 'SNEAKERS',
    color: '#7b1c2a',
    textColor: '#ffffff',
    heat: 78,
    retail: 'R3 500',
    releaseDate: '18 APR 2026',
    dropsInSeconds: 5 * 24 * 3600 + 3 * 3600 + 22 * 60 + 10,
    description:
      'LeBron James\' personal colorway of the Air Force 1 Low in rich Cardinal red with gold accents. Set an alert — this one will go fast.',
    images: [
      'https://images.unsplash.com/photo-1600185365926-3a2ce3cdb9eb?w=700&q=80',
    ],
  },
  {
    id: '5',
    brand: 'NEW BALANCE',
    brandLogo: 'N',
    type: 'upcoming',
    timeLabel: 'DROPS IN',
    title: 'New Balance 1906R "Protection Pack" — Mineral Grey',
    category: 'SNEAKERS',
    color: '#e8e0d4',
    textColor: '#1a1a1a',
    heat: 55,
    retail: 'R3 100',
    releaseDate: '21 APR 2026',
    dropsInSeconds: 8 * 24 * 3600 + 11 * 3600 + 45 * 60,
    description:
      'New Balance revisits the Protection Pack with the 1906R in Mineral Grey — weathered, reflective, and incredibly limited.',
    images: [
      'https://images.unsplash.com/photo-1539185441755-769473a23570?w=700&q=80',
    ],
  },
  {
    id: '6',
    brand: 'JORDAN',
    brandLogo: '✦',
    type: 'upcoming',
    timeLabel: 'DROPS IN',
    title: 'Air Jordan 1 High OG "Chicago Reimagined"',
    category: 'SNEAKERS',
    color: '#cc2200',
    textColor: '#ffffff',
    heat: 99,
    retail: 'R4 500',
    releaseDate: '25 APR 2026',
    dropsInSeconds: 12 * 24 * 3600 + 7 * 3600,
    description:
      'The most anticipated Jordan 1 of the decade. A modern reimagining of the 1985 Chicago colorway with premium leather and updated tooling.',
    images: [
      'https://images.unsplash.com/photo-1552346154-21d32810aba3?w=700&q=80',
      'https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=400&q=80',
    ],
  },
];

// ─── Countdown hook ───────────────────────────────────────────────────────────

function useCountdown(totalSeconds: number) {
  const [secs, setSecs] = useState(totalSeconds);
  useEffect(() => {
    const t = setInterval(() => setSecs((s) => Math.max(0, s - 1)), 1000);
    return () => clearInterval(t);
  }, []);
  const d = Math.floor(secs / 86400);
  const h = Math.floor((secs % 86400) / 3600);
  const m = Math.floor((secs % 3600) / 60);
  const s = secs % 60;
  if (d > 0) return `${d}D  ${String(h).padStart(2,'0')}H  ${String(m).padStart(2,'0')}M`;
  return `${String(h).padStart(2,'0')}:${String(m).padStart(2,'0')}:${String(s).padStart(2,'0')}`;
}

function useExpiry(hours: number) {
  const totalSecs = hours * 3600;
  const [secs, setSecs] = useState(totalSecs);
  useEffect(() => {
    const t = setInterval(() => setSecs((s) => Math.max(0, s - 1)), 1000);
    return () => clearInterval(t);
  }, []);
  const h = Math.floor(secs / 3600);
  const m = Math.floor((secs % 3600) / 60);
  const s = secs % 60;
  return `${String(h).padStart(2,'0')}:${String(m).padStart(2,'0')}:${String(s).padStart(2,'0')}`;
}

// ─── Image collage ────────────────────────────────────────────────────────────

const Collage = ({ images }: { images: string[] }) => {
  if (images.length === 1) {
    return <Image source={{ uri: images[0] }} style={st.collageSingle} resizeMode="cover" />;
  }
  return (
    <View style={st.collageGrid}>
      <Image source={{ uri: images[0] }} style={st.collageLeft} resizeMode="cover" />
      <View style={st.collageRight}>
        {images.slice(1, 3).map((uri, i) => (
          <Image key={i} source={{ uri }} style={[st.collageSmall, i === 0 && { marginBottom: 2 }]} resizeMode="cover" />
        ))}
      </View>
    </View>
  );
};

// ─── Available card ───────────────────────────────────────────────────────────

const AvailableCard = ({ drop, onPress }: { drop: Drop; onPress: () => void }) => {
  const tc = drop.textColor;
  const timer = useExpiry(drop.expiresInHours ?? 24);
  const urgent = (drop.expiresInHours ?? 24) < 24;

  return (
    <TouchableOpacity activeOpacity={0.95} onPress={onPress}>
      <View style={[st.card, { backgroundColor: drop.color }]}>
        <View style={st.cardTop}>
          <Text style={[st.brandLogo, { color: tc }]}>{drop.brandLogo}</Text>
          <Text style={[st.brandName, { color: tc }]}>{drop.brand}</Text>
          <View style={{ flex: 1 }} />
          <View style={[st.expiryBadge, { borderColor: urgent ? '#ff3c6e' : tc + '40' }]}>
            <Text style={[st.expiryLabel, { color: urgent ? '#ff3c6e' : tc + '80' }]}>ENDS IN</Text>
            <Text style={[st.expiryTimer, { color: urgent ? '#ff3c6e' : tc }]}>{timer}</Text>
          </View>
        </View>
        <Collage images={drop.images} />
        <Text style={[st.cardTitle, { color: tc }]} numberOfLines={3}>{drop.title}</Text>
        <View style={[st.cardBottom, { borderTopColor: tc + '25' }]}>
          <Text style={[st.category, { color: tc + 'aa' }]}>{drop.category}</Text>
          <View style={{ flex: 1 }} />
          <Text style={[st.heatNum, { color: tc }]}>🔥 {drop.heat}°H</Text>
          <View style={{ width: 12 }} />
          <TouchableOpacity style={[st.actionBtn, { borderColor: tc + '60' }]}>
            <Text style={[st.actionBtnText, { color: tc }]}>BUY  {drop.retail}</Text>
          </TouchableOpacity>
        </View>
      </View>
    </TouchableOpacity>
  );
};

// ─── Upcoming card ────────────────────────────────────────────────────────────

const UpcomingCard = ({ drop, onPress }: { drop: Drop; onPress: () => void }) => {
  const tc = drop.textColor;
  const countdown = useCountdown(drop.dropsInSeconds ?? 0);

  return (
    <TouchableOpacity activeOpacity={0.95} onPress={onPress}>
      <View style={[st.card, { backgroundColor: drop.color }]}>
        <View style={st.cardTop}>
          <Text style={[st.brandLogo, { color: tc }]}>{drop.brandLogo}</Text>
          <Text style={[st.brandName, { color: tc }]}>{drop.brand}</Text>
          <View style={{ flex: 1 }} />
          <Text style={[st.upcomingLabel, { color: tc + '80' }]}>DROPS IN</Text>
        </View>

        {/* Big countdown */}
        <View style={[st.countdownRow, { borderTopColor: tc + '15', borderBottomColor: tc + '15' }]}>
          <Text style={[st.countdownText, { color: tc }]}>{countdown}</Text>
        </View>

        <Collage images={drop.images} />
        <Text style={[st.cardTitle, { color: tc }]} numberOfLines={3}>{drop.title}</Text>
        <View style={[st.cardBottom, { borderTopColor: tc + '25' }]}>
          <Text style={[st.category, { color: tc + 'aa' }]}>{drop.category}</Text>
          <View style={{ flex: 1 }} />
          <Text style={[st.releaseDate, { color: tc + '80' }]}>{drop.releaseDate}</Text>
          <View style={{ width: 12 }} />
          <TouchableOpacity style={[st.actionBtn, { borderColor: tc + '60' }]}>
            <Text style={[st.actionBtnText, { color: tc }]}>SET ALERT</Text>
          </TouchableOpacity>
        </View>
      </View>
    </TouchableOpacity>
  );
};

// ─── Detail modal ─────────────────────────────────────────────────────────────

const DetailModal = ({ drop, onClose }: { drop: Drop; onClose: () => void }) => {
  const tc = drop.textColor;
  const translateY = useRef(new Animated.Value(0)).current;
  const scrollOffset = useRef(0);

  const pilePan = useRef(
    PanResponder.create({
      onStartShouldSetPanResponder: () => true,
      onMoveShouldSetPanResponder: (_, g) => Math.abs(g.dy) > 4,
      onPanResponderMove: (_, g) => { if (g.dy > 0) translateY.setValue(g.dy); },
      onPanResponderRelease: (_, g) => {
        if (g.dy > 80 || g.vy > 0.5) {
          Animated.timing(translateY, { toValue: H, duration: 220, useNativeDriver: true }).start(onClose);
        } else {
          Animated.spring(translateY, { toValue: 0, useNativeDriver: true }).start();
        }
      },
    })
  ).current;

  const scrollPan = useRef(
    PanResponder.create({
      onMoveShouldSetPanResponder: (_, g) =>
        scrollOffset.current <= 0 && g.dy > 12 && g.dy > Math.abs(g.dx),
      onPanResponderMove: (_, g) => { if (g.dy > 0) translateY.setValue(g.dy); },
      onPanResponderRelease: (_, g) => {
        if (g.dy > 100 || g.vy > 0.6) {
          Animated.timing(translateY, { toValue: H, duration: 220, useNativeDriver: true }).start(onClose);
        } else {
          Animated.spring(translateY, { toValue: 0, useNativeDriver: true }).start();
        }
      },
    })
  ).current;

  return (
    <Modal visible animationType="slide" onRequestClose={onClose} statusBarTranslucent>
      <StatusBar barStyle={tc === '#ffffff' ? 'light-content' : 'dark-content'} />
      <Animated.View style={[st.modal, { backgroundColor: drop.color, transform: [{ translateY }] }]}>

        {/* Drag pill */}
        <View style={st.dragHandleArea} {...pilePan.panHandlers}>
          <View style={[st.dragHandle, { backgroundColor: tc + '55' }]} />
          <View style={st.modalTopBar}>
            <Text style={[st.modalStatus, { color: tc + 'aa' }]}>
              {drop.type === 'available' ? '● AVAILABLE NOW' : '◎ DROPPING SOON'}
            </Text>
            <View style={{ flex: 1 }} />
            <Text style={[st.modalHeat, { color: tc + 'aa' }]}>🔥 {drop.heat}°H</Text>
          </View>
        </View>

        <ScrollView
          showsVerticalScrollIndicator={false}
          contentContainerStyle={{ paddingBottom: 120 }}
          onScroll={(e) => { scrollOffset.current = e.nativeEvent.contentOffset.y; }}
          scrollEventThrottle={16}
          {...scrollPan.panHandlers}
        >
          {/* Heat score box */}
          <View style={[st.heatScoreBox, { borderColor: tc + '40' }]}>
            <Text style={[st.heatScoreBoxLabel, { color: tc + '90' }]}>
              {drop.brand}{'   '}{drop.releaseDate}
            </Text>
            <Text style={[st.heatScoreBoxScore, { color: tc }]}>{drop.heat}°H</Text>
            <Text style={[st.heatScoreBoxName, { color: tc + '90' }]} numberOfLines={2}>
              {drop.title}
            </Text>
          </View>

          <View style={{ marginTop: 16 }}>
            <Collage images={drop.images} />
          </View>

          <Text style={[st.detailTitle, { color: tc }]}>{drop.title}</Text>

          <View style={[st.detailBrandRow, { borderBottomColor: tc + '25', borderTopColor: tc + '25' }]}>
            <Text style={[st.brandLogo, { color: tc, fontSize: 22 }]}>{drop.brandLogo}</Text>
            <View style={{ flex: 1 }} />
            <TouchableOpacity style={[st.actionBtn, { borderColor: tc + '60' }]}>
              <Text style={[st.actionBtnText, { color: tc }]}>RATE DROP</Text>
            </TouchableOpacity>
            <View style={{ width: 10 }} />
            <TouchableOpacity style={[st.actionBtn, { borderColor: tc + '60' }]}>
              <Text style={[st.actionBtnText, { color: tc }]}>⇧</Text>
            </TouchableOpacity>
          </View>

          <View style={st.detailGrid}>
            {[
              ['RETAIL', drop.retail],
              ['RELEASE', drop.releaseDate],
              ['BRAND', drop.brand],
              ['HEAT', `${drop.heat}°H`],
            ].map(([label, value]) => (
              <View key={label} style={[st.detailRow, { borderBottomColor: tc + '20' }]}>
                <Text style={[st.detailLabel, { color: tc + '70' }]}>{label}</Text>
                <Text style={[st.detailValue, { color: tc }]}>{value}</Text>
              </View>
            ))}
          </View>

          <Text style={[st.detailDesc, { color: tc + 'cc' }]}>{drop.description}</Text>
        </ScrollView>

        {/* Bottom bar */}
        <View style={[st.modalBottom, { borderTopColor: tc + '20', backgroundColor: drop.color }]}>
          <TouchableOpacity>
            <Text style={[st.modalBottomAlerts, { color: tc + 'aa' }]}>ALERTS</Text>
          </TouchableOpacity>
          <View style={st.modalBottomCenter}>
            <Text style={[st.modalBottomTitle, { color: tc }]} numberOfLines={1}>{drop.title}</Text>
            <Text style={[st.modalBottomDate, { color: tc + '80' }]}>{drop.releaseDate}</Text>
          </View>
          <TouchableOpacity style={[st.buyNowBtn, { borderColor: tc + '60' }]}>
            <Text style={[st.buyNowText, { color: tc }]}>
              {drop.type === 'available' ? 'BUY' : 'ALERT'}
            </Text>
          </TouchableOpacity>
        </View>

      </Animated.View>
    </Modal>
  );
};

// ─── Screen ───────────────────────────────────────────────────────────────────

type Tab = 'available' | 'upcoming';

export default function Merch() {
  const [selected, setSelected] = useState<Drop | null>(null);
  const [tab, setTab] = useState<Tab>('available');

  const visible = DROPS.filter((d) => d.type === tab);

  return (
    <View style={st.screen}>
      <SafeAreaView>
        {/* Header */}
        <View style={st.header}>
          <Text style={st.headerTitle}>DROPS</Text>
          <View style={st.headerRight}>
            <View style={st.liveIndicator} />
            <Text style={st.liveText}>LIVE</Text>
            <TouchableOpacity style={st.cartBtn}>
              <Text style={st.cartIcon}>🛒</Text>
            </TouchableOpacity>
          </View>
        </View>

        {/* Toggle */}
        <View style={st.toggle}>
          <TouchableOpacity
            style={[st.toggleBtn, tab === 'available' && st.toggleBtnActive]}
            onPress={() => setTab('available')}
          >
            <Text style={[st.toggleText, tab === 'available' && st.toggleTextActive]}>
              AVAILABLE NOW
            </Text>
          </TouchableOpacity>
          <TouchableOpacity
            style={[st.toggleBtn, tab === 'upcoming' && st.toggleBtnActive]}
            onPress={() => setTab('upcoming')}
          >
            <Text style={[st.toggleText, tab === 'upcoming' && st.toggleTextActive]}>
              DROPPING SOON
            </Text>
          </TouchableOpacity>
        </View>
      </SafeAreaView>

      <FlatList
        data={visible}
        keyExtractor={(d) => d.id}
        renderItem={({ item }) =>
          item.type === 'available'
            ? <AvailableCard drop={item} onPress={() => setSelected(item)} />
            : <UpcomingCard drop={item} onPress={() => setSelected(item)} />
        }
        contentContainerStyle={{ paddingBottom: 110 }}
        showsVerticalScrollIndicator={false}
      />

      {selected && (
        <DetailModal drop={selected} onClose={() => setSelected(null)} />
      )}
    </View>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const st = StyleSheet.create({
  screen: { flex: 1, backgroundColor: '#0a0a0a' },

  // Header
  header:        { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: 20, paddingTop: 8, paddingBottom: 4 },
  headerTitle:   { fontSize: 32, fontWeight: '900', color: '#fff', letterSpacing: 2 },
  headerRight:   { flexDirection: 'row', alignItems: 'center', gap: 10 },
  liveIndicator: { width: 8, height: 8, borderRadius: 4, backgroundColor: '#ff3c6e' },
  liveText:      { fontSize: 11, fontWeight: '700', color: '#ff3c6e', letterSpacing: 1.5 },
  cartBtn:       { padding: 4 },
  cartIcon:      { fontSize: 20 },

  // Toggle
  toggle:           { flexDirection: 'row', marginHorizontal: 20, marginTop: 10, marginBottom: 14, borderRadius: 8, backgroundColor: '#1a1a1a', padding: 3 },
  toggleBtn:        { flex: 1, paddingVertical: 10, alignItems: 'center', borderRadius: 6 },
  toggleBtnActive:  { backgroundColor: '#ff3c6e' },
  toggleText:       { fontSize: 11, fontWeight: '800', color: '#555', letterSpacing: 1.5 },
  toggleTextActive: { color: '#fff' },

  // Card
  card:       { marginBottom: 2 },
  cardTop:    { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 16, paddingVertical: 12, gap: 6 },
  brandLogo:  { fontSize: 18, fontWeight: '900' },
  brandName:  { fontSize: 11, fontWeight: '800', letterSpacing: 1.5 },
  cardTitle:  { fontSize: 22, fontWeight: '700', lineHeight: 28, paddingHorizontal: 16, paddingTop: 14, paddingBottom: 4 },
  cardBottom: { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 16, paddingVertical: 14, borderTopWidth: 1, marginTop: 8 },
  category:   { fontSize: 10, fontWeight: '700', letterSpacing: 1.5 },
  heatNum:    { fontSize: 11, fontWeight: '800' },
  releaseDate:{ fontSize: 10, fontWeight: '700', letterSpacing: 1 },

  // Expiry badge (available)
  expiryBadge: { borderWidth: 1, paddingHorizontal: 10, paddingVertical: 6, borderRadius: 3, alignItems: 'center' },
  expiryLabel: { fontSize: 8, fontWeight: '800', letterSpacing: 1.5, marginBottom: 2 },
  expiryTimer: { fontSize: 13, fontWeight: '900', letterSpacing: 1, fontVariant: ['tabular-nums'] },

  // Countdown (upcoming)
  upcomingLabel: { fontSize: 10, fontWeight: '700', letterSpacing: 1.5 },
  countdownRow:  { borderTopWidth: 1, borderBottomWidth: 1, paddingVertical: 14, paddingHorizontal: 16, marginBottom: 2 },
  countdownText: { fontSize: 36, fontWeight: '900', letterSpacing: 2, fontVariant: ['tabular-nums'] },

  // Buttons
  actionBtn:     { borderWidth: 1, paddingHorizontal: 14, paddingVertical: 7, borderRadius: 2 },
  actionBtnText: { fontSize: 10, fontWeight: '700', letterSpacing: 1.5 },

  // Collage
  collageSingle: { width: '100%', height: 300 },
  collageGrid:   { flexDirection: 'row', height: 250 },
  collageLeft:   { flex: 1, marginRight: 2 },
  collageRight:  { width: W * 0.38, flexDirection: 'column' },
  collageSmall:  { flex: 1 },

  // Modal
  modal:          { flex: 1 },
  dragHandleArea: { paddingTop: 14, paddingBottom: 4, alignItems: 'center' },
  dragHandle:     { width: 44, height: 5, borderRadius: 3, marginBottom: 10 },
  modalTopBar:    { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 16, paddingBottom: 8, width: '100%' },
  modalStatus:    { fontSize: 10, fontWeight: '700', letterSpacing: 1.5 },
  modalHeat:      { fontSize: 12, fontWeight: '700' },

  // Heat score box
  heatScoreBox:       { borderWidth: 1, marginHorizontal: 16, padding: 14, borderRadius: 2 },
  heatScoreBoxLabel:  { fontSize: 10, fontWeight: '700', letterSpacing: 1.5, marginBottom: 4 },
  heatScoreBoxScore:  { fontSize: 40, fontWeight: '900', letterSpacing: -1 },
  heatScoreBoxName:   { fontSize: 11, fontWeight: '500', marginTop: 4, lineHeight: 16 },

  // Detail
  detailTitle:     { fontSize: 30, fontWeight: '800', lineHeight: 36, paddingHorizontal: 16, paddingTop: 20, paddingBottom: 8 },
  detailBrandRow:  { flexDirection: 'row', alignItems: 'center', paddingHorizontal: 16, paddingVertical: 14, borderTopWidth: 1, borderBottomWidth: 1, gap: 8 },
  detailGrid:      { paddingHorizontal: 16, marginTop: 16 },
  detailRow:       { flexDirection: 'row', justifyContent: 'space-between', paddingVertical: 12, borderBottomWidth: 1 },
  detailLabel:     { fontSize: 11, fontWeight: '600', letterSpacing: 1.5 },
  detailValue:     { fontSize: 13, fontWeight: '700', letterSpacing: 0.5 },
  detailDesc:      { fontSize: 15, lineHeight: 24, paddingHorizontal: 16, paddingTop: 20 },

  // Modal bottom bar
  modalBottom:       { position: 'absolute', bottom: 0, left: 0, right: 0, flexDirection: 'row', alignItems: 'center', paddingHorizontal: 20, paddingVertical: 16, paddingBottom: 36, borderTopWidth: 1, gap: 12 },
  modalBottomAlerts: { fontSize: 10, fontWeight: '700', letterSpacing: 1.5 },
  modalBottomCenter: { flex: 1, alignItems: 'center' },
  modalBottomTitle:  { fontSize: 11, fontWeight: '700', letterSpacing: 0.5 },
  modalBottomDate:   { fontSize: 10, fontWeight: '500', marginTop: 2 },
  buyNowBtn:         { borderWidth: 1, paddingHorizontal: 18, paddingVertical: 8, borderRadius: 2 },
  buyNowText:        { fontSize: 12, fontWeight: '800', letterSpacing: 1.5 },
}); 