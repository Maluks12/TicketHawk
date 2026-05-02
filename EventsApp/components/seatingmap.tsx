import React, { useMemo } from 'react';
import {
  View,
  Text,
  TouchableOpacity,
  Dimensions,
} from 'react-native';
import type { TicketType } from '../lib/api';

const { width: SW } = Dimensions.get('window');

// ─── Types ────────────────────────────────────────────────────────────────────

export type VenueType = 'stadium' | 'auditorium' | 'none';

interface SeatingMapProps {
  venueType: VenueType;
  ticketTypes: TicketType[];
  selectedTypeId: string | null;
  onSelectType: (typeId: string) => void;
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

export function detectVenueType(category: string, venueName?: string): VenueType {
  const cat   = category.toLowerCase();
  const venue = (venueName ?? '').toLowerCase();
  if (cat.includes('sport') || cat.includes('soccer') || cat.includes('football') ||
      cat.includes('rugby') || cat.includes('cricket') ||
      venue.includes('stadium') || venue.includes('park') ||
      venue.includes('arena') || venue.includes('field')) return 'stadium';
  if (cat.includes('concert') || cat.includes('music') || cat.includes('show') ||
      cat.includes('theatre') || cat.includes('theater') || cat.includes('comedy') ||
      cat.includes('opera') || cat.includes('dance') || cat.includes('performance') ||
      venue.includes('theatre') || venue.includes('theater') ||
      venue.includes('hall') || venue.includes('auditorium')) return 'auditorium';
  return 'none';
}

// ─── Color Palette ────────────────────────────────────────────────────────────

const C = {
  upper:    { bg: '#9B8DC8', border: '#7A6AAD', text: '#fff' },
  lower:    { bg: '#5A9E60', border: '#3D7A43', text: '#fff' },
  club:     { bg: '#5B9BD5', border: '#3A7DBD', text: '#fff' },
  access:   { bg: '#F0A030', border: '#C07B10', text: '#fff' },
  unavail:  { bg: '#C0C4D0', border: '#A0A6BA', text: '#666' },
  selected: { bg: '#1d6cf5', border: '#1558d0', text: '#fff' },
};

// ─── Layout constants ─────────────────────────────────────────────────────────

const MAP_W = SW - 32;

// Band thicknesses
const UH = Math.round(MAP_W * 0.108); // upper tier top/bottom row height
const CH = Math.round(MAP_W * 0.088); // club tier top/bottom row height
const LH = Math.round(MAP_W * 0.072); // lower tier top/bottom row height

const MAP_H = Math.round(MAP_W * 0.82); // total map height

// Side column widths
const UW = Math.round(MAP_W * 0.112); // upper left/right col
const CW = Math.round(MAP_W * 0.092); // club left/right col
const LW = Math.round(MAP_W * 0.074); // lower left/right col

// Pitch size (derived)
const PW = MAP_W - 2 * (UW + CW + LW);
const PH = MAP_H - 2 * (UH + CH + LH);

const GAP = 1.5;

// ─── Section labels ───────────────────────────────────────────────────────────

const TOP_UPPER   = ['301','302','303','304','305','306','307'];
const BOT_UPPER   = ['326','325','324','323','322','320','319'];
const LEFT_UPPER  = ['331','330','329','328','327','326'];
const RIGHT_UPPER = ['309','310','311','312','313','318'];

const TOP_CLUB    = ['201','202','203','204','205','206'];
const BOT_CLUB    = ['221','223','224','225','226','227'];
const LEFT_CLUB   = ['231','230','228','227'];
const RIGHT_CLUB  = ['207','208','209','210'];

const TOP_LOWER   = ['101','102','103','104','105','106'];
const BOT_LOWER   = ['118','117','116','115','114','112'];
const LEFT_LOWER  = ['138','137','136'];
const RIGHT_LOWER = ['108','109','111'];

// ─── Stadium Map Component ────────────────────────────────────────────────────

function StadiumMap({ ticketTypes, selectedTypeId, onSelectType }: Omit<SeatingMapProps, 'venueType'>) {
  const sorted = useMemo(() => [...ticketTypes].sort((a, b) => a.price - b.price), [ticketTypes]);

  function ttFor(idx: number): TicketType | null {
    if (!sorted.length) return null;
    if (idx === 0) return sorted[0];
    if (idx === 2) return sorted[sorted.length - 1];
    return sorted[Math.min(1, sorted.length - 1)];
  }

  function isSel(idx: number) { return ttFor(idx)?.id === selectedTypeId; }
  function press(idx: number) { const tt = ttFor(idx); if (tt) onSelectType(tt.id); }

  function cc(tier: number) {
    if (isSel(tier)) return C.selected;
    return [C.upper, C.lower, C.club][tier] ?? C.unavail;
  }

  // ── Sub-components ──────────────────────────────────────────────────────────

  const Tile = ({ label, tier, style }: { label: string; tier: number; style?: object }) => {
    const col = cc(tier);
    const sel = isSel(tier);
    return (
      <TouchableOpacity
        onPress={() => press(tier)}
        activeOpacity={0.72}
        style={[{
          backgroundColor: col.bg,
          alignItems: 'center',
          justifyContent: 'center',
          borderRadius: 2,
          borderWidth: sel ? 1.5 : 0.5,
          borderColor: sel ? '#fff' : 'rgba(255,255,255,0.18)',
        }, style]}
      >
        <Text style={{
          fontSize: 5.8,
          fontWeight: '800',
          color: col.text,
          textAlign: 'center',
          lineHeight: 7.5,
          letterSpacing: 0.1,
        }}>
          {label}
        </Text>
      </TouchableOpacity>
    );
  };

  const TileRow = ({ labels, tier, height }: { labels: string[]; tier: number; height: number }) => (
    <View style={{ flexDirection: 'row', height, gap: GAP }}>
      {labels.map((lbl, i) => <Tile key={i} label={lbl} tier={tier} style={{ flex: 1 }} />)}
    </View>
  );

  const TileCol = ({ labels, tier, width }: { labels: string[]; tier: number; width: number }) => (
    <View style={{ width, gap: GAP }}>
      {labels.map((lbl, i) => <Tile key={i} label={lbl} tier={tier} style={{ flex: 1 }} />)}
    </View>
  );

  const Corner = ({ w, h, type }: { w: number; h: number; type: 'access' | 'unavail' }) => (
    <View style={{
      width: w, height: h,
      backgroundColor: type === 'access' ? C.access.bg : C.unavail.bg,
      borderRadius: 2,
      borderWidth: 0.5,
      borderColor: type === 'access' ? C.access.border : C.unavail.border,
    }} />
  );

  // Football pitch with markings
  const Pitch = () => {
    const circR = PH * 0.22;
    return (
      <View style={{
        flex: 1,
        backgroundColor: '#2E7D32',
        borderRadius: 3,
        overflow: 'hidden',
      }}>
        {/* Centre line */}
        <View style={{
          position: 'absolute',
          left: '50%', top: 0, bottom: 0,
          width: 1, backgroundColor: 'rgba(255,255,255,0.42)',
        }} />

        {/* Centre circle */}
        <View style={{
          position: 'absolute',
          width: circR * 2, height: circR * 2,
          borderRadius: circR,
          borderWidth: 1, borderColor: 'rgba(255,255,255,0.42)',
          left: PW / 2 - circR,
          top: PH / 2 - circR,
        }} />

        {/* Centre spot */}
        <View style={{
          position: 'absolute',
          width: 4, height: 4, borderRadius: 2,
          backgroundColor: 'rgba(255,255,255,0.6)',
          left: PW / 2 - 2, top: PH / 2 - 2,
        }} />

        {/* Left penalty box */}
        <View style={{
          position: 'absolute',
          left: 0, top: PH * 0.2,
          width: PW * 0.155, height: PH * 0.6,
          borderRightWidth: 1, borderTopWidth: 1, borderBottomWidth: 1,
          borderColor: 'rgba(255,255,255,0.4)',
        }} />

        {/* Right penalty box */}
        <View style={{
          position: 'absolute',
          right: 0, top: PH * 0.2,
          width: PW * 0.155, height: PH * 0.6,
          borderLeftWidth: 1, borderTopWidth: 1, borderBottomWidth: 1,
          borderColor: 'rgba(255,255,255,0.4)',
        }} />

        {/* Left 6-yard box */}
        <View style={{
          position: 'absolute',
          left: 0, top: PH * 0.33,
          width: PW * 0.07, height: PH * 0.34,
          borderRightWidth: 1, borderTopWidth: 1, borderBottomWidth: 1,
          borderColor: 'rgba(255,255,255,0.26)',
        }} />

        {/* Right 6-yard box */}
        <View style={{
          position: 'absolute',
          right: 0, top: PH * 0.33,
          width: PW * 0.07, height: PH * 0.34,
          borderLeftWidth: 1, borderTopWidth: 1, borderBottomWidth: 1,
          borderColor: 'rgba(255,255,255,0.26)',
        }} />

        {/* Left penalty spot */}
        <View style={{
          position: 'absolute',
          width: 3, height: 3, borderRadius: 1.5,
          backgroundColor: 'rgba(255,255,255,0.55)',
          left: PW * 0.115 - 1.5, top: PH / 2 - 1.5,
        }} />

        {/* Right penalty spot */}
        <View style={{
          position: 'absolute',
          width: 3, height: 3, borderRadius: 1.5,
          backgroundColor: 'rgba(255,255,255,0.55)',
          right: PW * 0.115 - 1.5, top: PH / 2 - 1.5,
        }} />
      </View>
    );
  };

  // ── Render ──────────────────────────────────────────────────────────────────

  return (
    <View style={{
      width: MAP_W,
      height: MAP_H,
      alignSelf: 'center',
      backgroundColor: '#D5D9E4',
      borderRadius: 8,
      overflow: 'hidden',
      gap: GAP,
      padding: GAP,
    }}>

      {/* ── Row 1: Top upper tier ─────────────────────────────────── */}
      <View style={{ flexDirection: 'row', height: UH, gap: GAP }}>
        <Corner w={UW} h={UH} type="unavail" />
        {TOP_UPPER.map((lbl, i) => <Tile key={i} label={lbl} tier={0} style={{ flex: 1 }} />)}
        <Corner w={UW} h={UH} type="unavail" />
      </View>

      {/* ── Row 2: Middle ─────────────────────────────────────────── */}
      <View style={{ flex: 1, flexDirection: 'row', gap: GAP }}>

        {/* Left upper column */}
        <TileCol labels={LEFT_UPPER} tier={0} width={UW} />

        {/* ── Inner zone ─────────────────────────────────────────── */}
        <View style={{ flex: 1, gap: GAP }}>

          {/* Club top + accessible corners */}
          <View style={{ flexDirection: 'row', height: CH, gap: GAP }}>
            <Corner w={CW} h={CH} type="access" />
            {TOP_CLUB.map((lbl, i) => <Tile key={i} label={lbl} tier={2} style={{ flex: 1 }} />)}
            <Corner w={CW} h={CH} type="access" />
          </View>

          {/* Club side columns + lower ring + pitch */}
          <View style={{ flex: 1, flexDirection: 'row', gap: GAP }}>

            {/* Left club column */}
            <TileCol labels={LEFT_CLUB} tier={2} width={CW} />

            {/* Lower ring + pitch */}
            <View style={{ flex: 1, gap: GAP }}>
              <TileRow labels={TOP_LOWER} tier={1} height={LH} />

              {/* Pitch with lower side cols */}
              <View style={{ flex: 1, flexDirection: 'row', gap: GAP }}>
                <TileCol labels={LEFT_LOWER} tier={1} width={LW} />
                <Pitch />
                <TileCol labels={RIGHT_LOWER} tier={1} width={LW} />
              </View>

              <TileRow labels={BOT_LOWER} tier={1} height={LH} />
            </View>

            {/* Right club column */}
            <TileCol labels={RIGHT_CLUB} tier={2} width={CW} />
          </View>

          {/* Club bottom + accessible corners */}
          <View style={{ flexDirection: 'row', height: CH, gap: GAP }}>
            <Corner w={CW} h={CH} type="access" />
            {BOT_CLUB.map((lbl, i) => <Tile key={i} label={lbl} tier={2} style={{ flex: 1 }} />)}
            <Corner w={CW} h={CH} type="access" />
          </View>
        </View>

        {/* Right upper column */}
        <TileCol labels={RIGHT_UPPER} tier={0} width={UW} />
      </View>

      {/* ── Row 3: Bottom upper tier ──────────────────────────────── */}
      <View style={{ flexDirection: 'row', height: UH, gap: GAP }}>
        <Corner w={UW} h={UH} type="access" />
        {BOT_UPPER.map((lbl, i) => <Tile key={i} label={lbl} tier={0} style={{ flex: 1 }} />)}
        <Corner w={UW} h={UH} type="access" />
      </View>
    </View>
  );
}

// ─── Map Legend ───────────────────────────────────────────────────────────────

export function MapLegend({
  ticketTypes,
  selectedTypeId,
}: {
  ticketTypes: TicketType[];
  selectedTypeId: string | null;
}) {
  const sorted = [...ticketTypes].sort((a, b) => a.price - b.price);
  const tiers  = [C.upper, C.lower, C.club];
  const labels = ['Upper', 'Lower', 'Club'];

  return (
    <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: 10 }}>
      {sorted.map((tt, i) => {
        const sel = tt.id === selectedTypeId;
        const col = sel ? C.selected : (tiers[i] ?? C.upper);
        return (
          <View key={tt.id} style={{ flexDirection: 'row', alignItems: 'center', gap: 5 }}>
            <View style={{ width: 11, height: 11, borderRadius: 2, backgroundColor: col.bg, borderWidth: 1, borderColor: col.border }} />
            <Text style={{ fontSize: 10, fontWeight: sel ? '700' : '500', color: sel ? C.selected.bg : '#555' }}>
              {labels[i] ?? tt.name}
            </Text>
          </View>
        );
      })}
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 5 }}>
        <View style={{ width: 11, height: 11, borderRadius: 2, backgroundColor: C.access.bg, borderWidth: 1, borderColor: C.access.border }} />
        <Text style={{ fontSize: 10, fontWeight: '500', color: '#555' }}>Accessible</Text>
      </View>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 5 }}>
        <View style={{ width: 11, height: 11, borderRadius: 2, backgroundColor: C.unavail.bg, borderWidth: 1, borderColor: C.unavail.border }} />
        <Text style={{ fontSize: 10, fontWeight: '500', color: '#555' }}>Unavailable</Text>
      </View>
    </View>
  );
}

