import { StyleSheet } from 'react-native';

// ─── className strings ────────────────────────────────────────────────────────
// Import as:  import { s, sheet } from '../styles/home';
// Use as:     <View className={s.screen}>

export const s = {

  // ── Screen shell ────────────────────────────────────────────────────────────
  screen:           'flex-1 bg-[#0a0a0a]',
  scroll:           'flex-1 bg-[#0a0a0a]',

  // ── Location header ─────────────────────────────────────────────────────────
  locationHeader:         'flex-row items-center justify-between px-5 pt-16 pb-6 bg-[#0a0a0a] z-10',
  locationHeaderScrolled: 'flex-row items-center justify-between px-5 pt-16 pb-6 bg-[#111111] z-10',
  locationCity:     'text-[22px] font-extrabold text-white tracking-[0.3px]',
  locationSub:      'text-[17px] font-extrabold text-white/30 mt-0.5',
  menuBtn:          'p-1.5',
  menuIcon:         'text-2xl text-white',

  // ── Hero banner ──────────────────────────────────────────────────────────────
  hero:                  'relative h-[380px]',
  heroImagePlaceholder:  'absolute inset-0 bg-[#1a1a1a]',
  heroContent:           'absolute bottom-0 left-0 right-0 p-5 pb-8',
  heroBadge:             'text-[11px] font-bold text-[#ff3c6e] tracking-[2px] uppercase mb-2.5',
  heroTitle:             'text-[42px] font-black text-white tracking-[2px] leading-[42px] mb-3',
  heroSub:               'text-[13px] text-white/70 leading-5 mb-5 max-w-[300px]',
  heroActions:           'flex-row gap-3',

  // ── Buttons ──────────────────────────────────────────────────────────────────
  btnPrimary:       'bg-[#ff3c6e] px-[22px] py-3 rounded-[2px]',
  btnPrimaryText:   'text-white font-extrabold text-[11px] tracking-[1.5px]',
  btnOutline:       'border border-white/30 px-[22px] py-3 rounded-[2px]',
  btnOutlineText:   'text-white font-bold text-[11px] tracking-[1px]',

  // ── Category filter ──────────────────────────────────────────────────────────
  filterRow:            'flex-row items-center px-5 py-3.5 gap-2.5',
  filterClearBtn:       'w-[34px] h-[34px] rounded-[2px] border border-white/15 items-center justify-center',
  filterClearText:      'text-white text-xs',
  filterChip:           'flex-row items-center px-4 py-2 rounded-[2px] border border-white/15 bg-white/[0.04]',
  filterChipActive:     'bg-[#ff3c6e] border-[#ff3c6e]',
  filterChipText:       'text-white/40 text-[10px] font-extrabold tracking-[1.2px]',
  filterChipTextActive: 'text-white',
  filterChipArrow:      'text-white/30 text-[11px]',

  // ── Category modal ───────────────────────────────────────────────────────────
  modalBackdrop:        'flex-1 bg-black/60 justify-end',
  modalSheet:           'bg-[#111] rounded-t-[4px] pt-5 pb-10 max-h-[80%] items-center border-t border-white/10',
  modalItem:            'py-[18px] px-10 w-full items-start',
  modalItemText:        'text-[22px] text-white/25 font-normal',
  modalItemTextActive:  'text-white font-black tracking-[1px]',
  modalCloseBtn:        'mt-4 w-11 h-11 rounded-[2px] bg-white/[0.06] items-center justify-center',
  modalCloseBtnText:    'text-white text-base font-bold',

  // ── Section header ───────────────────────────────────────────────────────────
  section:              'px-5 mb-4',
  sectionMt:            'px-5 mb-4 mt-7',
  sectionHeader:        'flex-row items-center justify-between',
  sectionTitle:         'text-[14px] font-black text-white tracking-[2px]',
  sectionLink:          'text-[10px] font-extrabold text-[#ff3c6e] tracking-[1.5px]',
  hScrollContent:       'px-5 gap-3.5 pb-2',

  // ── Event card ───────────────────────────────────────────────────────────────
  hCard:                  'w-[200px]',
  hCardImageArea:         'w-full h-[130px] bg-[#1a1a1a] rounded-[14px] overflow-hidden',
  // LIVE badge — top-left inside image
  hCardBadge:             'absolute top-2.5 left-2.5 bg-[#ff3c6e] rounded-[5px] px-2.5 py-1 flex-row items-center gap-1',
  hCardBadgeText:         'text-white text-[9px] font-extrabold tracking-[1.2px]',
  // Price badge — top-right inside image
  hCardPriceBadge:        'absolute top-2.5 right-2.5 bg-black/70 border border-white/20 rounded-[5px] px-2.5 py-1',
  hCardPriceBadgeText:    'text-white text-[9px] font-bold',
  hCardBody:              'pt-2.5 px-0.5',
  hCardCat:               'text-[10px] font-extrabold text-[#ff3c6e] tracking-[1.5px] uppercase mb-0.5',
  hCardTitle:             'text-sm font-bold text-white mb-1 leading-[18px]',
  hCardMeta:              'text-[11px] text-white/30',

  // ── Rank badge (Top 10 cards) ─────────────────────────────────────────────
  rankBadge:   'absolute bottom-2.5 left-2.5 bg-[#e8c14a] rounded-[2px] px-2 py-0.5',
  rankText:    'text-black text-xs font-black',

  // ── Sports Elite banner ───────────────────────────────────────────────────
  sportsBanner:        'rounded-[3px] overflow-hidden min-h-[200px] relative mx-5',
  sportsBannerBg:      'absolute inset-0 bg-[#0d1a0d]',
  sportsBannerContent: 'p-6 relative z-[2]',
  sportsBadge:         'text-[10px] font-extrabold text-[#e8c14a] tracking-[2px] uppercase mb-1.5',
  sportsTitle:         'text-[28px] font-black text-white tracking-[1px] mb-2',
  sportsSub:           'text-xs text-white/40 mb-4',

  // ── Sports list ────────────────────────────────────────────────────────────
  sportsListContainer: 'mt-4 px-5',
  sportRow:            'flex-row items-center gap-3.5 py-3.5 border-b border-white/[0.06]',
  sportRowLast:        'flex-row items-center gap-3.5 py-3.5',
  sportThumb:          'w-32 h-[64px] rounded-[3px] bg-[#1a1a1a] shrink-0 overflow-hidden',
  sportInfo:           'flex-1',
  sportCat:            'text-[10px] font-extrabold text-[#ff3c6e] tracking-[1.5px] uppercase mb-0.5',
  sportTitle:          'text-sm font-semibold text-white mb-0.5',
  sportLoc:            'text-[11px] text-white/30',
  sportArrow:          'text-[20px] text-white/20',

  // ── Loading ────────────────────────────────────────────────────────────────
  loadingRow:          'items-center py-10',
};

// ─── StyleSheet ────────────────────────────────────────────────────────────────
export const sheet = StyleSheet.create({
  absoluteFill: { position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 },
  locationHeaderShadow: {
    // iOS
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.4,
    shadowRadius: 12,
    // Android
    elevation: 8,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: 'rgba(255,255,255,0.08)',
  },
});