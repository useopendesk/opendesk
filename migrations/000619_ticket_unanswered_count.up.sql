-- How many public messages the customer sent since the last outbound public reply:
-- what a person has to catch up on. 0 once someone (or the AI agent) replied.
ALTER TABLE tickets ADD COLUMN unanswered_count integer NOT NULL DEFAULT 0;

UPDATE tickets t SET unanswered_count = w.n
FROM (
    SELECT m.ticket_id, count(*) AS n
    FROM messages m
    WHERE m.kind = 'reply' AND m.direction = 'inbound'
      AND m.created_at > COALESCE((SELECT max(o.created_at) FROM messages o
          WHERE o.ticket_id = m.ticket_id AND o.kind = 'reply' AND o.direction = 'outbound'), '-infinity')
    GROUP BY m.ticket_id
) w
WHERE t.id = w.ticket_id AND t.waiting_since IS NOT NULL;
