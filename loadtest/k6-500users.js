// TicketHawk load test — 500 concurrent virtual users.
//
// Run:   k6 run k6-500users.js
// Env:   BASE_URL   (default https://api.ticket-hawk.com)
//        TEST_EMAIL / TEST_PASSWORD  (optional — enables the logged-in flow
//        for ~10% of users: login → /v1/home personalised → /v1/bookings)
//
// The journey mirrors what the consumer app actually does on open:
//   home feed → categories → pick a real event from the feed → view ping →
//   event detail → ticket types → think-time sleeps between steps.
// It is deliberately READ-heavy: no bookings are created, so it leaves no
// garbage rows behind and never touches Paystack.

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Rate } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'https://api.ticket-hawk.com';
const EMAIL = __ENV.TEST_EMAIL || '';
const PASS = __ENV.TEST_PASSWORD || '';

export const options = {
  stages: [
    { duration: '1m', target: 100 },  // warm up
    { duration: '2m', target: 500 },  // ramp to full load
    { duration: '5m', target: 500 },  // hold — this is the real measurement
    { duration: '1m', target: 0 },    // ramp down
  ],
  thresholds: {
    http_req_failed: ['rate<0.02'],          // <2% errors
    http_req_duration: ['p(95)<1500'],       // 95% of requests under 1.5s
    'http_req_duration{endpoint:home}': ['p(95)<2000'],
  },
};

const homeTrend = new Trend('home_ms');
const detailTrend = new Trend('event_detail_ms');
const rateLimited = new Rate('rate_limited_429');

export default function () {
  const params = { headers: { 'Content-Type': 'application/json' }, tags: {} };

  // ~10% of VUs behave as logged-in users if creds are provided.
  let auth = null;
  if (EMAIL && __VU % 10 === 0) {
    const login = http.post(
      `${BASE}/v1/auth/login`,
      JSON.stringify({ email: EMAIL, password: PASS }),
      { headers: { 'Content-Type': 'application/json' }, tags: { endpoint: 'login' } },
    );
    if (login.status === 200) auth = login.json('access_token');
  }
  const authed = auth
    ? { headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${auth}` } }
    : params;

  // 1. Home feed — the heaviest read (multi-section + cache path)
  const home = http.get(`${BASE}/v1/home`, { ...authed, tags: { endpoint: 'home' } });
  homeTrend.add(home.timings.duration);
  rateLimited.add(home.status === 429);
  check(home, { 'home 200': (r) => r.status === 200 });

  // Pull real event IDs out of the feed so detail hits are realistic.
  let eventIds = [];
  try {
    const sections = home.json('sections') || [];
    for (const s of sections) for (const e of s.events || []) eventIds.push(e.id);
  } catch (_) { /* non-JSON (e.g. 429 body) — skip */ }

  sleep(Math.random() * 2 + 1); // user looks at the feed

  // 2. Categories (cheap, cached)
  const cats = http.get(`${BASE}/v1/categories`, { ...params, tags: { endpoint: 'categories' } });
  check(cats, { 'categories 200': (r) => r.status === 200 });

  sleep(Math.random() * 1.5 + 0.5);

  // 3. Open a random event from the feed: view ping + detail + ticket types
  if (eventIds.length > 0) {
    const id = eventIds[Math.floor(Math.random() * eventIds.length)];

    http.post(`${BASE}/v1/events/${id}/view`, null, { ...params, tags: { endpoint: 'view' } });

    const detail = http.get(`${BASE}/v1/events/${id}`, { ...params, tags: { endpoint: 'event_detail' } });
    detailTrend.add(detail.timings.duration);
    rateLimited.add(detail.status === 429);
    check(detail, { 'detail 200': (r) => r.status === 200 });

    const tiers = http.get(`${BASE}/v1/events/${id}/ticket-types`, {
      ...params, tags: { endpoint: 'ticket_types' },
    });
    check(tiers, { 'tiers 200': (r) => r.status === 200 });
  }

  // 4. Logged-in users check their bookings
  if (auth) {
    http.get(`${BASE}/v1/bookings`, { ...authed, tags: { endpoint: 'bookings' } });
  }

  sleep(Math.random() * 3 + 2); // browse pause before next loop
}
