'use client';

import Link from 'next/link';
import { useAppTheme } from '@/themes/colors';
import { LAST_UPDATED } from '@/lib/legal';

// ─────────────────────────────────────────────────────────────────────────────
// Legal document renderer.
//
// One component for Terms, Privacy and Legal so the three pages cannot drift
// apart typographically, and so a change to how (say) a bullet list is spaced
// happens once. Content lives in the page files; this file owns only layout.
//
// Measure is capped at ~68ch. Legal text is the longest continuous prose on the
// site and full-width lines on a desktop monitor are genuinely hard to track
// back from the end of one line to the start of the next.
// ─────────────────────────────────────────────────────────────────────────────

export type Block =
  | { kind: 'p'; text: string }
  | { kind: 'list'; items: string[] }
  | { kind: 'table'; head: [string, string]; rows: [string, string][] }
  // A bordered, visually distinct panel. Not decoration: CPA Section 49
  // requires terms covering risk, admission and limitation of liability to be
  // drawn to the consumer's attention conspicuously, not buried in body copy.
  | { kind: 'callout'; title: string; blocks: Block[] };

export interface Section {
  heading: string;
  blocks: Block[];
}

export interface LegalDocProps {
  title: string;
  intro?: string;
  sections: Section[];
  /** Cross-links shown at the foot of the document. */
  seeAlso?: { href: string; label: string }[];
}

/**
 * Turns a section heading into a URL-safe anchor id, so a section can be
 * deep-linked as e.g. /legal#organizer-agreement rather than only reachable by
 * landing on the page and scrolling.
 *
 * "1. Organizer Agreement" -> "organizer-agreement": the leading "N. " section
 * number is presentation, not part of the section's identity, and stripping it
 * is what lets events-organizer's lib/legalLinks.ts build the exact fragment
 * without having to hand-maintain a parallel slug list that would drift the
 * first time a heading's wording changed.
 */
