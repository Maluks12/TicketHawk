// TicketHawk load test — DIRECT (on-EC2, bypasses Cloudflare tunnel).
//
// Purpose: isolate whether the minute-long latencies seen from the PC test are
// caused by the Cloudflare Tunnel / single-IP funnel, or by the Go+Postgres
// backend itself. This script runs ON the EC2 host and hits the API's local
// bind (127.0.0.1:8080) directly, so neither Cloudflare nor your home IP is in
// the path. If latencies are healthy here but terrible via the tunnel, the
// tunnel is the bottleneck. If they're bad here too, look at the DB pool.
//
// Run ON EC2:
//   1) install k6 once (see instructions in chat)
//   2) LOAD_TEST_MODE=true must still be enabled on the api container
//   3) k6 run k6-direct-ec2.js
//
// Read-only: no bookings created, no Paystack, no rows left behind.

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Rate } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://localhost:8080';

export const options = {
  stages: [
    { duration: '1m', target: 100 },
    { duration: '2m', target: 500 },
    { duration: '5m', target: 500 },
    { duration: '1m', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.02'],
    http_req_duration: ['p(95)<1500'],
    'http_req_duration{endpoint:home}': ['p(95)<2000'],
  },
};

const homeTrend = new Trend('home_ms');
const detailTrend = new Trend('event_detail_ms');

export default function () {
  const params = { headers: { 'Content-Type': 'application/json' }, tags: {} };

  const home = http.get(`${BASE}/v1/home`, { ...params, tags: { endpoint: 'home' } });
  homeTrend.add(home.timings.duration);
  check(home, { 'home 200': (r) => r.status === 200 });

  let eventIds = [];
  try {
    const sections = home.json('sections') || [];
    for (const s of sections) for (const e of s.events || []) eventIds.push(e.id);
  } catch (_) { /* skip */ }

  sleep(Math.random() * 2 + 1);

  const cats = http.get(`${BASE}/v1/categories`, { ...params, tags: { endpoint: 'categories' } });
  check(cats, { 'categories 200': (r) => r.status === 200 });

  sleep(Math.random() * 1.5 + 0.5);

  if (eventIds.length > 0) {
    const id = eventIds[Math.floor(Math.random() * eventIds.length)];
    http.post(`${BASE}/v1/events/${id}/view`, null, { ...params, tags: { endpoint: 'view' } });

    const detail = http.get(`${BASE}/v1/events/${id}`, { ...params, tags: { endpoint: 'event_detail' } });
    detailTrend.add(detail.timings.duration);
    check(detail, { 'detail 200': (r) => r.status === 200 });

    const tiers = http.get(`${BASE}/v1/events/${id}/ticket-types`, {
      ...params, tags: { endpoint: 'ticket_types' },
    });
    check(tiers, { 'tiers 200': (r) => r.status === 200 });
  }

  sleep(Math.random() * 3 + 2);
}