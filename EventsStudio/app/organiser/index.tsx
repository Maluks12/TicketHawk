import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  RefreshControl,
  StyleSheet,
  Alert,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';
import { Feather, Ionicons } from '@expo/vector-icons';
import { useAuthStore } from '../../store/authStore';
import { STUDIO } from '../../components/studio/theme';
import { StatCard, Card, Chip, ProgressBar, Loader, Empty } from '../../components/studio/ui';
import {
  getOrgStats,
  getOrgEvents,
  getOrgCampaigns,
  OrgStats,
  OrgEventRow,
  OrgCampaign,
  fmtMoney,
  fmtMoneyCompact,
  fmtNum,
  fmtDate,
  ctr,
  capacityPct,
} from '../../lib/organiser';

type TabId = 'overview' | 'events' | 'campaigns';

const capColor = (pct: number) =>
  pct >= 90 ? STUDIO.red : pct >= 70 ? STUDIO.amber : STUDIO.accent;

export default function StudioHome() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const { user, authReady } = useAuthStore();

  const [tab, setTab] = useState<TabId>('overview');
  const [stats, setStats] = useState<OrgStats | null>(null);
  const [events, setEvents] = useState<OrgEventRow[]>([]);
  const [campaigns, setCampaigns] = useState<OrgCampaign[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState(false);

  // Monotonic id for in-flight loads. Only the LATEST call may write state:
  // without this, a slow first request (cold API, token refresh) that finally
  // times out AFTER a successful Retry would setError(true) and stomp the
  // already-rendered dashboard.
  const loadSeq = React.useRef(0);

  const load = useCallback(async () => {
    const seq = ++loadSeq.current;
    setError(false);
    // allSettled instead of all: one slow/failed endpoint no longer sinks the
    // whole screen — we render whatever arrived and only error out if
    // EVERYTHING failed.
    const [s, e, c] = await Promise.allSettled([
      getOrgStats(),
      getOrgEvents(),
      getOrgCampaigns(),
    ]);
    if (seq !== loadSeq.current) return; // a newer load superseded this one

    if (s.status === 'fulfilled') setStats(s.value);
    if (e.status === 'fulfilled') setEvents(e.value);
    if (c.status === 'fulfilled') setCampaigns(c.value);

    const allFailed =
      s.status === 'rejected' && e.status === 'rejected' && c.status === 'rejected';
    setError(allFailed);
    setLoading(false);
    setRefreshing(false);
  }, []);

  useEffect(() => {
    // Wait for the session restore to settle before the first fetch — firing
    // before the token is attached is what made cold launches land on the
    // error screen while an immediate Retry succeeded.
    if (!authReady) return;
    load();
  }, [authReady, load]);

  const onRefresh = useCallback(() => {
    setRefreshing(true);
    load();
  }, [load]);

  const eventTitleById = useMemo(() => {
    const m: Record<string, string> = {};
    events.forEach((e) => (m[e.id] = e.title));
    return m;
  }, [events]);

  const activeCampaigns = useMemo(
    () => campaigns.filter((c) => c.status === 'active'),
    [campaigns],
  );

  const fullName =
    user?.first_name ?? user?.email?.split('@')[0] ?? 'Organiser';
  const initials = fullName
    .split(' ')
    .map((w) => w[0])
    .join('')
    .toUpperCase()
    .slice(0, 2);

  // Standalone Studio app: the dashboard IS home, so the top-right action is
  // sign-out rather than "close back to the consumer profile".
  const { logout } = useAuthStore();
  const exit = () => {
    Alert.alert('Sign out', 'Sign out of TicketHawk Studio on this device?', [
      { text: 'Cancel', style: 'cancel' },
      {
        text: 'Sign out',
        style: 'destructive',
        onPress: async () => {
          try {
            await logout();
          } finally {
            router.replace('/auth/login' as any);
          }
        },
      },
    ]);
  };

  return (
    <View style={[styles.root, { paddingTop: insets.top }]}>
      {/* ── Studio top bar ───────────────────────────────────────────────── */}
      <View style={styles.topbar}>
        <View style={styles.brandRow}>
          <View style={styles.avatar}>
            <Text style={styles.avatarTxt}>{initials || '?'}</Text>
          </View>
          <View style={{ flex: 1, minWidth: 0 }}>
            <View style={styles.brandLine}>
              <Text style={styles.brandWord}>Studio</Text>
              <View style={styles.betaPill}>
                <Text style={styles.betaTxt}>ORGANISER</Text>
              </View>
            </View>
            <Text style={styles.brandSub} numberOfLines={1}>
              {fullName}
            </Text>
          </View>
        </View>
        <TouchableOpacity onPress={exit} style={styles.closeBtn} hitSlop={10}>
          <Feather name="log-out" size={18} color={STUDIO.text} />
        </TouchableOpacity>
      </View>

      {loading ? (
        <Loader label="Loading your numbers…" />
      ) : error && !stats ? (
        <View style={{ flex: 1 }}>
          <Empty
            icon={<Feather name="wifi-off" size={26} color={STUDIO.muted} />}
            title="Couldn't load the dashboard"
            desc="Check your connection and try again."
          />
          <TouchableOpacity style={styles.retryBtn} onPress={onRefresh}>
            <Text style={styles.retryTxt}>Retry</Text>
          </TouchableOpacity>
        </View>
      ) : (
        <ScrollView
          style={{ flex: 1 }}
          contentContainerStyle={{ padding: 16, paddingBottom: 120 }}
          showsVerticalScrollIndicator={false}
          refreshControl={
            <RefreshControl
              refreshing={refreshing}
              onRefresh={onRefresh}
              tintColor={STUDIO.accent}
            />
          }
        >
          {tab === 'overview' && (
            <OverviewTab
              stats={stats}
              events={events}
              activeCampaigns={activeCampaigns}
              eventTitleById={eventTitleById}
              onOpenEvent={(id) => router.push(`/organiser/event/${id}` as any)}
              onSeeEvents={() => setTab('events')}
              onSeeCampaigns={() => setTab('campaigns')}
            />
          )}

          {tab === 'events' && (
            <EventsTab
              events={events}
              onOpenEvent={(id) => router.push(`/organiser/event/${id}` as any)}
            />
          )}

          {tab === 'campaigns' && (
            <CampaignsTab campaigns={campaigns} eventTitleById={eventTitleById} />
          )}
        </ScrollView>
      )}

      {/* ── Bottom nav ───────────────────────────────────────────────────── */}
      <View style={[styles.bottomNav, { paddingBottom: insets.bottom + 8 }]}>
        <NavItem
          icon="grid"
          label="Overview"
          active={tab === 'overview'}
          onPress={() => setTab('overview')}
        />
        <NavItem
          icon="calendar"
          label="Events"
          active={tab === 'events'}
          onPress={() => setTab('events')}
        />
        <NavItem
          icon="bar-chart-2"
          label="Campaigns"
          active={tab === 'campaigns'}
          onPress={() => setTab('campaigns')}
        />
        <TouchableOpacity
          style={styles.scanCta}
          activeOpacity={0.85}
          onPress={() => router.push('/organiser/scan' as any)}
        >
          <Ionicons name="qr-code" size={18} color="#fff" />
          <Text style={styles.scanCtaTxt}>Scan</Text>
        </TouchableOpacity>
      </View>
    </View>
  );
}

