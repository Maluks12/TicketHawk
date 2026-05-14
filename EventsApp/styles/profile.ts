import { Dimensions, StyleSheet } from 'react-native';

const SCREEN_HEIGHT = Dimensions.get('window').height;

// ─── NativeWind className strings ─────────────────────────────────────────────
// Import: import { s, sheet } from '../../styles/profile';

export const s = {

  // ── Shell ─────────────────────────────────────────────────────────────────
  root:                 'flex-1 bg-dark-base',
  scroll:               'px-4 pt-1',

  // ── Header ────────────────────────────────────────────────────────────────
  header:               'px-6 pb-[18px]',
  headerTitle:          'text-[28px] font-extrabold text-white tracking-[-0.5px]',

  // ── Avatar ────────────────────────────────────────────────────────────────
  avatarSection:        'flex-row items-center gap-4 px-1 pb-1',
  avatarCircle:         'w-[60px] h-[60px] rounded-full bg-brand-pink items-center justify-center',
  avatarInitials:       'text-[22px] font-extrabold text-white tracking-widest',
  avatarName:           'text-[18px] font-extrabold text-white tracking-[-0.2px]',
  avatarEmail:          'text-[13px] text-white/40 mt-0.5',

  // ── Sign-in prompt ────────────────────────────────────────────────────────
  signInRow:            'flex-row items-center gap-3.5 p-4',
  signInAvatar:         'w-[50px] h-[50px] rounded-full bg-white/[0.07] items-center justify-center',
  signInTitle:          'text-[16px] font-bold text-white',
  signInSub:            'text-[12px] text-white/35 mt-0.5',

  // ── Section ───────────────────────────────────────────────────────────────
  section:              'gap-2',
  sectionTitle:         'text-[10px] font-extrabold tracking-[2.5px] text-white/28 ml-1',

  // ── Row ───────────────────────────────────────────────────────────────────
  row:                  'flex-row items-center justify-between px-4 py-[15px]',
  rowLeft:              'flex-row items-center gap-[13px] flex-1',
  rowIconWrap:          'w-[22px] items-center',
  rowLabel:             'text-[15px] font-medium text-white',
  rowLabelDestructive:  'text-status-cancelled',
  rowRight:             'flex-row items-center gap-1.5',
  rowValue:             'text-[13px] text-white/40',

  // ── Following sheet — empty state ─────────────────────────────────────────
  emptyTitle:           'text-[19px] font-extrabold text-white text-center tracking-[-0.2px]',
  emptySub:             'text-[13px] text-center leading-5',

  // ── Misc ──────────────────────────────────────────────────────────────────
  version:              'text-center text-[12px] text-white/[0.18] mt-1',
};

// ─── StyleSheet ───────────────────────────────────────────────────────────────
export const sheet = StyleSheet.create({

  // ── Profile section card + divider ────────────────────────────────────────
  sectionCard: {
    backgroundColor: 'rgba(255,255,255,0.05)',
    borderRadius: 16,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: 'rgba(255,255,255,0.08)',
    overflow: 'hidden',
  },
  divider: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: 'rgba(255,255,255,0.07)',
    marginLeft: 51,
  },

  // ── Following bottom sheet ─────────────────────────────────────────────────
  // Full-screen overlay that sits behind the sheet panel
  sheetOverlay: {
    flex: 1,
    backgroundColor: 'rgba(0,0,0,0.6)',
    justifyContent: 'flex-end',
  },
  // The sliding panel itself — 88 % of screen height so a sliver of the
  // profile page peeks above it
  sheetPanel: {
    height: SCREEN_HEIGHT * 0.88,
    backgroundColor: '#0f0f0f',
    borderTopLeftRadius: 24,
    borderTopRightRadius: 24,
    overflow: 'hidden',
    borderTopWidth: StyleSheet.hairlineWidth,
    borderLeftWidth: StyleSheet.hairlineWidth,
    borderRightWidth: StyleSheet.hairlineWidth,
    borderColor: 'rgba(255,255,255,0.09)',
  },
  // Drag-to-dismiss handle area (tappable + pan)
  sheetHandleArea: {
    paddingTop: 12,
    paddingBottom: 4,
    alignItems: 'center',
  },
  sheetHandle: {
    width: 38,
    height: 4,
    borderRadius: 2,
    backgroundColor: 'rgba(255,255,255,0.18)',
  },
  // Sheet header row: title left, close button right
  sheetHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 20,
    paddingTop: 4,
    paddingBottom: 14,
  },
  sheetTitle: {
    fontSize: 20,
    fontWeight: '800',
    color: '#ffffff',
    letterSpacing: -0.3,
  },
  sheetCount: {
    fontSize: 13,
    fontWeight: '600',
    color: 'rgba(255,255,255,0.35)',
    marginTop: 1,
  },
  sheetCloseBtn: {
    width: 32,
    height: 32,
    borderRadius: 16,
    backgroundColor: 'rgba(255,255,255,0.08)',
    alignItems: 'center',
    justifyContent: 'center',
  },

  // ── Event card ────────────────────────────────────────────────────────────
  cardTouchable: {
    marginHorizontal: 16,
    marginBottom: 12,
    borderRadius: 18,
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 8 },
    shadowOpacity: 0.5,
    shadowRadius: 14,
    elevation: 12,
  },
  card: {
    height: 182,
    borderRadius: 18,
    overflow: 'hidden',
    padding: 14,
    justifyContent: 'space-between',
  },
  cardScrim: {
    ...StyleSheet.absoluteFillObject,
  } as any,
  cardTopRow: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
  },
  cardPills: {
    flexDirection: 'row',
    gap: 6,
    flexWrap: 'wrap',
    flex: 1,
    marginRight: 8,
  },
  cardPill: {
    paddingHorizontal: 8,
    paddingVertical: 4,
    borderRadius: 6,
    borderWidth: 1,
  },
  cardPillText: {
    fontSize: 9,
    fontWeight: '800',
    letterSpacing: 1.2,
  },
  cardHeartBtn: {
    width: 34,
    height: 34,
    borderRadius: 17,
    backgroundColor: 'rgba(0,0,0,0.45)',
    alignItems: 'center',
    justifyContent: 'center',
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.1)',
  },
  cardBottom: {
    gap: 4,
  },
  cardDateRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
  },
  cardDateTxt: {
    fontSize: 11,
    fontWeight: '600',
    color: 'rgba(255,255,255,0.55)',
    letterSpacing: 0.2,
  },
  cardTitle: {
    fontSize: 19,
    fontWeight: '800',
    color: '#ffffff',
    letterSpacing: -0.3,
    lineHeight: 23,
  },
  cardMetaRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
  },
  cardMetaTxt: {
    fontSize: 11,
    color: 'rgba(255,255,255,0.42)',
    fontWeight: '500',
    flex: 1,
  },
  cardPrice: {
    fontSize: 12,
    fontWeight: '800',
  },

  // ── Empty / loading states ─────────────────────────────────────────────────
  centred: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    paddingHorizontal: 40,
    gap: 12,
  },
  emptyIcon: {
    width: 76,
    height: 76,
    borderRadius: 22,
    backgroundColor: 'rgba(255,255,255,0.04)',
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: 4,
  },
});