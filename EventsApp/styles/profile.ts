import { StyleSheet } from 'react-native';

// ─── className strings ────────────────────────────────────────────────────────
// Import: import { s, sheet } from '../../styles/profile';

export const s = {

  // ── Shell ─────────────────────────────────────────────────────────────────
  root:               'flex-1 bg-dark-base',
  scroll:             'px-4 pt-1',

  // ── Header ────────────────────────────────────────────────────────────────
  header:             'px-6 pb-[18px]',
  headerTitle:        'text-[28px] font-extrabold text-white tracking-[-0.5px]',

  // ── Avatar ────────────────────────────────────────────────────────────────
  avatarSection:      'flex-row items-center gap-4 px-1 pb-1',
  avatarCircle:       'w-[60px] h-[60px] rounded-full bg-brand-pink items-center justify-center',
  avatarInitials:     'text-[22px] font-extrabold text-white tracking-widest',
  avatarName:         'text-[18px] font-extrabold text-white tracking-[-0.2px]',
  avatarEmail:        'text-[13px] text-white/40 mt-0.5',

  // ── Sign-in prompt ────────────────────────────────────────────────────────
  signInRow:          'flex-row items-center gap-3.5 p-4',
  signInAvatar:       'w-[50px] h-[50px] rounded-full bg-white/[0.07] items-center justify-center',
  signInTitle:        'text-[16px] font-bold text-white',
  signInSub:          'text-[12px] text-white/35 mt-0.5',

  // ── Section ───────────────────────────────────────────────────────────────
  section:            'gap-2',
  sectionTitle:       'text-[10px] font-extrabold tracking-[2.5px] text-white/28 ml-1',

  // ── Row ───────────────────────────────────────────────────────────────────
  row:                'flex-row items-center justify-between px-4 py-[15px]',
  rowLeft:            'flex-row items-center gap-[13px] flex-1',
  rowIconWrap:        'w-[22px] items-center',
  rowLabel:           'text-[15px] font-medium text-white',
  rowLabelDestructive: 'text-status-cancelled',
  rowRight:           'flex-row items-center gap-1.5',
  rowValue:           'text-[13px] text-white/40',

  // ── Misc ──────────────────────────────────────────────────────────────────
  version:            'text-center text-[12px] text-white/[0.18] mt-1',
};

// ─── StyleSheet ───────────────────────────────────────────────────────────────
export const sheet = StyleSheet.create({
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
});