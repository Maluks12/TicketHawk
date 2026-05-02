import React, { useMemo } from 'react';
import {
  View,
  Text,
  TouchableOpacity,
  StyleSheet,
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

// ─── Color tokens ─────────────────────────────────────────────────────────────

// tier 0 = cheapest (upper/gray), 1 = lower (navy), 2 = club (gold)
const TIER_COLORS = [
  { bg: '#c8cdd8', border: '#a8b0bf', text: '#4a5568' },   // upper
  { bg: '#1a2e5e', border: '#0f1e3e', text: '#ffffff' },   // lower
  { bg: '#b8820f', border: '#8a6200', text: '#ffffff' },   // club
];
const SEL = { bg: '#1d6cf5', border: '#1558d0', text: '#ffffff' };

function tc(i: number) { return TIER_COLORS[Math.min(i, TIER_COLORS.length - 1)]; }

// ─── Geometry helpers ─────────────────────────────────────────────────────────

const MAP_W   = SW - 32;
const MAP_H   = MAP_W * 0.82;   // slightly taller for a football oval
const CX      = MAP_W / 2;
const CY      = MAP_H / 2;

// Ellipse radii for each tier ring centre-line
const RX_UPPER = MAP_W * 0.455;
const RY_UPPER = MAP_H * 0.455;

const RX_CLUB  = MAP_W * 0.335;
const RY_CLUB  = MAP_H * 0.335;

const RX_LOWER = MAP_W * 0.225;
const RY_LOWER = MAP_H * 0.255;

// Pitch
const RX_PITCH = MAP_W * 0.148;
const RY_PITCH = MAP_H * 0.185;

// Oval ring widths (for background layers)
const BG_UPPER_W  = MAP_W;
const BG_UPPER_H  = MAP_H;
const BG_CLUB_W   = MAP_W * 0.76;
const BG_CLUB_H   = MAP_H * 0.76;
const BG_LOWER_W  = MAP_W * 0.558;
const BG_LOWER_H  = MAP_H * 0.604;
const BG_PITCH_W  = MAP_W * 0.355;
const BG_PITCH_H  = MAP_H * 0.445;

// Convert polar → absolute left/top for a section tile
function pos(rx: number, ry: number, angleDeg: number) {
  const rad = (angleDeg * Math.PI) / 180;
  return {
    left: CX + rx * Math.cos(rad),
    top:  CY + ry * Math.sin(rad),
  };
}

// ─── Section definitions ──────────────────────────────────────────────────────
// angle 0=right, -90=top, 90=bottom, 180/±180=left

// Upper tier — 20 sections spread around the full oval
const UPPER_SECTIONS = [
  { label:'301', angle:-155 },
  { label:'302', angle:-135 },
  { label:'301', angle:-115 },
  { label:'302', angle:-90  },
  { label:'302', angle:-65  },
  { label:'301', angle:-45  },
  { label:'301', angle:-25  },
  { label:'303', angle:  0  },
  { label:'304', angle: 25  },
  { label:'304', angle: 45  },
  { label:'305', angle: 65  },
  { label:'306', angle: 90  },
  { label:'307', angle:115  },
  { label:'307', angle:135  },
  { label:'302', angle:155  },
  { label:'303', angle: 180 },
];

// Club level — 6 wide sections
const CLUB_SECTIONS = [
  { label:'CLUB\n201', angle:-90  },
  { label:'CLUB\n201', angle:-30  },
  { label:'CLUB\n201', angle: 30  },
  { label:'CLUB\n202', angle: 90  },
  { label:'CLUB\n202', angle:150  },
  { label:'CLUB\n202', angle:-150 },
];

// Lower tier — 16 sections tightly around the pitch
const LOWER_SECTIONS = [
  { label:'101', angle:-155 },
  { label:'102', angle:-130 },
  { label:'103', angle:-105 },
  { label:'104', angle:-80  },
  { label:'105', angle:-55  },
  { label:'106', angle:-30  },
  { label:'107', angle:  0  },
  { label:'108', angle: 30  },
  { label:'105', angle: 55  },
  { label:'104', angle: 80  },
  { label:'103', angle:105  },
  { label:'102', angle:130  },
  { label:'101', angle:155  },
  { label:'100', angle:180  },
  { label:'109', angle:-180 },
];

// ─── Stadium Map ──────────────────────────────────────────────────────────────

function StadiumMap({ ticketTypes, selectedTypeId, onSelectType }: Omit<SeatingMapProps, 'venueType'>) {
  const sorted = useMemo(() => [...ticketTypes].sort((a, b) => a.price - b.price), [ticketTypes]);

  function ttFor(tierIdx: number): TicketType | null {
    if (!sorted.length) return null;
    if (tierIdx === 0) return sorted[0];
    if (tierIdx === 2) return sorted[sorted.length - 1];
    return sorted[Math.min(1, sorted.length - 1)];
  }

  function isSel(tierIdx: number) { return ttFor(tierIdx)?.id === selectedTypeId; }
  function press(tierIdx: number) { const tt = ttFor(tierIdx); if (tt) onSelectType(tt.id); }

  // Section tile style per tier
  function tileStyle(tierIdx: number) {
    const sel = isSel(tierIdx);
    const c   = sel ? SEL : tc(tierIdx);
    return {
      backgroundColor: c.bg,
      borderColor:     c.border,
      borderWidth:     sel ? 1.5 : 1,
    };
  }
  function textStyle(tierIdx: number) {
    return { color: isSel(tierIdx) ? SEL.text : tc(tierIdx).text };
  }

  // Oval background helper
  const ovalLayer = (w: number, h: number, color: string, borderColor: string) => ({
    position: 'absolute' as const,
    width:  w,
    height: h,
    left:   (MAP_W - w) / 2,
    top:    (MAP_H - h) / 2,
    borderRadius: h / 2,
    backgroundColor: color,
    borderWidth: 1,
    borderColor,
  });

  // Section tile sizes
  const UT_W = 28, UT_H = 16;   // upper tile
  const CT_W = 32, CT_H = 20;   // club tile
  const LT_W = 24, LT_H = 14;   // lower tile

  return (
    <View style={{ width: MAP_W, height: MAP_H, position: 'relative', alignSelf: 'center' }}>

      {/* ── Oval background layers (outermost → innermost) ── */}

      {/* Upper tier oval (full stadium shape) */}
      <View style={ovalLayer(BG_UPPER_W, BG_UPPER_H,
        isSel(0) ? SEL.bg       : tc(0).bg,
        isSel(0) ? SEL.border   : tc(0).border)} />

      {/* Club oval */}
      <View style={ovalLayer(BG_CLUB_W, BG_CLUB_H,
        isSel(2) ? SEL.bg     : tc(2).bg,
        isSel(2) ? SEL.border : tc(2).border)} />

      {/* Lower tier oval */}
      <View style={ovalLayer(BG_LOWER_W, BG_LOWER_H,
        isSel(1) ? SEL.bg     : tc(1).bg,
        isSel(1) ? SEL.border : tc(1).border)} />

      {/* Pitch oval (green, not tappable) */}
      <View style={[ovalLayer(BG_PITCH_W, BG_PITCH_H, '#2e7d32', '#1b5e20'), { overflow: 'hidden' }]}>
        {/* Field markings */}
        <View style={{ position:'absolute', left:0, right:0, top:'50%', height:1, backgroundColor:'rgba(255,255,255,0.35)' }} />
        <View style={{
          position:'absolute',
          width: BG_PITCH_W * 0.42,
          height: BG_PITCH_W * 0.42,
          borderRadius: BG_PITCH_W * 0.21,
          borderWidth: 1,
          borderColor: 'rgba(255,255,255,0.40)',
          alignSelf: 'center',
          top: (BG_PITCH_H - BG_PITCH_W * 0.42) / 2,
        }} />
        {/* Goal boxes */}
        <View style={{ position:'absolute', alignSelf:'center', top:4, width:'52%', height:'20%', borderWidth:1, borderColor:'rgba(255,255,255,0.30)', borderRadius:2 }} />
        <View style={{ position:'absolute', alignSelf:'center', bottom:4, width:'52%', height:'20%', borderWidth:1, borderColor:'rgba(255,255,255,0.30)', borderRadius:2 }} />
        {/* Small goal areas */}
        <View style={{ position:'absolute', alignSelf:'center', top:4, width:'28%', height:'9%', borderWidth:1, borderColor:'rgba(255,255,255,0.25)', borderRadius:1 }} />
        <View style={{ position:'absolute', alignSelf:'center', bottom:4, width:'28%', height:'9%', borderWidth:1, borderColor:'rgba(255,255,255,0.25)', borderRadius:1 }} />
        {/* Penalty spots */}
        <View style={{ position:'absolute', alignSelf:'center', top:'24%', width:4, height:4, borderRadius:2, backgroundColor:'rgba(255,255,255,0.5)' }} />
        <View style={{ position:'absolute', alignSelf:'center', bottom:'24%', width:4, height:4, borderRadius:2, backgroundColor:'rgba(255,255,255,0.5)' }} />
      </View>

      {/* ── Touchable overlay rings — make entire oval band tappable ── */}

      {/* Upper tier ring touch area (tap outermost oval, not covered by club oval) */}
      <TouchableOpacity
        onPress={() => press(0)}
        style={{
          position: 'absolute',
          width: BG_UPPER_W, height: BG_UPPER_H,
          left: 0, top: 0,
          borderRadius: BG_UPPER_H / 2,
          backgroundColor: 'transparent',
        }}
        activeOpacity={0.6}
      />

      {/* Club touch area */}
      <TouchableOpacity
        onPress={() => press(2)}
        style={{
          position: 'absolute',
          width: BG_CLUB_W, height: BG_CLUB_H,
          left: (MAP_W - BG_CLUB_W) / 2,
          top:  (MAP_H - BG_CLUB_H) / 2,
          borderRadius: BG_CLUB_H / 2,
          backgroundColor: 'transparent',
        }}
        activeOpacity={0.6}
      />

      {/* Lower tier touch area */}
      <TouchableOpacity
        onPress={() => press(1)}
        style={{
          position: 'absolute',
          width: BG_LOWER_W, height: BG_LOWER_H,
          left: (MAP_W - BG_LOWER_W) / 2,
          top:  (MAP_H - BG_LOWER_H) / 2,
          borderRadius: BG_LOWER_H / 2,
          backgroundColor: 'transparent',
        }}
        activeOpacity={0.6}
      />

      {/* ── Section label tiles — positioned on each oval ring ── */}

      {/* Upper tier labels */}
      {UPPER_SECTIONS.map((sec, i) => {
        const { left, top } = pos(RX_UPPER, RY_UPPER, sec.angle);
        return (
          <TouchableOpacity
            key={`u${i}`}
            onPress={() => press(0)}
            activeOpacity={0.75}
            style={[
              {
                position: 'absolute',
                width: UT_W, height: UT_H,
                left: left - UT_W / 2,
                top:  top  - UT_H / 2,
                borderRadius: 3,
                alignItems: 'center',
                justifyContent: 'center',
              },
              tileStyle(0),
            ]}
          >
            <Text style={[{ fontSize: 5.5, fontWeight: '800', letterSpacing: 0.2, textAlign: 'center' }, textStyle(0)]}>
              {sec.label}
            </Text>
          </TouchableOpacity>
        );
      })}

      {/* Club labels */}
      {CLUB_SECTIONS.map((sec, i) => {
        const { left, top } = pos(RX_CLUB, RY_CLUB, sec.angle);
        return (
          <TouchableOpacity
            key={`c${i}`}
            onPress={() => press(2)}
            activeOpacity={0.75}
            style={[
              {
                position: 'absolute',
                width: CT_W, height: CT_H,
                left: left - CT_W / 2,
                top:  top  - CT_H / 2,
                borderRadius: 3,
                alignItems: 'center',
                justifyContent: 'center',
              },
              tileStyle(2),
            ]}
          >
            <Text style={[{ fontSize: 5, fontWeight: '900', letterSpacing: 0.3, textAlign: 'center', lineHeight: 7 }, textStyle(2)]}>
              {sec.label}
            </Text>
          </TouchableOpacity>
        );
      })}

      {/* Lower tier labels */}
      {LOWER_SECTIONS.map((sec, i) => {
        const { left, top } = pos(RX_LOWER, RY_LOWER, sec.angle);
        return (
          <TouchableOpacity
            key={`l${i}`}
            onPress={() => press(1)}
            activeOpacity={0.75}
            style={[
              {
                position: 'absolute',
                width: LT_W, height: LT_H,
                left: left - LT_W / 2,
                top:  top  - LT_H / 2,
                borderRadius: 2,
                alignItems: 'center',
                justifyContent: 'center',
              },
              tileStyle(1),
            ]}
          >
            <Text style={[{ fontSize: 5, fontWeight: '800', letterSpacing: 0.2, textAlign: 'center' }, textStyle(1)]}>
              {sec.label}
            </Text>
          </TouchableOpacity>
        );
      })}

      {/* Compass */}
      <View style={{ position:'absolute', bottom:8, right:10, alignItems:'center', opacity:0.55 }}>
        <Text style={{ fontSize:7, fontWeight:'900', color:'#333' }}>N</Text>
        <View style={{ width:5, height:5, borderRadius:2.5, backgroundColor:'#333', marginVertical:1 }} />
        <Text style={{ fontSize:7, fontWeight:'900', color:'#333' }}>S</Text>
      </View>
    </View>
  );
}

// ─── Legend ───────────────────────────────────────────────────────────────────

export function MapLegend({
  ticketTypes,
  selectedTypeId,
}: {
  ticketTypes: TicketType[];
  selectedTypeId: string | null;
}) {
  const sorted = [...ticketTypes].sort((a, b) => a.price - b.price);
  const labels = ['Upper tier', 'Lower tier', 'Club level'];
  return (
    <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: 10 }}>
      {sorted.map((tt, i) => {
        const sel = tt.id === selectedTypeId;
        const c   = sel ? SEL : tc(i);
        return (
          <View key={tt.id} style={{ flexDirection: 'row', alignItems: 'center', gap: 5 }}>
            <View style={{ width: 11, height: 11, borderRadius: 3, backgroundColor: c.bg, borderWidth: 1, borderColor: c.border }} />
            <Text style={{ fontSize: 10, fontWeight: sel ? '700' : '500', color: sel ? SEL.bg : '#555' }}>
              {labels[i] ?? tt.name}
            </Text>
          </View>
        );
      })}
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
            const c   = sel ? SEL : AUD_COLORS[Math.min(sec.tier, AUD_COLORS.length - 1)];
            return (
              <TouchableOpacity key={sec.id} onPress={() => tt && onSelectType(tt.id)}
                style={{ flex: sec.wide ? 3 : 1, paddingVertical: 14, borderRadius: 6,
                  alignItems: 'center', justifyContent: 'center',
                  borderWidth: sel ? 2 : 1, backgroundColor: c.bg, borderColor: c.border }}>
                <Text style={{ fontSize: 8, fontWeight: '800', letterSpacing: 0.4, textAlign: 'center', lineHeight: 12, color: c.text }}>
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

// ─── Main Export ──────────────────────────────────────────────────────────────

export default function SeatingMap(props: SeatingMapProps) {
  const { venueType, ...rest } = props;
  if (venueType === 'stadium') return <StadiumMap {...rest} />;
  if (venueType === 'auditorium') return <AuditoriumMap {...rest} />;
  return null;
}