import { StyleSheet, Dimensions } from 'react-native';

// ─── className strings ────────────────────────────────────────────────────────
// Import: import { s, sheet } from '../../styles/booking';
// Use:    <View className={s.root} />   or   <View style={sheet.heroFade} />

const SW = Dimensions.get('window').width;

let dark = true;
export const setBookingTheme = (d: boolean) => { dark = d; };
const pick = <T extends object>(D: T, L: T): T =>
  new Proxy({} as T, { get: (_, k) => (dark ? D : L)[k as keyof T] }) as T;

const sDark = {

  // ── Shell ────────────────────────────────────────────────────────────────
  root:             'flex-1 bg-dark-base',

  // ── Floating header ──────────────────────────────────────────────────────
  floatingHeader:   'absolute top-0 left-0 right-0 z-50 flex-row items-center px-4 pb-3',
  headerBack:       'w-9 h-9 rounded-[10px] bg-white/[0.12] items-center justify-center',
  headerTitle:      'flex-1 text-center text-[15px] font-bold text-white mx-2',

  // ── Hero ─────────────────────────────────────────────────────────────────
  hero:             'h-80 relative',
  heroBg:           'absolute inset-0 bg-[#1a1a1a]',
  heroContent:      'absolute bottom-0 left-0 right-0 p-6 gap-2',
  heroTitle:        'text-[28px] font-extrabold text-white tracking-[-0.5px] leading-[33px]',
  heroDate:         'text-[13px] text-white/55 font-medium',

  // ── Status badge ─────────────────────────────────────────────────────────
  statusBadge:      'flex-row items-center gap-1.5 px-2.5 py-[5px] rounded-full border self-start',
  statusDot:        'w-1.5 h-1.5 rounded-full',
  statusText:       'text-[10px] font-extrabold tracking-[1.5px]',

  // ── QR section ───────────────────────────────────────────────────────────
  qrSection:        'items-center px-6 pt-2 pb-1',
  qrNumber:         'text-[17px] font-extrabold text-white tracking-[2px]',
  qrHint:           'text-[12px] text-white/35 text-center',

  // ── Section label ─────────────────────────────────────────────────────────
  sectionLabel:     'text-[10px] font-extrabold tracking-[2.5px] text-white/30 mb-3.5 ml-0.5',

  // ── Info rows ─────────────────────────────────────────────────────────────
  infoRow:          'flex-row items-start justify-between gap-3',
  infoRowLeft:      'flex-row items-center gap-2.5 flex-1',
  infoLabel:        'text-[13px] text-white/45 font-medium',
  infoValue:        'text-[13px] text-white font-semibold text-right flex-[1.2]',

  // ── Pricing ───────────────────────────────────────────────────────────────
  priceLine:        'flex-row justify-between items-center',
  priceLabel:       'text-[13px] text-white/50',
  priceValue:       'text-[13px] text-white/70 font-medium',
  priceLabelBold:   'text-[15px] text-white font-bold',
  priceValueBold:   'text-[15px] text-white font-extrabold',

  // ── Refund banner ─────────────────────────────────────────────────────────
  refundedBanner:   'flex-row items-start gap-2 bg-[#00e5a0]/[0.08] rounded-[10px] p-3',
  refundedText:     'flex-1 text-[13px] text-status-confirmed leading-[18px]',
  refundReason:     'text-[12px] text-white/30 italic mt-1',

  // ── Actions ───────────────────────────────────────────────────────────────
  actions:          'mx-4 mt-5 gap-3',
  refundBtn:        'flex-row items-center justify-center gap-2 bg-status-cancelled/[0.08] rounded-[14px] border border-status-cancelled/20 py-[15px]',
  refundBtnText:    'text-status-cancelled text-[14px] font-bold',

  // ── Error / empty ─────────────────────────────────────────────────────────
  errTitle:         'text-[18px] font-bold text-white',
  backBtnFull:      'mt-2 px-5 py-2.5 rounded-[10px] bg-white/[0.07]',
  backBtnFullText:  'text-white text-[13px] font-bold',

  // ── Refund modal ──────────────────────────────────────────────────────────
  modalOverlay:     'flex-1 bg-black/70 justify-end',
  modalHandle:      'w-10 h-1 rounded-sm bg-white/15 self-center mb-2',
  modalTitle:       'text-[20px] font-extrabold text-white',
  modalSub:         'text-[13px] text-white/40 leading-[19px]',
  modalFieldLabel:  'text-[10px] font-extrabold tracking-[2px] text-white/30',
  charCount:        'text-[11px] text-white/20 text-right -mt-2',
  modalBtnText:     'text-white text-[15px] font-extrabold',
  modalCancel:      'items-center py-2.5',
  modalCancelText:  'text-white/40 text-[14px] font-semibold',
};

