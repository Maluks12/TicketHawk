// ── Organiser marketing landing page ──────────────────────────────────────────
// PUBLIC + server-rendered (no 'use client') so Google can index it — a bare
// login page gives search engines nothing to rank, so the organiser product
// never surfaced in search. This page carries real headings, metadata and
// JSON-LD; every CTA leads to /login (which itself sends already-signed-in
// organisers straight to /dashboard). NOTE: the login page lives at
// app/(auth)/login/page.tsx, but "(auth)" is a Next.js ROUTE GROUP —
// parenthesised folders organise files without adding a URL segment, so the
// real path is /login, NOT /auth/login. Linking to the latter 404s.
//
// VISUAL LANGUAGE — rewritten from the dark splash to a light Swiss-editorial
// layout: off-white canvas, hairline rules that divide the page into a visible
// grid, oversized tight-tracked uppercase headlines, and ONE accent colour used
// sparingly. The restraint is the point — a dark page with a glow reads as a
// template; a light page with a real grid reads as a product.
//
// The inverted ticket stub in the hero is the only dark element on the page, so
// it becomes the focal point without needing a glow to earn attention. It is
// built as an actual stub — notched on both sides, perforated down the middle,
// logo on the left panel — rather than a rounded rectangle standing in for one.
//
// All styles stay namespaced under `.thl` so they cannot collide with
// globals.css (.btn, .field, .metric…).
import type { Metadata } from 'next';
import Link from 'next/link';
import { SITE_URL } from './site';

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: 'Sell tickets in South Africa | TicketHawk for Organisers',
  description:
    'TicketHawk is the South African platform for event organisers: sell tickets, scan at the door offline, and get paid after your event. Instant EFT, card and Apple Pay, tiered ticketing, and a real-time dashboard.',
  keywords: [
    'sell event tickets South Africa', 'event ticketing platform', 'organiser dashboard',
    'ticketing software', 'door scanning', 'Paystack ticketing', 'instant EFT tickets',
  ],
  alternates: { canonical: SITE_URL },
  openGraph: {
    type: 'website',
    siteName: 'TicketHawk',
    title: 'Sell tickets in South Africa | TicketHawk for Organisers',
    description:
      'Sell tickets, scan at the door offline, and get paid after your event. Built for South African event organisers.',
    url: SITE_URL,
    images: ['/brand-logo.png'],
  },
  twitter: {
    card: 'summary',
    title: 'TicketHawk for Organisers',
    description: 'Sell tickets, scan at the door, and get paid fast. Built for South African events.',
    images: ['/brand-logo.png'],
  },
};

const jsonLd = {
  '@context': 'https://schema.org',
  '@type': 'SoftwareApplication',
  name: 'TicketHawk for Organisers',
  applicationCategory: 'BusinessApplication',
  operatingSystem: 'Web',
  url: SITE_URL,
  description:
    'South African event ticketing platform for organisers: sell tickets, scan at the door offline, and get paid after your event.',
  offers: {
    '@type': 'Offer',
    price: '0',
    priceCurrency: 'ZAR',
    description: 'No setup or monthly fees — a per-ticket service fee applies at checkout.',
  },
  publisher: { '@type': 'Organization', name: 'TicketHawk', url: 'https://ticket-hawk.com' },
};

// Merch is deliberately absent. It exists in the product, but it is not why an
// organiser chooses a ticketing platform, and listing it alongside payments and
// door scanning made the grid read as a feature dump rather than a case.
const FEATURES = [
  {
    n: '01',
    tag: 'Ticketing',
    title: 'Tiered tickets, sold your way',
    body: 'Early bird, general, VIP and group tiers, each with its own price and cap. Publish publicly or share a private link — you decide who ever sees the event.',
  },
  {
    n: '02',
    tag: 'Payments',
    title: 'Card, instant EFT and Apple Pay',
    body: 'Buyers pay in rands the way they already pay for everything else. No redirect to a payment page that looks like it belongs to someone else.',
  },
  {
    n: '03',
    tag: 'At the door',
    title: 'Scanning that works without signal',
    body: 'Rotating QR codes that cannot be screenshotted and reused, scanned from your phone with no reception at all. Every scan syncs the moment you reconnect.',
  },
  {
    n: '04',
    tag: 'Your team',
    title: 'Staff, roles and join codes',
    body: 'Add door staff with a join code — no accounts to create, no devices to configure. Each person only sees the event you assigned them to.',
  },
  {
    n: '05',
    tag: 'Analytics',
    title: 'See the night as it happens',
    body: 'Live sales, page views, conversion and door numbers in one place. Know how a night is tracking while you can still do something about it.',
  },
  {
    n: '06',
    tag: 'Trust',
    title: 'Identity verified against Home Affairs',
    body: 'Every organiser is checked against the DHA registry before they can take money. Buyers know the event behind the ticket is real, and so do we.',
  },
];

