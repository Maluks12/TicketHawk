import { StyleSheet, Dimensions, Platform } from 'react-native';

const { height: SCREEN_H } = Dimensions.get('window');

// ─── Design tokens (DESIGN.md) ────────────────────────────────────────────────

export const C = {
  primary:              '#ba0034',
  primaryContainer:     '#e51245',
  cta:                  '#FF2D55',
  surfaceContainer:     '#ffe9e9',
  surface:              '#fff8f7',
  onSurface:            '#281718',
  onSurfaceVariant:     '#5d3f40',
  outlineVariant:       '#e6bcbd',
  outline:              '#916e6f',
};

// ~60 vh
export const HERO_H = Math.round(SCREEN_H * 0.58);

// ─── className strings ────────────────────────────────────────────────────────

export const s = {

  screen:   'flex-1 bg-[#fff8f7]',
  scroll:   'flex-1',

  // ── Hero ─────────────────────────────────────────────────────────────────────
  hero:                 'relative bg-[#1a1a1a]',
  heroImagePlaceholder: 'absolute inset-0 bg-[#2a2a2a]',
  backBtn:  'absolute top-14 left-5 z-10 w-12 h-12 rounded-full bg-white/20 items-center justify-center',
  backIcon: 'text-white text-[22px] font-black leading-none',
  shareBtn: 'absolute top-14 right-5 z-10 w-12 h-12 rounded-full bg-white/20 items-center justify-center',
  shareIcon:'text-white text-base font-bold',
  soldOutBadge: 'absolute top-16 left-1/2 bg-black/60 rounded-full px-4 py-1',
  soldOutText:  'text-[#FF2D55] text-[9px] font-extrabold tracking-[2px]',

  // ── Metadata items inside card ────────────────────────────────────────────────
  metaItemRow:       'flex-row items-center gap-4 mb-5',
  metaIconBox:       'w-12 h-12 rounded-xl bg-[#ffe9e9] items-center justify-center',
  metaIconText:      'text-[22px]',
  metaLabelText:     'text-[10px] font-bold tracking-[0.1em] uppercase text-[#5d3f40] mb-0.5',
  metaValueText:     'text-[16px] font-bold text-[#281718] leading-snug',

  // ── CTA ───────────────────────────────────────────────────────────────────────
  ctaWrap:           'mt-1',
  btnPrimary:        'w-full bg-[#FF2D55] py-5 rounded-xl items-center mb-3',
  btnPrimaryText:    'text-white text-[18px] font-extrabold tracking-[0.07em] uppercase',
  btnSoldOut:        'w-full bg-black/10 py-5 rounded-xl items-center mb-3',
  btnSoldOutText:    'text-black/30 text-[18px] font-extrabold tracking-[0.07em] uppercase',
  actionRow:         'flex-row gap-3',
  btnOutline:        'flex-1 border border-[#1c2e61] bg-[#1c2e61] py-4 rounded-xl flex-row items-center justify-center gap-1.5',
  btnOutlineText:    'text-white text-[12px] font-bold tracking-[0.1em] uppercase',
  btnOutlineIcon:    'text-[16px]',

  // ── Sections ─────────────────────────────────────────────────────────────────
  content:           'px-5',
  sectionHeader:     'flex-row items-center justify-between mb-4',
  sectionTitle:      'text-[18px] font-extrabold text-[#281718] tracking-[0.05em] uppercase',
  sectionLink:       'text-[10px] font-bold text-[#ba0034] tracking-[0.1em]',
  divider:           'h-[1px] bg-[#e6bcbd] my-6',

  // ── About ─────────────────────────────────────────────────────────────────────
  descText:          'text-[15px] text-[#5d3f40] leading-[24px]',
  descReadMore:      'text-[12px] font-bold text-[#ba0034] tracking-[0.1em] mt-3 border-b-2 border-[#ba0034] self-start pb-0.5',

  // ── Bento ─────────────────────────────────────────────────────────────────────
  bentoGrid:         'flex-row gap-3',
  bentoCard:         'flex-1 bg-[#ffe9e9] rounded-xl p-5 border border-[#e6bcbd]/30',
  bentoIcon:         'text-[24px] mb-2',
  bentoLabel:        'text-[10px] font-bold text-[#5d3f40] tracking-[0.1em] uppercase mb-1',
  bentoValue:        'text-[16px] font-bold text-[#281718] leading-snug',
  bentoSub:          'text-[11px] text-[#5d3f40] mt-0.5',

  // ── Location ─────────────────────────────────────────────────────────────────
  locationCard:            'rounded-xl overflow-hidden border border-[#e6bcbd]/50 bg-white',
  locationMapPlaceholder:  'h-48 bg-[#ffe1e1] items-center justify-center',
  locationMapPin:          'w-10 h-10 bg-[#ba0034] rounded-full items-center justify-center',
  locationMapPinText:      'text-white text-[22px]',
  locationInfoRow:         'p-5 flex-row items-start justify-between',
  locationInfoLeft:        'flex-1 mr-3',
  locationName:            'text-[16px] font-bold text-[#281718] mb-0.5',
  locationAddr:            'text-[13px] text-[#5d3f40]',
  locationBtnRow:          'flex-col gap-2',
  locationBtnFilled:       'bg-[#ff2d55] px-4 py-2.5 rounded-lg items-center',
  locationBtnFilledText:   'text-[11px] font-bold text-white tracking-[0.1em] uppercase',
  locationBtnOutline: 'bg-[#1c2e61] border border-[#1c2e61] px-4 py-2.5 rounded-lg items-center',
  locationBtnOutlineText: 'text-[11px] font-bold text-white tracking-[0.1em] uppercase',

  // ── Participants ──────────────────────────────────────────────────────────────
  participantRow:        'flex-row items-center gap-3.5 py-3.5 border-b border-[#e6bcbd]',
  participantRowLast:    'flex-row items-center gap-3.5 py-3.5',
  participantAvatar:     'w-10 h-10 rounded-full bg-[#ffe9e9] border border-[#e6bcbd] items-center justify-center',
  participantAvatarText: 'text-[15px] font-black text-[#ba0034]',
  participantName:       'flex-1 text-[14px] font-semibold text-[#281718]',
  participantArrow:      'text-[20px] text-[#e6bcbd]',

  // ── Organizer ─────────────────────────────────────────────────────────────────
  organizerCard:        'flex-row items-center justify-between p-5 bg-white border border-[#e6bcbd]/50 rounded-xl',
  organizerLeft:        'flex-row items-center gap-4 flex-1',
  organizerAvatar:      'w-14 h-14 rounded-full bg-[#ffe9e9] border border-[#e6bcbd] items-center justify-center overflow-hidden',
  organizerAvatarText:  'text-[22px] font-black text-[#ba0034]',
  organizerByLabel:     'text-[10px] font-bold tracking-[0.1em] uppercase text-[#5d3f40] mb-0.5',
  organizerName:        'text-[15px] font-bold text-[#281718] uppercase',

  bottomPad:            'h-32',

  // ── Legacy (kept for other screens that may import) ──────────────────────────
  metaRow:              'flex-row items-center gap-1.5 mb-1.5',
  metaIcon:             'text-[13px]',
  metaText:             'text-[12px] text-black/55 font-medium',
  metaTextBold:         'text-[12px] text-black/80 font-semibold',
  galleryWrap:          'py-4',
  galleryScroll:        'px-5 gap-3',
  galleryThumb:         'w-[192px] h-[110px] rounded-[10px] overflow-hidden bg-[#ffe9e9]',
  availTrack:           'h-1.5 bg-black/[0.07] rounded-full mt-2.5 overflow-hidden',
  availFill:            'h-full bg-[#FF2D55] rounded-full',
};

