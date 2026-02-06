ALTER TABLE monitor.events ADD COLUMN IF NOT EXISTS user_id String AFTER trace_id;
ALTER TABLE monitor.events ADD INDEX IF NOT EXISTS idx_user_id user_id TYPE bloom_filter(0.01) GRANULARITY 4;
