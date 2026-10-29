'use client';

import Link from 'next/link';
import { useAppTheme } from '@/themes/colors';

// ─────────────────────────────────────────────────────────────────────────────
// CPA Section 49 risk acknowledgement.
//
// ── Why this is a checkbox and not a line of text above the Pay button ───────
//
// Section 49 does not govern what the terms SAY, it governs how agreement to
// them is obtained. For terms that limit our liability, ask the consumer to
// assume risk, or acknowledge a fact, the consumer must have their attention
// drawn to the term in a conspicuous manner, and must be given an adequate
// opportunity to receive and comprehend it.
//
// Section 49(5) goes further for activities carrying a risk of an unusual
// nature, or one that could result in serious injury or death: the consumer
// must assent by signing, initialling, "or otherwise acting in a manner
// consistent with acknowledgement of the notice". Crowd crush, structural
// failure and stampede at a live music or sports event plausibly fall inside
// that. A single Pay button cannot be that act, because it simultaneously means
// "I want this ticket", "I accept your refund policy" and "I accept I might be
// seriously hurt" — an act that ambiguous is not referable to the risk notice.
//
// Hence one checkbox, tied to one clause, naming what is being acknowledged.
// NOT a checkbox for the whole terms document: general purchase terms are
// adequately covered by the notice already displayed above the button, and
// burying clause 5 inside a blanket "I agree to everything" tick would recreate
// exactly the problem this is meant to solve.
//
// ── Rules for anyone editing this ────────────────────────────────────────────
//
//   1. Never default `checked` to true. Pre-ticked consent is not assent.
//   2. Never let the Pay button proceed while it is false.
//   3. Keep the three risks NAMED in the label. "I accept the Terms" is not
//      referable to a specific notice; "I accept the risk of injury, the right
//      of admission, and age limits" is.
//   4. The link must open the terms without destroying the checkout state.
// ─────────────────────────────────────────────────────────────────────────────

export interface RiskAcknowledgementProps {
  checked: boolean;
  onChange: (next: boolean) => void;
  /** Set true after a failed submit attempt to draw the eye back here. */
  highlight?: boolean;
}

export default function RiskAcknowledgement({
  checked,
  onChange,
  highlight = false,
}: RiskAcknowledgementProps) {
  const { colors, isDark } = useAppTheme();

  const borderColor = highlight && !checked
    ? '#ff2d55'
    : checked
      ? (isDark ? 'rgba(52,211,153,0.45)' : 'rgba(22,163,74,0.4)')
      : colors.dividerColor;

  return (
    <div
      style={{
        margin: '0 0 16px',
        padding: 14,
        borderRadius: 14,
        border: `1.5px solid ${borderColor}`,
        background: colors.sectionCardBg,
        transition: 'border-color 140ms ease',
      }}
    >
      <label
        style={{
          display: 'flex',
          alignItems: 'flex-start',
          gap: 11,
          cursor: 'pointer',
        }}
      >
        <input
          type="checkbox"
          checked={checked}
          onChange={(e) => onChange(e.target.checked)}
          // A real checkbox, not a styled div: it gets keyboard focus, screen
          // reader semantics and the browser's own required-field behaviour for
          // free. "Adequate opportunity to receive and comprehend" is hard to
          // argue for a control a screen reader cannot announce.
          style={{
            width: 19,
            height: 19,
            marginTop: 1,
            flexShrink: 0,
            accentColor: '#ff2d55',
            cursor: 'pointer',
          }}
        />
        <span
          style={{
            fontSize: 13,
            lineHeight: 1.5,
            color: colors.text,
            opacity: 0.9,
          }}
        >
          I accept that attending a live event carries risks including personal injury, that
          the organiser and venue reserve the right of admission, and that age restrictions
          may apply — as set out in{' '}
          <Link
            href="/terms#risk-admission-and-liability"
            target="_blank"
            rel="noopener noreferrer"
            onClick={(e) => e.stopPropagation()}
            style={{ color: '#ff2d55', textDecoration: 'underline' }}
          >
            clause 5 of the Terms of Use
          </Link>
          .
        </span>
      </label>

      {highlight && !checked && (
        <p
          style={{
            margin: '9px 0 0 30px',
            fontSize: 12,
            fontWeight: 600,
            color: '#ff2d55',
          }}
        >
          Please confirm this before continuing.
        </p>
      )}
    </div>
  );
}