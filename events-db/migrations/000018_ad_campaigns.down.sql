-- migrations/000018_ad_campaigns.down.sql

DROP TABLE IF EXISTS ad_daily_spend    CASCADE;
DROP TABLE IF EXISTS ad_campaigns      CASCADE;
DROP TABLE IF EXISTS ad_wallet_topups  CASCADE;
DROP TABLE IF EXISTS ad_wallets        CASCADE;
DROP SEQUENCE IF EXISTS ad_topup_ref_seq;
DROP TYPE IF EXISTS campaign_status;
DROP TYPE IF EXISTS topup_status;