function slugify(heading: string): string {
  return heading
    .replace(/^\d+\.\s*/, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

/**
 * Renders **bold** spans inline.
 *
 * The source text uses bold heavily for defined terms and for the emphasis CPA
 * s49 expects, so it has to survive into the page. A tiny split-on-delimiter
 * pass is deliberate — pulling in a markdown renderer to bold four words would
 * mean shipping a parser that can also emit arbitrary HTML from a content file.
 */
function Rich({ text }: { text: string }) {
  const parts = text.split(/(\*\*[^*]+\*\*)/g);
  return (
    <>
      {parts.map((part, i) =>
        part.startsWith('**') && part.endsWith('**') ? (
          <strong key={i} style={{ fontWeight: 700 }}>{part.slice(2, -2)}</strong>
        ) : (
          <span key={i}>{part}</span>
        ),
      )}
    </>
  );
}

function Blocks({ blocks, depth = 0 }: { blocks: Block[]; depth?: number }) {
  const { colors } = useAppTheme();

  return (
    <>
      {blocks.map((block, i) => {
        switch (block.kind) {
          case 'p':
            return (
              <p
                key={i}
                className="text-[14.5px] leading-[1.7] mb-4"
                style={{ color: colors.text, opacity: 0.85 }}
              >
                <Rich text={block.text} />
              </p>
            );

          case 'list':
            return (
              <ul key={i} className="mb-4 pl-1 space-y-2">
                {block.items.map((item, j) => (
                  <li
                    key={j}
                    className="text-[14.5px] leading-[1.7] pl-4 relative"
                    style={{ color: colors.text, opacity: 0.85 }}
                  >
                    <span
                      className="absolute left-0 top-[0.7em] w-[4px] h-[4px] rounded-full"
                      style={{ backgroundColor: '#ff2d55' }}
                    />
                    <Rich text={item} />
                  </li>
                ))}
              </ul>
            );

          case 'table':
            return (
              <div
                key={i}
                className="mb-5 rounded-xl overflow-hidden border"
                style={{ borderColor: colors.dividerColor }}
              >
                <div
                  className="grid grid-cols-[1.6fr_1fr] text-[11px] font-bold uppercase tracking-[0.08em] px-4 py-2.5"
                  style={{ backgroundColor: colors.sectionCardBg, color: colors.subText }}
                >
                  <span>{block.head[0]}</span>
                  <span className="text-right">{block.head[1]}</span>
                </div>
                {block.rows.map((row, j) => (
                  <div
                    key={j}
                    className="grid grid-cols-[1.6fr_1fr] text-[13.5px] px-4 py-3 border-t"
                    style={{ borderColor: colors.dividerColor, color: colors.text }}
                  >
                    <span style={{ opacity: 0.85 }}>{row[0]}</span>
                    <span className="text-right font-semibold">{row[1]}</span>
                  </div>
                ))}
              </div>
            );

          case 'callout':
            return (
              <div
                key={i}
                className="mb-5 rounded-xl border-2 p-5"
                style={{ borderColor: '#ff2d55', backgroundColor: colors.sectionCardBg }}
              >
                <p
                  className="text-[12px] font-extrabold uppercase tracking-[0.1em] mb-3"
                  style={{ color: '#ff2d55' }}
                >
                  {block.title}
                </p>
                <Blocks blocks={block.blocks} depth={depth + 1} />
              </div>
            );
        }
      })}
    </>
  );
}

export default function LegalDoc({ title, intro, sections, seeAlso }: LegalDocProps) {
  const { colors } = useAppTheme();

  return (
    <main
      className="w-full min-h-screen px-5 py-10 sm:px-8 sm:py-14"
      style={{ backgroundColor: colors.background }}
    >
      <article className="mx-auto" style={{ maxWidth: '68ch' }}>
        <Link
          href="/"
          className="text-[12px] font-bold uppercase tracking-[0.1em] no-underline"
          style={{ color: '#ff2d55' }}
        >
          ← Back to TicketHawk
        </Link>

        <h1
          className="text-[30px] sm:text-[36px] font-extrabold tracking-[-0.02em] mt-6 mb-2"
          style={{ color: colors.text }}
        >
          {title}
        </h1>
        <p className="text-[12.5px] mb-8" style={{ color: colors.subText }}>
          Last updated: {LAST_UPDATED}
        </p>

        {intro && (
          <p
            className="text-[15.5px] leading-[1.7] mb-9 pb-8 border-b"
            style={{ color: colors.text, opacity: 0.9, borderColor: colors.dividerColor }}
          >
            <Rich text={intro} />
          </p>
        )}

        {sections.map((section, i) => (
          // id + scroll-mt-24 is what makes a #fragment link (the Organiser
          // Agreement link from events-organizer, for one) land on the right
          // section rather than the top of the page — scroll-mt-24 clears the
          // sticky h-16 nav (navBar.tsx) so the heading isn't hidden under it
          // after the jump.
          <section key={i} id={slugify(section.heading)} className="mb-9 scroll-mt-24">
            <h2
              className="text-[19px] font-bold tracking-[-0.01em] mb-4"
              style={{ color: colors.text }}
            >
              {section.heading}
            </h2>
            <Blocks blocks={section.blocks} />
          </section>
        ))}

        {seeAlso && seeAlso.length > 0 && (
          <div className="mt-12 pt-7 border-t" style={{ borderColor: colors.dividerColor }}>
            <p
              className="text-[11px] font-bold uppercase tracking-[0.1em] mb-3"
              style={{ color: colors.subText }}
            >
              Related documents
            </p>
            <div className="flex flex-wrap gap-x-5 gap-y-2">
              {seeAlso.map((link) => (
                <Link
                  key={link.href}
                  href={link.href}
                  className="text-[13.5px] no-underline"
                  style={{ color: '#ff2d55' }}
                >
                  {link.label}
                </Link>
              ))}
            </div>
          </div>
        )}
      </article>
    </main>
  );
}