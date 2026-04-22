import { StyleSheet } from 'react-native';

// ─── className strings ────────────────────────────────────────────────────────
// Import as:  import { s, sheet } from '../styles/home';
// Use as:     <View className={s.screen}>

export const s = {

  // ── Screen shell ────────────────────────────────────────────────────────────
  screen:           'flex-1 bg-white',
  scroll:           'flex-1 bg-white',

  // ── Location header ─────────────────────────────────────────────────────────
  locationHeader:         'flex-row items-center justify-between px-5 pt-16 pb-6 bg-white z-10',
  locationHeaderScrolled: 'flex-row items-center justify-between px-5 pt-16 pb-6 bg-white z-10',
  locationCity:     'text-[22px] font-extrabold text-black tracking-[0.3px]',
  locationSub:      'text-[17px] font-extrabold text-black/35 mt-0.5',
  menuBtn:          'p-1.5',
  menuIcon:         'text-2xl text-black',

  // ── Hero banner ──────────────────────────────────────────────────────────────
  hero:                  'relative h-[380px]',
  heroImagePlaceholder:  'absolute inset-0 bg-[#e5e5e5]',
  heroContent:           'absolute bottom-0 left-0 right-0 p-5 pb-8',
  heroBadge:             'text-[11px] font-bold text-[#ff3c6e] tracking-[2px] uppercase mb-2.5',
  // Hero text stays white — it sits on top of a dark image scrim
  heroTitle:             'text-[42px] font-black text-white tracking-[2px] leading-[42px] mb-3',
  heroSub:               'text-[13px] text-white/80 leading-5 mb-5 max-w-[300px]',
  heroActions:           'flex-row gap-3',

  // ── Buttons ──────────────────────────────────────────────────────────────────
  btnPrimary:       'bg-[#ff3c6e] px-[22px] py-3 rounded-[2px]',
  btnPrimaryText:   'text-white font-extrabold text-[11px] tracking-[1.5px]',
  // Outline variant used inside the dark hero scrim — keep white border/text
  btnOutline:       'border border-white/50 px-[22px] py-3 rounded-[2px]',
  btnOutlineText:   'text-white font-bold text-[11px] tracking-[1px]',
  // Outline variant used outside the hero (sports banner etc.)
  btnOutlineLight:       'border border-black/20 px-[22px] py-3 rounded-[2px]',
  btnOutlineLightText:   'text-black font-bold text-[11px] tracking-[1px]',

  // ── Category filter ──────────────────────────────────────────────────────────
  filterRow:            'flex-row items-center px-5 py-3.5 gap-2.5',
  filterClearBtn:       'w-[34px] h-[34px] rounded-[2px] border border-black/15 items-center justify-center',
  filterClearText:      'text-black text-xs',
  filterChip:           'flex-row items-center px-4 py-2 rounded-[2px] border border-black/15 bg-black/[0.04]',
  filterChipActive:     'bg-[#ff3c6e] border-[#ff3c6e]',
  filterChipText:       'text-black/45 text-[10px] font-extrabold tracking-[1.2px]',
  filterChipTextActive: 'text-white',
  filterChipArrow:      'text-black/35 text-[11px]',

  // ── Category modal ───────────────────────────────────────────────────────────
  modalBackdrop:        'flex-1 bg-black/40 justify-end',
  modalSheet:           'bg-white rounded-t-[4px] pt-5 pb-10 max-h-[80%] items-center border-t border-black/10',
  modalItem:            'py-[18px] px-10 w-full items-start',
  modalItemText:        'text-[22px] text-black/25 font-normal',
  modalItemTextActive:  'text-black font-black tracking-[1px]',
  modalCloseBtn:        'mt-4 w-11 h-11 rounded-[2px] bg-black/[0.06] items-center justify-center',
  modalCloseBtnText:    'text-black text-base font-bold',

  // ── Section header ───────────────────────────────────────────────────────────
  section:              'px-5 mb-4',
  sectionMt:            'px-5 mb-4 mt-7',
  sectionHeader:        'flex-row items-center justify-between',
  sectionTitle:         'text-[14px] font-black text-black tracking-[2px]',
  sectionLink:          'text-[10px] font-extrabold text-[#ff3c6e] tracking-[1.5px]',
  hScrollContent:       'px-5 gap-3.5 pb-2',

  // ── Event card ───────────────────────────────────────────────────────────────
  hCard:                  'w-[200px]',
  hCardImageArea:         'w-full h-[130px] bg-[#e5e5e5] rounded-[14px] overflow-hidden',
  hCardBadge:             'absolute top-2.5 left-2.5 bg-[#ff3c6e] rounded-[5px] px-2.5 py-1 flex-row items-center gap-1',
  hCardBadgeText:         'text-white text-[9px] font-extrabold tracking-[1.2px]',
  hCardPriceBadge:        'absolute top-2.5 right-2.5 bg-black/60 rounded-[5px] px-2.5 py-1',
  hCardPriceBadgeText:    'text-white text-[9px] font-bold',
  hCardBody:              'pt-2.5 px-0.5',
  hCardCat:               'text-[10px] font-extrabold text-[#ff3c6e] tracking-[1.5px] uppercase mb-0.5',
  hCardTitle:             'text-sm font-bold text-black mb-1 leading-[18px]',
  hCardMeta:              'text-[11px] text-black/40',

  // ── Rank badge (Top 10 cards) ─────────────────────────────────────────────
  rankBadge:   'absolute bottom-2.5 left-2.5 bg-[#e8c14a] rounded-[2px] px-2 py-0.5',
  rankText:    'text-black text-xs font-black',

  // ── Sports Elite banner ───────────────────────────────────────────────────
  sportsBanner:        'rounded-[3px] overflow-hidden min-h-[200px] relative mx-5',
  sportsBannerBg:      'absolute inset-0 bg-[#1a1a1a]',
  sportsBannerContent: 'p-6 relative z-[2]',
  sportsBadge:         'text-[10px] font-extrabold text-[#e8c14a] tracking-[2px] uppercase mb-1.5',
  sportsTitle:         'text-[28px] font-black text-white tracking-[1px] mb-2',
  sportsSub:           'text-xs text-white/50 mb-4',

  // ── Sports list ────────────────────────────────────────────────────────────
  sportsListContainer: 'mt-4 px-5',
  sportRow:            'flex-row items-center gap-3.5 py-3.5 border-b border-black/[0.07]',
  sportRowLast:        'flex-row items-center gap-3.5 py-3.5',
  sportThumb:          'w-32 h-[64px] rounded-[8px] bg-[#e5e5e5] shrink-0 overflow-hidden',
  sportInfo:           'flex-1',
  sportCat:            'text-[10px] font-extrabold text-[#ff3c6e] tracking-[1.5px] uppercase mb-0.5',
  sportTitle:          'text-sm font-semibold text-black mb-0.5',
  sportLoc:            'text-[11px] text-black/40',
  sportArrow:          'text-[20px] text-black/20',

  // ── Loading ────────────────────────────────────────────────────────────────
  loadingRow:          'items-center py-10',
};

// ─── StyleSheet ────────────────────────────────────────────────────────────────
export const sheet = StyleSheet.create({
  absoluteFill: { position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 },
  locationHeaderShadow: {
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.08,
    shadowRadius: 12,
    elevation: 6,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: 'rgba(0,0,0,0.08)',
  },
});