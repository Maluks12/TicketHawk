import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  StyleSheet,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { Feather, Ionicons } from '@expo/vector-icons';
import { STUDIO } from '../../../components/studio/theme';
import { StatCard, Card, Chip, ProgressBar, Loader, Empty } from '../../../components/studio/ui';
import {
  getOrgEventStats,
  getOrgEventBookings,
  getOrgCampaigns,
  OrgEventStats,
  OrgBooking,
  OrgCampaign,
  fmtMoney,
  fmtNum,
  fmtDate,
  fmtDateTime,
  ctr,
  capacityPct,
} from '../../../lib/organiser';

type Tab = 'overview' | 'attendees';

const capColor = (pct: number) =>
  pct >= 90 ? STUDIO.red : pct >= 70 ? STUDIO.amber : STUDIO.accent;

export default function StudioEventDetail() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();

  const [tab, setTab] = useState<Tab>('overview');
  const [stats, setStats] = useState<OrgEventStats | null>(null);
  const [campaign, setCampaign] = useState<OrgCampaign | null>(null);
  const [bookings, setBookings] = useState<OrgBooking[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingTab, setLoadingTab] = useState(false);
  const [notFound, setNotFound] = useState(false);

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const [s, camps] = await Promise.all([
          getOrgEventStats(id),
          getOrgCampaigns().catch(() => [] as OrgCampaign[]),
        ]);
        if (!alive) return;
        setStats(s);
        // active_campaign isn't joined server-side here, so resolve it ourselves
        setCampaign(
          camps.find((c) => c.event_id === id && c.status === 'active') ?? null,
        );
      } catch {
        if (alive) setNotFound(true);
      } finally {
        if (alive) setLoading(false);
      }
    })();
    return () => {
      alive = false;
    };
  }, [id]);

  const loadTab = useCallback(
    async (t: Tab) => {
      if (t === 'attendees' && bookings.length === 0) {
        setLoadingTab(true);
        try {
          setBookings(await getOrgEventBookings(id));
        } catch {
          /* leave empty */
        } finally {
          setLoadingTab(false);
        }
      }
    },
    [id, bookings.length],
  );

  const switchTab = (t: Tab) => {
    setTab(t);
    loadTab(t);
  };

  // Organisers only need to see who's actually coming — cancelled bookings
  // aren't attendees and shouldn't clutter (or be countable in) this list.
  const confirmedBookings = useMemo(
    () => bookings.filter((b) => b.booking_status === 'confirmed'),
    [bookings],
  );

  const ev = stats?.event;
  const pct = useMemo(
    () => (ev ? capacityPct(ev.tickets_sold, ev.max_capacity) : null),
    [ev],
  );

  return (
    <View style={[styles.root, { paddingTop: insets.top }]}>
      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.iconBtn} hitSlop={10}>
          <Feather name="arrow-left" size={20} color={STUDIO.text} />
        </TouchableOpacity>
        <Text style={styles.headerTitle} numberOfLines={1}>
          {ev?.title ?? 'Event'}
        </Text>
        <View style={{ width: 36 }} />
      </View>

      {loading ? (
        <Loader />
      ) : notFound || !ev ? (
        <Empty
          icon={<Feather name="calendar" size={26} color={STUDIO.muted} />}
          title="Event not found"
          desc="It may belong to a different organiser account."
        />
      ) : (
        <>
          <ScrollView
            style={{ flex: 1 }}
            contentContainerStyle={{ padding: 16, paddingBottom: 110 }}
            showsVerticalScrollIndicator={false}
          >
            {/* Title block */}
            <View style={{ gap: 8, marginBottom: 16 }}>
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
                <Text style={styles.evTitle}>{ev.title}</Text>
                <Chip status={ev.status} />
              </View>
              <Text style={styles.evMeta}>
                {fmtDate(ev.start_time)} · {ev.venue_name}
                {ev.venue_name ? ', ' : ''}
                {ev.city}
              </Text>
            </View>

            {/* Tab switch */}
            <View style={styles.segment}>
              {(['overview', 'attendees'] as Tab[]).map((t) => (
                <TouchableOpacity
                  key={t}
                  style={[styles.segItem, tab === t && styles.segItemActive]}
                  onPress={() => switchTab(t)}
                  activeOpacity={0.8}
                >
                  <Text style={[styles.segTxt, tab === t && styles.segTxtActive]}>
                    {t === 'overview' ? 'Overview' : 'Attendees'}
                  </Text>
                </TouchableOpacity>
              ))}
            </View>

            <View style={{ marginTop: 16 }}>
              {tab === 'overview' && (
                <View style={{ gap: 14 }}>
                  <View style={styles.grid}>
                    <StatCard
                      label="Revenue"
                      value={fmtMoney(stats!.total_revenue, ev.currency)}
                      sub="net earnings"
                      accent="green"
                      icon={<Feather name="dollar-sign" size={15} color={STUDIO.green} />}
                    />
                    <StatCard
                      label="Tickets sold"
                      value={fmtNum(ev.tickets_sold)}
                      sub={ev.max_capacity ? `of ${fmtNum(ev.max_capacity)}` : 'no cap'}
                      accent="accent"
                      icon={<Ionicons name="ticket-outline" size={15} color={STUDIO.accent} />}
                    />
                  </View>
                  <View style={styles.grid}>
                    <StatCard
                      label="Confirmed"
                      value={fmtNum(stats!.confirmed_bookings)}
                      sub={`${fmtNum(stats!.pending_bookings)} pending`}
                      accent="blue"
                      icon={<Feather name="check-circle" size={15} color={STUDIO.blue} />}
                    />
                    <StatCard
                      label="Page views"
                      value={fmtNum(ev.views)}
                      sub="total"
                      accent="amber"
                      icon={<Feather name="eye" size={15} color={STUDIO.amber} />}
                    />
                  </View>

                  {pct !== null && (
                    <Card title="Capacity">
                      <View style={styles.capRow}>
                        <Text style={styles.capTxt}>
                          {fmtNum(ev.tickets_sold)} / {fmtNum(ev.max_capacity!)}
                        </Text>
                        <Text style={[styles.capPct, { color: capColor(pct) }]}>
                          {pct.toFixed(0)}%
                        </Text>
                      </View>
                      <ProgressBar pct={pct} color={capColor(pct)} height={8} />
                      {pct >= 90 && (
                        <Text style={styles.capWarn}>⚠ Almost sold out</Text>
                      )}
                    </Card>
                  )}

                  {campaign && (
                    <Card title="Ad campaign">
                      <View style={styles.metricRow}>
                        <Metric label="Today spend" value={fmtMoney(campaign.today_spent)} />
                        <Metric label="Daily budget" value={fmtMoney(campaign.daily_budget)} />
                      </View>
                      <View style={[styles.metricRow, { marginTop: 12 }]}>
                        <Metric label="Impressions" value={fmtNum(campaign.total_impressions)} />
                        <Metric
                          label="CTR"
                          value={ctr(campaign.total_clicks, campaign.total_impressions)}
                        />
                      </View>
                    </Card>
                  )}
                </View>
              )}

              {tab === 'attendees' &&
                (loadingTab ? (
                  <Loader />
                ) : confirmedBookings.length === 0 ? (
                  <Card noPad>
                    <Empty
                      icon={<Feather name="users" size={24} color={STUDIO.muted} />}
                      title="No attendees yet"
                      desc="Confirmed bookings will appear here."
                    />
                  </Card>
                ) : (
                  <Card noPad>
                    {confirmedBookings.map((b, i) => (
                      <View
                        key={b.id}
                        style={[
                          styles.bookingRow,
                          i < confirmedBookings.length - 1 && styles.rowDivider,
                        ]}
                      >
                        <View style={{ flex: 1, minWidth: 0, gap: 3 }}>
                          <Text style={styles.bookName} numberOfLines={1}>
                            {b.attendee_name || 'Guest'}
                          </Text>
                          <Text style={styles.bookMeta} numberOfLines={1}>
                            {b.booking_number}
                          </Text>
                          <Text style={styles.bookMeta}>
                            {fmtNum(b.ticket_quantity)} ticket
                            {b.ticket_quantity === 1 ? '' : 's'} ·{' '}
                            {fmtDateTime(b.confirmed_at)}
                          </Text>
                        </View>
                        <View style={{ alignItems: 'flex-end', gap: 6 }}>
                          <Text style={styles.bookAmt}>
                            {fmtMoney(b.net_earning, b.currency)}
                          </Text>
                          <Chip status={b.booking_status} />
                        </View>
                      </View>
                    ))}
                  </Card>
                ))}
            </View>
          </ScrollView>

          {/* Scan this event's tickets */}
          <View style={[styles.footer, { paddingBottom: insets.bottom + 10 }]}>
            <TouchableOpacity
              style={[styles.scanBtn, {
                backgroundColor: 'rgba(255,255,255,0.08)',
                marginBottom: 10,
              }]}
              activeOpacity={0.85}
              onPress={() =>
                router.push({
                  pathname: '/organiser/staff/[eventId]',
                  params: { eventId: ev.id, title: ev.title },
                } as any)
              }
            >
              <Ionicons name="people-outline" size={18} color="#fff" />
              <Text style={styles.scanBtnTxt}>Manage door staff</Text>
            </TouchableOpacity>
            <TouchableOpacity
              style={styles.scanBtn}
              activeOpacity={0.85}
              onPress={() =>
                router.push({
                  pathname: '/organiser/scan',
                  params: { eventId: ev.id, title: ev.title },
                } as any)
              }
            >
              <Ionicons name="qr-code" size={18} color="#fff" />
              <Text style={styles.scanBtnTxt}>Scan tickets for this event</Text>
            </TouchableOpacity>
          </View>
        </>
      )}
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
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 12,
    paddingVertical: 10,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: STUDIO.border,
  },
  iconBtn: {
    width: 36,
    height: 36,
    borderRadius: 18,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: STUDIO.surface,
  },
  headerTitle: {
    flex: 1,
    textAlign: 'center',
    color: STUDIO.text,
    fontSize: 16,
    fontWeight: '700',
    marginHorizontal: 8,
  },

  evTitle: { color: STUDIO.text, fontSize: 22, fontWeight: '800', letterSpacing: -0.5, flexShrink: 1 },
  evMeta: { color: STUDIO.muted, fontSize: 13 },

  segment: {
    flexDirection: 'row',
    backgroundColor: STUDIO.surface,
    borderRadius: 12,
    padding: 4,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: STUDIO.border,
  },
  segItem: { flex: 1, alignItems: 'center', paddingVertical: 9, borderRadius: 9 },
  segItemActive: { backgroundColor: STUDIO.accentDeep },
  segTxt: { color: STUDIO.sub, fontSize: 13, fontWeight: '700' },
  segTxtActive: { color: '#fff' },

  grid: { flexDirection: 'row', gap: 12 },

  capRow: { flexDirection: 'row', justifyContent: 'space-between', marginBottom: 8 },
  capTxt: { color: STUDIO.sub, fontSize: 13, fontWeight: '600' },
  capPct: { fontSize: 13, fontWeight: '800' },
  capWarn: { color: STUDIO.red, fontSize: 12, fontWeight: '700', marginTop: 8 },

  metricRow: { flexDirection: 'row', gap: 10 },
  metricLabel: { color: STUDIO.faint, fontSize: 9.5, fontWeight: '700', letterSpacing: 0.6 },
  metricValue: { color: STUDIO.text, fontSize: 16, fontWeight: '700', marginTop: 3 },

  bookingRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    paddingHorizontal: 16,
    paddingVertical: 13,
  },
  rowDivider: { borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: STUDIO.border },
  bookName: { color: STUDIO.text, fontSize: 14.5, fontWeight: '700' },
  bookMeta: { color: STUDIO.muted, fontSize: 12 },
  bookAmt: { color: STUDIO.green, fontSize: 14, fontWeight: '800' },

  footer: {
    position: 'absolute',
    left: 0,
    right: 0,
    bottom: 0,
    paddingHorizontal: 16,
    paddingTop: 10,
    backgroundColor: 'rgba(8,8,12,0.96)',
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: STUDIO.border,
  },
  scanBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 9,
    backgroundColor: STUDIO.accentDeep,
    borderRadius: 14,
    paddingVertical: 15,
  },
  scanBtnTxt: { color: '#fff', fontSize: 15, fontWeight: '800' },
});