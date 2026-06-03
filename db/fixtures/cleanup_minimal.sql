-- Remove deterministic test fixture data inserted by db/fixtures/minimal.sql.
--
-- Keep this scoped to fixture-specific stable keys so it is safe to run
-- against a local database that also contains real ingested records.

BEGIN;

DELETE FROM agenda_item_window
WHERE agenda_item_id IN (
  SELECT id FROM agenda_item WHERE csi_agenda_item_id = 'fixture-agenda-item-9001'
);

DELETE FROM testifier
WHERE agenda_item_id IN (
  SELECT id FROM agenda_item WHERE csi_agenda_item_id = 'fixture-agenda-item-9001'
);

DELETE FROM person_organization_affiliation
WHERE organization_id IN (
  SELECT id FROM organization
  WHERE canonical_name = 'Fixture Housing Coalition'
    AND match_notes = 'Synthetic organization used by deterministic test fixtures.'
)
   OR raw_organization_name = 'Fixture Housing Coalition'
   OR raw_person_name IN ('Alex Fixture', 'Jordan Sample');

DELETE FROM vendor_entity_match_decision
WHERE candidate_id IN (
  SELECT c.id
  FROM vendor_entity_match_candidate c
  JOIN organization o ON o.id = c.organization_id
  WHERE o.canonical_name = 'Fixture Housing Coalition'
    AND o.match_notes = 'Synthetic organization used by deterministic test fixtures.'
);

DELETE FROM vendor_entity_match_candidate
WHERE organization_id IN (
  SELECT id FROM organization
  WHERE canonical_name = 'Fixture Housing Coalition'
    AND match_notes = 'Synthetic organization used by deterministic test fixtures.'
);

DELETE FROM speaker_assignment
WHERE diarization_job_id IN (
  SELECT id FROM diarization_job WHERE tvw_event_id = 'fixture-tvw-event-9001'
);

DELETE FROM speaker_identity_evidence
WHERE diarization_job_id IN (
  SELECT id FROM diarization_job WHERE tvw_event_id = 'fixture-tvw-event-9001'
);

DELETE FROM speaker_review_task
WHERE diarization_job_id IN (
  SELECT id FROM diarization_job WHERE tvw_event_id = 'fixture-tvw-event-9001'
);

DELETE FROM speaker_cluster
WHERE diarization_job_id IN (
  SELECT id FROM diarization_job WHERE tvw_event_id = 'fixture-tvw-event-9001'
);

DELETE FROM diarized_speech_segment
WHERE tvw_event_id = 'fixture-tvw-event-9001';

DELETE FROM diarization_job
WHERE tvw_event_id = 'fixture-tvw-event-9001';

DELETE FROM tvw_audio_asset
WHERE tvw_event_id = 'fixture-tvw-event-9001';

DELETE FROM tvw_media_asset
WHERE tvw_event_id = 'fixture-tvw-event-9001';

DELETE FROM agenda_item
WHERE csi_agenda_item_id = 'fixture-agenda-item-9001';

DELETE FROM hearing
WHERE lws_meeting_id = 'fixture-lws-meeting-9001'
   OR committee_schedule_agenda_id = 'fixture-agenda-9001'
   OR committee_schedule_video_id = 'fixture-video-9001'
   OR tvw_event_id = 'fixture-tvw-event-9001';

DELETE FROM bill_status_change
WHERE bill_id IN (
  SELECT id FROM bill WHERE biennium = '2099-00' AND prefix = 'HB' AND number = 9001
);

DELETE FROM bill_sponsor
WHERE bill_id IN (
  SELECT id FROM bill WHERE biennium = '2099-00' AND prefix = 'HB' AND number = 9001
)
   OR legislator_id IN (
     SELECT id FROM legislator
     WHERE lws_sponsor_id = '990001'
       AND name = 'Representative Fixture Sponsor'
   );

DELETE FROM bill
WHERE biennium = '2099-00' AND prefix = 'HB' AND number = 9001;

DELETE FROM organization
WHERE canonical_name = 'Fixture Housing Coalition'
  AND match_notes = 'Synthetic organization used by deterministic test fixtures.';

DELETE FROM tvw_event
WHERE tvw_event_id = 'fixture-tvw-event-9001';

DELETE FROM legislator_roster_membership
WHERE lws_sponsor_id = '990001'
  AND roster_name = 'Representative Fixture Sponsor';

DELETE FROM legislator
WHERE lws_sponsor_id = '990001'
  AND name = 'Representative Fixture Sponsor';

DELETE FROM source_record
WHERE source_system = 'lws'
  AND source_endpoint = 'Fixture.Minimal'
  AND source_url = 'fixture://wa-dd/minimal'
  AND content_hash = 'fixture-minimal-v1'
  AND transform_version = 'fixture-v1';

COMMIT;
