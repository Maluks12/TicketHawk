import { StyleSheet, Dimensions } from 'react-native';

// ─── Import ───────────────────────────────────────────────────────────────────
// import { makePurchaseSheet, PRIMARY } from '../../styles/purchase';
// const ss = useMemo(() => makePurchaseSheet(isDark), [isDark]);

const { width: SW } = Dimensions.get('window');

export const PRIMARY = '#FF2D55';

// ─── Theme tokens ─────────────────────────────────────────────────────────────
const LIGHT = {
  bg: '#ffffff', sheet: '#ffffff', surface: '#fafafa', faintSurface: '#f3f4f6',
  border2: '#e5e7eb', border3: '#d1d5db',
  text: '#111111', sub: '#6b7280', subStrong: '#374151', faint: '#9ca3af',
  link: '#2563eb',
  selBg: '#fff5f7', selBorder: '#fecdd3',
  infoBg: '#f8faff', infoBorder: '#e0eaff',
  vipBg: '#fef3c7', vipText: '#92400e',
  okBg: '#f0fdf4', okText: '#15803d',
  soldBg: '#fff1f2', soldText: '#be123c',
  // Trust badges. A single green that reads as "verified" on either surface —
  // #2ecc71 (what web-mobile uses) is legible on white but glares on #0e0e12,
  // so the dark value is lifted for contrast rather than reused verbatim.
  trustIcon: '#16a34a', trustSurface: '#fafafa', trustBorder: '#e5e7eb',
};

const DARK: typeof LIGHT = {
  bg: '#0e0e12', sheet: '#1b1b20', surface: '#17171c', faintSurface: '#232329',
  border2: '#2e2e35', border3: '#43434b',
  text: '#ffffff', sub: '#a1a1aa', subStrong: '#c8c8cf', faint: '#8a8a93',
  link: '#60a5fa',
  selBg: '#2a141b', selBorder: '#59202e',
  infoBg: '#131a28', infoBorder: '#26334d',
  vipBg: '#3a2d0d', vipText: '#fbbf24',
  okBg: '#10261a', okText: '#4ade80',
  soldBg: '#2c1317', soldText: '#f87171',
  trustIcon: '#34d399', trustSurface: '#17171c', trustBorder: '#2e2e35',
};

export type PurchaseTokens = typeof LIGHT;
export const purchaseTokens = (dark: boolean): PurchaseTokens => (dark ? DARK : LIGHT);