// ─── StyleSheet ───────────────────────────────────────────────────────────────

const PARALLAX_EXTRA = Math.round(HERO_H * 0.35);

export const sheet = StyleSheet.create({
  absoluteFill: { position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 },

  // ── Parallax hero ─────────────────────────────────────────────────────────
  // Outer clip container — exactly HERO_H tall, overflow:hidden so the
  // oversized image is clipped as translateY pulls it upward on scroll
  heroContainer: {
    height: HERO_H,
    overflow: 'hidden',
    backgroundColor: '#1a1a1a',
  },

  // Image is taller than the container by PARALLAX_EXTRA so no gap appears
  // Image is taller than container by PARALLAX_EXTRA — no gap when translating up
  heroImage: {
    position: "absolute",
    top: 0,
    left: 0,
    right: 0,
    bottom: -PARALLAX_EXTRA,
  } as any,

  // Fixed nav overlay — pointerEvents="box-none" on the View so touches pass through
  navOverlay: {
    position: "absolute",
    top: 0,
    left: 0,
    right: 0,
    zIndex: 100,
    paddingTop: Platform.OS === "ios" ? 52 : 40,
    paddingHorizontal: 20,
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  } as any,

  // Both nav buttons use the same static semi-transparent style
  navBtn: {
    width: 48,
    height: 48,
    borderRadius: 24,
    backgroundColor: "rgba(0,0,0,0.30)",
    alignItems: "center",
    justifyContent: "center",
  },

  // Kept as alias so no import errors if referenced elsewhere
  navBtnRight: {
    width: 48,
    height: 48,
    borderRadius: 24,
    backgroundColor: "rgba(0,0,0,0.30)",
    alignItems: "center",
    justifyContent: "center",
  },

  // ── Hero title block ──────────────────────────────────────────────────────
  heroTitleBlock: {
    position: 'absolute',
    bottom: 36,
    left: 20,
    right: 20,
  },

  trendingBadge: {
    backgroundColor: '#e51245',
    paddingHorizontal: 12,
    paddingVertical: 4,
    borderRadius: 9999,
  },
  trendingBadgeText: {
    color: '#fff',
    fontSize: 11,
    fontWeight: '700',
    letterSpacing: 1.5,
  },
  viewsText: {
    color: 'rgba(255,255,255,0.85)',
    fontSize: 13,
    fontWeight: '500',
  },
  heroCategory: {
    color: '#FF2D55',
    fontSize: 11,
    fontWeight: '800',
    letterSpacing: 2,
    textTransform: 'uppercase',
    marginBottom: 6,
  },
  heroTitle: {
    color: '#ffffff',
    fontSize: 32,
    fontWeight: '800',
    letterSpacing: -0.5,
    lineHeight: 36,
    textTransform: 'uppercase',
  },

  // ── Floating action card ──────────────────────────────────────────────────
  actionCard: {
    backgroundColor: '#ffffff',
    borderRadius: 16,
    marginHorizontal: 16,
    marginTop: -28,
    padding: 24,
    shadowColor: '#281718',
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.10,
    shadowRadius: 16,
    elevation: 6,
    borderWidth: 1,
    borderColor: 'rgba(230,188,189,0.3)',
    zIndex: 10,
  },

  metaDivider: {
    height: 1,
    backgroundColor: 'rgba(230,188,189,0.5)',
    marginBottom: 20,
  },

  soldOutBadge: {
    position: 'absolute',
    top: 64,
    alignSelf: 'center',
    backgroundColor: 'rgba(0,0,0,0.6)',
    borderRadius: 9999,
    paddingHorizontal: 16,
    paddingVertical: 4,
  },
  soldOutText: {
    color: '#FF2D55',
    fontSize: 9,
    fontWeight: '800',
    letterSpacing: 2,
  },

  // Kept for back-compat
  hero: { height: HERO_H, backgroundColor: '#1a1a1a' },
  centerTranslate: { transform: [{ translateX: -50 }] },
  shadow: {
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.06,
    shadowRadius: 8,
    elevation: 3,
  },
});