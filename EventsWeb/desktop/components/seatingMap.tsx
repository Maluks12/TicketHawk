'use client';

import { useMemo } from 'react';
import type { TicketType } from '@/lib/api';

export type VenueType = 'stadium' | 'auditorium' | 'none';

interface SeatingMapProps {
  venueType: VenueType;
  ticketTypes: TicketType[];
  selectedTypeId: string | null;
  onSelectType: (typeId: string) => void;
}

export function detectVenueType(category: string, venueName?: string): VenueType {
  const cat   = category.toLowerCase();
  const venue = (venueName ?? '').toLowerCase();
  if (cat.includes('sport') || cat.includes('soccer') || cat.includes('football') ||
      cat.includes('rugby') || cat.includes('cricket') ||
      venue.includes('stadium') || venue.includes('park') ||
      venue.includes('arena')   || venue.includes('field')) return 'stadium';
  if (cat.includes('concert') || cat.includes('music')  || cat.includes('show') ||
      cat.includes('theatre')  || cat.includes('theater') || cat.includes('comedy') ||
      cat.includes('opera')    || cat.includes('dance')  || cat.includes('performance') ||
      venue.includes('theatre') || venue.includes('theater') ||
      venue.includes('hall')   || venue.includes('auditorium')) return 'auditorium';
  return 'none';
}

const C = {
  upper:    { bg: '#9B8DC8', border: '#7A6AAD', text: '#fff' },
  lower:    { bg: '#5A9E60', border: '#3D7A43', text: '#fff' },
  club:     { bg: '#5B9BD5', border: '#3A7DBD', text: '#fff' },
  access:   { bg: '#F0A030', border: '#C07B10', text: '#fff' },
  unavail:  { bg: '#C0C4D0', border: '#A0A6BA', text: '#666' },
  selected: { bg: '#1d6cf5', border: '#1558d0', text: '#fff' },
};

const MAP_W = 340;
const UH = Math.round(MAP_W * 0.108); const CH = Math.round(MAP_W * 0.088); const LH = Math.round(MAP_W * 0.072);
const MAP_H = Math.round(MAP_W * 0.82);
const UW = Math.round(MAP_W * 0.112); const CW = Math.round(MAP_W * 0.092); const LW = Math.round(MAP_W * 0.074);
const PW = MAP_W - 2 * (UW + CW + LW); const PH = MAP_H - 2 * (UH + CH + LH);
const GAP = 1.5;

const TOP_UPPER=['301','302','303','304','305','306','307'], BOT_UPPER=['326','325','324','323','322','320','319'];
const LEFT_UPPER=['331','330','329','328','327','326'],      RIGHT_UPPER=['309','310','311','312','313','318'];
const TOP_CLUB=['201','202','203','204','205','206'],         BOT_CLUB=['221','223','224','225','226','227'];
const LEFT_CLUB=['231','230','228','227'],                    RIGHT_CLUB=['207','208','209','210'];
const TOP_LOWER=['101','102','103','104','105','106'],        BOT_LOWER=['118','117','116','115','114','112'];
const LEFT_LOWER=['138','137','136'],                         RIGHT_LOWER=['108','109','111'];