// ─── StyleSheet factory ──────────────────────────────────────────────────────
export const makePurchaseSheet = (dark: boolean) => {
  const T = purchaseTokens(dark);
  return StyleSheet.create({

  // ── Shell ──────────────────────────────────────────────────────────────────
  screen:       { flex: 1, backgroundColor: T.bg },
  centered:     { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 12 },
  scroll:       { flex: 1 },
  errorText:    { fontSize: 13, fontWeight: '800', color: T.text, letterSpacing: 1 },
  goBackBtn:    { borderWidth: 1, borderColor: T.border3, paddingHorizontal: 16, paddingVertical: 8, borderRadius: 20 },
  goBackText:   { fontSize: 12, fontWeight: '700', color: T.text },
  loadingText:  { fontSize: 12, color: T.faint, marginTop: 8, fontWeight: '600' },

  // ── Header ─────────────────────────────────────────────────────────────────
  header: {
    backgroundColor: T.bg,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 20,
    paddingBottom: 12,
    borderBottomWidth: 1,
    borderBottomColor: T.faintSurface,
  },
  // 40px tap target; glyph is optically centered (lineHeight == fontSize kills
  // the baseline offset that pushed the old 26/32 chevron low and left).
  backBtn: {
    width: 40, height: 40, borderRadius: 20,
    backgroundColor: T.faintSurface,
    alignItems: 'center', justifyContent: 'center',
  },
  backIcon:     { fontSize: 24, fontWeight: '400', color: T.text, lineHeight: 24, textAlign: 'center', marginTop: -2 },
  closeBtn: {
    width: 32, height: 32, borderRadius: 16,
    backgroundColor: T.faintSurface,
    alignItems: 'center', justifyContent: 'center',
  },
  closeIcon:    { fontSize: 13, fontWeight: '700', color: T.text },
  headerTitle:  { fontSize: 16, fontWeight: '800', color: T.text, letterSpacing: -0.2 },
  stepDots:     { flexDirection: 'row', gap: 5, alignItems: 'center' },
  stepDot:      { width: 6, height: 6, borderRadius: 3, backgroundColor: T.border2 },
  stepDotActive: { backgroundColor: PRIMARY, width: 16 },

  // ── Banner ─────────────────────────────────────────────────────────────────
  bannerWrap: {
    marginHorizontal: 16,
    marginTop: 14,
    marginBottom: 4,
    height: 190,
    borderRadius: 16,
    overflow: 'hidden',
    position: 'relative',
    backgroundColor: '#1a1a1a',
  },
  bannerImage:          { width: '100%', height: '100%' },
  bannerPlaceholder:    { width: '100%', height: '100%', backgroundColor: '#1f1f2e', alignItems: 'center', justifyContent: 'center' },
  bannerPlaceholderIcon: { fontSize: 48 },
  bannerOverlay: {
    ...StyleSheet.absoluteFillObject,
    backgroundColor: 'rgba(0,0,0,0.3)',
  },
  bannerMeta:   { position: 'absolute', bottom: 14, left: 16, right: 16 },
  bannerDate: {
    fontSize: 12, fontWeight: '700', color: 'rgba(255,255,255,0.95)',
    letterSpacing: 0.2,
    textShadowColor: 'rgba(0,0,0,0.6)', textShadowOffset: { width: 0, height: 1 }, textShadowRadius: 4,
  },
  bannerVenue: {
    fontSize: 11, fontWeight: '500', color: 'rgba(255,255,255,0.75)', marginTop: 2,
    textShadowColor: 'rgba(0,0,0,0.6)', textShadowOffset: { width: 0, height: 1 }, textShadowRadius: 4,
  },

  // ── Event title ────────────────────────────────────────────────────────────
  eventTitleSection: {
    paddingHorizontal: 20, paddingTop: 20, paddingBottom: 16,
    borderBottomWidth: 1, borderBottomColor: T.faintSurface,
  },
  eventTitle:       { fontSize: 22, fontWeight: '800', color: T.text, letterSpacing: -0.4, marginBottom: 4 },
  eventSubtitle:    { fontSize: 14, fontWeight: '500', color: T.sub },
  eventVenueSmall:  { fontSize: 13, fontWeight: '400', color: T.faint, marginTop: 2 },

  // ── Section header ─────────────────────────────────────────────────────────
  sectionHeader: {
    flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between',
    paddingHorizontal: 20, paddingTop: 16, paddingBottom: 10,
  },
  sectionHeaderText: { fontSize: 11, fontWeight: '800', color: T.faint, letterSpacing: 1.2 },
  sortChip:          { backgroundColor: T.faintSurface, borderRadius: 20, paddingHorizontal: 10, paddingVertical: 4 },
  sortChipText:      { fontSize: 11, fontWeight: '600', color: T.subStrong },

  // ── Ticket cards ───────────────────────────────────────────────────────────
  ticketCard: {
    flexDirection: 'row', alignItems: 'center',
    backgroundColor: T.bg,
    borderBottomWidth: 1, borderBottomColor: T.faintSurface,
    paddingHorizontal: 20, paddingVertical: 16, gap: 14,
  },
  ticketCardSelected: {
    backgroundColor: T.selBg,
    borderLeftWidth: 3, borderLeftColor: PRIMARY, paddingLeft: 17,
  },
  ticketCardSoldOut: { opacity: 0.45 },
  radioOuter: {
    width: 20, height: 20, borderRadius: 10,
    borderWidth: 2, borderColor: T.border3,
    alignItems: 'center', justifyContent: 'center', flexShrink: 0,
  },
  radioOuterSelected: { borderColor: PRIMARY },
  radioInner:         { width: 10, height: 10, borderRadius: 5, backgroundColor: PRIMARY },
  ticketInfo:         { flex: 1, gap: 4 },
  ticketNameRow:      { flexDirection: 'row', alignItems: 'center', gap: 6 },
  ticketName:         { fontSize: 15, fontWeight: '700', color: T.text, letterSpacing: -0.1 },
  ticketNameSoldOut:  { color: T.faint },
  ticketAvail:        { fontSize: 12, color: T.faint, fontWeight: '500' },
  vipBadge:           { backgroundColor: T.vipBg, borderRadius: 4, paddingHorizontal: 5, paddingVertical: 1 },
  vipBadgeText:       { fontSize: 10, fontWeight: '700', color: T.vipText },
  dealBadge:          { backgroundColor: T.okBg, borderRadius: 4, paddingHorizontal: 6, paddingVertical: 2, alignSelf: 'flex-start' },
  dealBadgeText:      { fontSize: 10, fontWeight: '700', color: T.okText },
  soldBadge:          { backgroundColor: T.soldBg, borderRadius: 4, paddingHorizontal: 6, paddingVertical: 2, alignSelf: 'flex-start' },
  soldBadgeText:      { fontSize: 10, fontWeight: '700', color: T.soldText },
  ticketPriceCol:     { alignItems: 'flex-end', gap: 2 },
  ticketPrice:        { fontSize: 17, fontWeight: '900', color: T.text },
  ticketPriceSoldOut: { color: T.border3, textDecorationLine: 'line-through' },
  ticketFeeNote:      { fontSize: 10, color: T.faint, fontWeight: '500' },

  // ── Quantity picker ────────────────────────────────────────────────────────
  qtySection: {
    backgroundColor: T.surface, marginTop: 8,
    paddingHorizontal: 20, paddingTop: 20, paddingBottom: 24,
    borderTopWidth: 1, borderTopColor: T.faintSurface,
  },
  qtySectionHeader:   { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 },
  qtySectionTitle:    { fontSize: 14, fontWeight: '700', color: T.text },
  seatedNote:         { fontSize: 11, color: T.faint, fontWeight: '500' },
  qtyGrid:            { flexDirection: 'row', flexWrap: 'wrap', gap: 10 },
  qtyBtn: {
    flex: 1, minWidth: (SW - 60) / 3 - 8, paddingVertical: 16,
    borderRadius: 12, backgroundColor: T.faintSurface,
    borderWidth: 1.5, borderColor: 'transparent', alignItems: 'center',
  },
  qtyBtnSel:          { backgroundColor: T.bg, borderColor: PRIMARY },
  qtyBtnText:         { fontSize: 17, fontWeight: '800', color: T.faint },
  qtyBtnTextSel:      { color: PRIMARY },

  // ── Seating map ────────────────────────────────────────────────────────────
  mapSubtitle:  { fontSize: 12, color: T.faint, fontWeight: '500', paddingHorizontal: 20, marginBottom: 10 },
  legendRow:    { backgroundColor: T.bg, paddingHorizontal: 20, paddingVertical: 10, borderBottomWidth: 1, borderBottomColor: T.faintSurface },
  mapWrap:      { backgroundColor: T.bg, paddingHorizontal: 20, paddingTop: 16, paddingBottom: 4 },
  tierCard: {
    flexDirection: 'row', alignItems: 'center',
    marginHorizontal: 20, marginTop: 12, padding: 16,
    backgroundColor: T.selBg, borderRadius: 12,
    borderWidth: 1, borderColor: T.selBorder, gap: 12,
  },
  tierCardLeft:   { flex: 1 },
  tierCardName:   { fontSize: 13, fontWeight: '900', color: T.text, letterSpacing: 0.4 },
  tierCardMeta:   { fontSize: 11, color: T.faint, fontWeight: '500', marginTop: 2 },
  tierCardTotal:  { fontSize: 18, fontWeight: '900', color: PRIMARY },

  // ── Checkout summary ───────────────────────────────────────────────────────
  checkoutWrap: { paddingBottom: 8 },
  // No top border: eventTitleSection above already draws a bottom hairline,
  // so a top border here rendered as a double line above "Contact".
  detailsList: {
    borderBottomWidth: 1, borderColor: T.faintSurface,
    marginHorizontal: 20, marginTop: 4, marginBottom: 24,
  },
  detailRow: {
    flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between',
    paddingVertical: 16, borderBottomWidth: 1, borderBottomColor: T.faintSurface,
  },
  detailRowMultiline: {
    flexDirection: 'row', alignItems: 'flex-start',
    paddingVertical: 16, borderBottomWidth: 1, borderBottomColor: T.faintSurface,
  },
  detailLabel:    { width: 80, fontSize: 15, fontWeight: '700', color: T.text, flexShrink: 0 },
  detailRight:    { flex: 1, flexDirection: 'row', alignItems: 'center', justifyContent: 'flex-end', gap: 6, marginLeft: 12 },
  detailValue:    { fontSize: 14, color: T.sub, fontWeight: '500' },
  detailValueSub: { fontSize: 12, color: T.faint, marginTop: 2, fontWeight: '400' },
  chevron:        { fontSize: 18, color: T.border3, fontWeight: '300', lineHeight: 20 },

  // ── Trust badges ───────────────────────────────────────────────────────────
  // Card, not a bare list: on web these two lines sit inside their own bordered
  // panel between the ticket summary and the price breakdown, and the app looked
  // thinner at exactly the moment the buyer is deciding whether to hand over card
  // details. Matching the web layout keeps the reassurance in the same place on
  // both surfaces.
  trustCard: {
    marginHorizontal: 20,
    marginBottom: 16,
    padding: 16,
    gap: 12,
    backgroundColor: T.trustSurface,
    borderWidth: 1,
    borderColor: T.trustBorder,
    borderRadius: 14,
  },
  trustRow:     { flexDirection: 'row', alignItems: 'flex-start', gap: 10 },
  trustText:    { flex: 1 },
  trustTitle:   { fontSize: 13, fontWeight: '700', color: T.text },
  trustBody:    { fontSize: 12, color: T.sub, marginTop: 1, lineHeight: 16 },

  // Legacy keys, kept so any older reference still compiles.
  trustSection: { paddingHorizontal: 20, marginBottom: 24, gap: 14 },
  trustIcon:    { fontSize: 18, width: 24, textAlign: 'center' },
  trustHelp:    { fontSize: 16, color: T.border3 },

  // ── Price breakdown ────────────────────────────────────────────────────────
  priceBreakdown: {
    backgroundColor: T.surface, borderRadius: 14,
    marginHorizontal: 20, padding: 18, marginBottom: 16, gap: 10,
  },
  priceRow:         { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  priceLabel:       { fontSize: 13, color: T.sub, fontWeight: '500' },
  priceValue:       { fontSize: 13, color: T.sub, fontWeight: '600' },
  priceLabelLight:  { fontSize: 13, color: T.faint, fontWeight: '400' },
  priceValueLight:  { fontSize: 13, color: T.faint, fontWeight: '500' },
  priceDivider:     { height: 1, backgroundColor: T.border2, marginVertical: 2 },
  priceTotalLabel:  { fontSize: 15, fontWeight: '800', color: T.text },
  // Terms agreement under the price breakdown (fills the former dead space)
  // Aligned to the 20px page gutter used by the detail rows and price card.
  termsText: {
    marginTop: 16,
    marginHorizontal: 20,
    fontSize: 12.5,
    lineHeight: 19,
    color: T.sub,
  },
  termsLink: {
    color: T.link,
    textDecorationLine: 'underline',
  },

  priceTotalValue:  { fontSize: 17, fontWeight: '900', color: T.text },

  // ── CTA bar ────────────────────────────────────────────────────────────────
  ctaBar: {
    position: 'absolute', bottom: 0, left: 0, right: 0,
    backgroundColor: T.bg,
    paddingHorizontal: 20, paddingTop: 14,
    borderTopWidth: 1, borderTopColor: T.faintSurface,
    gap: 12,
    shadowColor: '#000', shadowOffset: { width: 0, height: -3 },
    shadowOpacity: 0.06, shadowRadius: 10, elevation: 8,
  },
  ctaTotalLabel:  { fontSize: 14, fontWeight: '800', color: T.text },
  payBtn: {
    backgroundColor: PRIMARY, paddingVertical: 16,
    borderRadius: 14, alignItems: 'center',
    shadowColor: PRIMARY, shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.25, shadowRadius: 8, elevation: 4,
  },
  payBtnDisabled: { opacity: 0.6 },
  payBtnText:     { fontSize: 16, fontWeight: '800', color: '#FFFFFF', letterSpacing: 0.3 },

  // ── Empty state ────────────────────────────────────────────────────────────
  emptyState:   { padding: 48, alignItems: 'center', gap: 8 },
  emptyIcon:    { fontSize: 36 },
  emptyText:    { fontSize: 13, color: T.faint, fontWeight: '600' },

  // ── Contact modal ──────────────────────────────────────────────────────────
  modalBackdrop:  { flex: 1, backgroundColor: 'rgba(0,0,0,0.45)' },
  modalSheet: {
    backgroundColor: T.sheet,
    borderTopLeftRadius: 20,
    borderTopRightRadius: 20,
    paddingBottom: 36,
    shadowColor: '#000',
    shadowOffset: { width: 0, height: -4 },
    shadowOpacity: 0.12,
    shadowRadius: 16,
    elevation: 20,
  },
  modalHandle: {
    width: 36, height: 4, borderRadius: 2,
    backgroundColor: T.border3,
    alignSelf: 'center',
    marginTop: 10, marginBottom: 4,
  },
  modalHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 20,
    paddingVertical: 14,
    borderBottomWidth: 1,
    borderBottomColor: T.faintSurface,
  },
  modalCloseBtn: {
    width: 32, height: 32, borderRadius: 16,
    backgroundColor: T.faintSurface,
    alignItems: 'center', justifyContent: 'center',
  },
  modalCloseText:   { fontSize: 13, fontWeight: '700', color: T.sub },
  modalTitle:       { fontSize: 17, fontWeight: '700', color: T.text },
  modalDoneBtn:     { paddingHorizontal: 4, paddingVertical: 4 },
  modalDoneText:    { fontSize: 16, fontWeight: '600', color: T.text },
  modalFieldWrap: {
    marginHorizontal: 16,
    marginTop: 16,
    borderWidth: 1.5,
    borderColor: T.text,
    borderRadius: 12,
    paddingHorizontal: 16,
    paddingVertical: 14,
  },
  modalInput:         { fontSize: 16, color: T.text, fontWeight: '400' },
  modalAccountRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginHorizontal: 16,
    marginTop: 20,
    paddingVertical: 4,
  },
  modalAccountText:   { fontSize: 16, fontWeight: '500', color: T.text },
  modalAccountChevron: { fontSize: 18, color: T.faint },
  modalFootnote: {
    fontSize: 13, color: T.sub, lineHeight: 19,
    marginHorizontal: 16, marginTop: 16, fontWeight: '400',
  },

  // ── CPA s49 risk acknowledgement ───────────────────────────────────────────
  // Styled to be conspicuous rather than to blend in: s49(1) requires the
  // consumer's attention to be DRAWN to terms of this kind. A control that looks
  // like the rest of the form is doing the opposite of what the section asks.
  //
  // Rules for anyone editing this: never default the checkbox to checked, and
  // never let the Pay button through while it is false. Pre-ticked consent is
  // not assent, and an ungated button makes the whole control decorative.
  riskBox: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    gap: 11,
    marginHorizontal: 20,
    marginBottom: 12,
    padding: 14,
    borderRadius: 14,
    borderWidth: 1.5,
    borderColor: T.border2,
    backgroundColor: T.trustSurface,
  },
  riskBoxChecked: { borderColor: T.trustIcon },
  riskBoxError:   { borderColor: PRIMARY },
  riskCheck: {
    width: 20,
    height: 20,
    borderRadius: 6,
    borderWidth: 1.6,
    borderColor: T.border3,
    alignItems: 'center',
    justifyContent: 'center',
    marginTop: 1,
  },
  riskCheckOn: { backgroundColor: PRIMARY, borderColor: PRIMARY },
  riskLabel: {
    flex: 1,
    fontSize: 12.5,
    lineHeight: 18,
    color: T.text,
  },
  riskError: {
    marginHorizontal: 20,
    marginTop: -6,
    marginBottom: 12,
    fontSize: 12,
    fontWeight: '600',
    color: PRIMARY,
  },

  // ── Payment method sheet ───────────────────────────────────────────────────
  // These were inline styles in purchase.tsx with a hardcoded '#fff' sheet and
  // '#f0f0f0' row dividers, while the LABELS inside used T.text. In dark mode
  // that is white text on a white sheet: the "Add credit card" row was rendering
  // the whole time, it just could not be seen. Hoisted here so the sheet is
  // themed from the same tokens as everything else on the screen.
  paySheetBackdrop: { flex: 1, backgroundColor: 'rgba(0,0,0,0.5)' },
  paySheet: {
    position: 'absolute', bottom: 0, left: 0, right: 0,
    backgroundColor: T.sheet,
    borderTopLeftRadius: 20, borderTopRightRadius: 20,
    paddingBottom: 40,
    // A dark sheet on a dark page needs a hairline to separate it from the
    // backdrop; a shadow alone does nothing there.
    borderTopWidth: 1, borderColor: T.border2,
    shadowColor: '#000', shadowOffset: { width: 0, height: -4 },
    shadowOpacity: 0.18, shadowRadius: 16, elevation: 20,
  },
  paySheetHandleWrap: { alignItems: 'center', paddingTop: 12, paddingBottom: 8 },
  paySheetHandle:     { width: 36, height: 4, borderRadius: 2, backgroundColor: T.border3 },
  paySheetTitle:      { fontSize: 17, fontWeight: '700', color: T.text, paddingHorizontal: 20, paddingBottom: 8 },
  payRow: {
    flexDirection: 'row', alignItems: 'center',
    paddingHorizontal: 20, paddingVertical: 16,
    borderTopWidth: 1, borderTopColor: T.border2,
  },
  payRowIcon: {
    width: 52, height: 34, borderRadius: 8,
    borderWidth: 1.5, borderColor: T.border3,
    alignItems: 'center', justifyContent: 'center', marginRight: 14,
  },
  payRowIconFilled: {
    width: 52, height: 34, borderRadius: 8,
    alignItems: 'center', justifyContent: 'center', marginRight: 14,
  },
  payRowLabel:    { flex: 1, fontSize: 16, fontWeight: '500', color: T.text },
  payRowLabelTop: { fontSize: 16, fontWeight: '500', color: T.text },
  payRowSub:      { fontSize: 12, color: T.faint, marginTop: 1 },

  // ── Guest / input (legacy, kept for compatibility) ─────────────────────────
  guestSection: {
    marginHorizontal: 20, marginTop: 16, marginBottom: 20,
    backgroundColor: T.infoBg, borderRadius: 14, padding: 18,
    borderWidth: 1, borderColor: T.infoBorder,
  },
  guestSectionTitle:  { fontSize: 14, fontWeight: '800', color: T.text, letterSpacing: -0.1, marginBottom: 4 },
  guestSectionSub:    { fontSize: 12, color: T.sub, fontWeight: '400', marginBottom: 16, lineHeight: 17 },
  inputGroup:         { marginBottom: 14 },
  inputLabel: {
    fontSize: 11, fontWeight: '700', color: T.subStrong,
    letterSpacing: 0.5, marginBottom: 6, textTransform: 'uppercase',
  },
  input: {
    backgroundColor: T.bg, borderWidth: 1.5, borderColor: T.border3,
    borderRadius: 10, paddingHorizontal: 14, paddingVertical: 12,
    fontSize: 14, fontWeight: '500', color: T.text,
  },

  // ── Unused stubs (kept to avoid TS errors if referenced) ──────────────────
  backToMapBtn:       { alignSelf: 'center', paddingHorizontal: 16, paddingVertical: 8, borderRadius: 20, borderWidth: 1, borderColor: T.border2 },
  backToMapText:      { fontSize: 12, fontWeight: '700', color: T.subStrong },
  editLink:           { alignSelf: 'center', paddingVertical: 14 },
  editLinkText:       { fontSize: 12, fontWeight: '700', color: T.sub },
  viewDetailsBtn:     { flexDirection: 'row', alignItems: 'center' },
  viewDetailsBtnText: { fontSize: 13, fontWeight: '700', color: T.text },
  viewDetailsChevron: { fontSize: 13, fontWeight: '700', color: T.text },
  ctaTotalRow:        { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: 2 },
});
};

// Backwards-compatible light sheet for any legacy imports.
export const sheet = makePurchaseSheet(false);