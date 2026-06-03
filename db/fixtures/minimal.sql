-- Minimal deterministic test fixture for integration and e2e smoke tests.
--
-- Idempotent by design: safe to rerun against a migrated database. Values are
-- synthetic and intentionally use high bill/source IDs so they do not collide
-- with normal ingested records.

BEGIN;

-- Clean up an early draft of this fixture that used a non-numeric LWS sponsor
-- id. Some application queries sort lws_sponsor_id::int, so fixture legislator
-- IDs must look like real numeric LWS IDs.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM legislator WHERE lws_sponsor_id = 'fixture-legislator-1') THEN
    IF EXISTS (SELECT 1 FROM legislator WHERE lws_sponsor_id = '990001') THEN
      DELETE FROM bill_sponsor WHERE legislator_id IN (SELECT id FROM legislator WHERE lws_sponsor_id = 'fixture-legislator-1');
      DELETE FROM legislator WHERE lws_sponsor_id = 'fixture-legislator-1';
    ELSE
      UPDATE legislator SET lws_sponsor_id = '990001' WHERE lws_sponsor_id = 'fixture-legislator-1';
    END IF;
  END IF;
END $$;

INSERT INTO legislator (
  lws_sponsor_id, name, chamber, district, party, official_url,
  first_name, last_name, email, phone, acronym
) VALUES (
  '990001',
  'Representative Fixture Sponsor',
  'House',
  '99',
  'D',
  'https://example.test/legislators/fixture-sponsor',
  'Fixture',
  'Sponsor',
  'fixture.sponsor@example.test',
  '360-555-0100',
  'FIX'
)
ON CONFLICT (lws_sponsor_id) DO UPDATE SET
  name = EXCLUDED.name,
  chamber = EXCLUDED.chamber,
  district = EXCLUDED.district,
  party = EXCLUDED.party,
  official_url = EXCLUDED.official_url,
  first_name = EXCLUDED.first_name,
  last_name = EXCLUDED.last_name,
  email = EXCLUDED.email,
  phone = EXCLUDED.phone,
  acronym = EXCLUDED.acronym,
  updated_at = NOW();

INSERT INTO bill (
  biennium, prefix, number, title, description, chamber_origin,
  current_status, status_date, official_url
) VALUES (
  '2099-00',
  'HB',
  9001,
  'Fixture Housing Stability Act',
  'Synthetic fixture bill used by integration and end-to-end tests.',
  'House',
  'Public hearing scheduled',
  DATE '2099-01-10',
  'https://example.test/bills/HB9001'
)
ON CONFLICT (biennium, prefix, number) DO UPDATE SET
  title = EXCLUDED.title,
  description = EXCLUDED.description,
  chamber_origin = EXCLUDED.chamber_origin,
  current_status = EXCLUDED.current_status,
  status_date = EXCLUDED.status_date,
  official_url = EXCLUDED.official_url,
  updated_at = NOW();

INSERT INTO bill_sponsor (bill_id, legislator_id, sponsor_type)
SELECT b.id, l.id, 'Primary'
FROM bill b
JOIN legislator l ON l.lws_sponsor_id = '990001'
WHERE b.biennium = '2099-00' AND b.prefix = 'HB' AND b.number = 9001
ON CONFLICT (bill_id, legislator_id, sponsor_type) DO NOTHING;

DELETE FROM bill_status_change
WHERE bill_id IN (SELECT id FROM bill WHERE biennium = '2099-00' AND prefix = 'HB' AND number = 9001);

INSERT INTO bill_status_change (bill_id, action_date, history_line, actor)
SELECT b.id, x.action_date, x.history_line, x.actor
FROM bill b
CROSS JOIN (VALUES
  (DATE '2099-01-08', 'First reading, referred to Housing.', 'House'),
  (DATE '2099-01-10', 'Public hearing in the House Committee on Housing.', 'House')
) AS x(action_date, history_line, actor)
WHERE b.biennium = '2099-00' AND b.prefix = 'HB' AND b.number = 9001
ON CONFLICT (bill_id, action_date, history_line) DO NOTHING;

