DROP TABLE IF EXISTS push_devices;
DROP TABLE IF EXISTS app_attest_challenges;
DROP TABLE IF EXISTS app_attest_keys;
ALTER TABLE sessions DROP COLUMN IF EXISTS client;
