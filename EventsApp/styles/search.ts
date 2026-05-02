import { StyleSheet } from 'react-native';

// ─── className strings ────────────────────────────────────────────────────────
// Import: import { s, sheet } from '../../styles/search';

export const s = {

  // ── Shell ─────────────────────────────────────────────────────────────────
  safe:             'flex-1 bg-white',

  // ── Header ────────────────────────────────────────────────────────────────
  header:           'px-5 pt-3 pb-1',
  headerTitle:      'text-[28px] font-extrabold text-[#0a0a0a] tracking-[0.2px]',

  // ── Search bar ────────────────────────────────────────────────────────────
  searchRow:        'flex-row items-center px-4 py-2.5 gap-2.5',
  searchInput:      'flex-1 text-[15px] text-[#0a0a0a]',
  cancelBtn:        'py-1.5 pl-0.5',
  cancelText:       'text-[16px] text-brand-pink font-semibold',

  // ── Suggestions ───────────────────────────────────────────────────────────
  suggHeading:      'text-[18px] font-extrabold text-[#0a0a0a] px-5 pt-3 pb-2',
  suggRow:          'flex-row items-center px-5 py-4 bg-white',
  suggLabel:        'flex-1 text-[16px] font-medium text-[#0a0a0a]',

  // ── Back nav ──────────────────────────────────────────────────────────────
  backRow:          'flex-row items-center px-4 py-2.5',
  backText:         'text-[16px] font-bold text-brand-pink',

  // ── Result row ────────────────────────────────────────────────────────────
  resultRow:        'flex-row items-center px-4 py-3.5 gap-3.5',
  resultInfo:       'flex-1',
  resultCat:        'text-[9px] font-extrabold text-brand-pink tracking-[1.4px] uppercase mb-0.5',
  resultTitle:      'text-[14px] font-bold text-[#0a0a0a] leading-[19px] mb-[3px]',
  resultMeta:       'text-[11px] text-[#0a0a0a]/45 mb-[1px]',
  resultDate:       'text-[11px] text-[#0a0a0a]/45',
  resultPriceCol:   'items-end justify-center min-w-[56px]',
  resultPrice:      'text-[13px] font-extrabold text-[#0a0a0a]',

  // ── Empty / loading ───────────────────────────────────────────────────────
  centred:          'flex-1 items-center justify-center pt-20 px-8',
  emptyTitle:       'text-[17px] font-bold text-[#0a0a0a] mt-4 mb-1.5',
  emptySubtitle:    'text-[13px] text-[#0a0a0a]/45 text-center leading-[19px]',
};

// ─── StyleSheet ───────────────────────────────────────────────────────────────
export const sheet = StyleSheet.create({
  // Search box — border state toggled inline
  searchBox: {
    flex: 1,
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: 'rgba(10,10,10,0.06)',
    borderRadius: 14,
    paddingHorizontal: 14,
    height: 46,
    borderWidth: 1.5,
    borderColor: 'transparent',
  },
  searchBoxFocused: {
    borderColor: '#ff3c6e',
    backgroundColor: '#fff',
  },

  // Suggestion icon pill
  suggIcon: {
    width: 42,
    height: 42,
    borderRadius: 21,
    alignItems: 'center',
    justifyContent: 'center',
    marginRight: 16,
  },

  // Divider
  divider: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: 'rgba(10,10,10,0.10)',
    marginLeft: 20,
  },

  // Result thumbnail
  resultThumb: {
    width: 72,
    height: 60,
    borderRadius: 10,
    overflow: 'hidden',
    backgroundColor: '#e5e5e5',
    flexShrink: 0,
  },
});