INSERT INTO tvw_event (
  tvw_event_id, wp_post_id, wp_slug, wp_link, title, description,
  start_datetime, caption_url, thumbnail_url, custom_id, location_name,
  total_runtime, total_runtime_seconds, published_audio_url,
  audio_download_url, video_download_url, streaming_uris, raw_categories,
  raw_keywords, raw_wp_tags, raw_wp_categories
) VALUES (
  'fixture-tvw-event-9001',
  9001,
  'fixture-tvw-event-9001',
  'https://example.test/tvw/fixture-tvw-event-9001',
  'House Housing Committee - Fixture Hearing',
  'Synthetic TVW event for test fixtures.',
  TIMESTAMPTZ '2099-01-10 17:30:00+00',
  'https://example.test/captions/fixture-tvw-event-9001.vtt',
  'https://example.test/thumbs/fixture-tvw-event-9001.jpg',
  'fixture-custom-9001',
  'House Hearing Room A',
  '00:10:00',
  600,
  'https://example.test/audio/fixture-tvw-event-9001.mp3',
  'https://example.test/audio-download/fixture-tvw-event-9001.mp3',
  'https://example.test/video/fixture-tvw-event-9001.mp4',
  '{}'::jsonb,
  ARRAY['fixture', 'housing'],
  ARRAY['fixture', 'housing'],
  '[]'::jsonb,
  '[]'::jsonb
)
ON CONFLICT (tvw_event_id) DO UPDATE SET
  title = EXCLUDED.title,
  description = EXCLUDED.description,
  start_datetime = EXCLUDED.start_datetime,
  caption_url = EXCLUDED.caption_url,
  thumbnail_url = EXCLUDED.thumbnail_url,
  wp_slug = EXCLUDED.wp_slug,
  wp_link = EXCLUDED.wp_link,
  custom_id = EXCLUDED.custom_id,
  location_name = EXCLUDED.location_name,
  total_runtime = EXCLUDED.total_runtime,
  total_runtime_seconds = EXCLUDED.total_runtime_seconds,
  published_audio_url = EXCLUDED.published_audio_url,
  audio_download_url = EXCLUDED.audio_download_url,
  video_download_url = EXCLUDED.video_download_url,
  streaming_uris = EXCLUDED.streaming_uris,
  raw_categories = EXCLUDED.raw_categories,
  raw_keywords = EXCLUDED.raw_keywords,
  raw_wp_tags = EXCLUDED.raw_wp_tags,
  raw_wp_categories = EXCLUDED.raw_wp_categories,
  updated_at = NOW();

INSERT INTO hearing (
  bill_id, committee_name, committee_acronym, chamber, meeting_datetime,
  location, lws_meeting_id, committee_schedule_agenda_id,
  committee_schedule_video_id, tvw_event_id, official_agenda_url,
  tvw_url
)
SELECT
  b.id,
  'House Committee on Housing',
  'HOUS',
  'House',
  TIMESTAMPTZ '2099-01-10 17:30:00+00',
  'House Hearing Room A',
  'fixture-lws-meeting-9001',
  'fixture-agenda-9001',
  'fixture-video-9001',
  'fixture-tvw-event-9001',
  'https://example.test/agendas/fixture-9001',
  'https://example.test/tvw/fixture-tvw-event-9001'
FROM bill b
WHERE b.biennium = '2099-00' AND b.prefix = 'HB' AND b.number = 9001
  AND NOT EXISTS (
    SELECT 1 FROM hearing h
    WHERE h.chamber = 'House'
      AND h.committee_name = 'House Committee on Housing'
      AND h.meeting_datetime = TIMESTAMPTZ '2099-01-10 17:30:00+00'
  );

UPDATE hearing h
SET bill_id = b.id,
    committee_acronym = 'HOUS',
    location = 'House Hearing Room A',
    lws_meeting_id = 'fixture-lws-meeting-9001',
    committee_schedule_agenda_id = 'fixture-agenda-9001',
    committee_schedule_video_id = 'fixture-video-9001',
    tvw_event_id = 'fixture-tvw-event-9001',
    official_agenda_url = 'https://example.test/agendas/fixture-9001',
    tvw_url = 'https://example.test/tvw/fixture-tvw-event-9001',
    updated_at = NOW()
FROM bill b
WHERE h.chamber = 'House'
  AND h.committee_name = 'House Committee on Housing'
  AND h.meeting_datetime = TIMESTAMPTZ '2099-01-10 17:30:00+00'
  AND b.biennium = '2099-00' AND b.prefix = 'HB' AND b.number = 9001;

