-- Restore the original invariant (organizer_fee included in total_amount).
--
-- NOTE: this rollback will FAIL if any bookings created under the new fee model
-- already exist, because their total_amount excludes organizer_fee and would
-- violate the restored constraint. That is expected — do not force it without
-- reconciling those rows first.
ALTER TABLE bookings DROP CONSTRAINT IF EXISTS chk_total_calculation;

ALTER TABLE bookings
    ADD CONSTRAINT chk_total_calculation CHECK (
        total_amount = subtotal + platform_fee + organizer_fee - discount_amount
    );