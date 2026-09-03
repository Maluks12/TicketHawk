// ─────────────────────────────────────────────────────────────────────────────
// Money formatting — ONE implementation, used everywhere.
//
// There were eight of these, each written as
//
//     `R${Math.round(price).toLocaleString('en-ZA')}`
//
// in api.ts, purchase.tsx, event/[id].tsx, booking/[id].tsx, payment/[id].tsx,
// category/[name].tsx, organiser/[slug].tsx and profile/delete-account.tsx.
// Math.round does not truncate, it rounds to nearest — so a R3 ticket with its
// 60c service fee displayed as "R4", and the buyer was quoted 40c MORE than
// they were charged. R7.50 showed as "R8". Every one of those is a number on a
// button face that the payment sheet then contradicts.
//
// lib/refunds.ts had already been fixed for the same bug on refund quotes. This
// module is that fix, hoisted somewhere the rest of the app can reach it, so
// the next screen that needs to print an amount has an obvious right answer
// instead of a ninth copy of the wrong one.
//
// The server is authoritative and carries cents (models.roundRand), so display
// has to as well. Whole amounts still render clean — R10, not R10.00 — because
// most prices are whole and R10.00 everywhere reads like a spreadsheet.
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Does this amount have cents worth showing?
 *
 * The tolerance guards against float noise: 0.1 + 0.2 lands at
 * 0.30000000000000004, and a naive `n * 100 % 100 !== 0` would treat a value
 * that is a whole number in every meaningful sense as fractional and print
 * "R10,00". Anything more than half a cent away from a round cent is treated
 * as real cents.
 */
function hasCents(n: number): boolean {
  const cents = n * 100;
  if (Math.abs(cents - Math.round(cents)) >= 0.5) return true;
  return Math.round(cents) % 100 !== 0;
}

/**
 * The app's money format: "R10", "R3,60", "R1 250,50".
 *
 * en-ZA puts a comma in the decimal position and a space in the thousands
 * separator. That is correct for the locale and matches what Paystack shows the
 * buyer at checkout, so leave it alone even though it looks odd in code.
 */
export function fmtRand(n: number | null | undefined): string {
  if (n === null || n === undefined || Number.isNaN(n)) return 'R0';
  return `R${n.toLocaleString('en-ZA', {
    minimumFractionDigits: hasCents(n) ? 2 : 0,
    maximumFractionDigits: 2,
  })}`;
}

/**
 * Same, with an explicit currency code in front: "ZAR 3,60".
 *
 * Used on the payment screen, where the amount sits beside a gateway that names
 * the currency, so dropping the code would lose information the buyer is being
 * asked to authorise.
 */
export function fmtCurrency(n: number | null | undefined, currency = 'ZAR'): string {
  if (n === null || n === undefined || Number.isNaN(n)) return `${currency} 0`;
  return `${currency} ${n.toLocaleString('en-ZA', {
    minimumFractionDigits: hasCents(n) ? 2 : 0,
    maximumFractionDigits: 2,
  })}`;
}

/**
 * Price for a listing or a ticket tier, where "Free" is meaningful and "R0" is
 * not. A zero-priced event is a real product here — the App Store review event
 * is one — and a card reading "R0" looks like missing data rather than a free
 * ticket.
 */
export function fmtPrice(n: number | null | undefined): string {
  if (n === null || n === undefined || Number.isNaN(n)) return 'Free';
  return n <= 0 ? 'Free' : fmtRand(n);
}