INSERT INTO agenda_item (
  hearing_id, bill_id, label, csi_meeting_family_id,
  csi_agenda_item_family_id, csi_agenda_item_id, order_index
)
SELECT
  h.id,
  b.id,
  'HB 9001 Fixture Housing Stability Act',
  'fixture-meeting-family-9001',
  'fixture-agenda-family-9001',
  'fixture-agenda-item-9001',
  1
FROM hearing h
JOIN bill b ON b.biennium = '2099-00' AND b.prefix = 'HB' AND b.number = 9001
WHERE h.chamber = 'House'
  AND h.committee_name = 'House Committee on Housing'
  AND h.meeting_datetime = TIMESTAMPTZ '2099-01-10 17:30:00+00'
ON CONFLICT (csi_agenda_item_id) DO UPDATE SET
  hearing_id = EXCLUDED.hearing_id,
  bill_id = EXCLUDED.bill_id,
  label = EXCLUDED.label,
  csi_meeting_family_id = EXCLUDED.csi_meeting_family_id,
  csi_agenda_item_family_id = EXCLUDED.csi_agenda_item_family_id,
  order_index = EXCLUDED.order_index;

INSERT INTO organization (canonical_name, aliases, match_confidence, match_notes)
VALUES (
  'Fixture Housing Coalition',
  ARRAY['Fixture Housing Coalition', 'FHC'],
  'confirmed',
  'Synthetic organization used by deterministic test fixtures.'
)
ON CONFLICT (canonical_name) DO UPDATE SET
  aliases = EXCLUDED.aliases,
  match_confidence = EXCLUDED.match_confidence,
  match_notes = EXCLUDED.match_notes,
  updated_at = NOW();

DELETE FROM testifier
WHERE agenda_item_id IN (
  SELECT id FROM agenda_item WHERE csi_agenda_item_id = 'fixture-agenda-item-9001'
);

INSERT INTO testifier (
  agenda_item_id, raw_name, raw_organization, normalized_org_id,
  position, testified, time_signed_in
)
SELECT
  a.id,
  x.raw_name,
  x.raw_organization,
  o.id,
  x.position::testifier_position,
  x.testified,
  x.time_signed_in
FROM agenda_item a
JOIN organization o ON o.canonical_name = 'Fixture Housing Coalition'
CROSS JOIN (VALUES
  ('Alex Fixture', 'Fixture Housing Coalition', 'Pro', TRUE, TIMESTAMPTZ '2099-01-10 17:40:00+00'),
  ('Jordan Sample', 'Fixture Housing Coalition', 'Con', FALSE, TIMESTAMPTZ '2099-01-10 17:42:00+00')
) AS x(raw_name, raw_organization, position, testified, time_signed_in)
WHERE a.csi_agenda_item_id = 'fixture-agenda-item-9001';

DELETE FROM diarized_speech_segment
WHERE tvw_event_id = 'fixture-tvw-event-9001';
DELETE FROM diarization_job
WHERE tvw_event_id = 'fixture-tvw-event-9001';

INSERT INTO diarization_job (
  tvw_event_id, provider, model, status, submitted_at, finished_at
) VALUES (
  'fixture-tvw-event-9001', 'fixture', 'fixture-model', 'succeeded',
  TIMESTAMPTZ '2099-01-10 17:30:00+00', TIMESTAMPTZ '2099-01-10 17:45:00+00'
);

INSERT INTO diarized_speech_segment (
  diarization_job_id, tvw_event_id, cluster_label, start_ms, end_ms, text
)
SELECT j.id, 'fixture-tvw-event-9001', x.cluster_label, x.start_ms, x.end_ms, x.text
FROM diarization_job j
CROSS JOIN (VALUES
  ('speaker_0', 1000,  5000,  'We are opening testimony on HB 9001, the fixture housing stability act.'),
  ('speaker_1', 6000,  11000, 'This housing fixture bill helps renters and homeowners understand the test flow.'),
  ('speaker_2', 12000, 16000, 'Members discuss housing supply, affordability, and fixture data quality.')
) AS x(cluster_label, start_ms, end_ms, text)
WHERE j.tvw_event_id = 'fixture-tvw-event-9001';

COMMIT;
