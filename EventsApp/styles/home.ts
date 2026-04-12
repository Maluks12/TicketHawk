import { StyleSheet } from 'react-native';

// ─── className strings ────────────────────────────────────────────────────────
// Import as:  import { s, sheet } from '../styles/home';
// Use as:     <View className={s.screen}>

export const s = {

  // ── Screen shell ────────────────────────────────────────────────────────────
  screen:           'flex-1 bg-white',
  scroll:           'flex-1',

  // ── Location header ─────────────────────────────────────────────────────────
  locationHeader:   'flex-row items-center justify-between px-5 pt-14 pb-3 bg-white z-10',
  locationCity:     'text-[22px] font-extrabold text-black tracking-[0.3px]',
  locationSub:      'text-[13px] text-black/45 mt-0.5',
  menuBtn:          'p-1.5',
  menuIcon:         'text-2xl text-black',

  // ── Hero banner ──────────────────────────────────────────────────────────────
  hero:                  'relative h-[380px]',
  heroImagePlaceholder:  'absolute inset-0 bg-gray-200',
  heroContent:           'absolute bottom-0 left-0 right-0 p-5 pb-8',
  heroBadge:             'text-[11px] font-bold text-brand-pink tracking-[2px] uppercase mb-2.5',
  heroTitle:             'text-[42px] font-black text-white tracking-[2px] leading-[42px] mb-3',
  heroSub:               'text-[13px] text-white/80 leading-5 mb-5 max-w-[300px]',
  heroActions:           'flex-row gap-3',

  // ── Buttons ──────────────────────────────────────────────────────────────────
  btnPrimary:       'bg-brand-pink px-[22px] py-3 rounded-full',
  btnPrimaryText:   'text-white font-bold text-sm',
  btnOutline:       'border-[1.5px] border-white/40 px-[22px] py-3 rounded-full',
  btnOutlineText:   'text-white font-semibold text-sm',

  // ── Category filter ──────────────────────────────────────────────────────────
  filterRow:            'flex-row items-center px-5 py-3.5 gap-2.5',
  filterClearBtn:       'w-[34px] h-[34px] rounded-full border-[1.5px] border-black/15 items-center justify-center',
  filterClearText:      'text-black text-xs',
  filterChip:           'flex-row items-center px-4 py-2 rounded-full border-[1.5px] border-black/15 bg-black/[0.04]',
  filterChipActive:     'bg-brand-pink border-brand-pink',
  filterChipText:       'text-black/50 text-[13px] font-semibold',
  filterChipTextActive: 'text-white',
  filterChipArrow:      'text-black/40 text-[11px]',

  // ── Category modal ───────────────────────────────────────────────────────────
  modalBackdrop:        'flex-1 bg-black/40 justify-end',
  modalSheet:           'bg-white rounded-t-[20px] pt-5 pb-10 max-h-[80%] items-center',
  modalItem:            'py-[18px] px-10 w-full items-start',
  modalItemText:        'text-[22px] text-black/30 font-normal',
  modalItemTextActive:  'text-black font-bold',
  modalCloseBtn:        'mt-4 w-11 h-11 rounded-full bg-black/[0.06] items-center justify-center',
  modalCloseBtnText:    'text-black text-base font-bold',

  // ── Section header ───────────────────────────────────────────────────────────
  section:              'px-5 mb-4',
  sectionMt:            'px-5 mb-4 mt-7',
  sectionHeader:        'flex-row items-center justify-between',
  sectionTitle:         'text-[22px] font-black text-black tracking-[1px]',
  sectionLink:          'text-[11px] font-bold text-brand-pink tracking-[1.5px]',
  hScrollContent:       'px-5 gap-3.5 pb-2',

  // ── Event card ───────────────────────────────────────────────────────────────
  hCard:                  'w-[200px] rounded-[14px] overflow-hidden',
  hCardImageArea:         'w-full h-[130px] bg-gray-100 rounded-[14px] overflow-hidden',
  // LIVE badge — top-left inside image
  hCardBadge:             'absolute top-2.5 left-2.5 bg-brand-pink rounded-full px-2.5 py-1',
  hCardBadgeText:         'text-white text-[9px] font-bold tracking-[1px]',
  // Price badge — top-right inside image
  hCardPriceBadge:        'absolute top-2.5 right-2.5 bg-black/60 rounded-full px-2.5 py-1',
  hCardPriceBadgeText:    'text-white text-[9px] font-bold',
  hCardBody:              'pt-2.5 px-0.5',
  hCardCat:               'text-[10px] font-bold text-brand-pink tracking-[1.5px] uppercase mb-0.5',
  hCardTitle:             'text-sm font-bold text-black mb-1 leading-[18px]',
  hCardMeta:              'text-[11px] text-black/45',

  // ── Rank badge (Top 10 cards) ─────────────────────────────────────────────
  rankBadge:   'absolute bottom-2.5 left-2.5 bg-gold rounded-md px-2 py-0.5',
  rankText:    'text-black text-xs font-black',

  // ── Sports Elite banner ───────────────────────────────────────────────────
  sportsBanner:        'rounded-[14px] overflow-hidden min-h-[200px] relative mx-5',
  sportsBannerBg:      'absolute inset-0 bg-gray-900',
  sportsBannerContent: 'p-6 relative z-[2]',
  sportsBadge:         'text-[10px] font-bold text-gold tracking-[2px] uppercase mb-1.5',
  sportsTitle:         'text-[28px] font-black text-white tracking-[1px] mb-2',
  sportsSub:           'text-xs text-white/50 mb-4',

  // ── Sports list ────────────────────────────────────────────────────────────
  sportsListContainer: 'mt-4 px-5',
  sportRow:            'flex-row items-center gap-3.5 py-3.5 border-b border-black/[0.07]',
  sportRowLast:        'flex-row items-center gap-3.5 py-3.5',
  sportThumb:          'w-32 h-[64px] rounded-[8px] bg-gray-100 shrink-0 overflow-hidden',
  sportInfo:           'flex-1',
  sportCat:            'text-[10px] font-bold text-brand-pink tracking-[1.5px] uppercase mb-0.5',
  sportTitle:          'text-sm font-semibold text-black mb-0.5',
  sportLoc:            'text-[11px] text-black/45',
  sportArrow:          'text-[20px] text-black/25',

  // ── Loading ────────────────────────────────────────────────────────────────
  loadingRow:          'items-center py-10',
};

// ─── StyleSheet ────────────────────────────────────────────────────────────────
export const sheet = StyleSheet.create({
  absoluteFill: { position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 },
});