// ── Bottom nav item ───────────────────────────────────────────────────────────
function NavItem({
  icon,
  label,
  active,
  onPress,
}: {
  icon: keyof typeof Feather.glyphMap;
  label: string;
  active: boolean;
  onPress: () => void;
}) {
  return (
    <TouchableOpacity style={styles.navItem} onPress={onPress} activeOpacity={0.7}>
      <Feather
        name={icon}
        size={20}
        color={active ? STUDIO.accent : STUDIO.muted}
      />
      <Text
        style={[styles.navLabel, { color: active ? STUDIO.accent : STUDIO.muted }]}
      >
        {label}
      </Text>
    </TouchableOpacity>
  );
}

// ── Overview tab ──────────────────────────────────────────────────────────────
function OverviewTab({
  stats,
  events,
  activeCampaigns,
  eventTitleById,
  onOpenEvent,
  onSeeEvents,
  onSeeCampaigns,
}: {
  stats: OrgStats | null;
  events: OrgEventRow[];
  activeCampaigns: OrgCampaign[];
  eventTitleById: Record<string, string>;
  onOpenEvent: (id: string) => void;
  onSeeEvents: () => void;
  onSeeCampaigns: () => void;
}) {
  const recent = events.slice(0, 4);
  return (
    <View style={{ gap: 14 }}>
      {/* Hero — net earnings */}
      <View style={styles.hero}>
        <Text style={styles.heroLabel}>NET EARNINGS</Text>
        <Text style={styles.heroValue}>
          {fmtMoneyCompact(stats?.total_net_earnings ?? 0)}
        </Text>
        <Text style={styles.heroSub}>All-time, after platform fees</Text>
        <View style={styles.heroFooter}>
          <View style={styles.todayPill}>
            <Feather name="trending-up" size={13} color={STUDIO.green} />
            <Text style={styles.todayPillTxt}>
              {fmtMoney(stats?.revenue_today ?? 0)} today
            </Text>
          </View>
          <Text style={styles.heroFootMuted}>
            {fmtNum(stats?.bookings_today ?? 0)} bookings today
          </Text>
        </View>
      </View>

      {/* Stat grid */}
      <View style={styles.grid}>
        <StatCard
          label="Tickets sold"
          value={fmtNum(stats?.total_tickets_sold ?? 0)}
          sub={`${fmtNum(stats?.total_bookings ?? 0)} bookings`}
          accent="accent"
          icon={<Ionicons name="ticket-outline" size={15} color={STUDIO.accent} />}
        />
        <StatCard
          label="Gross sales"
          value={fmtMoneyCompact(stats?.gross_ticket_revenue ?? 0)}
          sub="ticket revenue"
          accent="green"
          icon={<Feather name="dollar-sign" size={15} color={STUDIO.green} />}
        />
      </View>
      <View style={styles.grid}>
        <StatCard
          label="Published"
          value={fmtNum(stats?.published_events ?? 0)}
          sub={`of ${fmtNum(stats?.total_events ?? 0)} events`}
          accent="blue"
          icon={<Feather name="calendar" size={15} color={STUDIO.blue} />}
        />
        <StatCard
          label="Active campaigns"
          value={fmtNum(activeCampaigns.length)}
          sub={
            activeCampaigns.length
              ? `${fmtNum(
                  activeCampaigns.reduce((a, c) => a + c.total_impressions, 0),
                )} impressions`
              : 'none running'
          }
          accent="amber"
          icon={<Feather name="bar-chart-2" size={15} color={STUDIO.amber} />}
        />
      </View>

      {/* Recent events */}
      <Card
        title="Recent events"
        action={
          <TouchableOpacity onPress={onSeeEvents}>
            <Text style={styles.linkTxt}>See all</Text>
          </TouchableOpacity>
        }
        noPad
      >
        {recent.length === 0 ? (
          <Empty title="No events yet" desc="Your events will appear here." />
        ) : (
          recent.map((e, i) => (
            <EventRow
              key={e.id}
              ev={e}
              divider={i < recent.length - 1}
              onPress={() => onOpenEvent(e.id)}
            />
          ))
        )}
      </Card>

      {/* Active campaigns */}
      <Card
        title="Active campaigns"
        action={
          <TouchableOpacity onPress={onSeeCampaigns}>
            <Text style={styles.linkTxt}>Manage</Text>
          </TouchableOpacity>
        }
        noPad
      >
        {activeCampaigns.length === 0 ? (
          <Empty
            title="No active campaigns"
            desc="Boosted events will show their live performance here."
          />
        ) : (
          activeCampaigns
            .slice(0, 3)
            .map((c, i) => (
              <CampaignRow
                key={c.id}
                c={c}
                title={c.event?.title ?? eventTitleById[c.event_id] ?? 'Event'}
                divider={i < Math.min(activeCampaigns.length, 3) - 1}
              />
            ))
        )}
      </Card>
    </View>
  );
}