// ─── Auditorium Map ───────────────────────────────────────────────────────────

const AUD_COLORS = [
  { bg: '#d4dae6', border: '#b0bac8', text: '#5a6478' },
  { bg: '#3a7d44', border: '#3a7d44', text: '#fff' },
  { bg: '#1a2e5e', border: '#1a2e5e', text: '#fff' },
  { bg: '#c8962a', border: '#c8962a', text: '#fff' },
];

const AUD_ROWS = [
  [{ id:'BAL_L', label:'BALCONY\nLEFT', tier:0 }, { id:'BAL_C', label:'BALCONY\nCENTER', tier:0 }, { id:'BAL_R', label:'BALCONY\nRIGHT', tier:0 }],
  [{ id:'MEZ_L', label:'MEZZ\nLEFT', tier:1 },    { id:'MEZ_C', label:'MEZZ\nCENTER', tier:1 },    { id:'MEZ_R', label:'MEZZ\nRIGHT', tier:1 }],
  [{ id:'ORC_L', label:'ORCH\nLEFT', tier:2 },    { id:'ORC_C', label:'ORCH\nCENTER', tier:2 },    { id:'ORC_R', label:'ORCH\nRIGHT', tier:2 }],
  [{ id:'PIT',   label:'PIT / GA', tier:3, wide:true }],
];