function StadiumMap({ ticketTypes, selectedTypeId, onSelectType }: Omit<SeatingMapProps,'venueType'>) {
  const sorted = useMemo(() => [...ticketTypes].sort((a,b) => a.price - b.price), [ticketTypes]);
  const ttFor  = (idx: number) => idx === 0 ? sorted[0] : idx === 2 ? sorted[sorted.length-1] : sorted[Math.min(1, sorted.length-1)];
  const isSel  = (idx: number) => ttFor(idx)?.id === selectedTypeId;
  const press  = (idx: number) => { const tt = ttFor(idx); if (tt) onSelectType(tt.id); };
  const cc     = (tier: number) => isSel(tier) ? C.selected : [C.upper, C.lower, C.club][tier] ?? C.unavail;

  const tile = (label: string, tier: number, style?: React.CSSProperties) => {
    const col = cc(tier); const sel = isSel(tier);
    return (
      <button key={label} onClick={() => press(tier)} style={{ backgroundColor: col.bg, color: col.text, border: `${sel ? 1.5 : 0.5}px solid ${sel ? '#fff' : 'rgba(255,255,255,0.18)'}`, borderRadius: 2, fontSize: 5.8, fontWeight: 800, lineHeight: 1.3, letterSpacing: 0.1, cursor: 'pointer', padding: 0, ...style }}>
        {label}
      </button>
    );
  };

  const tileRow = (labels: string[], tier: number, height: number) => (
    <div style={{ display: 'flex', height, gap: GAP }}>
      {labels.map(l => tile(l, tier, { flex: 1 }))}
    </div>
  );

  const tileCol = (labels: string[], tier: number, width: number) => (
    <div style={{ width, display: 'flex', flexDirection: 'column', gap: GAP }}>
      {labels.map(l => tile(l, tier, { flex: 1 }))}
    </div>
  );

  const corner = (w: number, h: number, type: 'access'|'unavail') => (
    <div style={{ width: w, height: h, backgroundColor: type === 'access' ? C.access.bg : C.unavail.bg, borderRadius: 2, border: `0.5px solid ${type === 'access' ? C.access.border : C.unavail.border}`, flexShrink: 0 }} />
  );

  const pitch = (
    <div style={{ flex: 1, backgroundColor: '#2E7D32', borderRadius: 3, overflow: 'hidden', position: 'relative' }}>
      <div style={{ position: 'absolute', left: '50%', top: 0, bottom: 0, width: 1, background: 'rgba(255,255,255,0.42)' }} />
      <div style={{ position: 'absolute', width: PH*0.44, height: PH*0.44, borderRadius: '50%', border: '1px solid rgba(255,255,255,0.42)', left: PW/2 - PH*0.22, top: PH/2 - PH*0.22 }} />
      <div style={{ position: 'absolute', width: 4, height: 4, borderRadius: 2, background: 'rgba(255,255,255,0.6)', left: PW/2-2, top: PH/2-2 }} />
      <div style={{ position: 'absolute', left: 0, top: PH*0.2, width: PW*0.155, height: PH*0.6, borderRight: '1px solid rgba(255,255,255,0.4)', borderTop: '1px solid rgba(255,255,255,0.4)', borderBottom: '1px solid rgba(255,255,255,0.4)' }} />
      <div style={{ position: 'absolute', right: 0, top: PH*0.2, width: PW*0.155, height: PH*0.6, borderLeft: '1px solid rgba(255,255,255,0.4)', borderTop: '1px solid rgba(255,255,255,0.4)', borderBottom: '1px solid rgba(255,255,255,0.4)' }} />
    </div>
  );

  return (
    <div style={{ width: MAP_W, height: MAP_H, backgroundColor: '#D5D9E4', borderRadius: 8, overflow: 'hidden', display: 'flex', flexDirection: 'column', gap: GAP, padding: GAP, boxSizing: 'border-box' }}>
      <div style={{ display: 'flex', height: UH, gap: GAP }}>
        {corner(UW, UH, 'unavail')}
        {TOP_UPPER.map(l => tile(l, 0, { flex: 1 }))}
        {corner(UW, UH, 'unavail')}
      </div>
      <div style={{ flex: 1, display: 'flex', gap: GAP }}>
        {tileCol(LEFT_UPPER, 0, UW)}
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: GAP }}>
          <div style={{ display: 'flex', height: CH, gap: GAP }}>
            {corner(CW, CH, 'access')}
            {TOP_CLUB.map(l => tile(l, 2, { flex: 1 }))}
            {corner(CW, CH, 'access')}
          </div>
          <div style={{ flex: 1, display: 'flex', gap: GAP }}>
            {tileCol(LEFT_CLUB, 2, CW)}
            <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: GAP }}>
              {tileRow(TOP_LOWER, 1, LH)}
              <div style={{ flex: 1, display: 'flex', gap: GAP }}>
                {tileCol(LEFT_LOWER, 1, LW)}
                {pitch}
                {tileCol(RIGHT_LOWER, 1, LW)}
              </div>
              {tileRow(BOT_LOWER, 1, LH)}
            </div>
            {tileCol(RIGHT_CLUB, 2, CW)}
          </div>
          <div style={{ display: 'flex', height: CH, gap: GAP }}>
            {corner(CW, CH, 'access')}
            {BOT_CLUB.map(l => tile(l, 2, { flex: 1 }))}
            {corner(CW, CH, 'access')}
          </div>
        </div>
        {tileCol(RIGHT_UPPER, 0, UW)}
      </div>
      <div style={{ display: 'flex', height: UH, gap: GAP }}>
        {corner(UW, UH, 'access')}
        {BOT_UPPER.map(l => tile(l, 0, { flex: 1 }))}
        {corner(UW, UH, 'access')}
      </div>
    </div>
  );
}