// ── Events tab ────────────────────────────────────────────────────────────────
const STATUS_FILTERS = ['all', 'published', 'draft', 'completed', 'cancelled'] as const;
type StatusFilter = (typeof STATUS_FILTERS)[number];

function EventsTab({
  events,
  onOpenEvent,
}: {
  events: OrgEventRow[];
  onOpenEvent: (id: string) => void;
}) {
  const [filter, setFilter] = useState<StatusFilter>('all');

  const counts = useMemo(() => {
    const c: Record<string, number> = { all: events.length };
    events.forEach((e) => (c[e.status] = (c[e.status] ?? 0) + 1));
    return c;
  }, [events]);

  const filtered = useMemo(
    () => (filter === 'all' ? events : events.filter((e) => e.status === filter)),
    [events, filter],
  );

  return (
    <View style={{ gap: 14 }}>
      <ScrollView
        horizontal
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={{ gap: 8, paddingVertical: 2 }}
      >
        {STATUS_FILTERS.map((f) => {
          const active = filter === f;
          const n = counts[f] ?? 0;
          if (f !== 'all' && n === 0) return null;
          return (
            <TouchableOpacity
              key={f}
              onPress={() => setFilter(f)}
              activeOpacity={0.8}
              style={[styles.pill, active && styles.pillActive]}
            >
              <Text style={[styles.pillTxt, active && styles.pillTxtActive]}>
                {f === 'all' ? 'All' : f.charAt(0).toUpperCase() + f.slice(1)}
                {n > 0 ? `  ${n}` : ''}
              </Text>
            </TouchableOpacity>
          );
        })}
      </ScrollView>

      <Card noPad>
        {filtered.length === 0 ? (
          <Empty
            icon={<Feather name="calendar" size={24} color={STUDIO.muted} />}
            title="No events here"
            desc="Try a different filter."
          />
        ) : (
          filtered.map((e, i) => (
            <EventRow
              key={e.id}
              ev={e}
              divider={i < filtered.length - 1}
              onPress={() => onOpenEvent(e.id)}
              showEarnings
            />
          ))
        )}
      </Card>
    </View>
  );
}

