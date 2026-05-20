-- +goose Up
-- +goose StatementBegin

-- agenda_item_window persists the bill-discussion windows that
-- SegmentTranscript decides on, so downstream readers (the bundle
-- assembler, future search-result deep-links) read back exactly what
-- the segmenter wrote — no re-deriving in SQL with a different
-- threshold. Replaces the in-SQL window query that previously lived
-- in the page assemblers.
--
-- One window is one [start_ms, end_ms] span on the same TVW event
-- where a particular bill is being discussed. Most agenda items have
-- exactly one; bills that get revisited later in the same hearing
-- (e.g. exec session after public testimony on a different bill in
-- between) get multiple.

CREATE TABLE agenda_item_window (
    id              BIGSERIAL PRIMARY KEY,
    agenda_item_id  BIGINT NOT NULL REFERENCES agenda_item(id) ON DELETE CASCADE,
    start_ms        INT NOT NULL,
    end_ms          INT NOT NULL,
    mentions        INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (end_ms > start_ms)
);

CREATE INDEX idx_agenda_item_window_agenda ON agenda_item_window (agenda_item_id, start_ms);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_agenda_item_window_agenda;
DROP TABLE IF EXISTS agenda_item_window;

-- +goose StatementEnd
