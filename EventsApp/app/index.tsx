import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  StyleSheet,
  Modal,
  FlatList,
} from "react-native";
import { useState } from "react";
import { LinearGradient } from "expo-linear-gradient";

const CATEGORIES = ["All", "Concerts", "Sports", "Gala", "Tech", "Food"];

const TRENDING = [
  { id: "1", category: "● Live Concert", title: "After Hours: Electronic Pulse", venue: "Fabric, London" },
  { id: "2", category: "Tech Summit", title: "The Future of AI: Private Gala", venue: "Paris Convention" },
  { id: "3", category: "● Live Concert", title: "Neon Pulse Festival", venue: "Grand Central Arena" },
  { id: "4", category: "Exclusive", title: "The Midnight Affair", venue: "Metropolitan Club" },
  { id: "5", category: "Sports", title: "Championship Sprint Final", venue: "National Stadium" },
  { id: "6", category: "Gala", title: "Black Tie Masquerade", venue: "The Ritz, London" },
  { id: "7", category: "Tech", title: "Global Founders Summit", venue: "Silicon Roundabout" },
  { id: "8", category: "Food", title: "Gourmet Garden Festival", venue: "City Park" },
  { id: "9", category: "Sports", title: "Wimbledon Finals Night", venue: "All England Club" },
  { id: "10", category: "● Live Concert", title: "The Indie Sessions", venue: "Warehouse 4" },
];

const NEAR_ME = [
  { id: "1", category: "● Live Concert", title: "Neon Pulse Festival", venue: "Grand Central Arena" },
  { id: "2", category: "Exclusive", title: "The Midnight Affair", venue: "Metropolitan Club" },
  { id: "3", category: "Sports", title: "Championship Sprint Final", venue: "National Stadium" },
  { id: "4", category: "Gala", title: "Black Tie Masquerade", venue: "The Ritz" },
  { id: "5", category: "Tech", title: "Global Founders Summit", venue: "Silicon Roundabout" },
  { id: "6", category: "Food", title: "Gourmet Garden Festival", venue: "City Park" },
  { id: "7", category: "Sports", title: "Wimbledon Finals Night", venue: "All England Club" },
  { id: "8", category: "● Live Concert", title: "The Indie Sessions", venue: "Warehouse 4" },
  { id: "9", category: "Exclusive", title: "Monaco Paddock Club", venue: "Monte Carlo" },
  { id: "10", category: "Sports", title: "The Clash in Vegas", venue: "MGM Grand" },
];

const POPULAR = [
  { id: "1", title: "Global Tech Summit", venue: "Convention Center" },
  { id: "2", title: "Underground Beats", venue: "The Cellar" },
  { id: "3", title: "Gourmet Garden", venue: "City Park" },
  { id: "4", title: "The Indie Sessions", venue: "Warehouse 4" },
  { id: "5", title: "Black Tie Masquerade", venue: "The Ritz" },
  { id: "6", title: "Champions League Final", venue: "Wembley Stadium" },
  { id: "7", title: "Jazz Under the Stars", venue: "Rooftop Arena" },
  { id: "8", title: "Art Basel After Party", venue: "Gallery One" },
  { id: "9", title: "EDM World Tour", venue: "O2 Arena" },
  { id: "10", title: "Comedy Night Live", venue: "The Apollo" },
];

const TOP10 = [
  { id: "1", rank: 1, category: "Sports", title: "World Cup Final 2026", venue: "Lusail Stadium" },
  { id: "2", rank: 2, category: "● Live Concert", title: "Beyoncé Renaissance III", venue: "Madison Square Garden" },
  { id: "3", rank: 3, category: "Tech", title: "Apple WWDC 2026", venue: "San Jose Convention" },
  { id: "4", rank: 4, category: "Gala", title: "Met Gala After Party", venue: "The Plaza, NY" },
  { id: "5", rank: 5, category: "Sports", title: "Wimbledon Finals", venue: "All England Club" },
  { id: "6", rank: 6, category: "● Live Concert", title: "Coldplay World Tour", venue: "Wembley Stadium" },
  { id: "7", rank: 7, category: "Food", title: "World's 50 Best Dinner", venue: "Eleven Madison Park" },
  { id: "8", rank: 8, category: "Sports", title: "Monaco Grand Prix", venue: "Circuit de Monaco" },
  { id: "9", rank: 9, category: "Gala", title: "Cannes Film Premiere", venue: "Palais des Festivals" },
  { id: "10", rank: 10, category: "● Live Concert", title: "Drake Summer Tour", venue: "O2 Arena, London" },
];