// ── Campaigns tab ─────────────────────────────────────────────────────────────
function CampaignsTab({
  campaigns,
  eventTitleById,
}: {
  campaigns: OrgCampaign[];
  eventTitleById: Record<string, string>;
}) {
  if (campaigns.length === 0) {
    return (
      <Card noPad>
        <Empty
          icon={<Feather name="bar-chart-2" size={24} color={STUDIO.muted} />}
          title="No campaigns yet"
          desc="Campaigns you've created to boost events will appear here with their live performance."
        />
      </Card>
    );
  }
  return (
    <Card noPad>
      {campaigns.map((c, i) => (
        <CampaignRow
          key={c.id}
          c={c}
          title={c.event?.title ?? eventTitleById[c.event_id] ?? 'Event'}
          divider={i < campaigns.length - 1}
          detailed
        />
      ))}
    </Card>
  );
}

// ── Reusable rows ─────────────────────────────────────────────────────────────
function EventRow({
  ev,
  divider,
  onPress,
  showEarnings,
}: {
  ev: OrgEventRow;
  divider: boolean;
  onPress: () => void;
  showEarnings?: boolean;
}) {
  const pct = capacityPct(ev.tickets_sold, ev.max_capacity);
  return (
    <TouchableOpacity
      activeOpacity={0.7}
      onPress={onPress}
      style={[styles.row, divider && styles.rowDivider]}
    >
      <View style={{ flex: 1, minWidth: 0, gap: 5 }}>
        <View style={styles.rowTitleLine}>
          <Text style={styles.rowTitle} numberOfLines={1}>
            {ev.title}
          </Text>
          <Chip status={ev.status} />
        </View>
        <Text style={styles.rowMeta} numberOfLines={1}>
          {fmtDate(ev.start_time)} · {ev.city}
        </Text>
        <View style={styles.rowStatsLine}>
          <Text style={styles.rowStat}>
            <Text style={styles.rowStatStrong}>{fmtNum(ev.tickets_sold)}</Text>
            {ev.max_capacity ? ` / ${fmtNum(ev.max_capacity)}` : ''} sold
          </Text>
          {showEarnings && (
            <Text style={styles.rowStat}>
              <Text style={[styles.rowStatStrong, { color: STUDIO.green }]}>
                {fmtMoney(ev.net_ticket_earnings, ev.currency)}
              </Text>{' '}
              net
            </Text>
          )}
        </View>
        {pct !== null && (
          <View style={{ marginTop: 2 }}>
            <ProgressBar pct={pct} color={capColor(pct)} height={5} />
          </View>
        )}
      </View>
      <Feather name="chevron-right" size={18} color={STUDIO.faint} />
    </TouchableOpacity>
  );
}

