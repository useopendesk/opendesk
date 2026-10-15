ALTER TABLE subscriptions DROP COLUMN IF EXISTS pm_wallet;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS pm_exp_year;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS pm_exp_month;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS pm_last4;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS pm_brand;

DROP INDEX IF EXISTS ai_auto_recharges_kind_month;
ALTER TABLE ai_auto_recharges ADD COLUMN top_ups integer NOT NULL DEFAULT 1;
UPDATE ai_auto_recharges SET top_ups = GREATEST(1, LEAST(20, micro_usd / 10000000))::integer;
ALTER TABLE ai_auto_recharges ALTER COLUMN top_ups DROP DEFAULT;
DELETE FROM ai_auto_recharges WHERE kind = 'top_up';
ALTER TABLE ai_auto_recharges DROP COLUMN retry_at;
ALTER TABLE ai_auto_recharges DROP COLUMN tries;
ALTER TABLE ai_auto_recharges DROP COLUMN invoice_id;
ALTER TABLE ai_auto_recharges DROP COLUMN kind;

ALTER TABLE ai_auto_recharge ALTER COLUMN threshold_micro_usd SET DEFAULT 2000000;
ALTER TABLE ai_auto_recharge ADD COLUMN top_ups integer NOT NULL DEFAULT 1 CHECK (top_ups BETWEEN 1 AND 20);
ALTER TABLE ai_auto_recharge ADD COLUMN monthly_limit integer NOT NULL DEFAULT 5 CHECK (monthly_limit BETWEEN 1 AND 30);
UPDATE ai_auto_recharge SET top_ups = GREATEST(1, LEAST(20, amount_micro_usd / 10000000))::integer;
UPDATE ai_auto_recharge SET monthly_limit = GREATEST(1, LEAST(30, monthly_limit_micro_usd / amount_micro_usd))::integer;
ALTER TABLE ai_auto_recharge DROP COLUMN monthly_limit_micro_usd;
ALTER TABLE ai_auto_recharge DROP COLUMN amount_micro_usd;