const SPORTS = [
  { id: "1", category: "Wimbledon Finals", title: "Gentlemen's Singles Final", location: "London, United Kingdom" },
  { id: "2", category: "Grand Prix", title: "Monaco Paddock Club", location: "Monte Carlo, Monaco" },
  { id: "3", category: "Heavyweight Title", title: "The Clash in Vegas", location: "MGM Grand, Las Vegas" },
];

export default function Index() {
  const [activeCategory, setActiveCategory] = useState("All");
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [location] = useState("Johannesburg, ZA");

  const handleSelectCategory = (cat: string) => {
    setActiveCategory(cat);
    setDropdownOpen(false);
  };

  const EventCard = ({ item }: { item: any }) => (
    <TouchableOpacity style={styles.hCard}>
      <View style={styles.hCardImagePlaceholder}>
        {item.category?.includes("Live") && (
          <View style={styles.hCardBadge}>
            <Text style={styles.hCardBadgeText}>● LIVE</Text>
          </View>
        )}
      </View>
      <View style={styles.hCardBody}>
        <Text style={styles.hCardCat}>{item.category?.replace("● ", "")}</Text>
        <Text style={styles.hCardTitle}>{item.title}</Text>
        <Text style={styles.hCardMeta}>📍 {item.venue}</Text>
      </View>
    </TouchableOpacity>
  );

  const Top10Card = ({ item }: { item: any }) => (
    <TouchableOpacity style={styles.hCard}>
      <View style={styles.hCardImagePlaceholder}>
        <View style={styles.rankBadge}>
          <Text style={styles.rankText}>{item.rank}</Text>
        </View>
        {item.category?.includes("Live") && (
          <View style={styles.hCardBadge}>
            <Text style={styles.hCardBadgeText}>● LIVE</Text>
          </View>
        )}
      </View>
      <View style={styles.hCardBody}>
        <Text style={styles.hCardCat}>{item.category?.replace("● ", "")}</Text>
        <Text style={styles.hCardTitle}>{item.title}</Text>
        <Text style={styles.hCardMeta}>📍 {item.venue}</Text>
      </View>
    </TouchableOpacity>
  );

  return (
    <View style={styles.screen}>

      {/* Fixed Location Header — outside ScrollView so it never moves */}
      <View style={styles.locationHeader}>
        <View>
          <Text style={styles.locationCity}>{location}</Text>
          <Text style={styles.locationSub}>All dates</Text>
        </View>
        <TouchableOpacity style={styles.menuBtn}>
          <Text style={styles.menuIcon}>🛒</Text>
        </TouchableOpacity>
      </View>

      <ScrollView style={styles.scroll} showsVerticalScrollIndicator={false}>

        {/* Hero Banner */}
        <View style={styles.hero}>
          <View style={styles.heroImagePlaceholder} />
          <LinearGradient
            colors={["rgba(0,0,47,0.75)", "#000000"]}
            locations={[0.6, 1]}
            style={StyleSheet.absoluteFill}
          />
          <View style={styles.heroContent}>
            <Text style={styles.heroBadge}>✦ Limited Exclusive</Text>
            <Text style={styles.heroTitle}>THE WORLD{"\n"}FINALS 2026</Text>
            <Text style={styles.heroSub}>Secure your premium hospitality passes for the most anticipated match of the decade.</Text>
            <View style={styles.heroActions}>
              <TouchableOpacity style={styles.btnPrimary}>
                <Text style={styles.btnPrimaryText}>🎟 Get Tickets</Text>
              </TouchableOpacity>
              <TouchableOpacity style={styles.btnOutline}>
                <Text style={styles.btnOutlineText}>View Details</Text>
              </TouchableOpacity>
            </View>
          </View>
        </View>

        {/* Category Filter Row */}
        <View style={styles.filterRow}>
          {activeCategory !== "All" && (
            <TouchableOpacity style={styles.filterClearBtn} onPress={() => setActiveCategory("All")}>
              <Text style={styles.filterClearText}>✕</Text>
            </TouchableOpacity>
          )}
          <TouchableOpacity
            style={[styles.filterChip, activeCategory !== "All" && styles.filterChipActive]}
            onPress={() => setDropdownOpen(true)}
          >
            <Text style={[styles.filterChipText, activeCategory !== "All" && styles.filterChipTextActive]}>
              {activeCategory === "All" ? "Categories" : activeCategory}
            </Text>
            <Text style={[styles.filterChipArrow, activeCategory !== "All" && styles.filterChipTextActive]}> ▾</Text>
          </TouchableOpacity>
        </View>

        {/* Category Dropdown Modal */}
        <Modal visible={dropdownOpen} transparent animationType="fade" onRequestClose={() => setDropdownOpen(false)}>
          <View style={styles.modalBackdrop}>
            <View style={styles.modalSheet}>
              <FlatList
                data={CATEGORIES}
                keyExtractor={(item) => item}
                renderItem={({ item }) => (
                  <TouchableOpacity style={styles.modalItem} onPress={() => handleSelectCategory(item)}>
                    <Text style={[styles.modalItemText, activeCategory === item && styles.modalItemTextActive]}>
                      {item}
                    </Text>
                  </TouchableOpacity>
                )}
              />
              <TouchableOpacity style={styles.modalCloseBtn} onPress={() => setDropdownOpen(false)}>
                <Text style={styles.modalCloseBtnText}>✕</Text>
              </TouchableOpacity>
            </View>
          </View>
        </Modal>

        {/* Trending Now */}
        <View style={styles.section}>
          <View style={styles.sectionHeader}>
            <Text style={styles.sectionTitle}>TRENDING NOW</Text>
            <TouchableOpacity><Text style={styles.sectionLink}>VIEW ALL</Text></TouchableOpacity>
          </View>
        </View>
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.hScrollContent}>
          {TRENDING.map((event) => <EventCard key={event.id} item={event} />)}
        </ScrollView>

        {/* Near Me */}
        <View style={[styles.section, { marginTop: 28 }]}>
          <View style={styles.sectionHeader}>
            <Text style={styles.sectionTitle}>NEAR ME</Text>
            <TouchableOpacity><Text style={styles.sectionLink}>VIEW ALL</Text></TouchableOpacity>
          </View>
        </View>
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.hScrollContent}>
          {NEAR_ME.map((event) => <EventCard key={event.id} item={event} />)}
        </ScrollView>

        {/* Popular This Weekend */}
        <View style={[styles.section, { marginTop: 28 }]}>
          <View style={styles.sectionHeader}>
            <Text style={styles.sectionTitle}>POPULAR THIS WEEKEND</Text>
            <TouchableOpacity><Text style={styles.sectionLink}>VIEW ALL</Text></TouchableOpacity>
          </View>
        </View>
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.hScrollContent}>
          {POPULAR.map((item) => <EventCard key={item.id} item={item} />)}
        </ScrollView>

        {/* Top 10 Events */}
        <View style={[styles.section, { marginTop: 28 }]}>
          <View style={styles.sectionHeader}>
            <Text style={styles.sectionTitle}>TOP 10 EVENTS</Text>
            <TouchableOpacity><Text style={styles.sectionLink}>VIEW ALL</Text></TouchableOpacity>
          </View>
        </View>
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.hScrollContent}>
          {TOP10.map((item) => <Top10Card key={item.id} item={item} />)}
        </ScrollView>

        {/* Sports Elite Banner */}
        <View style={[styles.section, { marginTop: 32 }]}>
          <View style={styles.sectionHeader}>
            <Text style={styles.sectionTitle}>SPORTS ELITE</Text>
            <TouchableOpacity><Text style={styles.sectionLink}>EXPLORE</Text></TouchableOpacity>
          </View>
        </View>
        <View style={[styles.sportsBanner, { marginHorizontal: 20 }]}>
          <View style={styles.sportsBannerBg} />
          <View style={styles.sportsBannerContent}>
            <Text style={styles.sportsBadge}>Championship</Text>
            <Text style={styles.sportsTitle}>PRO LEAGUE FINALS</Text>
            <Text style={styles.sportsSub}>Courtside access available for select members.</Text>
            <TouchableOpacity style={styles.btnOutline}>
              <Text style={styles.btnOutlineText}>View Tickets</Text>
            </TouchableOpacity>
          </View>
        </View>

        {/* Sports List */}
        <View style={{ marginTop: 16, paddingHorizontal: 20 }}>
          {SPORTS.map((sport, index) => (
            <TouchableOpacity key={sport.id} style={[styles.sportRow, index === SPORTS.length - 1 && { borderBottomWidth: 0 }]}>
              <View style={styles.sportThumb} />
              <View style={styles.sportInfo}>
                <Text style={styles.sportCat}>{sport.category}</Text>
                <Text style={styles.sportTitle}>{sport.title}</Text>
                <Text style={styles.sportLoc}>{sport.location}</Text>
              </View>
              <Text style={styles.sportArrow}>›</Text>
            </TouchableOpacity>
          ))}
        </View>

        <View style={{ height: 100 }} />
      </ScrollView>
    </View>
  );
}