function CampaignRow({
  c,
  title,
  divider,
  detailed,
}: {
  c: OrgCampaign;
  title: string;
  divider: boolean;
  detailed?: boolean;
}) {
  const used =
    c.daily_budget > 0 ? Math.min((c.today_spent / c.daily_budget) * 100, 100) : 0;
  return (
    <View style={[styles.row, divider && styles.rowDivider]}>
      <View style={{ flex: 1, minWidth: 0, gap: 7 }}>
        <View style={styles.rowTitleLine}>
          <Text style={styles.rowTitle} numberOfLines={1}>
            {title}
          </Text>
          <Chip status={c.status} />
        </View>
        <View style={styles.metricRow}>
          <Metric label="Today" value={fmtMoney(c.today_spent)} />
          <Metric label="Budget" value={`${fmtMoney(c.daily_budget)}/d`} />
          <Metric label="CTR" value={ctr(c.total_clicks, c.total_impressions)} />
        </View>
        {detailed && (
          <View style={styles.metricRow}>
            <Metric label="Impressions" value={fmtNum(c.total_impressions)} />
            <Metric label="Clicks" value={fmtNum(c.total_clicks)} />
            <Metric label="Spent" value={fmtMoney(c.total_spent)} />
          </View>
        )}
        <View style={{ marginTop: 2 }}>
          <ProgressBar pct={used} color={STUDIO.accent} height={5} />
        </View>
      </View>
    </View>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <View style={{ flex: 1, minWidth: 0 }}>
      <Text style={styles.metricLabel}>{label.toUpperCase()}</Text>
      <Text style={styles.metricValue} numberOfLines={1}>
        {value}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: STUDIO.bg },

  // Top bar
  topbar: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 16,
    paddingVertical: 12,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: STUDIO.border,
  },
  brandRow: { flexDirection: 'row', alignItems: 'center', gap: 11, flex: 1, minWidth: 0 },
  avatar: {
    width: 38,
    height: 38,
    borderRadius: 19,
    backgroundColor: STUDIO.accentDeep,
    alignItems: 'center',
    justifyContent: 'center',
  },
  avatarTxt: { color: '#fff', fontWeight: '800', fontSize: 14, letterSpacing: 0.5 },
  brandLine: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  brandWord: { color: STUDIO.text, fontSize: 18, fontWeight: '800', letterSpacing: -0.3 },
  betaPill: {
    backgroundColor: STUDIO.accentSoft,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: STUDIO.accentBorder,
    borderRadius: 5,
    paddingHorizontal: 6,
    paddingVertical: 2,
  },
  betaTxt: { color: STUDIO.accent, fontSize: 8.5, fontWeight: '800', letterSpacing: 1 },
  brandSub: { color: STUDIO.muted, fontSize: 12, marginTop: 1 },
  closeBtn: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: STUDIO.surface,
    alignItems: 'center',
    justifyContent: 'center',
  },

  // Hero
  hero: {
    backgroundColor: STUDIO.bgElevated,
    borderRadius: 20,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: STUDIO.accentBorder,
    padding: 20,
  },
  heroLabel: { color: STUDIO.accent, fontSize: 11, fontWeight: '800', letterSpacing: 1.5 },
  heroValue: {
    color: STUDIO.text,
    fontSize: 40,
    fontWeight: '800',
    letterSpacing: -1.5,
    marginTop: 6,
  },
  heroSub: { color: STUDIO.muted, fontSize: 13, marginTop: 2 },
  heroFooter: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginTop: 16,
  },
  todayPill: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    backgroundColor: STUDIO.greenSoft,
    borderRadius: 999,
    paddingHorizontal: 11,
    paddingVertical: 6,
  },
  todayPillTxt: { color: STUDIO.green, fontSize: 12.5, fontWeight: '700' },
  heroFootMuted: { color: STUDIO.muted, fontSize: 12 },

  grid: { flexDirection: 'row', gap: 12 },

  linkTxt: { color: STUDIO.accent, fontSize: 13, fontWeight: '600' },

  // Rows
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 10,
    paddingHorizontal: 16,
    paddingVertical: 13,
  },
  rowDivider: {
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: STUDIO.border,
  },
  rowTitleLine: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  rowTitle: { color: STUDIO.text, fontSize: 14.5, fontWeight: '700', flexShrink: 1 },
  rowMeta: { color: STUDIO.muted, fontSize: 12 },
  rowStatsLine: { flexDirection: 'row', gap: 14, flexWrap: 'wrap' },
  rowStat: { color: STUDIO.sub, fontSize: 12.5 },
  rowStatStrong: { color: STUDIO.text, fontWeight: '700' },

  // Campaign metrics
  metricRow: { flexDirection: 'row', gap: 10 },
  metricLabel: { color: STUDIO.faint, fontSize: 9.5, fontWeight: '700', letterSpacing: 0.6 },
  metricValue: { color: STUDIO.text, fontSize: 14, fontWeight: '700', marginTop: 2 },

  // Filter pills
  pill: {
    paddingHorizontal: 14,
    paddingVertical: 7,
    borderRadius: 999,
    backgroundColor: STUDIO.surface,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: STUDIO.border,
  },
  pillActive: { backgroundColor: STUDIO.accentSoft, borderColor: STUDIO.accentBorder },
  pillTxt: { color: STUDIO.sub, fontSize: 13, fontWeight: '600' },
  pillTxtActive: { color: STUDIO.accent },

  // Bottom nav
  bottomNav: {
    position: 'absolute',
    left: 0,
    right: 0,
    bottom: 0,
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 10,
    paddingTop: 10,
    gap: 4,
    backgroundColor: 'rgba(8,8,12,0.96)',
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: STUDIO.border,
  },
  navItem: { flex: 1, alignItems: 'center', gap: 4, paddingVertical: 4 },
  navLabel: { fontSize: 10.5, fontWeight: '600' },
  scanCta: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 7,
    backgroundColor: STUDIO.accentDeep,
    borderRadius: 14,
    paddingHorizontal: 18,
    paddingVertical: 11,
    marginLeft: 4,
  },
  scanCtaTxt: { color: '#fff', fontSize: 14, fontWeight: '800' },

  retryBtn: {
    alignSelf: 'center',
    backgroundColor: STUDIO.accentDeep,
    borderRadius: 12,
    paddingHorizontal: 24,
    paddingVertical: 12,
  },
  retryTxt: { color: '#fff', fontWeight: '700', fontSize: 14 },
});