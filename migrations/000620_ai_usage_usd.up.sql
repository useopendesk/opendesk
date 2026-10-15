-- AI usage is metered and sold in US dollars (contracts/billing.md "Metered usage"):
-- balances, grants, usage rows and auto-recharge amounts are kept in micro-dollars
-- (millionths of a dollar). Every amount converts exactly: 1 credit was $0.01, so one
-- thousandth of a credit is 10 micro-dollars and one credit is 10,000.

ALTER TABLE ai_credits RENAME TO ai_balances;
ALTER INDEX ai_credits_pkey RENAME TO ai_balances_pkey;
ALTER TABLE ai_balances RENAME COLUMN included_used_milli TO included_used_micro_usd;
UPDATE ai_balances SET included_used_micro_usd = included_used_micro_usd * 10, purchased_milli = purchased_milli * 10;
ALTER TABLE ai_balances RENAME COLUMN purchased_milli TO purchased_micro_usd;

ALTER TABLE ai_credit_grants RENAME TO ai_balance_grants;
ALTER INDEX ai_credit_grants_pkey RENAME TO ai_balance_grants_pkey;
ALTER INDEX ai_credit_grants_external_id_key RENAME TO ai_balance_grants_external_id_key;
ALTER TABLE ai_balance_grants ALTER COLUMN credits TYPE bigint USING credits::bigint * 10000;
ALTER TABLE ai_balance_grants RENAME COLUMN credits TO micro_usd;

-- Usage rows keep their history; source 'credits' (the instance's provider) reads 'instance'.
UPDATE ai_usage SET millicredits = millicredits * 10;
ALTER TABLE ai_usage RENAME COLUMN millicredits TO micro_usd;
ALTER TABLE ai_usage DROP CONSTRAINT ai_usage_source_check;
UPDATE ai_usage SET source = 'instance' WHERE source = 'credits';
ALTER TABLE ai_usage ALTER COLUMN source SET DEFAULT 'instance';
ALTER TABLE ai_usage ADD CONSTRAINT ai_usage_source_check CHECK (source IN ('instance', 'own_key'));

-- Auto-recharge: the threshold in micro-dollars (default $2), and top-ups of $10 per
-- recharge. A pack added $20 of usage, so each pack is two top-ups.
ALTER TABLE ai_auto_recharge DROP CONSTRAINT ai_auto_recharge_threshold_check;
ALTER TABLE ai_auto_recharge ALTER COLUMN threshold DROP DEFAULT;
ALTER TABLE ai_auto_recharge ALTER COLUMN threshold TYPE bigint USING threshold::bigint * 10000;
ALTER TABLE ai_auto_recharge RENAME COLUMN threshold TO threshold_micro_usd;
ALTER TABLE ai_auto_recharge ALTER COLUMN threshold_micro_usd SET DEFAULT 2000000;
ALTER TABLE ai_auto_recharge ADD CONSTRAINT ai_auto_recharge_threshold_check CHECK (threshold_micro_usd BETWEEN 10000 AND 1000000000);
ALTER TABLE ai_auto_recharge DROP CONSTRAINT ai_auto_recharge_packs_check;
UPDATE ai_auto_recharge SET packs = LEAST(packs * 2, 20);
ALTER TABLE ai_auto_recharge RENAME COLUMN packs TO top_ups;
ALTER TABLE ai_auto_recharge ADD CONSTRAINT ai_auto_recharge_top_ups_check CHECK (top_ups BETWEEN 1 AND 20);

-- Recharge history keeps how many were bought and the exact usage they added.
ALTER TABLE ai_auto_recharges RENAME COLUMN packs TO top_ups;
ALTER TABLE ai_auto_recharges ALTER COLUMN credits TYPE bigint USING credits::bigint * 10000;
ALTER TABLE ai_auto_recharges RENAME COLUMN credits TO micro_usd;

ALTER TABLE ai_agent_test_runs RENAME COLUMN millicredits TO micro_usd;
UPDATE ai_agent_test_runs SET micro_usd = micro_usd * 10;
ALTER TABLE ai_agent_test_results RENAME COLUMN millicredits TO micro_usd;
UPDATE ai_agent_test_results SET micro_usd = micro_usd * 10;

-- The widget agent's monthly cap, in dollars.
UPDATE workspaces SET settings = jsonb_set(settings, '{ai}',
        ((settings->'ai') - 'widget_monthly_cap')
        || jsonb_build_object('widget_monthly_cap_usd', ((settings->'ai'->>'widget_monthly_cap')::numeric / 100)::float8))
    WHERE jsonb_typeof(settings->'ai'->'widget_monthly_cap') = 'number';
UPDATE ai_agent_test_runs SET settings = (settings - 'widget_monthly_cap')
        || jsonb_build_object('widget_monthly_cap_usd', ((settings->>'widget_monthly_cap')::numeric / 100)::float8)
    WHERE jsonb_typeof(settings->'widget_monthly_cap') = 'number';

-- Notices already sent, under their new names and amounts.
UPDATE notifications SET type = CASE type WHEN 'credits_low' THEN 'ai_balance_low' ELSE 'ai_balance_out' END,
        data = (data - 'remaining') || jsonb_build_object('remaining_usd', (COALESCE((data->>'remaining')::numeric, 0) / 100)::float8)
    WHERE type IN ('credits_low', 'credits_out');
UPDATE notifications SET data = (data - 'cap') || jsonb_build_object('cap_usd', (COALESCE((data->>'cap')::numeric, 0) / 100)::float8)
    WHERE type = 'ai_cap_reached' AND data ? 'cap';