const SEL_AUD = { bg: '#1d6cf5', border: '#1558d0', text: '#fff' };

function AuditoriumMap({ ticketTypes, selectedTypeId, onSelectType }: Omit<SeatingMapProps, 'venueType'>) {
  const sorted = useMemo(() => [...ticketTypes].sort((a, b) => a.price - b.price), [ticketTypes]);
  function ttForTier(t: number): TicketType | null {
    if (!sorted.length) return null;
    const idx = Math.round((t / 3) * (sorted.length - 1));
    return sorted[Math.min(idx, sorted.length - 1)];
  }
  return (
    <View style={{ gap: 5 }}>
      <View style={{ backgroundColor: '#111', borderRadius: 6, paddingVertical: 10, alignItems: 'center', marginBottom: 4 }}>
        <Text style={{ fontSize: 9, fontWeight: '900', letterSpacing: 4, color: '#fff' }}>S T A G E</Text>
      </View>
      {AUD_ROWS.map((row, ri) => (
        <View key={ri} style={{ flexDirection: 'row', gap: 5 }}>
          {row.map((sec: any) => {
            const tt  = ttForTier(sec.tier);
            const sel = tt?.id === selectedTypeId;
            const col = sel ? SEL_AUD : AUD_COLORS[Math.min(sec.tier, AUD_COLORS.length - 1)];
            return (
              <TouchableOpacity key={sec.id} onPress={() => tt && onSelectType(tt.id)}
                style={{ flex: sec.wide ? 3 : 1, paddingVertical: 14, borderRadius: 6,
                  alignItems: 'center', justifyContent: 'center',
                  borderWidth: sel ? 2 : 1, backgroundColor: col.bg, borderColor: col.border }}>
                <Text style={{ fontSize: 8, fontWeight: '800', letterSpacing: 0.4, textAlign: 'center', lineHeight: 12, color: col.text }}>
                  {sec.label}
                </Text>
              </TouchableOpacity>
            );
          })}
        </View>
      ))}
    </View>
  );
}

// ─── Main export ──────────────────────────────────────────────────────────────

export default function SeatingMap(props: SeatingMapProps) {
  const { venueType, ...rest } = props;
  if (venueType === 'stadium')    return <StadiumMap   {...rest} />;
  if (venueType === 'auditorium') return <AuditoriumMap {...rest} />;
  return null;
}