const AUD_COLORS = [
  { bg: '#d4dae6', border: '#b0bac8', text: '#5a6478' },
  { bg: '#3a7d44', border: '#3a7d44', text: '#fff' },
  { bg: '#1a2e5e', border: '#1a2e5e', text: '#fff' },
  { bg: '#c8962a', border: '#c8962a', text: '#fff' },
];
const AUD_ROWS = [
  [{ id:'BAL_L', label:'BALCONY\nLEFT', tier:0 },  { id:'BAL_C', label:'BALCONY\nCENTER', tier:0 }, { id:'BAL_R', label:'BALCONY\nRIGHT', tier:0 }],
  [{ id:'MEZ_L', label:'MEZZ\nLEFT', tier:1 },     { id:'MEZ_C', label:'MEZZ\nCENTER', tier:1 },    { id:'MEZ_R', label:'MEZZ\nRIGHT', tier:1 }],
  [{ id:'ORC_L', label:'ORCH\nLEFT', tier:2 },     { id:'ORC_C', label:'ORCH\nCENTER', tier:2 },    { id:'ORC_R', label:'ORCH\nRIGHT', tier:2 }],
  [{ id:'PIT',   label:'PIT / GA', tier:3, wide:true }],
];
const SEL_AUD = { bg: '#1d6cf5', border: '#1558d0', text: '#fff' };

function AuditoriumMap({ ticketTypes, selectedTypeId, onSelectType }: Omit<SeatingMapProps,'venueType'>) {
  const sorted = useMemo(() => [...ticketTypes].sort((a,b) => a.price - b.price), [ticketTypes]);
  const ttForTier = (t: number) => sorted[Math.min(Math.round((t/3)*(sorted.length-1)), sorted.length-1)] ?? null;
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 5, width: MAP_W }}>
      <div style={{ background: '#111', borderRadius: 6, padding: '10px 0', textAlign: 'center' }}>
        <span style={{ fontSize: 9, fontWeight: 900, letterSpacing: 4, color: '#fff' }}>S T A G E</span>
      </div>
      {AUD_ROWS.map((row, ri) => (
        <div key={ri} style={{ display: 'flex', gap: 5 }}>
          {row.map((sec: any) => {
            const tt  = ttForTier(sec.tier);
            const sel = tt?.id === selectedTypeId;
            const col = sel ? SEL_AUD : AUD_COLORS[Math.min(sec.tier, AUD_COLORS.length-1)];
            return (
              <button key={sec.id} onClick={() => tt && onSelectType(tt.id)}
                style={{ flex: sec.wide ? 3 : 1, padding: '14px 0', borderRadius: 6, textAlign: 'center', border: `${sel ? 2 : 1}px solid ${col.border}`, background: col.bg, color: col.text, fontSize: 8, fontWeight: 800, letterSpacing: 0.4, lineHeight: 1.5, cursor: 'pointer', whiteSpace: 'pre' }}>
                {sec.label}
              </button>
            );
          })}
        </div>
      ))}
    </div>
  );
}

export function MapLegend({ ticketTypes, selectedTypeId }: { ticketTypes: TicketType[]; selectedTypeId: string | null }) {
  const sorted = [...ticketTypes].sort((a,b) => a.price - b.price);
  const tiers  = [C.upper, C.lower, C.club];
  const labels = ['Upper', 'Lower', 'Club'];
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 10 }}>
      {sorted.map((tt, i) => {
        const sel = tt.id === selectedTypeId;
        const col = sel ? C.selected : (tiers[i] ?? C.upper);
        return (
          <div key={tt.id} style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
            <div style={{ width: 11, height: 11, borderRadius: 2, background: col.bg, border: `1px solid ${col.border}` }} />
            <span style={{ fontSize: 10, fontWeight: sel ? 700 : 500, color: sel ? C.selected.bg : '#555' }}>{labels[i] ?? tt.name}</span>
          </div>
        );
      })}
      {[{col: C.access, label: 'Accessible'}, {col: C.unavail, label: 'Unavailable'}].map(({col, label}) => (
        <div key={label} style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <div style={{ width: 11, height: 11, borderRadius: 2, background: col.bg, border: `1px solid ${col.border}` }} />
          <span style={{ fontSize: 10, fontWeight: 500, color: '#555' }}>{label}</span>
        </div>
      ))}
    </div>
  );
}

export default function SeatingMap(props: SeatingMapProps) {
  const { venueType, ...rest } = props;
  if (venueType === 'stadium')    return <StadiumMap    {...rest} />;
  if (venueType === 'auditorium') return <AuditoriumMap {...rest} />;
  return null;
}