const NAVY = "#002047";
const PINK = "#ff3c6e";
const GOLD = "#f5c842";
const DIM = "rgba(255,255,255,0.06)";
const BORDER = "rgba(255,255,255,0.10)";
const MUTED = "rgba(255,255,255,0.45)";

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: "#000" },
  scroll: { flex: 1 },

  // Location Header — fixed above ScrollView
  locationHeader: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: 20,
    paddingTop: 56,
    paddingBottom: 12,
    backgroundColor: "#000",
    zIndex: 10,
  },
  locationCity: { fontSize: 22, fontWeight: "800", color: "#fff", letterSpacing: 0.3 },
  locationSub: { fontSize: 13, color: MUTED, marginTop: 2 },
  menuBtn: { padding: 6 },
  menuIcon: { fontSize: 24, color: "#fff" },

  // Hero
  hero: { position: "relative", height: 380 },
  heroImagePlaceholder: { position: "absolute", top: 0, left: 0, right: 0, bottom: 0, backgroundColor: "#1a1a2e" },
  heroContent: { position: "absolute", bottom: 0, left: 0, right: 0, padding: 20, paddingBottom: 32 },
  heroBadge: { fontSize: 11, fontWeight: "700", color: GOLD, letterSpacing: 2, textTransform: "uppercase", marginBottom: 10 },
  heroTitle: { fontSize: 42, fontWeight: "900", color: "#fff", letterSpacing: 2, lineHeight: 42, marginBottom: 12 },
  heroSub: { fontSize: 13, color: "rgba(255,255,255,0.75)", lineHeight: 20, marginBottom: 20, maxWidth: 300 },
  heroActions: { flexDirection: "row", gap: 12 },

  btnPrimary: { backgroundColor: PINK, paddingHorizontal: 22, paddingVertical: 12, borderRadius: 50 },
  btnPrimaryText: { color: "#fff", fontWeight: "700", fontSize: 14 },
  btnOutline: { borderWidth: 1.5, borderColor: BORDER, paddingHorizontal: 22, paddingVertical: 12, borderRadius: 50 },
  btnOutlineText: { color: "#fff", fontWeight: "600", fontSize: 14 },

  filterRow: { flexDirection: "row", alignItems: "center", paddingHorizontal: 20, paddingVertical: 14, gap: 10 },
  filterClearBtn: { width: 34, height: 34, borderRadius: 17, borderWidth: 1.5, borderColor: BORDER, alignItems: "center", justifyContent: "center" },
  filterClearText: { color: "#fff", fontSize: 12 },
  filterChip: { flexDirection: "row", alignItems: "center", paddingHorizontal: 16, paddingVertical: 8, borderRadius: 50, borderWidth: 1.5, borderColor: BORDER, backgroundColor: DIM },
  filterChipActive: { backgroundColor: PINK, borderColor: PINK },
  filterChipText: { color: MUTED, fontSize: 13, fontWeight: "600" },
  filterChipTextActive: { color: "#fff" },
  filterChipArrow: { color: MUTED, fontSize: 11 },

  modalBackdrop: { flex: 1, backgroundColor: "rgba(0,0,0,0.75)", justifyContent: "flex-end" },
  modalSheet: { backgroundColor: "#141414", borderTopLeftRadius: 20, borderTopRightRadius: 20, paddingTop: 20, paddingBottom: 40, maxHeight: "80%", alignItems: "center" },
  modalItem: { paddingVertical: 18, paddingHorizontal: 40, width: "100%", alignItems: "flex-start" },
  modalItemText: { fontSize: 22, color: MUTED, fontWeight: "400" },
  modalItemTextActive: { color: "#fff", fontWeight: "700" },
  modalCloseBtn: { marginTop: 16, width: 44, height: 44, borderRadius: 22, backgroundColor: "#333", alignItems: "center", justifyContent: "center" },
  modalCloseBtnText: { color: "#fff", fontSize: 16, fontWeight: "700" },

  section: { paddingHorizontal: 20, marginBottom: 16 },
  sectionHeader: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  sectionTitle: { fontSize: 22, fontWeight: "900", color: "#fff", letterSpacing: 1 },
  sectionLink: { fontSize: 11, fontWeight: "700", color: PINK, letterSpacing: 1.5 },

  hScrollContent: { paddingHorizontal: 20, gap: 14, paddingBottom: 8 },

  hCard: { width: 200, borderRadius: 14, overflow: "hidden" },
  hCardImagePlaceholder: { width: "100%", height: 130, backgroundColor: "#2a2a2a", borderRadius: 14 },
  hCardBadge: { position: "absolute", top: 10, left: 10, backgroundColor: PINK, borderRadius: 20, paddingHorizontal: 10, paddingVertical: 4 },
  hCardBadgeText: { color: "#fff", fontSize: 9, fontWeight: "700", letterSpacing: 1 },
  hCardBody: { paddingTop: 10, paddingHorizontal: 2 },
  hCardCat: { fontSize: 10, fontWeight: "700", color: PINK, letterSpacing: 1.5, textTransform: "uppercase", marginBottom: 3 },
  hCardTitle: { fontSize: 14, fontWeight: "700", color: "#fff", marginBottom: 5, lineHeight: 18 },
  hCardMeta: { fontSize: 11, color: MUTED },

  rankBadge: { position: "absolute", bottom: 10, left: 10, backgroundColor: GOLD, borderRadius: 6, paddingHorizontal: 8, paddingVertical: 3 },
  rankText: { color: "#000", fontSize: 12, fontWeight: "900" },

  sportsBanner: { borderRadius: 14, overflow: "hidden", minHeight: 200, position: "relative" },
  sportsBannerBg: { position: "absolute", top: 0, left: 0, right: 0, bottom: 0, backgroundColor: NAVY },
  sportsBannerContent: { padding: 24, position: "relative", zIndex: 2 },
  sportsBadge: { fontSize: 10, fontWeight: "700", color: GOLD, letterSpacing: 2, textTransform: "uppercase", marginBottom: 6 },
  sportsTitle: { fontSize: 28, fontWeight: "900", color: "#fff", letterSpacing: 1, marginBottom: 8 },
  sportsSub: { fontSize: 12, color: MUTED, marginBottom: 16 },

  sportRow: { flexDirection: "row", alignItems: "center", gap: 14, paddingVertical: 14, borderBottomWidth: 1, borderBottomColor: BORDER },
  sportThumb: { width: 56, height: 42, borderRadius: 8, backgroundColor: "#2a2a2a", flexShrink: 0 },
  sportInfo: { flex: 1 },
  sportCat: { fontSize: 10, fontWeight: "700", color: GOLD, letterSpacing: 1.5, textTransform: "uppercase", marginBottom: 2 },
  sportTitle: { fontSize: 14, fontWeight: "600", color: "#fff", marginBottom: 2 },
  sportLoc: { fontSize: 11, color: MUTED },
  sportArrow: { fontSize: 20, color: MUTED },
});