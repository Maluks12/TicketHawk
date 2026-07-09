import { StyleSheet, Dimensions } from 'react-native';

// ─── Import ───────────────────────────────────────────────────────────────────
// import { s, sheet, PRIMARY } from '../../styles/purchase';
// Use:  <View style={sheet.header} />  or  <Text style={sheet.eventTitle} />

const { width: SW } = Dimensions.get('window');

export const PRIMARY = '#FF2D55';

// ─── StyleSheet ───────────────────────────────────────────────────────────────

export const sheet = StyleSheet.create({

  // ── Shell ──────────────────────────────────────────────────────────────────
  screen:       { flex: 1, backgroundColor: '#ffffff' },
  centered:     { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 12 },
  scroll:       { flex: 1 },
  errorText:    { fontSize: 13, fontWeight: '800', color: '#333', letterSpacing: 1 },
  goBackBtn:    { borderWidth: 1, borderColor: '#ddd', paddingHorizontal: 16, paddingVertical: 8, borderRadius: 20 },
  goBackText:   { fontSize: 12, fontWeight: '700', color: '#333' },
  loadingText:  { fontSize: 12, color: '#999', marginTop: 8, fontWeight: '600' },

  // ── Header ─────────────────────────────────────────────────────────────────
  header: {
    backgroundColor: '#ffffff',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 20,
    paddingBottom: 12,
    borderBottomWidth: 1,
    borderBottomColor: '#f3f4f6',
  },
  // 40px tap target; glyph is optically centered (lineHeight == fontSize kills
  // the baseline offset that pushed the old 26/32 chevron low and left).
  backBtn: {
    width: 40, height: 40, borderRadius: 20,
    backgroundColor: '#f3f4f6',
    alignItems: 'center', justifyContent: 'center',
  },
  backIcon:     { fontSize: 24, fontWeight: '400', color: '#222', lineHeight: 24, textAlign: 'center', marginTop: -2 },
  closeBtn: {
    width: 32, height: 32, borderRadius: 16,
    backgroundColor: '#f3f4f6',
    alignItems: 'center', justifyContent: 'center',
  },
  closeIcon:    { fontSize: 13, fontWeight: '700', color: '#222' },
  headerTitle:  { fontSize: 16, fontWeight: '800', color: '#111111', letterSpacing: -0.2 },
  stepDots:     { flexDirection: 'row', gap: 5, alignItems: 'center' },
  stepDot:      { width: 6, height: 6, borderRadius: 3, backgroundColor: '#e5e7eb' },
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
    borderBottomWidth: 1, borderBottomColor: '#f3f4f6',
  },
  eventTitle:       { fontSize: 22, fontWeight: '800', color: '#111111', letterSpacing: -0.4, marginBottom: 4 },
  eventSubtitle:    { fontSize: 14, fontWeight: '500', color: '#6b7280' },
  eventVenueSmall:  { fontSize: 13, fontWeight: '400', color: '#9ca3af', marginTop: 2 },

  // ── Section header ─────────────────────────────────────────────────────────
  sectionHeader: {
    flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between',
    paddingHorizontal: 20, paddingTop: 16, paddingBottom: 10,
  },
  sectionHeaderText: { fontSize: 11, fontWeight: '800', color: '#9ca3af', letterSpacing: 1.2 },
  sortChip:          { backgroundColor: '#f3f4f6', borderRadius: 20, paddingHorizontal: 10, paddingVertical: 4 },
  sortChipText:      { fontSize: 11, fontWeight: '600', color: '#374151' },

  // ── Ticket cards ───────────────────────────────────────────────────────────
  ticketCard: {
    flexDirection: 'row', alignItems: 'center',
    backgroundColor: '#ffffff',
    borderBottomWidth: 1, borderBottomColor: '#f3f4f6',
    paddingHorizontal: 20, paddingVertical: 16, gap: 14,
  },
  ticketCardSelected: {
    backgroundColor: '#fff5f7',
    borderLeftWidth: 3, borderLeftColor: PRIMARY, paddingLeft: 17,
  },
  ticketCardSoldOut: { opacity: 0.45 },
  radioOuter: {
    width: 20, height: 20, borderRadius: 10,
    borderWidth: 2, borderColor: '#d1d5db',
    alignItems: 'center', justifyContent: 'center', flexShrink: 0,
  },
  radioOuterSelected: { borderColor: PRIMARY },
  radioInner:         { width: 10, height: 10, borderRadius: 5, backgroundColor: PRIMARY },
  ticketInfo:         { flex: 1, gap: 4 },
  ticketNameRow:      { flexDirection: 'row', alignItems: 'center', gap: 6 },
  ticketName:         { fontSize: 15, fontWeight: '700', color: '#111111', letterSpacing: -0.1 },
  ticketNameSoldOut:  { color: '#9ca3af' },
  ticketAvail:        { fontSize: 12, color: '#9ca3af', fontWeight: '500' },
  vipBadge:           { backgroundColor: '#fef3c7', borderRadius: 4, paddingHorizontal: 5, paddingVertical: 1 },
  vipBadgeText:       { fontSize: 10, fontWeight: '700', color: '#92400e' },
  dealBadge:          { backgroundColor: '#f0fdf4', borderRadius: 4, paddingHorizontal: 6, paddingVertical: 2, alignSelf: 'flex-start' },
  dealBadgeText:      { fontSize: 10, fontWeight: '700', color: '#15803d' },
  soldBadge:          { backgroundColor: '#fff1f2', borderRadius: 4, paddingHorizontal: 6, paddingVertical: 2, alignSelf: 'flex-start' },
  soldBadgeText:      { fontSize: 10, fontWeight: '700', color: '#be123c' },
  ticketPriceCol:     { alignItems: 'flex-end', gap: 2 },
  ticketPrice:        { fontSize: 17, fontWeight: '900', color: '#111111' },
  ticketPriceSoldOut: { color: '#d1d5db', textDecorationLine: 'line-through' },
  ticketFeeNote:      { fontSize: 10, color: '#9ca3af', fontWeight: '500' },

  // ── Quantity picker ────────────────────────────────────────────────────────
  qtySection: {
    backgroundColor: '#fafafa', marginTop: 8,
    paddingHorizontal: 20, paddingTop: 20, paddingBottom: 24,
    borderTopWidth: 1, borderTopColor: '#f0f0f0',
  },
  qtySectionHeader:   { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 },
  qtySectionTitle:    { fontSize: 14, fontWeight: '700', color: '#111111' },
  seatedNote:         { fontSize: 11, color: '#9ca3af', fontWeight: '500' },
  qtyGrid:            { flexDirection: 'row', flexWrap: 'wrap', gap: 10 },
  qtyBtn: {
    flex: 1, minWidth: (SW - 60) / 3 - 8, paddingVertical: 16,
    borderRadius: 12, backgroundColor: '#f3f4f6',
    borderWidth: 1.5, borderColor: 'transparent', alignItems: 'center',
  },
  qtyBtnSel:          { backgroundColor: '#ffffff', borderColor: PRIMARY },
  qtyBtnText:         { fontSize: 17, fontWeight: '800', color: '#9ca3af' },
  qtyBtnTextSel:      { color: PRIMARY },

  // ── Seating map ────────────────────────────────────────────────────────────
  mapSubtitle:  { fontSize: 12, color: '#9ca3af', fontWeight: '500', paddingHorizontal: 20, marginBottom: 10 },
  legendRow:    { backgroundColor: '#fff', paddingHorizontal: 20, paddingVertical: 10, borderBottomWidth: 1, borderBottomColor: '#f0f0f0' },
  mapWrap:      { backgroundColor: '#fff', paddingHorizontal: 20, paddingTop: 16, paddingBottom: 4 },
  tierCard: {
    flexDirection: 'row', alignItems: 'center',
    marginHorizontal: 20, marginTop: 12, padding: 16,
    backgroundColor: '#fff5f7', borderRadius: 12,
    borderWidth: 1, borderColor: '#fecdd3', gap: 12,
  },
  tierCardLeft:   { flex: 1 },
  tierCardName:   { fontSize: 13, fontWeight: '900', color: '#111111', letterSpacing: 0.4 },
  tierCardMeta:   { fontSize: 11, color: '#9ca3af', fontWeight: '500', marginTop: 2 },
  tierCardTotal:  { fontSize: 18, fontWeight: '900', color: PRIMARY },

  // ── Checkout summary ───────────────────────────────────────────────────────
  checkoutWrap: { paddingBottom: 8 },
  // No top border: eventTitleSection above already draws a bottom hairline,
  // so a top border here rendered as a double line above "Contact".
  detailsList: {
    borderBottomWidth: 1, borderColor: '#f3f4f6',
    marginHorizontal: 20, marginTop: 4, marginBottom: 24,
  },
  detailRow: {
    flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between',
    paddingVertical: 16, borderBottomWidth: 1, borderBottomColor: '#f3f4f6',
  },
  detailRowMultiline: {
    flexDirection: 'row', alignItems: 'flex-start',
    paddingVertical: 16, borderBottomWidth: 1, borderBottomColor: '#f3f4f6',
  },
  detailLabel:    { width: 80, fontSize: 15, fontWeight: '700', color: '#111111', flexShrink: 0 },
  detailRight:    { flex: 1, flexDirection: 'row', alignItems: 'center', justifyContent: 'flex-end', gap: 6, marginLeft: 12 },
  detailValue:    { fontSize: 14, color: '#6b7280', fontWeight: '500' },
  detailValueSub: { fontSize: 12, color: '#9ca3af', marginTop: 2, fontWeight: '400' },
  chevron:        { fontSize: 18, color: '#d1d5db', fontWeight: '300', lineHeight: 20 },

  // ── Trust badges ───────────────────────────────────────────────────────────
  trustSection: { paddingHorizontal: 20, marginBottom: 24, gap: 14 },
  trustRow:     { flexDirection: 'row', alignItems: 'center', gap: 12 },
  trustIcon:    { fontSize: 18, width: 24, textAlign: 'center' },
  trustText:    { flex: 1 },
  trustTitle:   { fontSize: 13, fontWeight: '700', color: '#111111' },
  trustBody:    { fontSize: 12, color: '#9ca3af', marginTop: 1 },
  trustHelp:    { fontSize: 16, color: '#d1d5db' },

  // ── Price breakdown ────────────────────────────────────────────────────────
  priceBreakdown: {
    backgroundColor: '#fafafa', borderRadius: 14,
    marginHorizontal: 20, padding: 18, marginBottom: 16, gap: 10,
  },
  priceRow:         { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  priceLabel:       { fontSize: 13, color: '#6b7280', fontWeight: '500' },
  priceValue:       { fontSize: 13, color: '#6b7280', fontWeight: '600' },
  priceLabelLight:  { fontSize: 13, color: '#9ca3af', fontWeight: '400' },
  priceValueLight:  { fontSize: 13, color: '#9ca3af', fontWeight: '500' },
  priceDivider:     { height: 1, backgroundColor: '#e5e7eb', marginVertical: 2 },
  priceTotalLabel:  { fontSize: 15, fontWeight: '800', color: '#111111' },
  priceTotalValue:  { fontSize: 17, fontWeight: '900', color: '#111111' },

  // ── CTA bar ────────────────────────────────────────────────────────────────
  ctaBar: {
    position: 'absolute', bottom: 0, left: 0, right: 0,
    backgroundColor: '#ffffff',
    paddingHorizontal: 20, paddingTop: 14,
    borderTopWidth: 1, borderTopColor: '#f3f4f6',
    gap: 12,
    shadowColor: '#000', shadowOffset: { width: 0, height: -3 },
    shadowOpacity: 0.06, shadowRadius: 10, elevation: 8,
  },
  ctaTotalLabel:  { fontSize: 14, fontWeight: '800', color: '#111111' },
  payBtn: {
    backgroundColor: PRIMARY, paddingVertical: 16,
    borderRadius: 14, alignItems: 'center',
    shadowColor: PRIMARY, shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.25, shadowRadius: 8, elevation: 4,
  },
  payBtnDisabled: { opacity: 0.6 },
  payBtnText:     { fontSize: 16, fontWeight: '800', color: '#ffffff', letterSpacing: 0.3 },

  // ── Empty state ────────────────────────────────────────────────────────────
  emptyState:   { padding: 48, alignItems: 'center', gap: 8 },
  emptyIcon:    { fontSize: 36 },
  emptyText:    { fontSize: 13, color: '#9ca3af', fontWeight: '600' },

  // ── Contact modal ──────────────────────────────────────────────────────────
  modalBackdrop:  { flex: 1, backgroundColor: 'rgba(0,0,0,0.45)' },
  modalSheet: {
    backgroundColor: '#ffffff',
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
    backgroundColor: '#d1d5db',
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
    borderBottomColor: '#f0f0f0',
  },
  modalCloseBtn: {
    width: 32, height: 32, borderRadius: 16,
    backgroundColor: '#f3f4f6',
    alignItems: 'center', justifyContent: 'center',
  },
  modalCloseText:   { fontSize: 13, fontWeight: '700', color: '#555' },
  modalTitle:       { fontSize: 17, fontWeight: '700', color: '#111111' },
  modalDoneBtn:     { paddingHorizontal: 4, paddingVertical: 4 },
  modalDoneText:    { fontSize: 16, fontWeight: '600', color: '#111111' },
  modalFieldWrap: {
    marginHorizontal: 16,
    marginTop: 16,
    borderWidth: 1.5,
    borderColor: '#111111',
    borderRadius: 12,
    paddingHorizontal: 16,
    paddingVertical: 14,
  },
  modalInput:         { fontSize: 16, color: '#111111', fontWeight: '400' },
  modalAccountRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginHorizontal: 16,
    marginTop: 20,
    paddingVertical: 4,
  },
  modalAccountText:   { fontSize: 16, fontWeight: '500', color: '#111111' },
  modalAccountChevron: { fontSize: 18, color: '#9ca3af' },
  modalFootnote: {
    fontSize: 13, color: '#6b7280', lineHeight: 19,
    marginHorizontal: 16, marginTop: 16, fontWeight: '400',
  },

  // ── Guest / input (legacy, kept for compatibility) ─────────────────────────
  guestSection: {
    marginHorizontal: 20, marginTop: 16, marginBottom: 20,
    backgroundColor: '#f8faff', borderRadius: 14, padding: 18,
    borderWidth: 1, borderColor: '#e0eaff',
  },
  guestSectionTitle:  { fontSize: 14, fontWeight: '800', color: '#111111', letterSpacing: -0.1, marginBottom: 4 },
  guestSectionSub:    { fontSize: 12, color: '#6b7280', fontWeight: '400', marginBottom: 16, lineHeight: 17 },
  inputGroup:         { marginBottom: 14 },
  inputLabel: {
    fontSize: 11, fontWeight: '700', color: '#374151',
    letterSpacing: 0.5, marginBottom: 6, textTransform: 'uppercase',
  },
  input: {
    backgroundColor: '#ffffff', borderWidth: 1.5, borderColor: '#d1d5db',
    borderRadius: 10, paddingHorizontal: 14, paddingVertical: 12,
    fontSize: 14, fontWeight: '500', color: '#111111',
  },

  // ── Unused stubs (kept to avoid TS errors if referenced) ──────────────────
  backToMapBtn:       { alignSelf: 'center', paddingHorizontal: 16, paddingVertical: 8, borderRadius: 20, borderWidth: 1, borderColor: '#e5e7eb' },
  backToMapText:      { fontSize: 12, fontWeight: '700', color: '#374151' },
  editLink:           { alignSelf: 'center', paddingVertical: 14 },
  editLinkText:       { fontSize: 12, fontWeight: '700', color: '#6b7280' },
  viewDetailsBtn:     { flexDirection: 'row', alignItems: 'center' },
  viewDetailsBtnText: { fontSize: 13, fontWeight: '700', color: '#111111' },
  viewDetailsChevron: { fontSize: 13, fontWeight: '700', color: '#111111' },
  ctaTotalRow:        { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: 2 },
});