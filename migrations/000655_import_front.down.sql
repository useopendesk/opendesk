ALTER TABLE imports DROP CONSTRAINT imports_source_check;
ALTER TABLE imports ADD CONSTRAINT imports_source_check
    CHECK (source IN ('zendesk', 'helpscout', 'freshdesk', 'intercom', 'csv'));
