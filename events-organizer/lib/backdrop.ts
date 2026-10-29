// Click-outside-to-close for .modal-overlay.
//
// The overlay is the element that SCROLLS when a dialog is taller than the
// screen (overflow-y: auto in globals.css), so its scrollbar belongs to the
// overlay. A press on that scrollbar reports the overlay as its target, and the
// plain `e.target === e.currentTarget` test read it as a click on the backdrop:
// grabbing the scrollbar to scroll a long dialog closed the dialog instead.
//
// clientWidth / clientHeight exclude the scrollbar, so anything to the right
// of (or below) them is a press on a scrollbar, never on the backdrop.
import type { MouseEvent } from 'react';

export function isBackdropPress(e: MouseEvent<HTMLElement>): boolean {
  const el = e.currentTarget;
  if (e.target !== el) return false;
  const r = el.getBoundingClientRect();
  if (e.clientX >= r.left + el.clientLeft + el.clientWidth) return false;
  if (e.clientY >= r.top + el.clientTop + el.clientHeight) return false;
  return true;
}