const sLight = {

  // ── Shell ────────────────────────────────────────────────────────────────
  root:             'flex-1 bg-white',

  // ── Floating header ──────────────────────────────────────────────────────
  floatingHeader:   'absolute top-0 left-0 right-0 z-50 flex-row items-center px-4 pb-3',
  headerBack:       'w-9 h-9 rounded-[10px] bg-black/[0.12] items-center justify-center',
  headerTitle:      'flex-1 text-center text-[15px] font-bold text-white mx-2',

  // ── Hero ─────────────────────────────────────────────────────────────────
  hero:             'h-80 relative',
  heroBg:           'absolute inset-0 bg-[#1a1a1a]',
  heroContent:      'absolute bottom-0 left-0 right-0 p-6 gap-2',
  heroTitle:        'text-[28px] font-extrabold text-white tracking-[-0.5px] leading-[33px]',
  heroDate:         'text-[13px] text-white/70 font-medium',

  // ── Status badge ─────────────────────────────────────────────────────────
  statusBadge:      'flex-row items-center gap-1.5 px-2.5 py-[5px] rounded-full border self-start',
  statusDot:        'w-1.5 h-1.5 rounded-full',
  statusText:       'text-[10px] font-extrabold tracking-[1.5px]',

  // ── QR section ───────────────────────────────────────────────────────────
  qrSection:        'items-center px-6 pt-2 pb-1',
  qrNumber:         'text-[17px] font-extrabold text-[#0a0a0a] tracking-[2px]',
  qrHint:           'text-[12px] text-black/35 text-center',

  // ── Section label ─────────────────────────────────────────────────────────
  sectionLabel:     'text-[10px] font-extrabold tracking-[2.5px] text-black/30 mb-3.5 ml-0.5',

  // ── Info rows ─────────────────────────────────────────────────────────────
  infoRow:          'flex-row items-start justify-between gap-3',
  infoRowLeft:      'flex-row items-center gap-2.5 flex-1',
  infoLabel:        'text-[13px] text-black/45 font-medium',
  infoValue:        'text-[13px] text-[#0a0a0a] font-semibold text-right flex-[1.2]',

  // ── Pricing ───────────────────────────────────────────────────────────────
  priceLine:        'flex-row justify-between items-center',
  priceLabel:       'text-[13px] text-black/50',
  priceValue:       'text-[13px] text-black/70 font-medium',
  priceLabelBold:   'text-[15px] text-[#0a0a0a] font-bold',
  priceValueBold:   'text-[15px] text-[#0a0a0a] font-extrabold',

  // ── Refund banner ─────────────────────────────────────────────────────────
  refundedBanner:   'flex-row items-start gap-2 bg-[#00e5a0]/[0.08] rounded-[10px] p-3',
  refundedText:     'flex-1 text-[13px] text-status-confirmed leading-[18px]',
  refundReason:     'text-[12px] text-black/30 italic mt-1',

  // ── Actions ───────────────────────────────────────────────────────────────
  actions:          'mx-4 mt-5 gap-3',
  refundBtn:        'flex-row items-center justify-center gap-2 bg-status-cancelled/[0.08] rounded-[14px] border border-status-cancelled/20 py-[15px]',
  refundBtnText:    'text-status-cancelled text-[14px] font-bold',

  // ── Error / empty ─────────────────────────────────────────────────────────
  errTitle:         'text-[18px] font-bold text-[#0a0a0a]',
  backBtnFull:      'mt-2 px-5 py-2.5 rounded-[10px] bg-black/[0.07]',
  backBtnFullText:  'text-[#0a0a0a] text-[13px] font-bold',

  // ── Refund modal ──────────────────────────────────────────────────────────
  modalOverlay:     'flex-1 bg-black/70 justify-end',
  modalHandle:      'w-10 h-1 rounded-sm bg-black/15 self-center mb-2',
  modalTitle:       'text-[20px] font-extrabold text-[#0a0a0a]',
  modalSub:         'text-[13px] text-black/40 leading-[19px]',
  modalFieldLabel:  'text-[10px] font-extrabold tracking-[2px] text-black/30',
  charCount:        'text-[11px] text-black/20 text-right -mt-2',
  modalBtnText:     'text-[#0a0a0a] text-[15px] font-extrabold',
  modalCancel:      'items-center py-2.5',
  modalCancelText:  'text-black/40 text-[14px] font-semibold',
};

