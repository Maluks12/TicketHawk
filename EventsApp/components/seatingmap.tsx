import React, { useMemo } from 'react';
import {
  View,
  Text,
  TouchableOpacity,
  StyleSheet,
  Dimensions,
  ScrollView,
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

// Detect venue type from event category / venue name
export function detectVenueType(category: string, venueName?: string): VenueType {
  const cat = category.toLowerCase();
  const venue = (venueName ?? '').toLowerCase();

  if (
    cat.includes('sport') ||
    cat.includes('soccer') ||
    cat.includes('football') ||
    cat.includes('rugby') ||
    cat.includes('cricket') ||
    venue.includes('stadium') ||
    venue.includes('park') ||
    venue.includes('arena') ||
    venue.includes('field')
  ) return 'stadium';

  if (
    cat.includes('concert') ||
    cat.includes('music') ||
    cat.includes('show') ||
    cat.includes('theatre') ||
    cat.includes('theater') ||
    cat.includes('comedy') ||
    cat.includes('opera') ||
    cat.includes('dance') ||
    cat.includes('performance') ||
    venue.includes('theatre') ||
    venue.includes('theater') ||
    venue.includes('hall') ||
    venue.includes('auditorium')
  ) return 'auditorium';

  return 'none';
}

// Assign tier index to each ticket type (sorted cheapest → most expensive)
function buildTierMap(ticketTypes: TicketType[]): Map<string, number> {
  const sorted = [...ticketTypes].sort((a, b) => a.price - b.price);
  const map = new Map<string, number>();
  sorted.forEach((tt, i) => map.set(tt.id, i));
  return map;
}

// Tier colour palette — matches app design: pink accent, dark navy, gold, gray
const TIER_COLORS = [
  { bg: '#e8edf5', border: '#b0bec5', text: '#546e7a', label: 'UPPER' },   // 0 cheapest → upper
  { bg: '#1a2e5e', border: '#1a2e5e', text: '#ffffff', label: 'LOWER' },   // 1 → lower/mezzanine
  { bg: '#c8962a', border: '#c8962a', text: '#ffffff', label: 'CLUB' },    // 2 → club/premium
  { bg: '#ff3c6e', border: '#ff3c6e', text: '#ffffff', label: 'VIP' },     // 3+ → VIP/front
];

function tierColor(idx: number) {
  return TIER_COLORS[Math.min(idx, TIER_COLORS.length - 1)];
}

// ─── Stadium Map ──────────────────────────────────────────────────────────────
//
// Approximate soccer stadium layout:
//  Upper tier (outer) — cheapest
//  Club level  (mid)  — middle
//  Lower tier (inner) — expensive
//  Pitch at centre    — not a section

const STADIUM_SECTIONS = [
  // ── TOP ROW (Upper tier) ──
  { id: 'U301', label: '301', tier: 0, row: 0, col: 0, span: 1 },
  { id: 'U302', label: '302', tier: 0, row: 0, col: 1, span: 1 },
  { id: 'U303', label: '303', tier: 0, row: 0, col: 2, span: 1 },
  { id: 'U304', label: '304', tier: 0, row: 0, col: 3, span: 1 },
  { id: 'U305', label: '305', tier: 0, row: 0, col: 4, span: 1 },
  { id: 'U306', label: '306', tier: 0, row: 0, col: 5, span: 1 },

  // ── CLUB TOP ──
  { id: 'C201a', label: 'CLUB 201', tier: 2, row: 1, col: 1, span: 2 },
  { id: 'C201b', label: 'CLUB 201', tier: 2, row: 1, col: 3, span: 2 },

  // ── MIDDLE ROW LEFT ──
  { id: 'L101', label: '101', tier: 1, row: 2, col: 0, span: 1 },
  { id: 'L102', label: '102', tier: 1, row: 3, col: 0, span: 1 },

  // ── MIDDLE ROW RIGHT ──
  { id: 'L107', label: '107', tier: 1, row: 2, col: 5, span: 1 },
  { id: 'L108', label: '108', tier: 1, row: 3, col: 5, span: 1 },

  // ── LOWER INSIDE TOP ──
  { id: 'L104', label: '104', tier: 1, row: 2, col: 1, span: 1 },
  { id: 'L105', label: '105', tier: 1, row: 2, col: 2, span: 1 },
  { id: 'L106', label: '106', tier: 1, row: 2, col: 3, span: 1 },
  { id: 'L106b', label: '107', tier: 1, row: 2, col: 4, span: 1 },

  // ── LOWER INSIDE BOTTOM ──
  { id: 'L100', label: '100', tier: 1, row: 4, col: 1, span: 1 },
  { id: 'L103', label: '103', tier: 1, row: 4, col: 2, span: 1 },
  { id: 'L103b', label: '104', tier: 1, row: 4, col: 3, span: 1 },
  { id: 'L107b', label: '105', tier: 1, row: 4, col: 4, span: 1 },

  // ── CLUB BOTTOM ──
  { id: 'C202a', label: 'CLUB 202', tier: 2, row: 5, col: 1, span: 2 },
  { id: 'C202b', label: 'CLUB 202', tier: 2, row: 5, col: 3, span: 2 },

  // ── BOTTOM ROW (Upper tier) ──
  { id: 'U307', label: '307', tier: 0, row: 6, col: 0, span: 1 },
  { id: 'U308', label: '308', tier: 0, row: 6, col: 1, span: 1 },
  { id: 'U309', label: '309', tier: 0, row: 6, col: 2, span: 1 },
  { id: 'U310', label: '310', tier: 0, row: 6, col: 3, span: 1 },
  { id: 'U311', label: '311', tier: 0, row: 6, col: 4, span: 1 },
  { id: 'U312', label: '312', tier: 0, row: 6, col: 5, span: 1 },
];

// Pitch placeholder (rows 3, cols 1–4)
const PITCH_CELL = { row: 3, col: 1, colEnd: 5 };

function StadiumMap({
  ticketTypes,
  selectedTypeId,
  onSelectType,
}: Omit<SeatingMapProps, 'venueType'>) {
  const tierMap = useMemo(() => buildTierMap(ticketTypes), [ticketTypes]);

  // Map stadium tier (0=Upper, 1=Lower, 2=Club, 3=VIP) to ticket type
  // Clamp to number of actual ticket types
  function typeForTier(sectionTier: number): TicketType | null {
    const sorted = [...ticketTypes].sort((a, b) => a.price - b.price);
    if (sorted.length === 0) return null;
    // Upper → cheapest, Club → most expensive, Lower → middle
    if (sectionTier === 0) return sorted[0];
    if (sectionTier === 2) return sorted[sorted.length - 1];
    if (sectionTier === 1) return sorted[Math.floor(sorted.length / 2)];
    return sorted[sorted.length - 1];
  }

  const COLS = 6;
  const ROWS = 7;
  const cellW = (SW - 40) / COLS;
  const cellH = 34;

  // Build occupied grid for pitch
  const isPitch = (row: number, col: number) =>
    row === PITCH_CELL.row && col >= PITCH_CELL.col && col < PITCH_CELL.colEnd;

  // Check if a section is selected
  function isSectionSelected(sectionTier: number) {
    const tt = typeForTier(sectionTier);
    return tt?.id === selectedTypeId;
  }

  return (
    <View style={styles.mapContainer}>
      <Text style={styles.mapTitle}>VENUE MAP</Text>

      {/* Legend */}
      <View style={styles.legend}>
        {ticketTypes
          .sort((a, b) => a.price - b.price)
          .map((tt, idx) => {
            const color = tierColor(idx);
            return (
              <View key={tt.id} style={styles.legendItem}>
                <View style={[styles.legendDot, { backgroundColor: color.bg, borderColor: color.border }]} />
                <Text style={styles.legendText}>{tt.name.toUpperCase()}</Text>
              </View>
            );
          })}
      </View>

      {/* Stadium grid */}
      <View style={[styles.stadiumGrid, { height: ROWS * cellH + 4 }]}>
        {Array.from({ length: ROWS }, (_, row) => (
          <View key={row} style={[styles.gridRow, { height: cellH }]}>
            {Array.from({ length: COLS }, (_, col) => {
              if (isPitch(row, col)) {
                // Render pitch spanning cols
                if (col === PITCH_CELL.col) {
                  return (
                    <View
                      key={col}
                      style={[
                        styles.pitchCell,
                        { width: cellW * (PITCH_CELL.colEnd - PITCH_CELL.col) - 4, height: cellH - 4 },
                      ]}
                    >
                      <View style={styles.pitchLines}>
                        <View style={styles.pitchCircle} />
                        <View style={styles.pitchMidLine} />
                      </View>
                    </View>
                  );
                }
                return null;
              }

              // Find section at this row/col
              const section = STADIUM_SECTIONS.find(
                (s) => s.row === row && s.col === col
              );
              if (!section) {
                return <View key={col} style={{ width: cellW, height: cellH }} />;
              }
              if (section.span > 1 && STADIUM_SECTIONS.find(s => s.row === row && s.col !== col && s.id === section.id)) {
                return null;
              }

              const tt = typeForTier(section.tier);
              const color = tt ? tierColor(tierMap.get(tt.id) ?? section.tier) : tierColor(section.tier);
              const selected = isSectionSelected(section.tier);

              return (
                <TouchableOpacity
                  key={col}
                  onPress={() => tt && onSelectType(tt.id)}
                  style={[
                    styles.sectionCell,
                    {
                      width: cellW * (section.span ?? 1) - 4,
                      height: cellH - 4,
                      backgroundColor: selected ? '#ff3c6e' : color.bg,
                      borderColor: selected ? '#ff3c6e' : color.border,
                    },
                  ]}
                  activeOpacity={0.75}
                >
                  <Text
                    style={[
                      styles.sectionLabel,
                      { color: selected ? '#fff' : color.text },
                    ]}
                    numberOfLines={1}
                  >
                    {section.label}
                  </Text>
                </TouchableOpacity>
              );
            })}
          </View>
        ))}
      </View>

      <Text style={styles.mapNote}>Tap a section to select your ticket tier</Text>
    </View>
  );
}

// ─── Auditorium / Theater Map ─────────────────────────────────────────────────

const AUDITORIUM_SECTIONS = [
  // Orchestra Premium (row 0)
  { id: 'A', label: 'A', tier: 3, row: 0, col: 1 },
  { id: 'B', label: 'B', tier: 3, row: 0, col: 2 },
  { id: 'C', label: 'C', tier: 3, row: 0, col: 3 },
  // Orchestra (row 1)
  { id: 'D', label: 'D', tier: 2, row: 1, col: 1 },
  { id: 'E', label: 'E', tier: 2, row: 1, col: 2 },
  { id: 'F', label: 'F', tier: 2, row: 1, col: 3 },
  // Mezzanine (row 2)
  { id: 'G', label: 'G', tier: 1, row: 2, col: 1 },
  { id: 'H', label: 'H', tier: 1, row: 2, col: 2 },
  { id: 'I', label: 'I', tier: 1, row: 2, col: 3 },
  // Balcony (row 3)
  { id: 'J', label: 'J', tier: 0, row: 3, col: 1 },
  { id: 'K', label: 'K', tier: 0, row: 3, col: 2 },
  { id: 'L', label: 'L', tier: 0, row: 3, col: 3 },
  // Side sections
  { id: 'X1', label: 'X1', tier: 0, row: 0, col: 0 },
  { id: 'X2', label: 'X2', tier: 0, row: 1, col: 0 },
  { id: 'X3', label: 'X3', tier: 0, row: 2, col: 0 },
  { id: 'Y1', label: 'Y1', tier: 0, row: 0, col: 4 },
  { id: 'Y2', label: 'Y2', tier: 0, row: 1, col: 4 },
  { id: 'Y3', label: 'Y3', tier: 0, row: 2, col: 4 },
];

// Tier color for auditorium: 3=Premium, 2=Orchestra, 1=Mezzanine, 0=Balcony
const AUD_TIER_COLORS = [
  { bg: '#7c5cbf', border: '#7c5cbf', text: '#fff', label: 'BALCONY' },      // 0
  { bg: '#3a7d44', border: '#3a7d44', text: '#fff', label: 'MEZZANINE' },    // 1
  { bg: '#1a2e5e', border: '#1a2e5e', text: '#fff', label: 'ORCHESTRA' },    // 2
  { bg: '#c8962a', border: '#c8962a', text: '#fff', label: 'PREMIUM' },      // 3
];

function audTierColor(tier: number) {
  return AUD_TIER_COLORS[Math.min(tier, AUD_TIER_COLORS.length - 1)];
}

function AuditoriumMap({
  ticketTypes,
  selectedTypeId,
  onSelectType,
}: Omit<SeatingMapProps, 'venueType'>) {
  const sortedTypes = useMemo(
    () => [...ticketTypes].sort((a, b) => a.price - b.price),
    [ticketTypes]
  );

  // Map auditorium tier (0=Balcony … 3=Premium) → ticket type
  function typeForAudTier(tier: number): TicketType | null {
    if (sortedTypes.length === 0) return null;
    const idx = Math.floor((tier / 3) * (sortedTypes.length - 1));
    return sortedTypes[Math.min(idx, sortedTypes.length - 1)];
  }

  const COLS = 5;
  const ROWS = 4;
  const cellW = (SW - 40) / COLS;
  const cellH = 48;

  return (
    <View style={styles.mapContainer}>
      <Text style={styles.mapTitle}>VENUE MAP</Text>

      {/* Stage */}
      <View style={styles.stageBar}>
        <Text style={styles.stageText}>STAGE</Text>
      </View>

      {/* Legend */}
      <View style={styles.legend}>
        {sortedTypes.map((tt, idx) => {
          // assign display color based on sorted position
          const colorIdx = Math.round((idx / Math.max(sortedTypes.length - 1, 1)) * 3);
          const color = AUD_TIER_COLORS[colorIdx];
          return (
            <View key={tt.id} style={styles.legendItem}>
              <View style={[styles.legendDot, { backgroundColor: color.bg, borderColor: color.border }]} />
              <Text style={styles.legendText}>{tt.name.toUpperCase()}</Text>
            </View>
          );
        })}
      </View>

      {/* Auditorium grid */}
      <View style={[styles.auditoriumGrid, { borderRadius: 12 }]}>
        {Array.from({ length: ROWS }, (_, row) => (
          <View key={row} style={[styles.gridRow, { height: cellH }]}>
            {Array.from({ length: COLS }, (_, col) => {
              const section = AUDITORIUM_SECTIONS.find(
                (s) => s.row === row && s.col === col
              );

              // Empty corner cells (bottom row, cols 0 and 4)
              if (!section) {
                return (
                  <View key={col} style={{ width: cellW, height: cellH }} />
                );
              }

              const tt = typeForAudTier(section.tier);
              const color = audTierColor(section.tier);
              const selected = tt?.id === selectedTypeId;

              return (
                <TouchableOpacity
                  key={col}
                  onPress={() => tt && onSelectType(tt.id)}
                  style={[
                    styles.audSection,
                    {
                      width: cellW - 5,
                      height: cellH - 5,
                      backgroundColor: selected ? '#ff3c6e' : color.bg,
                      borderColor: selected ? '#ff3c6e' : color.border,
                      opacity: tt ? 1 : 0.4,
                    },
                  ]}
                  activeOpacity={0.75}
                >
                  <Text
                    style={[styles.audSectionLabel, { color: selected ? '#fff' : color.text }]}
                  >
                    {section.label}
                  </Text>
                </TouchableOpacity>
              );
            })}
          </View>
        ))}
      </View>

      <Text style={styles.mapNote}>Tap a section to select your ticket tier</Text>
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

// ─── Styles ───────────────────────────────────────────────────────────────────

const styles = StyleSheet.create({
  mapContainer: {
    marginHorizontal: 20,
    marginTop: 4,
    marginBottom: 8,
  },
  mapTitle: {
    fontSize: 10,
    fontWeight: '900',
    letterSpacing: 2,
    color: 'rgba(0,0,0,0.4)',
    marginBottom: 10,
  },
  stageBar: {
    backgroundColor: '#1a1a2e',
    marginBottom: 6,
    borderRadius: 6,
    paddingVertical: 8,
    alignItems: 'center',
  },
  stageText: {
    fontSize: 10,
    fontWeight: '900',
    letterSpacing: 3,
    color: '#ffffff',
  },
  legend: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 8,
    marginBottom: 10,
  },
  legendItem: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
  },
  legendDot: {
    width: 10,
    height: 10,
    borderRadius: 2,
    borderWidth: 1,
  },
  legendText: {
    fontSize: 9,
    fontWeight: '700',
    letterSpacing: 0.5,
    color: 'rgba(0,0,0,0.55)',
  },
  stadiumGrid: {
    backgroundColor: '#d0d8e4',
    borderRadius: 12,
    padding: 2,
    overflow: 'hidden',
  },
  auditoriumGrid: {
    backgroundColor: '#f0f0f0',
    padding: 3,
    overflow: 'hidden',
  },
  gridRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 2,
    marginVertical: 1,
  },
  sectionCell: {
    borderRadius: 4,
    borderWidth: 1,
    alignItems: 'center',
    justifyContent: 'center',
    marginHorizontal: 1,
  },
  sectionLabel: {
    fontSize: 8,
    fontWeight: '800',
    letterSpacing: 0.3,
    textAlign: 'center',
  },
  pitchCell: {
    backgroundColor: '#3a7d44',
    borderRadius: 4,
    alignItems: 'center',
    justifyContent: 'center',
    marginHorizontal: 2,
    overflow: 'hidden',
  },
  pitchLines: {
    position: 'absolute',
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    alignItems: 'center',
    justifyContent: 'center',
  },
  pitchCircle: {
    width: 14,
    height: 14,
    borderRadius: 7,
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.6)',
    position: 'absolute',
  },
  pitchMidLine: {
    width: '100%',
    height: 1,
    backgroundColor: 'rgba(255,255,255,0.5)',
    position: 'absolute',
  },
  audSection: {
    borderRadius: 6,
    borderWidth: 1,
    alignItems: 'center',
    justifyContent: 'center',
    marginHorizontal: 2,
    marginVertical: 2,
  },
  audSectionLabel: {
    fontSize: 13,
    fontWeight: '900',
    letterSpacing: 0.5,
  },
  mapNote: {
    fontSize: 9,
    color: 'rgba(0,0,0,0.35)',
    textAlign: 'center',
    marginTop: 6,
    letterSpacing: 0.5,
    fontWeight: '500',
  },
});