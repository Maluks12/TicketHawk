// ── Organiser marketing landing page ──────────────────────────────────────────
// PUBLIC + server-rendered (no 'use client') so Google can index it — a bare
// login page gives search engines nothing to rank, so the organiser product
// never surfaced in search. This page carries real headings, metadata and
// JSON-LD; every CTA leads to /login (which itself sends already-signed-in
// organisers straight to /dashboard). NOTE: the login page lives at
// app/(auth)/login/page.tsx, but "(auth)" is a Next.js ROUTE GROUP — parenthesised
// folders organise files without adding a URL segment, so the real path is
// /login, NOT /auth/login. Linking to the latter 404s.
//
// Visual language is intentionally NOT the light Meta-style dashboard theme —
// it's a dark marketing splash in TicketHawk's own palette (brand pink #FF2D55 +
// the eagle's gold), with the ticket-stub as the recurring motif. All styles are
// namespaced under `.thl` so they can't collide with globals.css (.btn, .field…).
import type { Metadata } from 'next';
import Link from 'next/link';
import { SITE_URL } from './site';

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: 'Sell tickets in South Africa | TicketHawk for Organisers',
  description:
    'TicketHawk is the South African platform for event organisers: sell tickets, scan at the door offline, and get paid within 48 hours. Instant EFT and card payments, tiered ticketing, and a real-time dashboard.',
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
      'Sell tickets, scan at the door offline, and get paid within 48 hours. Built for South African event organisers.',
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
    'South African event ticketing platform for organisers: sell tickets, scan at the door offline, and get paid within 48 hours.',
  offers: {
    '@type': 'Offer',
    price: '0',
    priceCurrency: 'ZAR',
    description: 'No setup or monthly fees — a per-ticket service fee applies at checkout.',
  },
  publisher: { '@type': 'Organization', name: 'TicketHawk', url: 'https://ticket-hawk.com' },
};

const FEATURES = [
  {
    tag: 'Ticketing',
    title: 'Tiered tickets, sold your way',
    body: 'Early bird, GA, VIP and group tickets with per-tier pricing and caps. Publish a public event or share a private link — you stay in control of who sees it.',
  },
  {
    tag: 'Payments',
    title: 'Instant EFT and card, paid out fast',
    body: 'Buyers pay by card or instant EFT in rands. Money settles to your bank within 48 hours of the event — and verified organisers can unlock instant payouts.',
  },
  {
    tag: 'At the door',
    title: 'Offline-first door scanning',
    body: 'Scan tickets from your phone even with no signal. Add staff and scanners with a join code, and every scan syncs the moment you are back online.',
  },
  {
    tag: 'Dashboard',
    title: 'See sales as they happen',
    body: 'Live sales, revenue and check-in numbers in one place. Know exactly how a night is tracking without waiting for a report.',
  },
];

const STEPS = [
  { n: '01', title: 'Create your event', body: 'Add the details, set your ticket tiers and prices, and upload your artwork. Minutes, not days.' },
  { n: '02', title: 'Share and sell', body: 'Publish publicly or share a private link. Buyers pay by card or instant EFT and get their tickets straight away.' },
  { n: '03', title: 'Scan and get paid', body: 'Scan at the door from your phone, then receive your payout to your bank within 48 hours of the event.' },
];

const VALUES = [
  { k: '48 hours', v: 'to your payout' },
  { k: 'Instant EFT', v: '& card in rands' },
  { k: 'Offline', v: 'door scanning' },
  { k: 'No', v: 'setup or monthly fees' },
];

const AUDIENCES = ['Festivals', 'Concerts', 'Club nights', 'Sports', 'Expos', 'Comedy', 'Conferences'];