export const s = pick(sDark, sLight);

const sheetDark = StyleSheet.create({

  // ── Hero ─────────────────────────────────────────────────────────────────
  heroFade: {
    ...StyleSheet.absoluteFillObject,
  },

  // ── QR card ──────────────────────────────────────────────────────────────
  qrCard: {
    width: '100%',
    backgroundColor: 'rgba(255,255,255,0.05)',
    borderRadius: 20,
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.08)',
    alignItems: 'center',
    padding: 28,
    gap: 16,
  },
  qrImage: {
    width: SW - 96,
    height: SW - 96,
    borderRadius: 8,
  },

  // ── Slot badge (multi-ticket label) ──────────────────────────────────────
  slotBadge: {
    alignSelf: 'center',
    backgroundColor: 'rgba(255,255,255,0.08)',
    borderRadius: 20,
    paddingHorizontal: 12,
    paddingVertical: 4,
    marginBottom: 4,
  },
  slotBadgeText: {
    color: 'rgba(255,255,255,0.45)',
    fontSize: 10,
    fontWeight: '700',
    letterSpacing: 1.5,
  },

  // ── Pending / unassigned QR placeholder ──────────────────────────────────
  pendingQR: {
    width: '100%',
    aspectRatio: 1,
    borderRadius: 8,
    backgroundColor: 'rgba(255,255,255,0.04)',
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.06)',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 10,
  },
  pendingText: {
    color: 'rgba(255,255,255,0.25)',
    fontSize: 13,
    textAlign: 'center',
  },
  assigneeName: {
    color: 'rgba(255,255,255,0.7)',
    fontSize: 14,
    fontWeight: '600',
    marginTop: 4,
  },

  // ── Assign button ─────────────────────────────────────────────────────────
  assignBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 7,
    marginTop: 14,
    backgroundColor: '#ffffff',
    borderRadius: 12,
    paddingVertical: 12,
    paddingHorizontal: 20,
  },
  assignBtnText: {
    color: '#0a0a0a',
    fontSize: 14,
    fontWeight: '700',
    letterSpacing: 0.3,
  },

  // ── Revoke button ─────────────────────────────────────────────────────────
  revokeBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 7,
    marginTop: 10,
    borderRadius: 12,
    paddingVertical: 11,
    paddingHorizontal: 20,
    borderWidth: 1,
    borderColor: 'rgba(255,107,107,0.35)',
    backgroundColor: 'rgba(255,107,107,0.08)',
  },
  revokeBtnText: {
    color: '#ff6b6b',
    fontSize: 13,
    fontWeight: '600',
  },

  // ── Assign bottom sheet ───────────────────────────────────────────────────
  assignSheet: {
    backgroundColor: '#141414',
    borderTopLeftRadius: 24,
    borderTopRightRadius: 24,
    paddingHorizontal: 24,
    paddingTop: 12,
    borderTopWidth: 1,
    borderColor: 'rgba(255,255,255,0.08)',
  },
  sheetHandle: {
    width: 36,
    height: 4,
    borderRadius: 2,
    backgroundColor: 'rgba(255,255,255,0.15)',
    alignSelf: 'center',
    marginBottom: 20,
  },
  sheetTitle: {
    color: '#ffffff',
    fontSize: 18,
    fontWeight: '700',
    marginBottom: 6,
  },
  sheetSub: {
    color: 'rgba(255,255,255,0.4)',
    fontSize: 13,
    lineHeight: 19,
    marginBottom: 22,
  },
  fieldLabel: {
    color: 'rgba(255,255,255,0.35)',
    fontSize: 10,
    fontWeight: '700',
    letterSpacing: 1.4,
    marginBottom: 7,
  },
  sheetInput: {
    backgroundColor: 'rgba(255,255,255,0.06)',
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.1)',
    borderRadius: 12,
    paddingHorizontal: 16,
    paddingVertical: 13,
    color: '#ffffff',
    fontSize: 15,
  },
  sheetInputError: {
    borderColor: 'rgba(255,107,107,0.5)',
  },
  sheetErrorBox: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    marginTop: 10,
    backgroundColor: 'rgba(255,107,107,0.08)',
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  sheetErrorText: {
    color: '#ff6b6b',
    fontSize: 13,
    flex: 1,
  },
  sendBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
    marginTop: 20,
    backgroundColor: '#ffffff',
    borderRadius: 14,
    paddingVertical: 15,
  },
  sendBtnText: {
    color: '#0a0a0a',
    fontSize: 15,
    fontWeight: '700',
    letterSpacing: 0.2,
  },
  cancelBtn: {
    alignItems: 'center',
    paddingVertical: 14,
    marginTop: 4,
  },
  cancelBtnText: {
    color: 'rgba(255,255,255,0.3)',
    fontSize: 14,
  },

  // ── Info card ─────────────────────────────────────────────────────────────
  card: {
    marginHorizontal: 16,
    marginTop: 20,
    backgroundColor: 'rgba(255,255,255,0.04)',
    borderRadius: 16,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: 'rgba(255,255,255,0.08)',
    padding: 18,
    gap: 14,
  },

  priceDivider: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: 'rgba(255,255,255,0.08)',
    marginVertical: 4,
  },

  // ── Refund modal sheet ────────────────────────────────────────────────────
  modalSheet: {
    backgroundColor: '#161616',
    borderTopLeftRadius: 24,
    borderTopRightRadius: 24,
    padding: 24,
    paddingBottom: 40,
    gap: 14,
  },
  modalInput: {
    backgroundColor: 'rgba(255,255,255,0.06)',
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.1)',
    borderRadius: 12,
    padding: 14,
    color: '#fff',
    fontSize: 14,
    lineHeight: 20,
    minHeight: 100,
  },
  modalInputError: {
    borderColor: 'rgba(255,77,77,0.5)',
  },
  modalError: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    gap: 7,
    backgroundColor: 'rgba(255,80,80,0.08)',
    borderRadius: 10,
    padding: 11,
  },
  modalErrorText: {
    flex: 1,
    fontSize: 13,
    color: '#ff8080',
    lineHeight: 18,
  },
  modalBtn: {
    backgroundColor: '#ff3c6e',
    borderRadius: 14,
    paddingVertical: 16,
    alignItems: 'center',
  },
});
const sheetLight = StyleSheet.create({

  // ── Hero ─────────────────────────────────────────────────────────────────
  heroFade: {
    ...StyleSheet.absoluteFillObject,
  },

  // ── QR card ──────────────────────────────────────────────────────────────
  qrCard: {
    width: '100%',
    backgroundColor: 'rgba(0,0,0,0.05)',
    borderRadius: 20,
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.08)',
    alignItems: 'center',
    padding: 28,
    gap: 16,
  },
  qrImage: {
    width: SW - 96,
    height: SW - 96,
    borderRadius: 8,
  },

  // ── Slot badge (multi-ticket label) ──────────────────────────────────────
  slotBadge: {
    alignSelf: 'center',
    backgroundColor: 'rgba(0,0,0,0.08)',
    borderRadius: 20,
    paddingHorizontal: 12,
    paddingVertical: 4,
    marginBottom: 4,
  },
  slotBadgeText: {
    color: 'rgba(0,0,0,0.45)',
    fontSize: 10,
    fontWeight: '700',
    letterSpacing: 1.5,
  },

  // ── Pending / unassigned QR placeholder ──────────────────────────────────
  pendingQR: {
    width: '100%',
    aspectRatio: 1,
    borderRadius: 8,
    backgroundColor: 'rgba(0,0,0,0.04)',
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.06)',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 10,
  },
  pendingText: {
    color: 'rgba(0,0,0,0.25)',
    fontSize: 13,
    textAlign: 'center',
  },
  assigneeName: {
    color: 'rgba(0,0,0,0.7)',
    fontSize: 14,
    fontWeight: '600',
    marginTop: 4,
  },

  // ── Assign button ─────────────────────────────────────────────────────────
  assignBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 7,
    marginTop: 14,
    backgroundColor: '#0a0a0a',
    borderRadius: 12,
    paddingVertical: 12,
    paddingHorizontal: 20,
  },
  assignBtnText: {
    color: '#ffffff',
    fontSize: 14,
    fontWeight: '700',
    letterSpacing: 0.3,
  },

  // ── Revoke button ─────────────────────────────────────────────────────────
  revokeBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 7,
    marginTop: 10,
    borderRadius: 12,
    paddingVertical: 11,
    paddingHorizontal: 20,
    borderWidth: 1,
    borderColor: 'rgba(255,107,107,0.35)',
    backgroundColor: 'rgba(255,107,107,0.08)',
  },
  revokeBtnText: {
    color: '#ff6b6b',
    fontSize: 13,
    fontWeight: '600',
  },

  // ── Assign bottom sheet ───────────────────────────────────────────────────
  assignSheet: {
    backgroundColor: '#0a0a0a',
    borderTopLeftRadius: 24,
    borderTopRightRadius: 24,
    paddingHorizontal: 24,
    paddingTop: 12,
    borderTopWidth: 1,
    borderColor: 'rgba(0,0,0,0.08)',
  },
  sheetHandle: {
    width: 36,
    height: 4,
    borderRadius: 2,
    backgroundColor: 'rgba(0,0,0,0.15)',
    alignSelf: 'center',
    marginBottom: 20,
  },
  sheetTitle: {
    color: '#0a0a0a',
    fontSize: 18,
    fontWeight: '700',
    marginBottom: 6,
  },
  sheetSub: {
    color: 'rgba(0,0,0,0.4)',
    fontSize: 13,
    lineHeight: 19,
    marginBottom: 22,
  },
  fieldLabel: {
    color: 'rgba(0,0,0,0.35)',
    fontSize: 10,
    fontWeight: '700',
    letterSpacing: 1.4,
    marginBottom: 7,
  },
  sheetInput: {
    backgroundColor: 'rgba(0,0,0,0.06)',
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.1)',
    borderRadius: 12,
    paddingHorizontal: 16,
    paddingVertical: 13,
    color: '#0a0a0a',
    fontSize: 15,
  },
  sheetInputError: {
    borderColor: 'rgba(255,107,107,0.5)',
  },
  sheetErrorBox: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    marginTop: 10,
    backgroundColor: 'rgba(255,107,107,0.08)',
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  sheetErrorText: {
    color: '#ff6b6b',
    fontSize: 13,
    flex: 1,
  },
  sendBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
    marginTop: 20,
    backgroundColor: '#0a0a0a',
    borderRadius: 14,
    paddingVertical: 15,
  },
  sendBtnText: {
    color: '#ffffff',
    fontSize: 15,
    fontWeight: '700',
    letterSpacing: 0.2,
  },
  cancelBtn: {
    alignItems: 'center',
    paddingVertical: 14,
    marginTop: 4,
  },
  cancelBtnText: {
    color: 'rgba(0,0,0,0.3)',
    fontSize: 14,
  },

  // ── Info card ─────────────────────────────────────────────────────────────
  card: {
    marginHorizontal: 16,
    marginTop: 20,
    backgroundColor: 'rgba(0,0,0,0.04)',
    borderRadius: 16,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: 'rgba(0,0,0,0.08)',
    padding: 18,
    gap: 14,
  },

  priceDivider: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: 'rgba(0,0,0,0.08)',
    marginVertical: 4,
  },

  // ── Refund modal sheet ────────────────────────────────────────────────────
  modalSheet: {
    backgroundColor: '#0a0a0a',
    borderTopLeftRadius: 24,
    borderTopRightRadius: 24,
    padding: 24,
    paddingBottom: 40,
    gap: 14,
  },
  modalInput: {
    backgroundColor: 'rgba(0,0,0,0.06)',
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.1)',
    borderRadius: 12,
    padding: 14,
    color: '#0a0a0a',
    fontSize: 14,
    lineHeight: 20,
    minHeight: 100,
  },
  modalInputError: {
    borderColor: 'rgba(255,77,77,0.5)',
  },
  modalError: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    gap: 7,
    backgroundColor: 'rgba(255,80,80,0.08)',
    borderRadius: 10,
    padding: 11,
  },
  modalErrorText: {
    flex: 1,
    fontSize: 13,
    color: '#ff8080',
    lineHeight: 18,
  },
  modalBtn: {
    backgroundColor: '#ff3c6e',
    borderRadius: 14,
    paddingVertical: 16,
    alignItems: 'center',
  },
});
export const sheet = pick(sheetDark, sheetLight);