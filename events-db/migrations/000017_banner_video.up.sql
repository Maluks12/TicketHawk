-- Add optional banner video URL to events.
-- Organisers may attach one video to the banner section alongside (or instead
-- of) the banner image. Keeping it as a nullable TEXT column mirrors the
-- existing banner_image_url pattern and requires no schema gymnastics.

ALTER TABLE events
    ADD COLUMN IF NOT EXISTS banner_video_url TEXT;