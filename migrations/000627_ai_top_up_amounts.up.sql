-- AI top-ups and auto-recharge in US dollars of any amount (contracts/billing.md
-- "AI top-ups" and "Auto-recharge"): a recharge adds a dollar amount, capped by a dollar
-- amount per calendar month.
ALTER TABLE ai_auto_recharge ADD COLUMN amount_micro_usd bigint NOT NULL DEFAULT 25000000
    CHECK (amount_micro_usd BETWEEN 5000000 AND 5000000000);
ALTER TABLE ai_auto_recharge ADD COLUMN monthly_limit_micro_usd bigint NOT NULL DEFAULT 250000000
    CHECK (monthly_limit_micro_usd BETWEEN 5000000 AND 50000000000);
UPDATE ai_auto_recharge SET amount_micro_usd = top_ups::bigint * 10000000,
    monthly_limit_micro_usd = GREATEST(top_ups::bigint * monthly_limit * 10000000, 5000000);
ALTER TABLE ai_auto_recharge DROP COLUMN top_ups;
ALTER TABLE ai_auto_recharge DROP COLUMN monthly_limit;
ALTER TABLE ai_auto_recharge ALTER COLUMN threshold_micro_usd SET DEFAULT 5000000;

-- Every charge on the saved payment method: auto-recharges and top-ups paid in the app.
-- Each is a Stripe invoice, so the customer gets one for every charge.
ALTER TABLE ai_auto_recharges ADD COLUMN kind text NOT NULL DEFAULT 'auto_recharge' CHECK (kind IN ('auto_recharge', 'top_up'));
ALTER TABLE ai_auto_recharges ADD COLUMN invoice_id text UNIQUE;
-- Payment tries so far; a soft decline is tried again later, a few times.
ALTER TABLE ai_auto_recharges ADD COLUMN tries integer NOT NULL DEFAULT 0;
ALTER TABLE ai_auto_recharges ADD COLUMN retry_at timestamptz;
ALTER TABLE ai_auto_recharges DROP COLUMN top_ups;
CREATE INDEX ai_auto_recharges_kind_month ON ai_auto_recharges (workspace_id, kind, created_at DESC);

-- The payment method charges use (the customer's default, else the newest), as shown in
-- the app: brand, last four digits and expiry only.
ALTER TABLE subscriptions ADD COLUMN pm_brand text;
ALTER TABLE subscriptions ADD COLUMN pm_last4 text;
ALTER TABLE subscriptions ADD COLUMN pm_exp_month integer;
ALTER TABLE subscriptions ADD COLUMN pm_exp_year integer;
ALTER TABLE subscriptions ADD COLUMN pm_wallet text;
