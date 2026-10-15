UPDATE notifications SET data = (data - 'cap_usd') || jsonb_build_object('cap', round(COALESCE((data->>'cap_usd')::numeric, 0) * 100)::int)
    WHERE type = 'ai_cap_reached' AND data ? 'cap_usd';
UPDATE notifications SET type = CASE type WHEN 'ai_balance_low' THEN 'credits_low' ELSE 'credits_out' END,
        data = (data - 'remaining_usd') || jsonb_build_object('remaining', (round(COALESCE((data->>'remaining_usd')::numeric, 0) * 100, 3))::float8)
    WHERE type IN ('ai_balance_low', 'ai_balance_out');

UPDATE ai_agent_test_runs SET settings = (settings - 'widget_monthly_cap_usd')
        || jsonb_build_object('widget_monthly_cap', ceil((settings->>'widget_monthly_cap_usd')::numeric * 100)::int)
    WHERE jsonb_typeof(settings->'widget_monthly_cap_usd') = 'number';
UPDATE workspaces SET settings = jsonb_set(settings, '{ai}',
        ((settings->'ai') - 'widget_monthly_cap_usd')
        || jsonb_build_object('widget_monthly_cap', ceil((settings->'ai'->>'widget_monthly_cap_usd')::numeric * 100)::int))
    WHERE jsonb_typeof(settings->'ai'->'widget_monthly_cap_usd') = 'number';

UPDATE ai_agent_test_results SET micro_usd = ceil(micro_usd / 10.0);
ALTER TABLE ai_agent_test_results RENAME COLUMN micro_usd TO millicredits;
UPDATE ai_agent_test_runs SET micro_usd = ceil(micro_usd / 10.0);
ALTER TABLE ai_agent_test_runs RENAME COLUMN micro_usd TO millicredits;

ALTER TABLE ai_auto_recharges RENAME COLUMN micro_usd TO credits;
ALTER TABLE ai_auto_recharges ALTER COLUMN credits TYPE integer USING ceil(credits / 10000.0)::integer;
ALTER TABLE ai_auto_recharges RENAME COLUMN top_ups TO packs;

ALTER TABLE ai_auto_recharge DROP CONSTRAINT ai_auto_recharge_top_ups_check;
ALTER TABLE ai_auto_recharge RENAME COLUMN top_ups TO packs;
UPDATE ai_auto_recharge SET packs = GREATEST(ceil(packs / 2.0)::integer, 1);
ALTER TABLE ai_auto_recharge ADD CONSTRAINT ai_auto_recharge_packs_check CHECK (packs BETWEEN 1 AND 20);
ALTER TABLE ai_auto_recharge DROP CONSTRAINT ai_auto_recharge_threshold_check;
ALTER TABLE ai_auto_recharge ALTER COLUMN threshold_micro_usd DROP DEFAULT;
ALTER TABLE ai_auto_recharge RENAME COLUMN threshold_micro_usd TO threshold;
ALTER TABLE ai_auto_recharge ALTER COLUMN threshold TYPE integer USING LEAST(GREATEST(ceil(threshold / 10000.0), 1), 100000)::integer;
ALTER TABLE ai_auto_recharge ALTER COLUMN threshold SET DEFAULT 200;
ALTER TABLE ai_auto_recharge ADD CONSTRAINT ai_auto_recharge_threshold_check CHECK (threshold BETWEEN 1 AND 100000);

ALTER TABLE ai_usage DROP CONSTRAINT ai_usage_source_check;
UPDATE ai_usage SET source = 'credits' WHERE source = 'instance';
ALTER TABLE ai_usage ALTER COLUMN source SET DEFAULT 'credits';
ALTER TABLE ai_usage ADD CONSTRAINT ai_usage_source_check CHECK (source IN ('credits', 'own_key'));
ALTER TABLE ai_usage RENAME COLUMN micro_usd TO millicredits;
UPDATE ai_usage SET millicredits = ceil(millicredits / 10.0);

ALTER TABLE ai_balance_grants RENAME COLUMN micro_usd TO credits;
ALTER TABLE ai_balance_grants ALTER COLUMN credits TYPE integer USING floor(credits / 10000.0)::integer;
ALTER INDEX ai_balance_grants_external_id_key RENAME TO ai_credit_grants_external_id_key;
ALTER INDEX ai_balance_grants_pkey RENAME TO ai_credit_grants_pkey;
ALTER TABLE ai_balance_grants RENAME TO ai_credit_grants;

ALTER TABLE ai_balances RENAME COLUMN purchased_micro_usd TO purchased_milli;
UPDATE ai_balances SET included_used_micro_usd = ceil(included_used_micro_usd / 10.0), purchased_milli = floor(purchased_milli / 10.0);
ALTER TABLE ai_balances RENAME COLUMN included_used_micro_usd TO included_used_milli;
ALTER INDEX ai_balances_pkey RENAME TO ai_credits_pkey;
ALTER TABLE ai_balances RENAME TO ai_credits;
