// Overlay that marks a feed card as an event COLLECTION rather than a single
// event: a stack icon, "COLLECTION" and the number of dates. Sits bottom-left
// of the card image so it never competes with the price chip (top-right) or
// the LIVE / rank markers (top-left). Kept identical in web-desktop and
// web-mobile.
import { Layers } from 'lucide-react';
import type { EventCollectionRef } from '@/lib/api';
import { collectionCountLabel } from '@/lib/eventCollections';

const POSITION = {
  'bottom-left':  'left-2.5 bottom-2.5',
  'bottom-right': 'right-2.5 bottom-2.5',
  'top-left':     'left-3 top-3',
} as const;

export function CollectionBadge({ collection, compact = false, position = 'bottom-left' }: {
  collection: EventCollectionRef;
  /** Smaller type for the narrow mobile cards. */
  compact?: boolean;
  /** Bottom-left by default; cards that already use that corner pick another. */
  position?: keyof typeof POSITION;
}) {
  return (
    <div
      className={`absolute ${POSITION[position]} flex items-center gap-1.5 rounded-[6px] px-2`}
      style={{
        height: compact ? 20 : 24,
        backgroundColor: 'rgba(0,0,0,0.72)',
        backdropFilter: 'blur(6px)',
        WebkitBackdropFilter: 'blur(6px)',
        border: '1px solid rgba(255,255,255,0.18)',
      }}
      aria-label={`Collection of ${collection.event_count} dates`}
    >
      <Layers size={compact ? 11 : 13} color="#ffffff" strokeWidth={2.5} />
      <span
        className="text-white font-extrabold leading-none"
        style={{ fontSize: compact ? 8.5 : 9.5, letterSpacing: '1.1px' }}
      >
        COLLECTION · {collectionCountLabel(collection)}
      </span>
    </div>
  );
}