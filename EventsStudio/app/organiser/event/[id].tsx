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
import {
  makeStyles,
  useStudioTheme,
  type StudioPalette,
} from '../../../components/studio/theme';
import { StatCard, Card, Chip, ProgressBar, Loader, Empty } from '../../../components/studio/ui';
import {
  getOrgEventStats,
  getOrgEventBookings,
  getOrgEventInfo,
  getOrgCampaigns,
  OrgEventStats,
  OrgBooking,
  OrgCampaign,
  EventInfoItem,
  INFO_KEY_DOORS,
  infoValue,
  fmtMoney,
  fmtNum,
  fmtDate,
  fmtDateTime,
  fmtClock,
  fmtDayShort,
  fmtDoors,
  fmtDuration,
  sameDay,
  scanWindow,
  scanStatus,
  ctr,
  capacityPct,
} from '../../../lib/organiser';

type Tab = 'overview' | 'attendees';

const capColor = (pct: number, S: StudioPalette) =>
  pct >= 90 ? S.red : pct >= 70 ? S.amber : S.accent;

// How often the scanning countdown refreshes. 30s is under the smallest unit
// the label shows (minutes), so it never looks stale, and it costs one setState
// on a screen that is otherwise idle.
const TICK_MS = 30_000;

export default function StudioEventDetail() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const { S } = useStudioTheme();
  const styles = useStyles();

  const [tab, setTab] = useState<Tab>('overview');
  const [stats, setStats] = useState<OrgEventStats | null>(null);
  const [campaign, setCampaign] = useState<OrgCampaign | null>(null);
  const [info, setInfo] = useState<EventInfoItem[]>([]);
  const [bookings, setBookings] = useState<OrgBooking[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingTab, setLoadingTab] = useState(false);
  const [notFound, setNotFound] = useState(false);
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        // Only the stats call is allowed to fail the screen. Campaigns and the
        // information block are decoration — a 404 on either must not turn a
        // working event page into "Event not found".
        const [s, camps, items] = await Promise.all([
          getOrgEventStats(id),
          getOrgCampaigns().catch(() => [] as OrgCampaign[]),
          getOrgEventInfo(id).catch(() => [] as EventInfoItem[]),
        ]);
        if (!alive) return;
        setStats(s);
        setInfo(items);
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

  // Keeps "closes in 4h 12m" honest without a per-second timer.
  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), TICK_MS);
    return () => clearInterval(t);
  }, []);

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

  // The admission window the SERVER will enforce, recomputed from the same
  // inputs (see lib/organiser.ts → scanWindow, mirroring models.ScanWindow).
  const win = useMemo(
    () =>
      ev
        ? scanWindow(
            ev.start_time,
            ev.end_time,
            ev.scan_open_offset_min,
            ev.scan_close_offset_min,
          )
        : null,
    [ev],
  );
  const winState = useMemo(() => (win ? scanStatus(win, now) : null), [win, now]);
  const doors = useMemo(() => fmtDoors(infoValue(info, INFO_KEY_DOORS)), [info]);

  const venueLine = ev
    ? [ev.venue_name, ev.city].filter(Boolean).join(', ')
    : '';

  return (
    <View style={[styles.root, { paddingTop: insets.top }]}>
      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity onPress={() => router.back()} style={styles.iconBtn} hitSlop={10}>
          <Feather name="arrow-left" size={20} color={S.text} />
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
          icon={<Feather name="calendar" size={26} color={S.muted} />}
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
            {/* Title block. The date used to stand alone here, which is how a
                screen for people working a door managed not to say what time
                the door opens. */}
            <View style={{ gap: 8, marginBottom: 16 }}>
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
                <Text style={styles.evTitle}>{ev.title}</Text>
                <Chip status={ev.status} />
              </View>
              <Text style={styles.evMeta}>
                {fmtDate(ev.start_time)} · {fmtClock(ev.start_time)}
                {venueLine ? ` · ${venueLine}` : ''}
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
                  {/* ── Schedule & admission ──────────────────────────────
                      Times first, because someone opening this screen at a
                      venue is asking one of three questions: when does it
                      start, when can people come in, and can I scan yet. */}
                  <Card title="Schedule">
                    <TimeRow
                      icon="play-circle"
                      label="Starts"
                      value={`${fmtDayShort(ev.start_time)} · ${fmtClock(ev.start_time)}`}
                    />
                    <TimeRow
                      icon="stop-circle"
                      label="Ends"
                      value={
                        sameDay(ev.start_time, ev.end_time)
                          ? fmtClock(ev.end_time)
                          : `${fmtDayShort(ev.end_time)} · ${fmtClock(ev.end_time)}`
                      }
                    />
                    {/* Only rendered when the organiser filled the field in —
                        an absent doors_open is normal, not missing data. */}
                    {doors && (
                      <TimeRow icon="log-in" label="Doors open" value={doors} accent />
                    )}

                    {win && winState && (
                      <>
                        <View style={styles.hr} />
                        <TimeRow
                          icon="maximize"
                          label="Scanning"
                          value={`${fmtClock(win.open)} → ${fmtClock(win.close)}`}
                        />
                        <ScanStatusPill state={winState} />
                        <Text style={styles.windowNote}>
                          {win.customOpen || win.customClose
                            ? 'Admission window set by you on the web dashboard.'
                            : 'Default window: opens 2h before the start, closes 2h after the start (or 30 min before the end, whichever is sooner).'}
                        </Text>
                      </>
                    )}
                  </Card>

                  <View style={styles.grid}>
                    <StatCard
                      label="Revenue"
                      value={fmtMoney(stats!.total_revenue, ev.currency)}
                      sub="net earnings"
                      accent="green"
                      icon={<Feather name="dollar-sign" size={15} color={S.green} />}
                    />
                    <StatCard
                      label="Tickets sold"
                      value={fmtNum(ev.tickets_sold)}
                      sub={ev.max_capacity ? `of ${fmtNum(ev.max_capacity)}` : 'no cap'}
                      accent="accent"
                      icon={<Ionicons name="ticket-outline" size={15} color={S.accent} />}
                    />
                  </View>
                  <View style={styles.grid}>
                    <StatCard
                      label="Confirmed"
                      value={fmtNum(stats!.confirmed_bookings)}
                      sub={`${fmtNum(stats!.pending_bookings)} pending`}
                      accent="blue"
                      icon={<Feather name="check-circle" size={15} color={S.blue} />}
                    />
                    <StatCard
                      label="Page views"
                      value={fmtNum(ev.views)}
                      sub="total"
                      accent="amber"
                      icon={<Feather name="eye" size={15} color={S.amber} />}
                    />
                  </View>

                  {pct !== null && (
                    <Card title="Capacity">
                      <View style={styles.capRow}>
                        <Text style={styles.capTxt}>
                          {fmtNum(ev.tickets_sold)} / {fmtNum(ev.max_capacity!)}
                        </Text>
                        <Text style={[styles.capPct, { color: capColor(pct, S) }]}>
                          {pct.toFixed(0)}%
                        </Text>
                      </View>
                      <ProgressBar pct={pct} color={capColor(pct, S)} height={8} />
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
                      icon={<Feather name="users" size={24} color={S.muted} />}
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

          {/* Scan this event's tickets.
              Deliberately still enabled when the window has closed: the phone's
              clock is not the authority, the server is, and blocking here would
              turn a wrong device time into a locked gate. The Schedule card
              above says what to expect; the scanner reports the server's word. */}
          <View style={[styles.footer, { paddingBottom: insets.bottom + 10 }]}>
            <TouchableOpacity
              style={[styles.scanBtn, styles.scanBtnSecondary]}
              activeOpacity={0.85}
              onPress={() =>
                router.push({
                  pathname: '/organiser/staff/[eventId]',
                  params: { eventId: ev.id, title: ev.title },
                } as any)
              }
            >
              <Ionicons name="people-outline" size={18} color={S.text} />
              <Text style={[styles.scanBtnTxt, { color: S.text }]}>Manage door staff</Text>
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
              <Ionicons name="qr-code" size={18} color={S.onAccent} />
              <Text style={styles.scanBtnTxt}>Scan tickets for this event</Text>
            </TouchableOpacity>
          </View>
        </>
      )}
    </View>
  );
}

