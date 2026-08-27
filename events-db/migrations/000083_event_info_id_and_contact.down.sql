-- Reverse of 000083.
--
-- Values stored against id_required survive as 'Yes' / 'No' strings, which are
-- valid prose for a text field, so reverting the type needs no data cleanup in
-- this direction.

-- id_accepted rows must go before the catalogue row: info_key references the
-- catalogue with ON DELETE RESTRICT, so deleting the entry while rows point at
-- it fails with 23503 rather than cascading.
DELETE FROM event_info_items      WHERE info_key = 'id_accepted';
DELETE FROM event_info_catalogue  WHERE key      = 'id_accepted';

-- enum_options must be nulled in the same statement as the type change:
-- chk_info_enum_options rejects a non-enum row that still carries options.
UPDATE event_info_catalogue
SET    value_type   = 'text',
       enum_options = NULL,
       hint         = 'Will attendees need to show identification at the door?'
WHERE  key = 'id_required';

UPDATE event_info_catalogue
SET    is_active = TRUE
WHERE  key = 'organiser_contact';