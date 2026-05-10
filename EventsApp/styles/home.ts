import { StyleSheet } from 'react-native';

//  className strings 
// Import as:  import { s, sheet } from '../styles/home';
// Use as:     <View className={s.screen}>

export const s = {

  //  Screen shell 
  screen:           'flex-1',
  scroll:           'flex-1',

  //  Location header 
  locationHeader:         'absolute top-0 left-0 right-0 flex-row items-center justify-between px-5 pt-16 pb-6 z-10',
  locationCity:           'text-[22px] font-extrabold tracking-[0.3px]',
  locationSub:            'text-[17px] font-extrabold text-black/30 mt-0.5',
  menuBtn:                'p-1.5',
  menuIcon:               'text-2xl text-[#0a0a0a]',

  //  Hero banner 
  hero:                  'relative h-[380px] mt-[130px] mx-4 rounded-[14px] overflow-hidden',
  heroImagePlaceholder:  'absolute inset-0 bg-[#e5e5e5]',
  heroContent:           'absolute bottom-0 left-0 right-0 p-5 pb-8',
  heroBadge:             'text-[11px] font-bold text-white/70 tracking-[2px] uppercase mb-2.5',
  heroTitle:             'text-[42px] font-black text-white tracking-[2px] leading-[42px] mb-3',
  heroSub:               'text-[13px] text-white/80 leading-5 mb-5 max-w-[300px]',
  heroActions:           'flex-row gap-3',

  //  Buttons 
  btnPrimary:            'bg-[#FF2D55] px-[22px] py-3 rounded-[2px]',
  btnPrimaryText:        'text-white font-extrabold text-[11px] tracking-[1.5px]',
  btnOutline:            'border border-white/50 px-[22px] py-3 rounded-[2px]',
  btnOutlineText:        'text-white font-bold text-[11px] tracking-[1px]',
  btnOutlineLight:       'border border-black/20 px-[22px] py-3 rounded-[2px]',
  btnOutlineLightText:   'text-black font-bold text-[11px] tracking-[1px]',
  btnBlue:               'bg-[#1c2e61] px-[22px] py-3 rounded-[2px]',
  btnBlueText:           'text-white font-bold text-[11px] tracking-[1px]',

  //  Category filter 
  filterRow:             'flex-row items-center px-5 py-3.5 gap-2.5',
  filterChipActive:      'bg-black border-black',
  filterClearBtn:        'w-[34px] h-[34px] rounded-[2px] border items-center justify-center',
  filterClearText:       'text-xs',
  filterChip:            'flex-row items-center px-4 py-2 rounded-[14px] border',
  filterChipText:        'text-[10px] font-extrabold tracking-[1.2px]',
  filterChipArrow:       'text-[11px]',
  filterChipTextActive:  'text-white',


  //  Category modal 
  modalBackdrop:  'flex-1 justify-end',
  modalSheet:     'w-full flex-1 pt-16 pb-10 items-center',
  modalItem:      'py-[18px] px-10 w-full items-start',
  modalItemText:  'text-[26px] font-normal',
  modalItemTextActive:   'font-black tracking-[1px]',
  modalCloseBtn:  'mt-6 w-12 h-12 rounded-full items-center justify-center',
  modalCloseBtnText:     'text-base font-bold',
  //  Section header 
  section:               'px-5 mb-4',
  sectionMt:             'px-5 mb-4 mt-7',
  sectionHeader:         'flex-row items-center justify-between',
  sectionTitle:          'text-[14px] font-black tracking-[2px]',
  sectionLink:           'text-[10px] font-extrabold text-[#FF2D55] tracking-[1.5px]',
  hScrollContent:        'px-5 gap-3.5 pb-2',

  //  Event card 
  hCard:                 'w-[200px]',
  hCardImageArea:         'w-full h-[130px] rounded-[14px] overflow-hidden',
  hCardBadge:            'absolute top-2.5 left-2.5 bg-black/70 rounded-[5px] px-2.5 py-1 flex-row items-center gap-1',
  hCardBadgeText:        'text-white text-[9px] font-extrabold tracking-[1.2px]',
  hCardPriceBadge:       'absolute top-2.5 right-2.5 bg-black/60 rounded-[5px] px-2.5 py-1',
  hCardPriceBadgeText:   'text-white text-[9px] font-bold',
  hCardBody:             'pt-2.5 px-0.5',
  hCardCat:              'text-[10px] font-semibold tracking-[0.5px] mb-0.5',
  hCardTitle:            'text-sm font-bold text-black mb-1 leading-[18px]',
  hCardMeta:             'text-[11px] text-black/40',

  //  Rank badge (kept for compat) 
  rankBadge:             'absolute bottom-2.5 left-2.5 bg-[#e8c14a] rounded-[2px] px-2 py-0.5',
  rankText:              'text-black text-xs font-black',

  //  Sports Elite banner 
  sportsBanner:          'rounded-[3px] overflow-hidden min-h-[200px] relative mx-5',
  sportsBannerBg:        'absolute inset-0 bg-[#1a1a1a]',
  sportsBannerContent:   'p-6 relative z-[2]',
  sportsBadge:           'text-[10px] font-extrabold text-[#e8c14a] tracking-[2px] uppercase mb-1.5',
  sportsTitle:           'text-[28px] font-black text-white tracking-[1px] mb-2',
  sportsSub:             'text-xs text-white/50 mb-4',

  //  Sports list 
  sportsListContainer:   'mt-4 px-5',
  sportRow:              'flex-row items-center gap-3.5 py-3.5',
  sportRowLast:          'flex-row items-center gap-3.5 py-3.5',
  sportThumb:            'w-32 h-[64px] rounded-[8px] bg-[#e5e5e5] shrink-0 overflow-hidden',
  sportInfo:             'flex-1',
  sportTitle:            'text-sm font-semibold text-black mb-0.5',
  sportCat:              'text-[10px] font-semibold tracking-[0.5px] mb-0.5',
  sportLoc:              'text-[11px]',
  sportArrow:            'text-[20px]',
  //  Loading 
  loadingRow:            'items-center py-10',
};

//  StyleSheet 
export const sheet = StyleSheet.create({
  absoluteFill: { position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 },

  // ── Apple TV-style Top 10 rank number ────────────────────────────────────
  // Sits top-left ON the card image. Card keeps its normal 200×130px dimensions.
  top10Rank: {
    position: 'absolute',
    top: 6,
    left: 10,
    fontSize: 52,
    fontWeight: '900',
    color: '#ffffff',
    lineHeight: 58,
    letterSpacing: -2,
    zIndex: 3,
    textShadowColor: 'rgba(0,0,0,0.4)',
    textShadowOffset: { width: 0, height: 2 },
    textShadowRadius: 6,
  },
});