// ── Schedule rows ─────────────────────────────────────────────────────────────

function TimeRow({
  icon,
  label,
  value,
  accent,
}: {
  icon: keyof typeof Feather.glyphMap;
  label: string;
  value: string;
  accent?: boolean;
}) {
  const { S } = useStudioTheme();
  const styles = useStyles();
  return (
    <View style={styles.timeRow}>
      <Feather name={icon} size={14} color={accent ? S.accent : S.muted} />
      <Text style={styles.timeLabel}>{label}</Text>
      <Text style={[styles.timeValue, accent && { color: S.accent }]} numberOfLines={1}>
        {value}
      </Text>
    </View>
  );
}

function ScanStatusPill({
  state,
}: {
  state: { phase: 'before' | 'open' | 'closed'; msRemaining: number };
}) {
  const { S } = useStudioTheme();
  const styles = useStyles();

  const { fg, bg, icon, text } =
    state.phase === 'open'
      ? {
          fg: S.green,
          bg: S.greenSoft,
          icon: 'checkmark-circle' as const,
          text: `Scanning is open · closes in ${fmtDuration(state.msRemaining)}`,
        }
      : state.phase === 'before'
      ? {
          fg: S.accent,
          bg: S.accentSoft,
          icon: 'time-outline' as const,
          text: `Scanning opens in ${fmtDuration(state.msRemaining)}`,
        }
      : {
          fg: S.red,
          bg: S.redSoft,
          icon: 'close-circle' as const,
          // Same wording as the scanner's own error, so the two screens
          // obviously describe one thing rather than two unrelated failures.
          text: "Admission window has ended — scanning is closed",
        };

  return (
    <View style={[styles.statusPill, { backgroundColor: bg }]}>
      <Ionicons name={icon} size={14} color={fg} />
      <Text style={[styles.statusTxt, { color: fg }]}>{text}</Text>
    </View>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  const styles = useStyles();
  return (
    <View style={{ flex: 1, minWidth: 0 }}>
      <Text style={styles.metricLabel}>{label.toUpperCase()}</Text>
      <Text style={styles.metricValue} numberOfLines={1}>
        {value}
      </Text>
    </View>
  );
}

const useStyles = makeStyles((S) => ({
  root: { flex: 1, backgroundColor: S.bg },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 12,
    paddingVertical: 10,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: S.border,
  },
  iconBtn: {
    width: 36,
    height: 36,
    borderRadius: 18,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: S.surfaceStrong,
  },
  headerTitle: {
    flex: 1,
    textAlign: 'center',
    color: S.text,
    fontSize: 16,
    fontWeight: '700',
    marginHorizontal: 8,
  },

  evTitle: { color: S.text, fontSize: 22, fontWeight: '800', letterSpacing: -0.5, flexShrink: 1 },
  evMeta: { color: S.muted, fontSize: 13 },

  segment: {
    flexDirection: 'row',
    backgroundColor: S.surfaceStrong,
    borderRadius: 12,
    padding: 4,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: S.border,
  },
  segItem: { flex: 1, alignItems: 'center', paddingVertical: 9, borderRadius: 9 },
  segItemActive: { backgroundColor: S.accentDeep },
  segTxt: { color: S.sub, fontSize: 13, fontWeight: '700' },
  segTxtActive: { color: S.onAccent },

  grid: { flexDirection: 'row', gap: 12 },

  // Schedule
  timeRow: { flexDirection: 'row', alignItems: 'center', gap: 9, paddingVertical: 6 },
  timeLabel: { color: S.sub, fontSize: 13, flex: 1 },
  timeValue: { color: S.text, fontSize: 13.5, fontWeight: '700', flexShrink: 1 },
  hr: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: S.border,
    marginVertical: 8,
  },
  statusPill: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 7,
    borderRadius: 10,
    paddingHorizontal: 11,
    paddingVertical: 9,
    marginTop: 6,
  },
  statusTxt: { fontSize: 12.5, fontWeight: '700', flex: 1 },
  windowNote: { color: S.faint, fontSize: 11.5, lineHeight: 16, marginTop: 8 },

  capRow: { flexDirection: 'row', justifyContent: 'space-between', marginBottom: 8 },
  capTxt: { color: S.sub, fontSize: 13, fontWeight: '600' },
  capPct: { fontSize: 13, fontWeight: '800' },
  capWarn: { color: S.red, fontSize: 12, fontWeight: '700', marginTop: 8 },

  metricRow: { flexDirection: 'row', gap: 10 },
  metricLabel: { color: S.faint, fontSize: 9.5, fontWeight: '700', letterSpacing: 0.6 },
  metricValue: { color: S.text, fontSize: 16, fontWeight: '700', marginTop: 3 },

  bookingRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    paddingHorizontal: 16,
    paddingVertical: 13,
  },
  rowDivider: { borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: S.border },
  bookName: { color: S.text, fontSize: 14.5, fontWeight: '700' },
  bookMeta: { color: S.muted, fontSize: 12 },
  bookAmt: { color: S.green, fontSize: 14, fontWeight: '800' },

  footer: {
    position: 'absolute',
    left: 0,
    right: 0,
    bottom: 0,
    paddingHorizontal: 16,
    paddingTop: 10,
    backgroundColor: S.overlay,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: S.border,
  },
  scanBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 9,
    backgroundColor: S.accentDeep,
    borderRadius: 14,
    paddingVertical: 15,
  },
  // Was a hardcoded rgba(255,255,255,0.08) fill with #fff text — invisible on
  // a light surface. Both halves now come from the palette.
  scanBtnSecondary: {
    backgroundColor: S.btnSecondaryBg,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: S.border,
    marginBottom: 10,
  },
  scanBtnTxt: { color: S.onAccent, fontSize: 15, fontWeight: '800' },
}));