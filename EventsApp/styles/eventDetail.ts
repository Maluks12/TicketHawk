import { StyleSheet } from 'react-native';

// ─── className strings ────────────────────────────────────────────────────────
// Import as:  import { s, sheet } from '../styles/eventDetail';

export const s = {

  // ── Screen shell ─────────────────────────────────────────────────────────────
  screen:           'flex-1 bg-white',
  scroll:           'flex-1 bg-white',

  // ── Hero ─────────────────────────────────────────────────────────────────────
  hero:                 'relative h-[320px] bg-[#1a1a1a]',
  heroImagePlaceholder: 'absolute inset-0 bg-[#2a2a2a]',

  // Back button — glassmorphism lite, overlaid on dark hero
  backBtn:              'absolute top-14 left-4 z-10 w-10 h-10 rounded-[2px] bg-black/40 items-center justify-center',
  backIcon:             'text-white text-lg font-black',
  shareBtn:             'absolute top-14 right-4 z-10 w-10 h-10 rounded-[2px] bg-black/40 items-center justify-center',
  shareIcon:            'text-white text-base',

  // Badges overlaid on hero
  heroCategoryBadge:    'absolute top-14 left-1/2 -translate-x-1/2 bg-[#ff3c6e] px-3 py-1 rounded-[2px]',
  heroCategoryText:     'text-white text-[9px] font-extrabold tracking-[2px] uppercase',
  heroPriceBadge:       'absolute top-[70px] right-14 bg-black/60 rounded-[5px] px-2.5 py-1',
  heroPriceText:        'text-white text-[9px] font-bold',

  // Sold out ribbon
  soldOutBadge:         'absolute top-[70px] left-14 bg-black/70 rounded-[5px] px-2.5 py-1',
  soldOutText:          'text-[#ff3c6e] text-[9px] font-extrabold tracking-[1.5px]',

  // ── Image gallery ─────────────────────────────────────────────────────────────
  galleryWrap:          'py-4',
  galleryScroll:        'px-5 gap-3',
  galleryThumb:         'w-[192px] h-[110px] rounded-[10px] overflow-hidden bg-[#e5e5e5]',

  // ── Content area ─────────────────────────────────────────────────────────────
  content:              'px-5',

  // ── Title block ──────────────────────────────────────────────────────────────
  category:             'text-[10px] font-extrabold text-[#ff3c6e] tracking-[2px] uppercase mb-1.5',
  title:                'text-[32px] font-black text-black tracking-[1px] leading-[36px] mb-3',

  // ── Meta row ─────────────────────────────────────────────────────────────────
  metaRow:              'flex-row items-center gap-1.5 mb-1.5',
  metaIcon:             'text-[13px]',
  metaText:             'text-[12px] text-black/55 font-medium',
  metaTextBold:         'text-[12px] text-black/80 font-semibold',

  // ── Divider ──────────────────────────────────────────────────────────────────
  divider:              'h-[1px] bg-black/[0.07] my-5',

  // ── Primary CTA ──────────────────────────────────────────────────────────────
  ctaWrap:              'mb-4',
  btnPrimary:           'bg-[#ff3c6e] py-4 rounded-[2px] items-center',
  btnPrimaryText:       'text-white font-extrabold text-[11px] tracking-[2px]',
  btnSoldOut:           'bg-black/10 py-4 rounded-[2px] items-center',
  btnSoldOutText:       'text-black/30 font-extrabold text-[11px] tracking-[2px]',

  // ── Secondary actions ────────────────────────────────────────────────────────
  actionRow:            'flex-row gap-3 mb-1',
  btnOutline:           'flex-1 border border-black/20 py-3 rounded-[2px] flex-row items-center justify-center gap-1.5',
  btnOutlineText:       'text-black font-extrabold text-[10px] tracking-[1.5px]',
  btnOutlineIcon:       'text-[14px]',

  // ── Section header ───────────────────────────────────────────────────────────
  sectionHeader:        'flex-row items-center justify-between mb-3.5',
  sectionTitle:         'text-[13px] font-black text-black tracking-[2px]',
  sectionLink:          'text-[10px] font-extrabold text-[#ff3c6e] tracking-[1.5px]',

  // ── Description ──────────────────────────────────────────────────────────────
  descText:             'text-[13px] text-black/60 leading-[20px]',
  descReadMore:         'text-[11px] font-extrabold text-[#ff3c6e] tracking-[1px] mt-1.5',

  // ── Bento info grid ──────────────────────────────────────────────────────────
  bentoGrid:            'flex-row gap-3',
  bentoCard:            'flex-1 bg-[#f5f5f5] rounded-[10px] p-3.5',
  bentoIcon:            'text-[18px] mb-1.5',
  bentoLabel:           'text-[10px] font-extrabold text-black/35 tracking-[1.5px] uppercase mb-0.5',
  bentoValue:           'text-[15px] font-black text-black',
  bentoSub:             'text-[10px] text-black/40 mt-0.5',

  // ── Location block ───────────────────────────────────────────────────────────
  locationCard:         'bg-[#f5f5f5] rounded-[10px] p-4',
  locationName:         'text-[15px] font-black text-black mb-0.5',
  locationAddr:         'text-[12px] text-black/50',
  locationDirections:   'flex-row items-center gap-1 mt-3',
  locationDirText:      'text-[10px] font-extrabold text-[#ff3c6e] tracking-[1.5px]',
  locationDirIcon:      'text-[12px] text-[#ff3c6e]',

  // ── Availability bar ─────────────────────────────────────────────────────────
  availTrack:           'h-1.5 bg-black/[0.07] rounded-full mt-2.5 overflow-hidden',
  availFill:            'h-full bg-[#ff3c6e] rounded-full',

  // ── Participants (teams / performers) ───────────────────────────────────────
  participantRow:         'flex-row items-center gap-3.5 py-3.5 border-b border-black/[0.07]',
  participantRowLast:     'flex-row items-center gap-3.5 py-3.5',
  participantAvatar:      'w-10 h-10 rounded-full bg-[#f0f0f0] border border-black/10 items-center justify-center',
  participantAvatarText:  'text-[15px] font-black text-black/60',
  participantName:        'flex-1 text-[14px] font-semibold text-black',
  participantArrow:       'text-[20px] text-black/20',

  // ── Bottom padding ───────────────────────────────────────────────────────────
  bottomPad:            'h-28',
};

// ─── StyleSheet ────────────────────────────────────────────────────────────────
export const sheet = StyleSheet.create({
  absoluteFill:    { position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 },
  centerTranslate: { transform: [{ translateX: -50 }] },
  shadow: {
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.06,
    shadowRadius: 8,
    elevation: 3,
  },
});