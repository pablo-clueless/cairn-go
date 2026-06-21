-- 0010_sprint_weekday_start: snap existing sprint start dates off weekends.
--
-- Sprints must start on a weekday (Mon–Fri); the sprint dialogs now enforce this
-- going forward. This one-time backfill moves any existing weekend start date
-- back to the preceding Friday. The weekday is evaluated in the app's primary
-- timezone (Africa/Lagos, WAT/UTC+1, no DST) so it matches the day the calendar
-- renders for each stored instant. Subtracting whole days shifts the wall-clock
-- date back without disturbing the time component.

UPDATE sprints
SET start_date = CASE EXTRACT(ISODOW FROM (start_date AT TIME ZONE 'Africa/Lagos'))
        WHEN 6 THEN start_date - INTERVAL '1 day'  -- Saturday → Friday
        WHEN 7 THEN start_date - INTERVAL '2 days'  -- Sunday   → Friday
        ELSE start_date
    END,
    updated_at = now()
WHERE start_date IS NOT NULL
  AND EXTRACT(ISODOW FROM (start_date AT TIME ZONE 'Africa/Lagos')) IN (6, 7);