const STEPS = [
  { n: '01', title: 'Create', body: 'Add your details, set your tiers and prices, upload your artwork. Minutes, not days.' },
  { n: '02', title: 'Sell', body: 'Publish to the feed or share a private link. Buyers get their tickets the moment they pay.' },
  { n: '03', title: 'Get paid', body: 'Scan the door from your phone, then receive your payout to your South African bank account.' },
];

// The fourth stat replaced a per-ticket price. A number on a landing page reads
// as the headline term of the deal, and the fee is not the reason anyone picks a
// ticketing platform — the offline door is. "0 bars" states the differentiator
// as a number without pricing the product on the way past.
const VALUES = [
  {
    k: '48h',
    v: 'to your payout',
    note: 'After the event ends, on the standard tier. Verified organisers can be moved to instant settlement, where each sale splits to your account at checkout. Refunds run on a sliding scale and are deducted before payout.',
  },
  { k: '0 bars', v: 'needed at the door' },
  { k: 'R0', v: 'setup and monthly cost' },
  { k: 'SA', v: 'infrastructure and support' },
];

const AUDIENCES = ['Festivals', 'Concerts', 'Club nights', 'Sports', 'Expos', 'Comedy', 'Conferences'];

const css = `
@import url('https://fonts.googleapis.com/css2?family=Archivo:wght@400;500;600;700;800;900&family=Figtree:wght@300;400;500;600;700&display=swap');
.thl {
  --paper:#F2F1EE; --paper2:#EAE8E3; --ink:#111110; --ink2:#57564F; --ink3:#8A8880;
  --rule:#D9D7D1; --pink:#FF2D55; --gold:#C6A064;
  --dsp:'Archivo','Figtree',system-ui,sans-serif;
  --bdy:'Figtree',system-ui,sans-serif;
  background:var(--paper); color:var(--ink); font-family:var(--bdy);
  min-height:100vh; overflow-x:hidden; line-height:1.6;
  -webkit-font-smoothing:antialiased;
}
.thl *{box-sizing:border-box;}
.thl a{color:inherit;text-decoration:none;}
.thl-wrap{max-width:1180px;margin:0 auto;padding:0 32px;}

.thl-nav{position:sticky;top:0;z-index:50;background:rgba(242,241,238,.88);
  backdrop-filter:saturate(140%) blur(10px);border-bottom:1px solid var(--rule);}
.thl-nav-in{display:flex;align-items:center;gap:28px;height:70px;}
.thl-brand{display:flex;align-items:center;gap:10px;font-family:var(--dsp);
  font-weight:800;font-size:17px;letter-spacing:-.02em;}
.thl-brand img{width:32px;height:32px;display:block;}
.thl-brand .pink{color:var(--pink);}
.thl-navlinks{display:flex;gap:28px;margin-left:12px;}
.thl-navlinks a{color:var(--ink2);font-size:14px;font-weight:500;transition:color .15s;}
.thl-navlinks a:hover{color:var(--ink);}
.thl-navcta{margin-left:auto;display:flex;align-items:center;gap:10px;}

.thl-btn{display:inline-flex;align-items:center;gap:7px;font-weight:600;font-size:14px;
  padding:11px 20px;border-radius:2px;transition:all .15s;border:1px solid transparent;cursor:pointer;}
/* a.thl-btn-*, not .thl-btn-*: the reset above is ".thl a" (class + element =
   specificity 0,1,1), which outranks a bare class (0,1,0). color:inherit
   therefore won and the dark button rendered dark text on a dark fill —
   invisible. Qualifying with the element tag takes these to 0,2,1. */
.thl a.thl-btn-pink,.thl button.thl-btn-pink{background:var(--ink);color:var(--paper);}
.thl a.thl-btn-pink:hover,.thl button.thl-btn-pink:hover{background:var(--pink);color:var(--paper);}
.thl a.thl-btn-ghost,.thl button.thl-btn-ghost{border-color:var(--rule);color:var(--ink);}
.thl a.thl-btn-ghost:hover,.thl button.thl-btn-ghost:hover{border-color:var(--ink);color:var(--ink);}
.thl a.thl-inline{color:var(--ink);}
.thl a.thl-inline:hover{color:var(--pink);}

.thl-hero{padding:64px 0 0;}
.thl-hero-top{display:flex;justify-content:space-between;gap:40px;margin-bottom:34px;}
.thl-eyebrow{font-size:12.5px;color:var(--ink2);letter-spacing:.01em;}
.thl-heroblurb{font-size:13.5px;color:var(--ink2);text-align:right;max-width:290px;line-height:1.6;}
.thl-h1{font-family:var(--dsp);font-weight:800;font-size:clamp(46px,8.4vw,116px);
  line-height:.9;letter-spacing:-.04em;text-transform:uppercase;margin:0;}
.thl-heroact{display:flex;align-items:center;gap:28px;margin:34px 0 40px;flex-wrap:wrap;}
.thl-inline{font-size:15px;font-weight:600;border-bottom:1.5px solid var(--ink);padding-bottom:3px;}
.thl-inline:hover{color:var(--pink);border-color:var(--pink);}
.thl-fine{font-size:13px;color:var(--ink3);}

/* ── Ticket stub ──────────────────────────────────────────────────────────
   A real stub, not a rounded rectangle: notched on both edges where it would
   tear, split by a perforation into a wide face and a narrow counterfoil.
   The notches are radial-gradient cut-outs painted in the PAGE colour, so
   they read as holes rather than as circles sitting on top. */
.thl-ticket{position:relative;background:var(--ink);color:var(--paper);
  border-radius:3px;display:grid;grid-template-columns:1fr 168px;overflow:hidden;}
.thl-ticket::before,.thl-ticket::after{content:'';position:absolute;right:168px;
  width:26px;height:26px;background:var(--paper);border-radius:50%;transform:translateX(50%);z-index:2;}
.thl-ticket::before{top:-13px;}
.thl-ticket::after{bottom:-13px;}
.thl-tface{padding:34px 34px 30px;}
.thl-tlive{display:inline-flex;align-items:center;gap:7px;font-size:11px;
  letter-spacing:.11em;color:var(--gold);margin-bottom:20px;}
.thl-tdot{width:6px;height:6px;border-radius:50%;background:var(--pink);display:block;}
.thl-tname{font-family:var(--dsp);font-size:30px;font-weight:700;letter-spacing:-.025em;line-height:1.1;}
.thl-tmeta{font-size:13.5px;color:var(--ink3);margin-top:7px;}
.thl-tbar{margin-top:26px;height:3px;background:#332F2C;border-radius:2px;overflow:hidden;}
.thl-tbar span{display:block;height:100%;width:78%;background:var(--pink);}
.thl-tsold{display:flex;justify-content:space-between;font-size:12.5px;color:var(--ink3);margin-top:9px;}
.thl-tstub{border-left:2px dashed rgba(242,241,238,.22);display:flex;flex-direction:column;
  align-items:center;justify-content:center;gap:12px;padding:28px 18px;text-align:center;}
.thl-tstub img{width:52px;height:52px;display:block;}
.thl-tprice{font-family:var(--dsp);font-size:32px;font-weight:700;letter-spacing:-.03em;}
.thl-ttier{font-size:11.5px;color:var(--ink3);letter-spacing:.06em;}

.thl-sec{padding:82px 0;border-top:1px solid var(--rule);}
.thl-sechead{display:flex;justify-content:space-between;gap:44px;margin-bottom:8px;}
.thl-h2{font-family:var(--dsp);font-weight:800;font-size:clamp(28px,4.4vw,52px);
  line-height:1;letter-spacing:-.035em;text-transform:uppercase;margin:0;}
.thl-lede{font-size:15px;color:var(--ink2);max-width:340px;line-height:1.65;}

.thl-grid{display:grid;grid-template-columns:1fr 1fr;border-top:1px solid var(--rule);margin-top:44px;}
.thl-cell{padding:32px 34px 34px;border-bottom:1px solid var(--rule);}
.thl-cell:nth-child(odd){border-right:1px solid var(--rule);}
.thl-cell:nth-last-child(-n+2){border-bottom:none;}
.thl-cellhead{display:flex;gap:10px;font-size:11.5px;letter-spacing:.09em;
  color:var(--pink);margin-bottom:14px;text-transform:uppercase;}
.thl-cellhead b{color:var(--ink3);font-weight:500;}
.thl-cell h3{font-family:var(--dsp);font-size:21px;font-weight:600;letter-spacing:-.02em;margin:0 0 9px;}
.thl-cell p{font-size:14.5px;color:var(--ink2);line-height:1.65;margin:0;}

.thl-steps{display:grid;grid-template-columns:repeat(3,1fr);gap:28px;margin-top:44px;}
.thl-step{border-top:2px solid var(--ink);padding-top:18px;}
.thl-step .n{font-size:12px;color:var(--ink3);margin-bottom:10px;}
.thl-step h3{font-family:var(--dsp);font-size:19px;font-weight:600;margin:0 0 7px;letter-spacing:-.02em;}
.thl-step p{font-size:14px;color:var(--ink2);margin:0;line-height:1.6;}

.thl-values{display:grid;grid-template-columns:repeat(4,1fr);border-top:1px solid var(--rule);
  border-bottom:1px solid var(--rule);}
.thl-val{padding:32px 30px;border-right:1px solid var(--rule);}
.thl-val:last-child{border-right:none;}
.thl-val .k{font-family:var(--dsp);font-size:38px;font-weight:700;letter-spacing:-.035em;line-height:1;}
.thl-val .v{font-size:13.5px;color:var(--ink2);margin-top:7px;}
.thl-val .note{font-size:11.5px;color:var(--ink3);margin-top:12px;line-height:1.55;
  padding-top:12px;border-top:1px solid var(--rule);}

.thl-auds{display:flex;flex-wrap:wrap;gap:9px;margin-top:32px;}
.thl-aud{border:1px solid var(--rule);border-radius:2px;padding:9px 16px;
  font-size:13.5px;color:var(--ink2);}

.thl-cta{border-top:1px solid var(--rule);padding:82px 0;}
.thl-cta-in{display:flex;justify-content:space-between;align-items:flex-end;gap:40px;flex-wrap:wrap;}

.thl-foot{border-top:1px solid var(--rule);padding:44px 0 56px;}
.thl-foot-in{display:flex;justify-content:space-between;gap:44px;flex-wrap:wrap;}
.thl-fcol{font-size:13.5px;color:var(--ink2);line-height:2;}
.thl-fcol .h{color:var(--ink);font-weight:600;}
.thl-fcol a:hover{color:var(--pink);}
.thl-copy{font-size:12.5px;color:var(--ink3);margin-top:6px;}

@media (max-width:900px){
  .thl-wrap{padding:0 20px;}
  .thl-hero-top{flex-direction:column;gap:12px;}
  .thl-heroblurb{text-align:left;max-width:none;}
  .thl-sechead{flex-direction:column;gap:16px;}
  .thl-grid{grid-template-columns:1fr;}
  .thl-cell:nth-child(odd){border-right:none;}
  .thl-cell:nth-last-child(2){border-bottom:1px solid var(--rule);}
  .thl-steps{grid-template-columns:1fr;gap:22px;}
  .thl-values{grid-template-columns:1fr 1fr;}
  .thl-val{border-bottom:1px solid var(--rule);}
  .thl-val:nth-child(2n){border-right:none;}
  .thl-ticket{grid-template-columns:1fr;}
  .thl-ticket::before,.thl-ticket::after{right:auto;left:50%;transform:translateX(-50%);}
  .thl-ticket::before{top:auto;bottom:118px;left:-13px;transform:none;}
  .thl-ticket::after{bottom:118px;left:auto;right:-13px;transform:none;}
  .thl-tstub{border-left:none;border-top:2px dashed rgba(242,241,238,.22);flex-direction:row;justify-content:space-between;}
  .thl-sec,.thl-cta{padding:56px 0;}
  .thl-navlinks{display:none;}
  .thl-hidephone{display:none;}
}
`;