const css = `
@import url('https://fonts.googleapis.com/css2?family=Archivo:ital,wght@0,600;0,700;0,800;0,900;1,800;1,900&family=Figtree:wght@300;400;500;600;700&display=swap');
.thl {
  --ink:#0a0a0c; --ink2:#121218; --ink3:#1a1a22; --line:rgba(255,255,255,.09);
  --pink:#FF2D55; --pink-d:#d81f44; --gold:#c6a064; --gold-soft:#d8b878;
  --tx:#f5f5f7; --mut:#a0a0aa; --mut2:#6f6f78;
  --dsp:'Archivo','Figtree',system-ui,sans-serif;
  --bdy:'Figtree',system-ui,sans-serif;
  background:var(--ink); color:var(--tx); font-family:var(--bdy);
  min-height:100vh; overflow-x:hidden; line-height:1.55;
}
.thl *{box-sizing:border-box;}
.thl a{color:inherit;text-decoration:none;}
.thl-wrap{max-width:1180px;margin:0 auto;padding:0 24px;}

.thl-nav{position:sticky;top:0;z-index:50;background:rgba(10,10,12,.82);
  backdrop-filter:saturate(140%) blur(10px);border-bottom:1px solid var(--line);}
.thl-nav-in{display:flex;align-items:center;gap:24px;height:66px;}
/* NOTE: the wordmark is wrapped in a single <span> in the markup. A bare
   "TICKET" text node + <span>HAWK</span> would be TWO flex items, so this
   gap would render the brand as "TICKET HAWK" - it is one word. */
.thl-brand{display:flex;align-items:center;gap:10px;font-family:var(--dsp);
  font-weight:900;font-size:20px;letter-spacing:.5px;}
.thl-brand img{width:34px;height:34px;display:block;}
.thl-brand .pink{color:var(--pink);}
.thl-navlinks{display:flex;gap:26px;margin-left:14px;}
.thl-navlinks a{color:var(--mut);font-size:14.5px;font-weight:500;transition:color .15s;}
.thl-navlinks a:hover{color:var(--tx);}
.thl-navcta{margin-left:auto;display:flex;align-items:center;gap:12px;}

.thl-btn{display:inline-flex;align-items:center;justify-content:center;gap:8px;
  font-family:var(--dsp);font-weight:800;font-size:14px;letter-spacing:.3px;
  padding:11px 20px;border-radius:11px;border:1px solid transparent;cursor:pointer;
  transition:transform .12s ease, background .15s, border-color .15s, color .15s;white-space:nowrap;}
.thl-btn:hover{transform:translateY(-1px);}
.thl-btn-pink{background:var(--pink);color:#fff;box-shadow:0 8px 24px rgba(255,45,85,.28);}
.thl-btn-pink:hover{background:var(--pink-d);}
.thl-btn-ghost{border-color:var(--line);color:var(--tx);background:transparent;}
.thl-btn-ghost:hover{border-color:rgba(255,255,255,.28);background:rgba(255,255,255,.04);}
.thl-btn-lg{padding:15px 28px;font-size:16px;border-radius:13px;}

.thl-hero{position:relative;padding:76px 0 54px;}
.thl-hero-grid{display:grid;grid-template-columns:1.05fr .95fr;gap:52px;align-items:center;}
.thl-eyebrow{display:inline-flex;align-items:center;gap:9px;font-family:var(--dsp);
  font-weight:800;font-size:12.5px;letter-spacing:1.6px;text-transform:uppercase;
  color:var(--gold-soft);margin-bottom:22px;}
.thl-eyebrow::before{content:"";width:26px;height:2px;background:var(--gold);display:inline-block;}
.thl-h1{font-family:var(--dsp);font-weight:900;font-size:clamp(40px,6.2vw,68px);
  line-height:1.02;letter-spacing:-1.5px;margin:0 0 20px;}
.thl-h1 em{font-style:italic;color:var(--pink);}
.thl-sub{font-size:18px;color:var(--mut);max-width:520px;margin:0 0 30px;}
.thl-hero-cta{display:flex;gap:14px;flex-wrap:wrap;}
.thl-hero-note{margin-top:18px;font-size:13.5px;color:var(--mut2);}

.thl-ticket{position:relative;background:linear-gradient(160deg,#1c1c26,#111118);
  border:1px solid var(--line);border-radius:20px;padding:26px;
  box-shadow:0 30px 80px rgba(0,0,0,.5);}
.thl-ticket::before,.thl-ticket::after{content:"";position:absolute;width:26px;height:26px;
  border-radius:50%;background:var(--ink);top:50%;transform:translateY(-50%);}
.thl-ticket::before{left:-13px;} .thl-ticket::after{right:-13px;}
.thl-tk-perf{position:absolute;top:50%;left:24px;right:24px;transform:translateY(-50%);
  border-top:2px dashed rgba(255,255,255,.14);}
.thl-tk-top{display:flex;justify-content:space-between;align-items:flex-start;padding-bottom:26px;}
.thl-tk-badge{font-family:var(--dsp);font-weight:800;font-size:11px;letter-spacing:1.4px;
  text-transform:uppercase;color:var(--gold-soft);border:1px solid rgba(198,160,100,.4);
  border-radius:999px;padding:5px 11px;display:inline-block;margin-bottom:12px;}
.thl-tk-price{font-family:var(--dsp);font-weight:900;font-size:26px;}
.thl-tk-title{font-family:var(--dsp);font-weight:800;font-size:23px;margin:2px 0 4px;letter-spacing:-.4px;}
.thl-tk-meta{font-size:13.5px;color:var(--mut);}
.thl-tk-bottom{padding-top:26px;}
.thl-tk-barlabel{display:flex;justify-content:space-between;font-size:12.5px;color:var(--mut);margin-bottom:8px;}
.thl-tk-bar{height:9px;border-radius:99px;background:rgba(255,255,255,.08);overflow:hidden;}
.thl-tk-fill{height:100%;width:78%;border-radius:99px;
  background:linear-gradient(90deg,var(--gold),var(--pink));}
.thl-tk-sold{margin-top:10px;font-size:13px;color:var(--tx);font-weight:600;}
.thl-tk-sold b{color:var(--pink);}

.thl-values{border-top:1px solid var(--line);border-bottom:1px solid var(--line);background:var(--ink2);}
.thl-values-in{display:grid;grid-template-columns:repeat(4,1fr);}
.thl-val{padding:30px 24px;text-align:center;border-right:1px dashed var(--line);}
.thl-val:last-child{border-right:none;}
.thl-val-k{font-family:var(--dsp);font-weight:900;font-size:27px;letter-spacing:-.6px;color:var(--tx);}
.thl-val-k span{color:var(--pink);}
.thl-val-v{font-size:13.5px;color:var(--mut);margin-top:4px;}

.thl-sec{padding:88px 0;}
.thl-sec-head{max-width:620px;margin-bottom:44px;}
.thl-kicker{font-family:var(--dsp);font-weight:800;font-size:12.5px;letter-spacing:1.6px;
  text-transform:uppercase;color:var(--pink);margin-bottom:14px;}
.thl-h2{font-family:var(--dsp);font-weight:900;font-size:clamp(28px,4vw,42px);
  letter-spacing:-1px;line-height:1.08;margin:0 0 14px;}
.thl-lead{font-size:17px;color:var(--mut);}

.thl-feat{display:grid;grid-template-columns:repeat(2,1fr);gap:18px;}
.thl-card{background:var(--ink2);border:1px solid var(--line);border-radius:18px;padding:30px;
  transition:transform .18s ease, border-color .18s;}
.thl-card:hover{transform:translateY(-4px);border-color:rgba(255,45,85,.35);}
.thl-card-tag{font-family:var(--dsp);font-weight:800;font-size:11.5px;letter-spacing:1.3px;
  text-transform:uppercase;color:var(--gold-soft);margin-bottom:14px;}
.thl-card h3{font-family:var(--dsp);font-weight:800;font-size:21px;letter-spacing:-.4px;margin:0 0 10px;}
.thl-card p{font-size:15px;color:var(--mut);margin:0;}

.thl-steps{display:grid;grid-template-columns:repeat(3,1fr);gap:20px;}
.thl-step{position:relative;padding:30px;border:1px solid var(--line);border-radius:18px;
  background:linear-gradient(180deg,var(--ink2),transparent);}
.thl-step-n{font-family:var(--dsp);font-weight:900;font-size:44px;letter-spacing:-2px;
  color:transparent;-webkit-text-stroke:1.5px var(--gold);opacity:.55;line-height:1;margin-bottom:16px;}
.thl-step h3{font-family:var(--dsp);font-weight:800;font-size:20px;margin:0 0 8px;letter-spacing:-.3px;}
.thl-step p{font-size:14.5px;color:var(--mut);margin:0;}

.thl-aud{display:flex;flex-wrap:wrap;gap:12px;margin-top:8px;}
.thl-chip{font-family:var(--dsp);font-weight:700;font-size:15px;letter-spacing:.2px;
  padding:11px 20px;border-radius:999px;border:1px solid var(--line);color:var(--tx);background:var(--ink2);}
.thl-chip:first-child{background:var(--pink);border-color:var(--pink);color:#fff;}

.thl-cta{position:relative;overflow:hidden;border-radius:26px;
  background:linear-gradient(135deg,#1b1b24,#101016);border:1px solid var(--line);
  padding:64px 40px;text-align:center;}
.thl-cta::after{content:"";position:absolute;inset:0;
  background:radial-gradient(600px 200px at 50% -20%, rgba(255,45,85,.22), transparent 70%);}
.thl-cta > *{position:relative;}
.thl-cta h2{font-family:var(--dsp);font-weight:900;font-size:clamp(30px,4.4vw,46px);letter-spacing:-1px;margin:0 0 14px;}
.thl-cta p{font-size:17px;color:var(--mut);max-width:520px;margin:0 auto 28px;}
.thl-cta-row{display:flex;gap:14px;justify-content:center;flex-wrap:wrap;}

.thl-foot{border-top:1px solid var(--line);padding:40px 0;margin-top:20px;}
.thl-foot-in{display:flex;align-items:center;justify-content:space-between;gap:20px;flex-wrap:wrap;}
.thl-foot-links{display:flex;gap:22px;flex-wrap:wrap;}
.thl-foot-links a{color:var(--mut);font-size:14px;transition:color .15s;}
.thl-foot-links a:hover{color:var(--tx);}
.thl-foot-copy{font-size:13px;color:var(--mut2);}

@media (prefers-reduced-motion:no-preference){
  .thl-rise{animation:thlRise .7s cubic-bezier(.22,.61,.36,1) both;}
  .thl-rise-2{animation-delay:.08s;}
  @keyframes thlRise{from{opacity:0;transform:translateY(16px);}to{opacity:1;transform:none;}}
}

@media (max-width:900px){
  .thl-hero-grid{grid-template-columns:1fr;gap:40px;}
  .thl-values-in{grid-template-columns:repeat(2,1fr);}
  .thl-val:nth-child(2){border-right:none;}
  .thl-val:nth-child(1),.thl-val:nth-child(2){border-bottom:1px dashed var(--line);}
  .thl-feat{grid-template-columns:1fr;}
  .thl-steps{grid-template-columns:1fr;}
  .thl-navlinks{display:none;}
}
@media (max-width:560px){
  .thl-values-in{grid-template-columns:1fr;}
  .thl-val{border-right:none!important;border-bottom:1px dashed var(--line);}
  .thl-val:last-child{border-bottom:none;}
  .thl-sec{padding:64px 0;}
  .thl-hero{padding:52px 0 40px;}
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
            <img src="/brand-logo.png" alt="TicketHawk" />
            <span>TICKET<span className="pink">HAWK</span></span>
          </Link>
          <div className="thl-navlinks">
            <a href="#features">Features</a>
            <a href="#how">How it works</a>
            <a href="#audiences">Events</a>
          </div>
          <div className="thl-navcta">
            <Link href="/login" className="thl-btn thl-btn-ghost thl-hidephone">Log in</Link>
            <Link href="/login?mode=register" className="thl-btn thl-btn-pink">Start selling</Link>
          </div>
        </div>
      </nav>

      <main>
        <section className="thl-hero">
          <div className="thl-wrap thl-hero-grid">
            <div className="thl-rise">
              <span className="thl-eyebrow">For event organisers in South Africa</span>
              <h1 className="thl-h1">Sell out your<br />next event, <em>faster</em>.</h1>
              <p className="thl-sub">
                TicketHawk is where South African organisers sell tickets, scan at
                the door, and get paid — all from one dashboard, on the web or your phone.
              </p>
              <div className="thl-hero-cta">
                <Link href="/login?mode=register" className="thl-btn thl-btn-pink thl-btn-lg">Start selling</Link>
                <Link href="/login" className="thl-btn thl-btn-ghost thl-btn-lg">Log in</Link>
              </div>
              <p className="thl-hero-note">No setup fees. No monthly cost. You only pay a small fee per ticket sold.</p>
            </div>

            <div className="thl-rise thl-rise-2" aria-hidden="true">
              <div className="thl-ticket">
                <div className="thl-tk-top">
                  <div>
                    <span className="thl-tk-badge">Live now</span>
                    <div className="thl-tk-title">Summer Season Opener</div>
                    <div className="thl-tk-meta">Sat 6 Dec · Cape Town</div>
                  </div>
                  <div style={{ textAlign: 'right' }}>
                    <div className="thl-tk-price">R180</div>
                    <div className="thl-tk-meta">General</div>
                  </div>
                </div>
                <div className="thl-tk-perf" />
                <div className="thl-tk-bottom">
                  <div className="thl-tk-barlabel"><span>Tickets sold</span><span>780 / 1000</span></div>
                  <div className="thl-tk-bar"><div className="thl-tk-fill" /></div>
                  <div className="thl-tk-sold"><b>R140 400</b> in sales · settling in 48h</div>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section className="thl-values" aria-label="Why TicketHawk">
          <div className="thl-wrap thl-values-in">
            {VALUES.map((v) => (
              <div className="thl-val" key={v.k}>
                <div className="thl-val-k"><span>{v.k}</span></div>
                <div className="thl-val-v">{v.v}</div>
              </div>
            ))}
          </div>
        </section>

        <section className="thl-sec" id="features">
          <div className="thl-wrap">
            <div className="thl-sec-head">
              <div className="thl-kicker">Everything in one place</div>
              <h2 className="thl-h2">Run the whole event, start to payout</h2>
              <p className="thl-lead">From the first ticket you list to the last person through the gate — and the money in your account after.</p>
            </div>
            <div className="thl-feat">
              {FEATURES.map((f) => (
                <article className="thl-card" key={f.tag}>
                  <div className="thl-card-tag">{f.tag}</div>
                  <h3>{f.title}</h3>
                  <p>{f.body}</p>
                </article>
              ))}
            </div>
          </div>
        </section>

        <section className="thl-sec" id="how" style={{ background: 'var(--ink2)', borderTop: '1px solid var(--line)', borderBottom: '1px solid var(--line)' }}>
          <div className="thl-wrap">
            <div className="thl-sec-head">
              <div className="thl-kicker">How it works</div>
              <h2 className="thl-h2">Three steps to a sold-out door</h2>
            </div>
            <div className="thl-steps">
              {STEPS.map((s) => (
                <div className="thl-step" key={s.n}>
                  <div className="thl-step-n">{s.n}</div>
                  <h3>{s.title}</h3>
                  <p>{s.body}</p>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section className="thl-sec" id="audiences">
          <div className="thl-wrap">
            <div className="thl-sec-head">
              <div className="thl-kicker">Built for every kind of event</div>
              <h2 className="thl-h2">From club nights to festivals</h2>
              <p className="thl-lead">Whatever you put on, TicketHawk handles the tickets, the door and the payout.</p>
            </div>
            <div className="thl-aud">
              {AUDIENCES.map((a) => <span className="thl-chip" key={a}>{a}</span>)}
            </div>
          </div>
        </section>

        <section className="thl-sec" style={{ paddingTop: 0 }}>
          <div className="thl-wrap">
            <div className="thl-cta">
              <h2>Your next event starts here</h2>
              <p>Create your organiser account and list your first event in minutes. It&rsquo;s free to get started.</p>
              <div className="thl-cta-row">
                <Link href="/login?mode=register" className="thl-btn thl-btn-pink thl-btn-lg">Start selling</Link>
                <Link href="/login" className="thl-btn thl-btn-ghost thl-btn-lg">Log in</Link>
              </div>
            </div>
          </div>
        </section>
      </main>

      <footer className="thl-foot">
        <div className="thl-wrap thl-foot-in">
          <Link href="/" className="thl-brand" style={{ fontSize: 17 }}>
            <img src="/brand-logo.png" alt="TicketHawk" style={{ width: 28, height: 28 }} />
            <span>TICKET<span className="pink">HAWK</span></span>
          </Link>
          <div className="thl-foot-links">
            <a href="#features">Features</a>
            <a href="#how">How it works</a>
            <Link href="/login">Log in</Link>
            <Link href="/login?mode=register">Start selling</Link>
          </div>
          <div className="thl-foot-copy">© {new Date().getFullYear()} TicketHawk</div>
        </div>
      </footer>
    </div>
  );
}