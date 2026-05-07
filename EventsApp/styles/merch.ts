import { Dimensions, StyleSheet } from 'react-native';

// ─── className strings ────────────────────────────────────────────────────────
// Import as:  import { s, sheet } from '../styles/merch';
// Use as:     <View className={s.screen}>
//
// Dynamic colours (bg, tc derived from item category) stay as inline style
// props in the component — className cannot express runtime values.

const { width: W, height: H } = Dimensions.get('window');

const IMG_SINGLE      = Math.round(H * 0.52);
const IMG_GRID        = Math.round(H * 0.46);
const IMG_PLACEHOLDER = Math.round(H * 0.42);

export const s = {

  // ── Screen shell ─────────────────────────────────────────────────────────────
  screen: 'flex-1',

  // ── Header ───────────────────────────────────────────────────────────────────
  header:           'flex-row items-center justify-between px-5 pt-2 pb-1',
  headerTitle:      'text-[32px] font-black text-white tracking-[2px]',
  headerRight:      'flex-row items-center gap-[10px]',
  liveIndicator:    'w-2 h-2 rounded-full bg-[#ff3c6e]',
  liveText:         'text-[11px] font-bold text-[#ff3c6e] tracking-[1.5px]',
  cartBtn:          'p-1',
  cartIcon:         'text-xl',

  // ── Available / Upcoming toggle ───────────────────────────────────────────
  toggle:           'flex-row mx-5 mt-[10px] mb-[14px] rounded-lg bg-[#1a1a1a] p-[3px]',
  toggleBtn:        'flex-1 py-[10px] items-center rounded-md',
  toggleBtnActive:  'bg-[#ff3c6e]',
  toggleText:       'text-[10px] font-extrabold text-[#555] tracking-[1.2px]',
  toggleTextActive: 'text-white',

  // ── Drop card ─────────────────────────────────────────────────────────────
  card:             'mb-[2px]',
  cardTop:          'flex-row items-center px-4 py-3 gap-1.5',
  brandName:        'text-[11px] font-extrabold tracking-[1.5px]',
  cardTitle:        'text-[22px] font-bold leading-7 px-4 pt-[14px] pb-1',
  cardBottom:       'flex-row items-center px-4 py-[14px] border-t-[1px] mt-2',
  category:         'text-[10px] font-bold tracking-[1.5px]',
  releaseDate:      'text-[10px] font-bold tracking-[1px]',

  // ── Expiry countdown badge (Available cards) ──────────────────────────────
  expiryBadge:      'border px-[10px] py-1.5 rounded-[3px] items-center',
  expiryLabel:      'text-[8px] font-extrabold tracking-[1.5px] mb-[2px]',
  expiryTimer:      'text-[13px] font-black tracking-[1px]',

  // ── Drop countdown (Upcoming cards) ──────────────────────────────────────
  upcomingLabel:    'text-[10px] font-bold tracking-[1.5px]',
  countdownRow:     'border-t border-b py-[14px] px-4 mb-[2px]',
  countdownText:    'text-[36px] font-black tracking-[2px]',

  // ── Action buttons ────────────────────────────────────────────────────────
  actionBtn:        'border px-[14px] py-[7px] rounded-[2px]',
  actionBtnText:    'text-[10px] font-bold tracking-[1.5px]',

  // ── Image collage ─────────────────────────────────────────────────────────
 collagePlaceholder: 'w-full bg-[#1a1a1a]',
  collageSingle:      'w-full',
  collageGrid:        'flex-row',
  collageLeft:        'flex-1 mr-[2px]',
  collageSmall:       'flex-1',

  // ── Loading / error / empty ───────────────────────────────────────────────
  loadingRow:       'flex-1 items-center justify-center',
  errorBanner:      'flex-row items-center bg-[#ff3c6e20] border border-[#ff3c6e] m-4 p-3 rounded-lg',
  errorText:        'flex-1 text-[#ff3c6e] text-xs',
  retryBtn:         'border border-[#ff3c6e] px-3 py-1.5 rounded-full',
  retryText:        'text-[#ff3c6e] text-[10px] font-bold tracking-[1.5px]',
  empty:            'flex-1 items-center justify-center p-10 mt-[60px]',
  emptyIcon:        'text-[48px] mb-4',
  emptyTitle:       'text-lg font-extrabold text-white text-center mb-2',
  emptySub:         'text-[13px] text-[#666] text-center leading-5',

  // ── Modal drag handle area ────────────────────────────────────────────────
  dragHandleArea:   'pt-[14px] pb-1 items-center',
  dragHandle:       'w-11 h-[5px] rounded-[3px] mb-[10px]',
  modalTopBar:      'flex-row items-center px-4 pb-2 w-full',
  modalStatus:      'text-[10px] font-bold tracking-[1.5px]',
  modalHeat:        'text-[11px] font-bold tracking-[1px]',

  // ── Modal price box ───────────────────────────────────────────────────────
  heatScoreBox:       'border mx-4 p-[14px] rounded-[2px]',
  heatScoreBoxLabel:  'text-[10px] font-bold tracking-[1.5px] mb-1',
  heatScoreBoxScore:  'text-[36px] font-black',
  heatScoreBoxName:   'text-[11px] font-medium mt-1 leading-4',

  // ── Variants ──────────────────────────────────────────────────────────────
  variantsRow:      'px-4 py-[14px] border-t border-b mt-1',
  variantsLabel:    'text-[10px] font-bold tracking-[1.5px]',
  variantChip:      'border px-[14px] py-2 rounded-[2px] mr-2 items-center',
  variantChipText:  'text-[10px] font-bold tracking-[1px]',
  variantSoldOut:   'text-[8px] tracking-[1px] mt-[2px]',

  // ── Detail view ───────────────────────────────────────────────────────────
  detailTitle:      'text-[28px] font-extrabold leading-[34px] px-4 pt-5 pb-2',
  detailGrid:       'px-4 mt-4',
  detailRow:        'flex-row justify-between py-3 border-b',
  detailLabel:      'text-[11px] font-semibold tracking-[1.5px]',
  detailValue:      'text-[13px] font-bold tracking-[0.5px]',
  detailDesc:       'text-[15px] leading-6 px-4 pt-5',

  // ── Modal bottom action bar ───────────────────────────────────────────────
  modalBottom:        'absolute bottom-0 left-0 right-0 flex-row items-center px-5 py-4 pb-9 border-t gap-3',
  modalBottomAlerts:  'text-[10px] font-bold tracking-[1.5px]',
  modalBottomCenter:  'flex-1 items-center',
  modalBottomTitle:   'text-[11px] font-bold tracking-[0.5px]',
  modalBottomDate:    'text-[10px] font-medium mt-[2px]',
  buyNowBtn:          'border px-[18px] py-2 rounded-[2px]',
  buyNowText:         'text-xs font-extrabold tracking-[1.5px]',
};

// ─── StyleSheet ───────────────────────────────────────────────────────────────
// Only for values NativeWind className cannot express:
//   • absoluteFill — position absolute covering parent (used on Image inside View)
//   • collageRight — width derived from screen dimension (runtime value)
//   • modalFlex    — flex:1 base for the Animated.View modal (bg added inline)

export const sheet = StyleSheet.create({
  absoluteFill: {
    position: 'absolute',
    top: 0, left: 0, right: 0, bottom: 0,
  },
  // collageRight width is 38% of screen — must be computed at module load time.
  collageRight: {
    width: W * 0.38,
    flexDirection: 'column',
  },
  // The modal Animated.View needs flex:1 as a base; bg colour is added inline
  // as a dynamic value (derived from item category), so we can't use className.
  modalFlex: {
    flex: 1,
  },
  collageSingleImg: {
  width: '100%',
  height: IMG_SINGLE,
},
collageGridContainer: {
  flexDirection: 'row',
  height: IMG_GRID,
},
collagePlaceholderBox: {
  width: '100%',
  height: IMG_PLACEHOLDER,
  backgroundColor: '#1a1a1a',
},
});