export default function OrganiserLanding() {
  return (
    <div className="thl">
      <style dangerouslySetInnerHTML={{ __html: css }} />
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }} />

      <nav className="thl-nav">
        <div className="thl-wrap thl-nav-in">
          <Link href="/" className="thl-brand" aria-label="TicketHawk home">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src="/brand-logo.png" alt="" />
            <span>TICKET<span className="pink">HAWK</span></span>
          </Link>
          <div className="thl-navlinks">
            <a href="#features">Features</a>
            <a href="#how">How it works</a>
            <a href="#audiences">Events</a>
          </div>
          <div className="thl-navcta">
            {/* On phones the header has room for one button, and it used to be
                "Start selling" — so opening the site on a phone and tapping the
                only button there landed returning organisers on Create account.
                The phone header now shows Log in; sign-up stays one tap away in
                the hero ("Start selling") and on the login card itself. */}
            <Link href="/login" className="thl-btn thl-btn-ghost">Log in</Link>
            <Link href="/login?mode=register" className="thl-btn thl-btn-pink thl-hidephone">Start selling</Link>
          </div>
        </div>
      </nav>

      <main>
        <section className="thl-hero">
          <div className="thl-wrap">
            <div className="thl-hero-top">
              <span className="thl-eyebrow">Event ticketing · South Africa</span>
              <p className="thl-heroblurb">
                Sell tickets, scan at the door, and get paid — all from one
                dashboard, on the web or your phone.
              </p>
            </div>

            <h1 className="thl-h1">Sell out your<br />next event.</h1>

            <div className="thl-heroact">
              <Link href="/login?mode=register" className="thl-inline">Start selling ↘</Link>
              <span className="thl-fine">No setup fees. No monthly cost.</span>
            </div>

            <div className="thl-ticket">
              <div className="thl-tface">
                <span className="thl-tlive"><span className="thl-tdot" />LIVE NOW</span>
                <div className="thl-tname">Summer Season Opener</div>
                <div className="thl-tmeta">Sat 6 Dec · Cape Town</div>
                <div className="thl-tbar"><span /></div>
                <div className="thl-tsold">
                  <span>780 / 1000 sold</span>
                  <span>R140 400 in sales</span>
                </div>
              </div>
              <div className="thl-tstub">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img src="/brand-logo.png" alt="" />
                <div>
                  <div className="thl-tprice">R180</div>
                  <div className="thl-ttier">GENERAL</div>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section className="thl-sec" id="features">
          <div className="thl-wrap">
            <div className="thl-sechead">
              <h2 className="thl-h2">Everything<br />you need.</h2>
              <p className="thl-lede">
                From the first ticket sold to the payout landing in your bank —
                built for South African organisers, not adapted for them.
              </p>
            </div>
            <div className="thl-grid">
              {FEATURES.map(f => (
                <div className="thl-cell" key={f.n}>
                  <div className="thl-cellhead"><b>{f.n}</b>{f.tag}</div>
                  <h3>{f.title}</h3>
                  <p>{f.body}</p>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section className="thl-sec" id="how">
          <div className="thl-wrap">
            <div className="thl-sechead">
              <h2 className="thl-h2">Three steps to<br />a sold-out door.</h2>
            </div>
            <div className="thl-steps">
              {STEPS.map(s => (
                <div className="thl-step" key={s.n}>
                  <div className="n">{s.n}</div>
                  <h3>{s.title}</h3>
                  <p>{s.body}</p>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section className="thl-sec" style={{ paddingBottom: 0 }}>
          <div className="thl-wrap">
            <div className="thl-values">
              {VALUES.map(v => (
                <div className="thl-val" key={v.k}>
                  <div className="k">{v.k}</div>
                  <div className="v">{v.v}</div>
                  {v.note && <div className="note">{v.note}</div>}
                </div>
              ))}
            </div>
          </div>
        </section>

        <section className="thl-sec" id="audiences" style={{ borderTop: 'none' }}>
          <div className="thl-wrap">
            <div className="thl-sechead">
              <h2 className="thl-h2">From club nights<br />to festivals.</h2>
            </div>
            <div className="thl-auds">
              {AUDIENCES.map(a => <span className="thl-aud" key={a}>{a}</span>)}
            </div>
          </div>
        </section>

        <section className="thl-cta">
          <div className="thl-wrap thl-cta-in">
            <h2 className="thl-h2">Your next event<br />starts here.</h2>
            <div style={{ display: 'flex', gap: 10 }}>
              <Link href="/login?mode=register" className="thl-btn thl-btn-pink">Start selling</Link>
              <Link href="/login" className="thl-btn thl-btn-ghost">Log in</Link>
            </div>
          </div>
        </section>
      </main>

      <footer className="thl-foot">
        <div className="thl-wrap thl-foot-in">
          <div>
            <div className="thl-brand" style={{ marginBottom: 8 }}>
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src="/brand-logo.png" alt="" />
              <span>TICKET<span className="pink">HAWK</span></span>
            </div>
            <div className="thl-copy">© {new Date().getFullYear()} Self Hawk Tech (Pty) Ltd</div>
          </div>
          <div style={{ display: 'flex', gap: 56, flexWrap: 'wrap' }}>
            <div className="thl-fcol">
              <div className="h">Product</div>
              <div><a href="#features">Features</a></div>
              <div><a href="#how">How it works</a></div>
              <div><a href="#audiences">Events</a></div>
            </div>
            <div className="thl-fcol">
              <div className="h">Account</div>
              <div><Link href="/login">Log in</Link></div>
              <div><Link href="/login?mode=register">Start selling</Link></div>
            </div>
            <div className="thl-fcol">
              <div className="h">Legal</div>
              <div><a href="https://ticket-hawk.com/terms">Terms</a></div>
              <div><a href="https://ticket-hawk.com/privacy">Privacy</a></div>
            </div>
          </div>
        </div>
      </footer>
    </div>
  );
}