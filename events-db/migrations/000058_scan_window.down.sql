ALTER TABLE events DROP CONSTRAINT IF EXISTS chk_scan_offsets;
ALTER TABLE events DROP COLUMN IF EXISTS scan_open_offset_min;
ALTER TABLE events DROP COLUMN IF EXISTS scan_close_offset_min;