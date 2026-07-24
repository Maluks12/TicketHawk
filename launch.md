# 1. edit .env.production — sk_test_ → sk_live_, pk_test_ → pk_live_
# 2. null the dead test-mode codes
docker run --rm -i --network host --env-file .env.production postgres:15 \
  sh -c 'psql "$DATABASE_URL"' <<'SQL'
UPDATE organizers
SET paystack_subaccount_code = NULL, paystack_recipient_code = NULL,
    payout_tier = 'held', payout_enabled = FALSE,
    subaccount_invalid_at = NULL
WHERE paystack_subaccount_code IS NOT NULL
RETURNING business_name;
SQL
# 3. restart
docker compose -f docker-compose.prod.yml up -d --build api