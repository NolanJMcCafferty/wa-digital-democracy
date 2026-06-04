-- +goose Up

CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;
CREATE EXTENSION IF NOT EXISTS postgis;

-- Schema below is the consolidated result of legacy migrations 0001-0036
-- collapsed into a single initial migration on 2026-06-04. Generated via
-- pg_dump --schema-only against a freshly migrated database, with
-- pg_dump preamble and goose_db_version blocks stripped.

--
-- PostgreSQL database dump
--


-- Dumped from database version 16.14
-- Dumped by pg_dump version 16.14


--
-- Name: public; Type: SCHEMA; Schema: -; Owner: -
--



--
-- Name: diarization_job_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.diarization_job_status AS ENUM (
    'pending',
    'running',
    'succeeded',
    'failed'
);


--
-- Name: entity_match_decision; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.entity_match_decision AS ENUM (
    'confirmed',
    'rejected',
    'needs_review'
);


--
-- Name: entity_match_source_kind; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.entity_match_source_kind AS ENUM (
    'datawa_contract_contractor',
    'datawa_master_contract_vendor',
    'datawa_master_contract_customer',
    'datawa_it_contract_contractor',
    'datawa_it_contract_dba',
    'datawa_webs_vendor',
    'pdc_lobbying_organization',
    'organization_alias',
    'fiscalwa_vendor_payment',
    'federal_award_recipient',
    'deepgram_organization_mention',
    'csi_testimony_organization'
);


--
-- Name: org_match_confidence; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.org_match_confidence AS ENUM (
    'confirmed',
    'probable',
    'possible',
    'unmatched'
);


--
-- Name: person_match_confidence; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.person_match_confidence AS ENUM (
    'confirmed',
    'probable',
    'possible',
    'unmatched'
);


--
-- Name: person_org_affiliation_type; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.person_org_affiliation_type AS ENUM (
    'signed_in_for',
    'testified_for',
    'lobbyist_for',
    'lobbying_firm_for',
    'paid_lobbying_for',
    'paid_by',
    'employed_by',
    'vendor_contact_for',
    'campaign_contributor_affiliation',
    'spoke_for_org',
    'reviewed_manual'
);


--
-- Name: person_review_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.person_review_status AS ENUM (
    'confirmed',
    'rejected',
    'needs_review',
    'auto'
);


--
-- Name: person_source_kind; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.person_source_kind AS ENUM (
    'csi_testifier',
    'pdc_lobbyist_employment',
    'pdc_lobbyist_compensation',
    'pdc_contribution',
    'webs_vendor_contact',
    'deepgram_speaker',
    'legislator_roster',
    'manual_import'
);


--
-- Name: speaker_candidate_kind; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.speaker_candidate_kind AS ENUM (
    'legislator',
    'testifier',
    'person',
    'unknown'
);


--
-- Name: speaker_evidence_type; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.speaker_evidence_type AS ENUM (
    'self_introduction',
    'chair_call',
    'entity_mention',
    'csi_order',
    'manual_note'
);


--
-- Name: speaker_review_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.speaker_review_status AS ENUM (
    'pending',
    'accepted',
    'rejected',
    'needs_more_evidence',
    'superseded'
);


--
-- Name: testifier_position; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.testifier_position AS ENUM (
    'Pro',
    'Con',
    'Other',
    'Unknown'
);


--
-- Name: wa_dd_normalize_entity_name(text); Type: FUNCTION; Schema: public; Owner: -
--

-- +goose StatementBegin
CREATE FUNCTION public.wa_dd_normalize_entity_name(raw text) RETURNS text
    LANGUAGE sql IMMUTABLE
    AS $$
    WITH tokens AS (
        SELECT token
          FROM unnest(
                 regexp_split_to_array(
                   upper(regexp_replace(coalesce(raw, ''), '&', ' AND ', 'g')),
                   '[^A-Z0-9]+'
                 )
               ) token
         WHERE token <> ''
    ), expanded AS (
        SELECT CASE token
                   WHEN 'WA'    THEN 'WASHINGTON'
                   WHEN 'DEPT'  THEN 'DEPARTMENT'
                   WHEN 'DEPTS' THEN 'DEPARTMENTS'
                   WHEN 'INTL'  THEN 'INTERNATIONAL'
                   WHEN 'NATL'  THEN 'NATIONAL'
                   WHEN 'NAT'   THEN 'NATIONAL'
                   WHEN 'COMM'  THEN 'COMMISSION'
                   WHEN 'COMMN' THEN 'COMMISSION'
                   WHEN 'FED'   THEN 'FEDERATION'
                   WHEN 'FEDN'  THEN 'FEDERATION'
                   WHEN 'GOVT'  THEN 'GOVERNMENT'
                   WHEN 'GOV'   THEN 'GOVERNMENT'
                   WHEN 'UNIV'  THEN 'UNIVERSITY'
                   WHEN 'COLL'  THEN 'COLLEGE'
                   WHEN 'INST'  THEN 'INSTITUTE'
                   WHEN 'SVC'   THEN 'SERVICES'
                   WHEN 'SVCS'  THEN 'SERVICES'
                   WHEN 'SERV'  THEN 'SERVICES'
                   WHEN 'SERVS' THEN 'SERVICES'
                   WHEN 'TECH'  THEN 'TECHNOLOGIES'
                   WHEN 'TECHS' THEN 'TECHNOLOGIES'
                   WHEN 'COUNCL' THEN 'COUNCIL'
                   WHEN 'CNTR'  THEN 'CENTER'
                   WHEN 'CTR'   THEN 'CENTER'
                   WHEN 'CTRS'  THEN 'CENTERS'
                   WHEN 'BRO'   THEN 'BROTHERHOOD'
                   WHEN 'BROS'  THEN 'BROTHERS'
                   WHEN 'INDUS' THEN 'INDUSTRIES'
                   WHEN 'MFG'   THEN 'MANUFACTURING'
                   WHEN 'MFR'   THEN 'MANUFACTURER'
                   WHEN 'MFRS'  THEN 'MANUFACTURERS'
                   WHEN 'CO'    THEN 'COMPANY'
                   WHEN 'CTY'   THEN 'COUNTY'
                   WHEN 'COUN'  THEN 'COUNTY'
                   WHEN 'AMER'  THEN 'AMERICAN'
                   WHEN 'ASSOC' THEN 'ASSOCIATION'
                   WHEN 'ASSOCS' THEN 'ASSOCIATES'
                   WHEN 'ORG'   THEN 'ORGANIZATION'
                   WHEN 'ORGS'  THEN 'ORGANIZATIONS'
                   WHEN 'GRP'   THEN 'GROUP'
                   WHEN 'GRPS'  THEN 'GROUPS'
                   WHEN 'EMPLR' THEN 'EMPLOYER'
                   WHEN 'EMPL'  THEN 'EMPLOYEES'
                   WHEN 'EMPLY' THEN 'EMPLOYEES'
                   WHEN 'UNI'   THEN 'UNIVERSITY'
                   WHEN 'PROF'  THEN 'PROFESSIONAL'
                   WHEN 'PROFS' THEN 'PROFESSIONALS'
                   WHEN 'SOC'   THEN 'SOCIETY'
                   WHEN 'AGNCY' THEN 'AGENCY'
                   WHEN 'AGY'   THEN 'AGENCY'
                   WHEN 'DPT'   THEN 'DEPARTMENT'
                   WHEN 'BD'    THEN 'BOARD'
                   WHEN 'CMTY'  THEN 'COMMUNITY'
                   WHEN 'COMTY' THEN 'COMMUNITY'
                   WHEN 'CO-OP' THEN 'COOPERATIVE'
                   WHEN 'COOP'  THEN 'COOPERATIVE'
                   WHEN 'TRANS' THEN 'TRANSPORTATION'
                   WHEN 'TRANSP' THEN 'TRANSPORTATION'
                   WHEN 'PWR'   THEN 'POWER'
                   WHEN 'ENVT'  THEN 'ENVIRONMENT'
                   WHEN 'ENV'   THEN 'ENVIRONMENTAL'
                   WHEN 'ED'    THEN 'EDUCATION'
                   WHEN 'EDU'   THEN 'EDUCATION'
                   WHEN 'EDUC'  THEN 'EDUCATION'
                   WHEN 'PUB'   THEN 'PUBLIC'
                   WHEN 'GEN'   THEN 'GENERAL'
                   WHEN 'NW'    THEN 'NORTHWEST'
                   WHEN 'NE'    THEN 'NORTHEAST'
                   WHEN 'SE'    THEN 'SOUTHEAST'
                   WHEN 'SW'    THEN 'SOUTHWEST'
                   ELSE token
               END AS token
          FROM tokens
    )
    SELECT NULLIF(
        array_to_string(
            ARRAY(
                SELECT token
                  FROM expanded
                 WHERE token <> ''
                   AND token NOT IN (
                       'THE','A','AN','OF','OR','AND','FOR',
                       'INC','INCORPORATED','LLC','L','LTD','LIMITED',
                       'CORP','CORPORATION','COMPANY','PLC','PC','PLLC','LP','LLP',
                       'ASSN','ASSOCIATION'
                   )
            ),
            ' '
        ),
        ''
    );
$$;
-- +goose StatementEnd




--
-- Name: admin_audit_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.admin_audit_log (
    id bigint NOT NULL,
    actor_user_id text NOT NULL,
    actor_email text NOT NULL,
    actor_name text,
    actor_role text NOT NULL,
    route text NOT NULL,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id text NOT NULL,
    previous_state jsonb,
    new_state jsonb,
    reviewer_notes text,
    request_id text,
    ip_address inet,
    user_agent text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: admin_audit_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.admin_audit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: admin_audit_log_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.admin_audit_log_id_seq OWNED BY public.admin_audit_log.id;


--
-- Name: agenda_item; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agenda_item (
    id bigint NOT NULL,
    hearing_id bigint NOT NULL,
    bill_id bigint,
    label text NOT NULL,
    csi_meeting_family_id text,
    csi_agenda_item_family_id text,
    csi_agenda_item_id text,
    order_index integer
);


--
-- Name: agenda_item_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.agenda_item_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: agenda_item_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.agenda_item_id_seq OWNED BY public.agenda_item.id;


--
-- Name: agenda_item_window; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agenda_item_window (
    id bigint NOT NULL,
    agenda_item_id bigint NOT NULL,
    start_ms integer NOT NULL,
    end_ms integer NOT NULL,
    mentions integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT agenda_item_window_check CHECK ((end_ms > start_ms))
);


--
-- Name: agenda_item_window_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.agenda_item_window_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: agenda_item_window_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.agenda_item_window_id_seq OWNED BY public.agenda_item_window.id;


--
-- Name: bill; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bill (
    id bigint NOT NULL,
    biennium text NOT NULL,
    prefix text NOT NULL,
    number integer NOT NULL,
    bill_number text GENERATED ALWAYS AS (((prefix || ' '::text) || number)) STORED,
    title text,
    description text,
    chamber_origin text,
    current_status text,
    status_date date,
    official_url text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: bill_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.bill_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: bill_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.bill_id_seq OWNED BY public.bill.id;


--
-- Name: bill_sponsor; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bill_sponsor (
    bill_id bigint NOT NULL,
    legislator_id bigint,
    sponsor_type text NOT NULL,
    person_id bigint,
    legislator_roster_membership_id bigint
);


--
-- Name: bill_status_change; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bill_status_change (
    id bigint NOT NULL,
    bill_id bigint NOT NULL,
    action_date date NOT NULL,
    history_line text NOT NULL,
    actor text
);


--
-- Name: bill_status_change_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.bill_status_change_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: bill_status_change_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.bill_status_change_id_seq OWNED BY public.bill_status_change.id;


--
-- Name: bls_observation; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bls_observation (
    id bigint NOT NULL,
    series_id text NOT NULL,
    year integer NOT NULL,
    period text NOT NULL,
    period_name text,
    value_numeric numeric,
    value_text text,
    footnotes jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: bls_observation_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.bls_observation_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: bls_observation_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.bls_observation_id_seq OWNED BY public.bls_observation.id;


--
-- Name: census_observation; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.census_observation (
    id bigint NOT NULL,
    dataset text NOT NULL,
    vintage_year integer NOT NULL,
    geography_id text NOT NULL,
    geography_name text,
    variable text NOT NULL,
    value_text text,
    value_numeric numeric,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: census_observation_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.census_observation_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: census_observation_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.census_observation_id_seq OWNED BY public.census_observation.id;


--
-- Name: datawa_contract; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.datawa_contract (
    id bigint NOT NULL,
    source_dataset_id text NOT NULL,
    source_row_id text NOT NULL,
    fiscal_year integer,
    agency_name text,
    agency_number text,
    contract_number text,
    amendment_number text,
    contractor_name text,
    statewide_vendor_number text,
    description text,
    start_date date,
    end_date date,
    period_start date,
    period_end date,
    federal_amount numeric,
    state_amount numeric,
    other_amount numeric,
    total_amount numeric,
    procurement_type text,
    minority_woman_owned text,
    small_business text,
    veteran_owned text,
    normalization_warnings jsonb DEFAULT '[]'::jsonb NOT NULL,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    normalized_contractor_name text
);


--
-- Name: datawa_contract_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.datawa_contract_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: datawa_contract_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.datawa_contract_id_seq OWNED BY public.datawa_contract.id;


--
-- Name: datawa_it_contract; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.datawa_it_contract (
    id bigint NOT NULL,
    source_dataset_id text NOT NULL,
    source_row_id text NOT NULL,
    report_fiscal_year integer,
    agency_number_agency_name text,
    agency_number text,
    agency_name text,
    contract_number text,
    contractor_name text,
    contractor_dba text,
    cooperative_purchase boolean,
    cooperative_name text,
    statewide_contract_purchase boolean,
    contract_start_date date,
    contract_end_date date,
    fiscal_year_start text,
    fiscal_year_end text,
    it_tower_application numeric,
    it_tower_compute numeric,
    it_tower_data_center numeric,
    it_tower_delivery numeric,
    it_tower_end_user numeric,
    it_tower_it_management numeric,
    it_tower_network numeric,
    it_tower_output numeric,
    it_tower_platform numeric,
    it_tower_security numeric,
    it_tower_storage numeric,
    other_non_it numeric,
    total_percentage numeric,
    contract_amount_fy20 numeric,
    contract_amount_fy21 numeric,
    contract_amount_fy22 numeric,
    contract_amount_fy23 numeric,
    contract_amount_fy24 numeric,
    contract_amount_fy25 numeric,
    contract_amount_fy26 numeric,
    contract_amount_fy27 numeric,
    contract_amount_fy28 numeric,
    contract_amount_fy29 numeric,
    contract_amount_fy30 numeric,
    total_contract_amount numeric,
    contract_amount_explanation text,
    normalization_warnings jsonb DEFAULT '[]'::jsonb NOT NULL,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    normalized_contractor_name text,
    normalized_contractor_dba text
);


--
-- Name: datawa_it_contract_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.datawa_it_contract_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: datawa_it_contract_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.datawa_it_contract_id_seq OWNED BY public.datawa_it_contract.id;


--
-- Name: datawa_master_contract_sale; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.datawa_master_contract_sale (
    id bigint NOT NULL,
    source_dataset_id text NOT NULL,
    source_row_id text NOT NULL,
    customer_type text,
    customer_name text,
    contract_number text,
    contract_title text,
    vendor_name text,
    report_year integer,
    q1_sales_reported numeric,
    q2_sales_reported numeric,
    q3_sales_reported numeric,
    q4_sales_reported numeric,
    total_sales_reported numeric,
    omwbe text,
    veteran_owned text,
    small_business text,
    diverse_options text,
    normalization_warnings jsonb DEFAULT '[]'::jsonb NOT NULL,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    normalized_vendor_name text,
    normalized_customer_name text
);


--
-- Name: datawa_master_contract_sale_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.datawa_master_contract_sale_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: datawa_master_contract_sale_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.datawa_master_contract_sale_id_seq OWNED BY public.datawa_master_contract_sale.id;


--
-- Name: datawa_webs_vendor; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.datawa_webs_vendor (
    id bigint NOT NULL,
    source_dataset_id text NOT NULL,
    source_row_id text NOT NULL,
    company_name text,
    normalized_company_name text,
    dba_name text,
    phone_number text,
    contact_email text,
    city text,
    state text,
    web_address text,
    commodity_code text,
    description_of_work text,
    small_business text,
    veteran_owned text,
    other_cert text,
    other_cert_2 text,
    normalization_warnings jsonb DEFAULT '[]'::jsonb NOT NULL,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: datawa_webs_vendor_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.datawa_webs_vendor_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: datawa_webs_vendor_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.datawa_webs_vendor_id_seq OWNED BY public.datawa_webs_vendor.id;


--
-- Name: diarization_job; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.diarization_job (
    id bigint NOT NULL,
    tvw_event_id text NOT NULL,
    audio_asset_id bigint,
    provider text NOT NULL,
    provider_job_id text,
    model text,
    status public.diarization_job_status DEFAULT 'pending'::public.diarization_job_status NOT NULL,
    submitted_at timestamp with time zone,
    finished_at timestamp with time zone,
    error text,
    raw_result_path text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: diarization_job_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.diarization_job_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: diarization_job_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.diarization_job_id_seq OWNED BY public.diarization_job.id;


--
-- Name: diarized_speech_segment; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.diarized_speech_segment (
    id bigint NOT NULL,
    diarization_job_id bigint NOT NULL,
    speaker_cluster_id bigint,
    tvw_event_id text NOT NULL,
    cluster_label text NOT NULL,
    start_ms integer NOT NULL,
    end_ms integer NOT NULL,
    confidence numeric,
    text text,
    raw jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT diarized_speech_segment_check CHECK ((end_ms > start_ms))
);


--
-- Name: diarized_speech_segment_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.diarized_speech_segment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: diarized_speech_segment_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.diarized_speech_segment_id_seq OWNED BY public.diarized_speech_segment.id;


--
-- Name: entity_mention; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.entity_mention (
    id bigint NOT NULL,
    tvw_event_id text,
    diarization_job_id bigint,
    source_kind text NOT NULL,
    source_id bigint,
    extractor text NOT NULL,
    model text,
    entity_type text NOT NULL,
    text text NOT NULL,
    normalized_text text,
    start_ms integer,
    end_ms integer,
    start_word integer,
    end_word integer,
    confidence numeric,
    raw jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT entity_mention_check CHECK (((end_ms IS NULL) OR (start_ms IS NULL) OR (end_ms >= start_ms))),
    CONSTRAINT entity_mention_source_kind_check CHECK ((source_kind = ANY (ARRAY['diarization_job'::text, 'transcript_turn'::text, 'testimony'::text, 'document'::text])))
);


--
-- Name: entity_mention_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.entity_mention_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: entity_mention_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.entity_mention_id_seq OWNED BY public.entity_mention.id;


--
-- Name: epa_ejscreen_observation; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.epa_ejscreen_observation (
    id bigint NOT NULL,
    geography_id text NOT NULL,
    indicator text NOT NULL,
    percentile numeric,
    value_numeric numeric,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: epa_ejscreen_observation_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.epa_ejscreen_observation_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: epa_ejscreen_observation_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.epa_ejscreen_observation_id_seq OWNED BY public.epa_ejscreen_observation.id;


--
-- Name: epa_facility; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.epa_facility (
    id bigint NOT NULL,
    registry_id text NOT NULL,
    name text,
    address text,
    city text,
    state_code text,
    zip text,
    latitude double precision,
    longitude double precision,
    programs text[] DEFAULT '{}'::text[] NOT NULL,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: epa_facility_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.epa_facility_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: epa_facility_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.epa_facility_id_seq OWNED BY public.epa_facility.id;


--
-- Name: federal_award; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.federal_award (
    id bigint NOT NULL,
    award_id text NOT NULL,
    recipient_name text,
    recipient_uei text,
    awarding_agency text,
    funding_agency text,
    award_type text,
    award_amount numeric,
    start_date date,
    end_date date,
    place_state_code text,
    place_county text,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    normalized_recipient_name text
);


--
-- Name: federal_award_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.federal_award_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: federal_award_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.federal_award_id_seq OWNED BY public.federal_award.id;


--
-- Name: fema_disaster_declaration; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fema_disaster_declaration (
    id bigint NOT NULL,
    disaster_number text NOT NULL,
    declaration_type text,
    state_code text,
    county_name text,
    incident_type text,
    incident_title text,
    declaration_date date,
    incident_begin_date date,
    incident_end_date date,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: fema_disaster_declaration_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.fema_disaster_declaration_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: fema_disaster_declaration_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.fema_disaster_declaration_id_seq OWNED BY public.fema_disaster_declaration.id;


--
-- Name: fiscalwa_vendor_payment; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fiscalwa_vendor_payment (
    id bigint NOT NULL,
    source_dataset_id text NOT NULL,
    source_row_id text NOT NULL,
    biennium text,
    fiscal_year integer,
    fiscal_month text,
    agency_number text,
    agency_name text,
    object_code text,
    object_category text,
    subobject_code text,
    subobject_name text,
    vendor_name text,
    amount numeric,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    normalized_vendor_name text
);


--
-- Name: fiscalwa_vendor_payment_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.fiscalwa_vendor_payment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: fiscalwa_vendor_payment_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.fiscalwa_vendor_payment_id_seq OWNED BY public.fiscalwa_vendor_payment.id;


--
-- Name: hearing; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.hearing (
    id bigint NOT NULL,
    bill_id bigint,
    committee_name text NOT NULL,
    committee_acronym text,
    chamber text NOT NULL,
    meeting_datetime timestamp with time zone NOT NULL,
    location text,
    lws_meeting_id text,
    committee_schedule_agenda_id text,
    committee_schedule_video_id text,
    tvw_event_id text,
    official_agenda_url text,
    tvw_url text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: hearing_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.hearing_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: hearing_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.hearing_id_seq OWNED BY public.hearing.id;


--
-- Name: hud_dataset_ref; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.hud_dataset_ref (
    id bigint NOT NULL,
    source_key text NOT NULL,
    name text NOT NULL,
    url text NOT NULL,
    kind text,
    program text,
    geography text,
    dataset_year integer,
    notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: hud_dataset_ref_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.hud_dataset_ref_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: hud_dataset_ref_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.hud_dataset_ref_id_seq OWNED BY public.hud_dataset_ref.id;


--
-- Name: hud_housing_observation; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.hud_housing_observation (
    id bigint NOT NULL,
    dataset_key text NOT NULL,
    geography_id text NOT NULL,
    geography_name text,
    indicator text NOT NULL,
    observation_year integer,
    value_numeric numeric,
    value_text text,
    unit text,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: hud_housing_observation_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.hud_housing_observation_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: hud_housing_observation_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.hud_housing_observation_id_seq OWNED BY public.hud_housing_observation.id;


--
-- Name: ingestion_run; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ingestion_run (
    id bigint NOT NULL,
    job text NOT NULL,
    args jsonb DEFAULT '{}'::jsonb NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    status text DEFAULT 'running'::text NOT NULL,
    rows_fetched integer DEFAULT 0 NOT NULL,
    rows_upserted integer DEFAULT 0 NOT NULL,
    error text,
    CONSTRAINT ingestion_run_status_check CHECK ((status = ANY (ARRAY['running'::text, 'succeeded'::text, 'failed'::text])))
);


--
-- Name: ingestion_run_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ingestion_run_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ingestion_run_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ingestion_run_id_seq OWNED BY public.ingestion_run.id;


--
-- Name: irs_bmf_organization; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.irs_bmf_organization (
    ein text NOT NULL,
    name text NOT NULL,
    normalized_name text NOT NULL,
    sort_name text,
    street text,
    city text,
    state text,
    zip text,
    subsection_code text,
    classification text,
    deductibility_code text,
    activity_codes text,
    foundation_code text,
    organization_code text,
    status_code text,
    ruling_date text,
    ntee_code text,
    income_amount bigint,
    revenue_amount bigint,
    asset_amount bigint,
    raw jsonb NOT NULL,
    fetched_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: job_lock; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.job_lock (
    name text NOT NULL,
    owner text NOT NULL,
    acquired_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone
);


--
-- Name: kingcounty_parcel; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kingcounty_parcel (
    id bigint NOT NULL,
    source_dataset_id text NOT NULL,
    source_row_id text NOT NULL,
    pin text,
    address text,
    jurisdiction text,
    latitude double precision,
    longitude double precision,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: kingcounty_parcel_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.kingcounty_parcel_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: kingcounty_parcel_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.kingcounty_parcel_id_seq OWNED BY public.kingcounty_parcel.id;


--
-- Name: legislator; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.legislator (
    id bigint NOT NULL,
    lws_sponsor_id text,
    name text NOT NULL,
    chamber text,
    district text,
    party text,
    official_url text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    first_name text,
    last_name text,
    email text,
    phone text,
    acronym text
);


--
-- Name: legislator_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.legislator_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: legislator_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.legislator_id_seq OWNED BY public.legislator.id;


--
-- Name: legislator_roster_membership; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.legislator_roster_membership (
    id bigint NOT NULL,
    person_id bigint NOT NULL,
    biennium text NOT NULL,
    chamber text NOT NULL,
    district text,
    party text,
    lws_sponsor_id text NOT NULL,
    roster_name text NOT NULL,
    first_name text,
    last_name text,
    email text,
    phone text,
    acronym text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: legislator_roster_membership_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.legislator_roster_membership_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: legislator_roster_membership_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.legislator_roster_membership_id_seq OWNED BY public.legislator_roster_membership.id;


--
-- Name: organization; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.organization (
    id bigint NOT NULL,
    canonical_name text NOT NULL,
    aliases text[] DEFAULT '{}'::text[] NOT NULL,
    pdc_lobbyist_employer_id text,
    pdc_committee_or_filer_id text,
    match_confidence public.org_match_confidence DEFAULT 'unmatched'::public.org_match_confidence NOT NULL,
    match_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    irs_bmf_ein text,
    verified_at timestamp with time zone,
    verification_source text,
    CONSTRAINT organization_verification_source_check CHECK (((verification_source IS NULL) OR (verification_source = ANY (ARRAY['irs_bmf'::text, 'pdc_employer'::text, 'manual'::text, 'csi_testimony_review'::text]))))
);


--
-- Name: organization_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.organization_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: organization_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.organization_id_seq OWNED BY public.organization.id;


--
-- Name: pdc_employer; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pdc_employer (
    employer_id text NOT NULL,
    name text NOT NULL,
    normalized_name text NOT NULL,
    last_employment_year text,
    last_report_number text,
    last_employment_url text,
    raw jsonb NOT NULL,
    first_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: pdc_lobbyist_compensation; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pdc_lobbyist_compensation (
    filer_id text NOT NULL,
    employer_id text NOT NULL,
    filing_period text NOT NULL,
    filer_name text NOT NULL,
    funding_source_id text,
    funding_source text,
    employer_name text NOT NULL,
    compensation numeric(15,2),
    total_expenses numeric(15,2),
    net_total numeric(15,2),
    url text,
    raw jsonb NOT NULL,
    fetched_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: person; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.person (
    id bigint NOT NULL,
    display_name text NOT NULL,
    first_name text,
    last_name text,
    aliases text[] DEFAULT '{}'::text[] NOT NULL,
    normalized_name text,
    match_confidence public.person_match_confidence DEFAULT 'possible'::public.person_match_confidence NOT NULL,
    match_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    pdc_lobbyist_id text
);


--
-- Name: person_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.person_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: person_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.person_id_seq OWNED BY public.person.id;


--
-- Name: person_organization_affiliation; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.person_organization_affiliation (
    id bigint NOT NULL,
    person_id bigint,
    person_mention_id bigint,
    organization_id bigint,
    raw_person_name text,
    raw_organization_name text,
    relationship_type public.person_org_affiliation_type NOT NULL,
    role_title text,
    start_date date,
    end_date date,
    record_year integer,
    source_kind public.person_source_kind NOT NULL,
    source_table text NOT NULL,
    source_pk bigint,
    source_row_id text,
    context jsonb DEFAULT '{}'::jsonb NOT NULL,
    confidence public.person_match_confidence DEFAULT 'possible'::public.person_match_confidence NOT NULL,
    review_status public.person_review_status DEFAULT 'auto'::public.person_review_status NOT NULL,
    evidence jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT person_organization_affiliation_check CHECK (((person_id IS NOT NULL) OR (person_mention_id IS NOT NULL) OR (raw_person_name IS NOT NULL))),
    CONSTRAINT person_organization_affiliation_check1 CHECK (((organization_id IS NOT NULL) OR (raw_organization_name IS NOT NULL)))
);


--
-- Name: person_organization_affiliation_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.person_organization_affiliation_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: person_organization_affiliation_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.person_organization_affiliation_id_seq OWNED BY public.person_organization_affiliation.id;


--
-- Name: person_source_mention; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.person_source_mention (
    id bigint NOT NULL,
    person_id bigint,
    source_kind public.person_source_kind NOT NULL,
    source_table text NOT NULL,
    source_pk bigint,
    source_row_id text,
    source_name text NOT NULL,
    normalized_name text,
    source_role text,
    context jsonb DEFAULT '{}'::jsonb NOT NULL,
    confidence public.person_match_confidence DEFAULT 'possible'::public.person_match_confidence NOT NULL,
    review_status public.person_review_status DEFAULT 'auto'::public.person_review_status NOT NULL,
    first_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: person_source_mention_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.person_source_mention_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: person_source_mention_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.person_source_mention_id_seq OWNED BY public.person_source_mention.id;


--
-- Name: vendor_entity_match_candidate; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vendor_entity_match_candidate (
    id bigint NOT NULL,
    source_kind public.entity_match_source_kind NOT NULL,
    source_table text NOT NULL,
    source_pk bigint,
    source_dataset_id text,
    source_row_id text,
    source_name text NOT NULL,
    normalized_name text NOT NULL,
    organization_id bigint NOT NULL,
    candidate_confidence public.org_match_confidence DEFAULT 'possible'::public.org_match_confidence NOT NULL,
    evidence jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: vendor_entity_match_decision; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vendor_entity_match_decision (
    id bigint NOT NULL,
    candidate_id bigint NOT NULL,
    organization_id bigint NOT NULL,
    decision public.entity_match_decision DEFAULT 'needs_review'::public.entity_match_decision NOT NULL,
    reviewed_confidence public.org_match_confidence DEFAULT 'possible'::public.org_match_confidence NOT NULL,
    reviewed_by text,
    review_notes text,
    reviewed_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: reviewed_vendor_entity_match; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.reviewed_vendor_entity_match AS
 SELECT c.id AS candidate_id,
    d.id AS decision_id,
    c.source_kind,
    c.source_table,
    c.source_pk,
    c.source_dataset_id,
    c.source_row_id,
    c.source_name,
    c.normalized_name,
    d.organization_id,
    d.reviewed_confidence AS match_confidence,
    d.review_notes,
    d.reviewed_at,
    c.evidence
   FROM (public.vendor_entity_match_candidate c
     JOIN public.vendor_entity_match_decision d ON ((d.candidate_id = c.id)))
  WHERE (d.decision = 'confirmed'::public.entity_match_decision);


--
-- Name: sao_audit_type; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sao_audit_type (
    audit_type_id text NOT NULL,
    name text,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL
);


--
-- Name: sao_government_entity; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sao_government_entity (
    id bigint NOT NULL,
    mcag text NOT NULL,
    name text NOT NULL,
    gov_type text,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: sao_government_entity_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sao_government_entity_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sao_government_entity_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sao_government_entity_id_seq OWNED BY public.sao_government_entity.id;


--
-- Name: sao_government_type; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sao_government_type (
    code text NOT NULL,
    name text,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL
);


--
-- Name: sao_report; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sao_report (
    id bigint NOT NULL,
    audit_report_number text NOT NULL,
    audit_number text,
    report_title text,
    audit_type_name text,
    gov_type_desc text,
    date_released date,
    begin_audit_period date,
    end_audit_period date,
    findings boolean DEFAULT false NOT NULL,
    audit_report_url text,
    findings_url text,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: sao_report_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sao_report_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sao_report_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sao_report_id_seq OWNED BY public.sao_report.id;


--
-- Name: seattle_auditor_recommendation; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.seattle_auditor_recommendation (
    id bigint NOT NULL,
    dashboard_id text NOT NULL,
    recommendation_id text NOT NULL,
    recommendation_number text,
    recommendation_text text NOT NULL,
    category text,
    audit_name text,
    audit_type text,
    department text,
    status text,
    latest_update text,
    source_timestamp text,
    audit_url text,
    issue_date date,
    finding_text text,
    finding_number text,
    year integer,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: seattle_auditor_recommendation_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.seattle_auditor_recommendation_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: seattle_auditor_recommendation_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.seattle_auditor_recommendation_id_seq OWNED BY public.seattle_auditor_recommendation.id;


--
-- Name: seattle_operating_budget; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.seattle_operating_budget (
    id bigint NOT NULL,
    source_dataset_id text NOT NULL,
    source_row_id text NOT NULL,
    fiscal_year integer,
    service text,
    department text,
    program text,
    fund text,
    fund_type text,
    expense_type text,
    description text,
    approved_amount numeric,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: seattle_operating_budget_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.seattle_operating_budget_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: seattle_operating_budget_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.seattle_operating_budget_id_seq OWNED BY public.seattle_operating_budget.id;


--
-- Name: seattle_permit; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.seattle_permit (
    id bigint NOT NULL,
    source_dataset_id text NOT NULL,
    source_row_id text NOT NULL,
    permit_number text,
    status text,
    address text,
    description text,
    category text,
    permit_type text,
    valuation numeric,
    latitude double precision,
    longitude double precision,
    raw_fields jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: seattle_permit_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.seattle_permit_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: seattle_permit_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.seattle_permit_id_seq OWNED BY public.seattle_permit.id;


--
-- Name: source_dataset; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.source_dataset (
    id bigint NOT NULL,
    source_system text NOT NULL,
    dataset_id text NOT NULL,
    name text,
    description text,
    domain text,
    web_url text,
    data_url text,
    metadata_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    first_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: source_dataset_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.source_dataset_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: source_dataset_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.source_dataset_id_seq OWNED BY public.source_dataset.id;


--
-- Name: speaker_assignment; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.speaker_assignment (
    id bigint NOT NULL,
    diarization_job_id bigint NOT NULL,
    speaker_cluster_id bigint NOT NULL,
    speaker_kind public.speaker_candidate_kind NOT NULL,
    speaker_id bigint,
    speaker_label text NOT NULL,
    confidence numeric,
    review_task_id bigint,
    review_status public.speaker_review_status NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: speaker_assignment_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.speaker_assignment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: speaker_assignment_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.speaker_assignment_id_seq OWNED BY public.speaker_assignment.id;


--
-- Name: speaker_cluster; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.speaker_cluster (
    id bigint NOT NULL,
    diarization_job_id bigint NOT NULL,
    tvw_event_id text NOT NULL,
    cluster_label text NOT NULL,
    total_speech_ms integer,
    turn_count integer,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL
);


--
-- Name: speaker_cluster_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.speaker_cluster_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: speaker_cluster_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.speaker_cluster_id_seq OWNED BY public.speaker_cluster.id;


--
-- Name: speaker_identity_evidence; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.speaker_identity_evidence (
    id bigint NOT NULL,
    evidence_key text NOT NULL,
    diarization_job_id bigint NOT NULL,
    speaker_cluster_id bigint NOT NULL,
    diarized_speech_segment_id bigint,
    evidence_type public.speaker_evidence_type NOT NULL,
    evidence_text text NOT NULL,
    candidate_kind public.speaker_candidate_kind NOT NULL,
    candidate_id bigint,
    candidate_label text NOT NULL,
    confidence numeric,
    start_ms integer,
    end_ms integer,
    raw jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT speaker_identity_evidence_check CHECK (((end_ms IS NULL) OR (start_ms IS NULL) OR (end_ms >= start_ms)))
);


--
-- Name: speaker_identity_evidence_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.speaker_identity_evidence_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: speaker_identity_evidence_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.speaker_identity_evidence_id_seq OWNED BY public.speaker_identity_evidence.id;


--
-- Name: speaker_review_task; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.speaker_review_task (
    id bigint NOT NULL,
    diarization_job_id bigint NOT NULL,
    speaker_cluster_id bigint NOT NULL,
    status public.speaker_review_status DEFAULT 'pending'::public.speaker_review_status NOT NULL,
    priority integer DEFAULT 0 NOT NULL,
    proposed_candidate_kind public.speaker_candidate_kind NOT NULL,
    proposed_candidate_id bigint,
    proposed_label text NOT NULL,
    proposed_confidence numeric,
    evidence_ids bigint[] DEFAULT '{}'::bigint[] NOT NULL,
    reviewer text,
    review_notes text,
    reviewed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: speaker_review_task_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.speaker_review_task_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: speaker_review_task_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.speaker_review_task_id_seq OWNED BY public.speaker_review_task.id;


--
-- Name: testifier; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.testifier (
    id bigint NOT NULL,
    agenda_item_id bigint NOT NULL,
    raw_name text NOT NULL,
    raw_organization text,
    normalized_person_id bigint,
    normalized_org_id bigint,
    "position" public.testifier_position DEFAULT 'Unknown'::public.testifier_position NOT NULL,
    testified boolean NOT NULL,
    time_signed_in timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    csi_order integer,
    csi_panel_class text,
    source_key text,
    active boolean DEFAULT true NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: testifier_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.testifier_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: testifier_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.testifier_id_seq OWNED BY public.testifier.id;


--
-- Name: tvw_audio_asset; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tvw_audio_asset (
    id bigint NOT NULL,
    tvw_event_id text NOT NULL,
    source_url text NOT NULL,
    source_kind text NOT NULL,
    original_path text,
    normalized_path text,
    content_hash text,
    duration_ms integer,
    sample_rate integer,
    channels integer,
    codec text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT tvw_audio_asset_source_kind_check CHECK ((source_kind = ANY (ARRAY['audio_download_url'::text, 'published_audio_url'::text, 'media_asset_audio'::text, 'video_download_url'::text, 'media_asset_video'::text])))
);


--
-- Name: tvw_audio_asset_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.tvw_audio_asset_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: tvw_audio_asset_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.tvw_audio_asset_id_seq OWNED BY public.tvw_audio_asset.id;


--
-- Name: tvw_event; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tvw_event (
    id bigint NOT NULL,
    tvw_event_id text NOT NULL,
    wp_post_id bigint,
    title text,
    description text,
    start_datetime timestamp with time zone,
    caption_url text,
    thumbnail_url text,
    raw_categories jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    wp_slug text,
    wp_link text,
    custom_id text,
    location_name text,
    total_runtime text,
    total_runtime_seconds integer,
    published_audio_url text,
    audio_download_url text,
    video_download_url text,
    streaming_uris jsonb DEFAULT '{}'::jsonb NOT NULL,
    raw_keywords jsonb DEFAULT '[]'::jsonb NOT NULL,
    raw_wp_tags jsonb DEFAULT '[]'::jsonb NOT NULL,
    raw_wp_categories jsonb DEFAULT '[]'::jsonb NOT NULL
);


--
-- Name: tvw_event_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.tvw_event_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: tvw_event_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.tvw_event_id_seq OWNED BY public.tvw_event.id;


--
-- Name: tvw_media_asset; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tvw_media_asset (
    id bigint NOT NULL,
    tvw_event_id text NOT NULL,
    asset_id text NOT NULL,
    asset_type text NOT NULL,
    name text,
    file_url text,
    thumbnail_url text,
    sprite_url text,
    preview_url text,
    file_size_bytes bigint,
    total_runtime text,
    total_runtime_seconds integer,
    current_status text,
    date_created timestamp with time zone,
    advanced_details jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: tvw_media_asset_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.tvw_media_asset_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: tvw_media_asset_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.tvw_media_asset_id_seq OWNED BY public.tvw_media_asset.id;


--
-- Name: vendor_entity_match_candidate_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.vendor_entity_match_candidate_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: vendor_entity_match_candidate_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.vendor_entity_match_candidate_id_seq OWNED BY public.vendor_entity_match_candidate.id;


--
-- Name: vendor_entity_match_decision_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.vendor_entity_match_decision_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: vendor_entity_match_decision_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.vendor_entity_match_decision_id_seq OWNED BY public.vendor_entity_match_decision.id;


--
-- Name: admin_audit_log id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_audit_log ALTER COLUMN id SET DEFAULT nextval('public.admin_audit_log_id_seq'::regclass);


--
-- Name: agenda_item id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agenda_item ALTER COLUMN id SET DEFAULT nextval('public.agenda_item_id_seq'::regclass);


--
-- Name: agenda_item_window id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agenda_item_window ALTER COLUMN id SET DEFAULT nextval('public.agenda_item_window_id_seq'::regclass);


--
-- Name: bill id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill ALTER COLUMN id SET DEFAULT nextval('public.bill_id_seq'::regclass);


--
-- Name: bill_status_change id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_status_change ALTER COLUMN id SET DEFAULT nextval('public.bill_status_change_id_seq'::regclass);


--
-- Name: bls_observation id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bls_observation ALTER COLUMN id SET DEFAULT nextval('public.bls_observation_id_seq'::regclass);


--
-- Name: census_observation id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.census_observation ALTER COLUMN id SET DEFAULT nextval('public.census_observation_id_seq'::regclass);


--
-- Name: datawa_contract id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.datawa_contract ALTER COLUMN id SET DEFAULT nextval('public.datawa_contract_id_seq'::regclass);


--
-- Name: datawa_it_contract id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.datawa_it_contract ALTER COLUMN id SET DEFAULT nextval('public.datawa_it_contract_id_seq'::regclass);


--
-- Name: datawa_master_contract_sale id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.datawa_master_contract_sale ALTER COLUMN id SET DEFAULT nextval('public.datawa_master_contract_sale_id_seq'::regclass);


--
-- Name: datawa_webs_vendor id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.datawa_webs_vendor ALTER COLUMN id SET DEFAULT nextval('public.datawa_webs_vendor_id_seq'::regclass);


--
-- Name: diarization_job id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.diarization_job ALTER COLUMN id SET DEFAULT nextval('public.diarization_job_id_seq'::regclass);


--
-- Name: diarized_speech_segment id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.diarized_speech_segment ALTER COLUMN id SET DEFAULT nextval('public.diarized_speech_segment_id_seq'::regclass);


--
-- Name: entity_mention id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.entity_mention ALTER COLUMN id SET DEFAULT nextval('public.entity_mention_id_seq'::regclass);


--
-- Name: epa_ejscreen_observation id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.epa_ejscreen_observation ALTER COLUMN id SET DEFAULT nextval('public.epa_ejscreen_observation_id_seq'::regclass);


--
-- Name: epa_facility id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.epa_facility ALTER COLUMN id SET DEFAULT nextval('public.epa_facility_id_seq'::regclass);


--
-- Name: federal_award id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.federal_award ALTER COLUMN id SET DEFAULT nextval('public.federal_award_id_seq'::regclass);


--
-- Name: fema_disaster_declaration id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fema_disaster_declaration ALTER COLUMN id SET DEFAULT nextval('public.fema_disaster_declaration_id_seq'::regclass);


--
-- Name: fiscalwa_vendor_payment id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fiscalwa_vendor_payment ALTER COLUMN id SET DEFAULT nextval('public.fiscalwa_vendor_payment_id_seq'::regclass);


--
-- Name: hearing id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hearing ALTER COLUMN id SET DEFAULT nextval('public.hearing_id_seq'::regclass);


--
-- Name: hud_dataset_ref id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hud_dataset_ref ALTER COLUMN id SET DEFAULT nextval('public.hud_dataset_ref_id_seq'::regclass);


--
-- Name: hud_housing_observation id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hud_housing_observation ALTER COLUMN id SET DEFAULT nextval('public.hud_housing_observation_id_seq'::regclass);


--
-- Name: ingestion_run id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ingestion_run ALTER COLUMN id SET DEFAULT nextval('public.ingestion_run_id_seq'::regclass);


--
-- Name: kingcounty_parcel id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kingcounty_parcel ALTER COLUMN id SET DEFAULT nextval('public.kingcounty_parcel_id_seq'::regclass);


--
-- Name: legislator id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.legislator ALTER COLUMN id SET DEFAULT nextval('public.legislator_id_seq'::regclass);


--
-- Name: legislator_roster_membership id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.legislator_roster_membership ALTER COLUMN id SET DEFAULT nextval('public.legislator_roster_membership_id_seq'::regclass);


--
-- Name: organization id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization ALTER COLUMN id SET DEFAULT nextval('public.organization_id_seq'::regclass);


--
-- Name: person id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.person ALTER COLUMN id SET DEFAULT nextval('public.person_id_seq'::regclass);


--
-- Name: person_organization_affiliation id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.person_organization_affiliation ALTER COLUMN id SET DEFAULT nextval('public.person_organization_affiliation_id_seq'::regclass);


--
-- Name: person_source_mention id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.person_source_mention ALTER COLUMN id SET DEFAULT nextval('public.person_source_mention_id_seq'::regclass);


--
-- Name: sao_government_entity id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sao_government_entity ALTER COLUMN id SET DEFAULT nextval('public.sao_government_entity_id_seq'::regclass);


--
-- Name: sao_report id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sao_report ALTER COLUMN id SET DEFAULT nextval('public.sao_report_id_seq'::regclass);


--
-- Name: seattle_auditor_recommendation id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.seattle_auditor_recommendation ALTER COLUMN id SET DEFAULT nextval('public.seattle_auditor_recommendation_id_seq'::regclass);


--
-- Name: seattle_operating_budget id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.seattle_operating_budget ALTER COLUMN id SET DEFAULT nextval('public.seattle_operating_budget_id_seq'::regclass);


--
-- Name: seattle_permit id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.seattle_permit ALTER COLUMN id SET DEFAULT nextval('public.seattle_permit_id_seq'::regclass);


--
-- Name: source_dataset id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_dataset ALTER COLUMN id SET DEFAULT nextval('public.source_dataset_id_seq'::regclass);


--
-- Name: speaker_assignment id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_assignment ALTER COLUMN id SET DEFAULT nextval('public.speaker_assignment_id_seq'::regclass);


--
-- Name: speaker_cluster id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_cluster ALTER COLUMN id SET DEFAULT nextval('public.speaker_cluster_id_seq'::regclass);


--
-- Name: speaker_identity_evidence id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_identity_evidence ALTER COLUMN id SET DEFAULT nextval('public.speaker_identity_evidence_id_seq'::regclass);


--
-- Name: speaker_review_task id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_review_task ALTER COLUMN id SET DEFAULT nextval('public.speaker_review_task_id_seq'::regclass);


--
-- Name: testifier id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.testifier ALTER COLUMN id SET DEFAULT nextval('public.testifier_id_seq'::regclass);


--
-- Name: tvw_audio_asset id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tvw_audio_asset ALTER COLUMN id SET DEFAULT nextval('public.tvw_audio_asset_id_seq'::regclass);


--
-- Name: tvw_event id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tvw_event ALTER COLUMN id SET DEFAULT nextval('public.tvw_event_id_seq'::regclass);


--
-- Name: tvw_media_asset id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tvw_media_asset ALTER COLUMN id SET DEFAULT nextval('public.tvw_media_asset_id_seq'::regclass);


--
-- Name: vendor_entity_match_candidate id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_entity_match_candidate ALTER COLUMN id SET DEFAULT nextval('public.vendor_entity_match_candidate_id_seq'::regclass);


--
-- Name: vendor_entity_match_decision id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_entity_match_decision ALTER COLUMN id SET DEFAULT nextval('public.vendor_entity_match_decision_id_seq'::regclass);


--
-- Name: admin_audit_log admin_audit_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_audit_log
    ADD CONSTRAINT admin_audit_log_pkey PRIMARY KEY (id);


--
-- Name: agenda_item agenda_item_csi_agenda_item_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agenda_item
    ADD CONSTRAINT agenda_item_csi_agenda_item_id_key UNIQUE (csi_agenda_item_id);


--
-- Name: agenda_item agenda_item_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agenda_item
    ADD CONSTRAINT agenda_item_pkey PRIMARY KEY (id);


--
-- Name: agenda_item_window agenda_item_window_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agenda_item_window
    ADD CONSTRAINT agenda_item_window_pkey PRIMARY KEY (id);


--
-- Name: bill bill_biennium_prefix_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill
    ADD CONSTRAINT bill_biennium_prefix_number_key UNIQUE (biennium, prefix, number);


--
-- Name: bill bill_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill
    ADD CONSTRAINT bill_pkey PRIMARY KEY (id);


--
-- Name: bill_status_change bill_status_change_bill_id_action_date_history_line_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_status_change
    ADD CONSTRAINT bill_status_change_bill_id_action_date_history_line_key UNIQUE (bill_id, action_date, history_line);


--
-- Name: bill_status_change bill_status_change_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_status_change
    ADD CONSTRAINT bill_status_change_pkey PRIMARY KEY (id);


--
-- Name: bls_observation bls_observation_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bls_observation
    ADD CONSTRAINT bls_observation_pkey PRIMARY KEY (id);


--
-- Name: bls_observation bls_observation_series_id_year_period_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bls_observation
    ADD CONSTRAINT bls_observation_series_id_year_period_key UNIQUE (series_id, year, period);


--
-- Name: census_observation census_observation_dataset_vintage_year_geography_id_variab_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.census_observation
    ADD CONSTRAINT census_observation_dataset_vintage_year_geography_id_variab_key UNIQUE (dataset, vintage_year, geography_id, variable);


--
-- Name: census_observation census_observation_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.census_observation
    ADD CONSTRAINT census_observation_pkey PRIMARY KEY (id);


--
-- Name: datawa_contract datawa_contract_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.datawa_contract
    ADD CONSTRAINT datawa_contract_pkey PRIMARY KEY (id);


--
-- Name: datawa_contract datawa_contract_source_dataset_id_source_row_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.datawa_contract
    ADD CONSTRAINT datawa_contract_source_dataset_id_source_row_id_key UNIQUE (source_dataset_id, source_row_id);


--
-- Name: datawa_it_contract datawa_it_contract_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.datawa_it_contract
    ADD CONSTRAINT datawa_it_contract_pkey PRIMARY KEY (id);


--
-- Name: datawa_it_contract datawa_it_contract_source_dataset_id_source_row_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.datawa_it_contract
    ADD CONSTRAINT datawa_it_contract_source_dataset_id_source_row_id_key UNIQUE (source_dataset_id, source_row_id);


--
-- Name: datawa_master_contract_sale datawa_master_contract_sale_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.datawa_master_contract_sale
    ADD CONSTRAINT datawa_master_contract_sale_pkey PRIMARY KEY (id);


--
-- Name: datawa_master_contract_sale datawa_master_contract_sale_source_dataset_id_source_row_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.datawa_master_contract_sale
    ADD CONSTRAINT datawa_master_contract_sale_source_dataset_id_source_row_id_key UNIQUE (source_dataset_id, source_row_id);


--
-- Name: datawa_webs_vendor datawa_webs_vendor_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.datawa_webs_vendor
    ADD CONSTRAINT datawa_webs_vendor_pkey PRIMARY KEY (id);


--
-- Name: datawa_webs_vendor datawa_webs_vendor_source_dataset_id_source_row_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.datawa_webs_vendor
    ADD CONSTRAINT datawa_webs_vendor_source_dataset_id_source_row_id_key UNIQUE (source_dataset_id, source_row_id);


--
-- Name: diarization_job diarization_job_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.diarization_job
    ADD CONSTRAINT diarization_job_pkey PRIMARY KEY (id);


--
-- Name: diarized_speech_segment diarized_speech_segment_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.diarized_speech_segment
    ADD CONSTRAINT diarized_speech_segment_pkey PRIMARY KEY (id);


--
-- Name: entity_mention entity_mention_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.entity_mention
    ADD CONSTRAINT entity_mention_pkey PRIMARY KEY (id);


--
-- Name: epa_ejscreen_observation epa_ejscreen_observation_geography_id_indicator_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.epa_ejscreen_observation
    ADD CONSTRAINT epa_ejscreen_observation_geography_id_indicator_key UNIQUE (geography_id, indicator);


--
-- Name: epa_ejscreen_observation epa_ejscreen_observation_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.epa_ejscreen_observation
    ADD CONSTRAINT epa_ejscreen_observation_pkey PRIMARY KEY (id);


--
-- Name: epa_facility epa_facility_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.epa_facility
    ADD CONSTRAINT epa_facility_pkey PRIMARY KEY (id);


--
-- Name: epa_facility epa_facility_registry_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.epa_facility
    ADD CONSTRAINT epa_facility_registry_id_key UNIQUE (registry_id);


--
-- Name: federal_award federal_award_award_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.federal_award
    ADD CONSTRAINT federal_award_award_id_key UNIQUE (award_id);


--
-- Name: federal_award federal_award_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.federal_award
    ADD CONSTRAINT federal_award_pkey PRIMARY KEY (id);


--
-- Name: fema_disaster_declaration fema_disaster_declaration_disaster_number_state_code_county_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fema_disaster_declaration
    ADD CONSTRAINT fema_disaster_declaration_disaster_number_state_code_county_key UNIQUE (disaster_number, state_code, county_name);


--
-- Name: fema_disaster_declaration fema_disaster_declaration_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fema_disaster_declaration
    ADD CONSTRAINT fema_disaster_declaration_pkey PRIMARY KEY (id);


--
-- Name: fiscalwa_vendor_payment fiscalwa_vendor_payment_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fiscalwa_vendor_payment
    ADD CONSTRAINT fiscalwa_vendor_payment_pkey PRIMARY KEY (id);


--
-- Name: fiscalwa_vendor_payment fiscalwa_vendor_payment_source_dataset_id_source_row_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fiscalwa_vendor_payment
    ADD CONSTRAINT fiscalwa_vendor_payment_source_dataset_id_source_row_id_key UNIQUE (source_dataset_id, source_row_id);


--
-- Name: hearing hearing_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hearing
    ADD CONSTRAINT hearing_pkey PRIMARY KEY (id);


--
-- Name: hud_dataset_ref hud_dataset_ref_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hud_dataset_ref
    ADD CONSTRAINT hud_dataset_ref_pkey PRIMARY KEY (id);


--
-- Name: hud_dataset_ref hud_dataset_ref_source_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hud_dataset_ref
    ADD CONSTRAINT hud_dataset_ref_source_key_key UNIQUE (source_key);


--
-- Name: hud_housing_observation hud_housing_observation_dataset_key_geography_id_indicator__key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hud_housing_observation
    ADD CONSTRAINT hud_housing_observation_dataset_key_geography_id_indicator__key UNIQUE (dataset_key, geography_id, indicator, observation_year);


--
-- Name: hud_housing_observation hud_housing_observation_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hud_housing_observation
    ADD CONSTRAINT hud_housing_observation_pkey PRIMARY KEY (id);


--
-- Name: ingestion_run ingestion_run_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ingestion_run
    ADD CONSTRAINT ingestion_run_pkey PRIMARY KEY (id);


--
-- Name: irs_bmf_organization irs_bmf_organization_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.irs_bmf_organization
    ADD CONSTRAINT irs_bmf_organization_pkey PRIMARY KEY (ein);


--
-- Name: job_lock job_lock_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.job_lock
    ADD CONSTRAINT job_lock_pkey PRIMARY KEY (name);


--
-- Name: kingcounty_parcel kingcounty_parcel_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kingcounty_parcel
    ADD CONSTRAINT kingcounty_parcel_pkey PRIMARY KEY (id);


--
-- Name: kingcounty_parcel kingcounty_parcel_source_dataset_id_source_row_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kingcounty_parcel
    ADD CONSTRAINT kingcounty_parcel_source_dataset_id_source_row_id_key UNIQUE (source_dataset_id, source_row_id);


--
-- Name: legislator legislator_lws_sponsor_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.legislator
    ADD CONSTRAINT legislator_lws_sponsor_id_key UNIQUE (lws_sponsor_id);


--
-- Name: legislator legislator_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.legislator
    ADD CONSTRAINT legislator_pkey PRIMARY KEY (id);


--
-- Name: legislator_roster_membership legislator_roster_membership_biennium_lws_sponsor_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.legislator_roster_membership
    ADD CONSTRAINT legislator_roster_membership_biennium_lws_sponsor_id_key UNIQUE (biennium, lws_sponsor_id);


--
-- Name: legislator_roster_membership legislator_roster_membership_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.legislator_roster_membership
    ADD CONSTRAINT legislator_roster_membership_pkey PRIMARY KEY (id);


--
-- Name: organization organization_canonical_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization
    ADD CONSTRAINT organization_canonical_name_key UNIQUE (canonical_name);


--
-- Name: organization organization_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization
    ADD CONSTRAINT organization_pkey PRIMARY KEY (id);


--
-- Name: pdc_employer pdc_employer_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pdc_employer
    ADD CONSTRAINT pdc_employer_pkey PRIMARY KEY (employer_id);


--
-- Name: person_organization_affiliation person_organization_affiliation_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.person_organization_affiliation
    ADD CONSTRAINT person_organization_affiliation_pkey PRIMARY KEY (id);


--
-- Name: person person_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.person
    ADD CONSTRAINT person_pkey PRIMARY KEY (id);


--
-- Name: person_source_mention person_source_mention_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.person_source_mention
    ADD CONSTRAINT person_source_mention_pkey PRIMARY KEY (id);


--
-- Name: person_source_mention person_source_mention_source_kind_source_table_source_pk_so_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.person_source_mention
    ADD CONSTRAINT person_source_mention_source_kind_source_table_source_pk_so_key UNIQUE (source_kind, source_table, source_pk, source_row_id, source_name);


--
-- Name: pdc_lobbyist_compensation pk_pdc_lobbyist_compensation; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pdc_lobbyist_compensation
    ADD CONSTRAINT pk_pdc_lobbyist_compensation PRIMARY KEY (filer_id, employer_id, filing_period);


--
-- Name: sao_audit_type sao_audit_type_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sao_audit_type
    ADD CONSTRAINT sao_audit_type_pkey PRIMARY KEY (audit_type_id);


--
-- Name: sao_government_entity sao_government_entity_mcag_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sao_government_entity
    ADD CONSTRAINT sao_government_entity_mcag_key UNIQUE (mcag);


--
-- Name: sao_government_entity sao_government_entity_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sao_government_entity
    ADD CONSTRAINT sao_government_entity_pkey PRIMARY KEY (id);


--
-- Name: sao_government_type sao_government_type_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sao_government_type
    ADD CONSTRAINT sao_government_type_pkey PRIMARY KEY (code);


--
-- Name: sao_report sao_report_audit_report_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sao_report
    ADD CONSTRAINT sao_report_audit_report_number_key UNIQUE (audit_report_number);


--
-- Name: sao_report sao_report_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sao_report
    ADD CONSTRAINT sao_report_pkey PRIMARY KEY (id);


--
-- Name: seattle_auditor_recommendation seattle_auditor_recommendatio_dashboard_id_recommendation_i_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.seattle_auditor_recommendation
    ADD CONSTRAINT seattle_auditor_recommendatio_dashboard_id_recommendation_i_key UNIQUE (dashboard_id, recommendation_id);


--
-- Name: seattle_auditor_recommendation seattle_auditor_recommendation_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.seattle_auditor_recommendation
    ADD CONSTRAINT seattle_auditor_recommendation_pkey PRIMARY KEY (id);


--
-- Name: seattle_operating_budget seattle_operating_budget_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.seattle_operating_budget
    ADD CONSTRAINT seattle_operating_budget_pkey PRIMARY KEY (id);


--
-- Name: seattle_operating_budget seattle_operating_budget_source_dataset_id_source_row_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.seattle_operating_budget
    ADD CONSTRAINT seattle_operating_budget_source_dataset_id_source_row_id_key UNIQUE (source_dataset_id, source_row_id);


--
-- Name: seattle_permit seattle_permit_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.seattle_permit
    ADD CONSTRAINT seattle_permit_pkey PRIMARY KEY (id);


--
-- Name: seattle_permit seattle_permit_source_dataset_id_source_row_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.seattle_permit
    ADD CONSTRAINT seattle_permit_source_dataset_id_source_row_id_key UNIQUE (source_dataset_id, source_row_id);


--
-- Name: source_dataset source_dataset_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_dataset
    ADD CONSTRAINT source_dataset_pkey PRIMARY KEY (id);


--
-- Name: source_dataset source_dataset_source_system_dataset_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_dataset
    ADD CONSTRAINT source_dataset_source_system_dataset_id_key UNIQUE (source_system, dataset_id);


--
-- Name: speaker_assignment speaker_assignment_diarization_job_id_speaker_cluster_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_assignment
    ADD CONSTRAINT speaker_assignment_diarization_job_id_speaker_cluster_id_key UNIQUE (diarization_job_id, speaker_cluster_id);


--
-- Name: speaker_assignment speaker_assignment_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_assignment
    ADD CONSTRAINT speaker_assignment_pkey PRIMARY KEY (id);


--
-- Name: speaker_cluster speaker_cluster_diarization_job_id_cluster_label_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_cluster
    ADD CONSTRAINT speaker_cluster_diarization_job_id_cluster_label_key UNIQUE (diarization_job_id, cluster_label);


--
-- Name: speaker_cluster speaker_cluster_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_cluster
    ADD CONSTRAINT speaker_cluster_pkey PRIMARY KEY (id);


--
-- Name: speaker_identity_evidence speaker_identity_evidence_evidence_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_identity_evidence
    ADD CONSTRAINT speaker_identity_evidence_evidence_key_key UNIQUE (evidence_key);


--
-- Name: speaker_identity_evidence speaker_identity_evidence_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_identity_evidence
    ADD CONSTRAINT speaker_identity_evidence_pkey PRIMARY KEY (id);


--
-- Name: speaker_review_task speaker_review_task_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_review_task
    ADD CONSTRAINT speaker_review_task_pkey PRIMARY KEY (id);


--
-- Name: testifier testifier_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.testifier
    ADD CONSTRAINT testifier_pkey PRIMARY KEY (id);


--
-- Name: tvw_audio_asset tvw_audio_asset_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tvw_audio_asset
    ADD CONSTRAINT tvw_audio_asset_pkey PRIMARY KEY (id);


--
-- Name: tvw_event tvw_event_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tvw_event
    ADD CONSTRAINT tvw_event_pkey PRIMARY KEY (id);


--
-- Name: tvw_event tvw_event_tvw_event_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tvw_event
    ADD CONSTRAINT tvw_event_tvw_event_id_key UNIQUE (tvw_event_id);


--
-- Name: tvw_media_asset tvw_media_asset_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tvw_media_asset
    ADD CONSTRAINT tvw_media_asset_pkey PRIMARY KEY (id);


--
-- Name: tvw_media_asset tvw_media_asset_tvw_event_id_asset_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tvw_media_asset
    ADD CONSTRAINT tvw_media_asset_tvw_event_id_asset_id_key UNIQUE (tvw_event_id, asset_id);


--
-- Name: vendor_entity_match_candidate vendor_entity_match_candidate_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_entity_match_candidate
    ADD CONSTRAINT vendor_entity_match_candidate_pkey PRIMARY KEY (id);


--
-- Name: vendor_entity_match_candidate vendor_entity_match_candidate_source_kind_source_dataset_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_entity_match_candidate
    ADD CONSTRAINT vendor_entity_match_candidate_source_kind_source_dataset_id_key UNIQUE (source_kind, source_dataset_id, source_row_id, source_name, organization_id);


--
-- Name: vendor_entity_match_decision vendor_entity_match_decision_candidate_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_entity_match_decision
    ADD CONSTRAINT vendor_entity_match_decision_candidate_id_key UNIQUE (candidate_id);


--
-- Name: vendor_entity_match_decision vendor_entity_match_decision_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_entity_match_decision
    ADD CONSTRAINT vendor_entity_match_decision_pkey PRIMARY KEY (id);


--
-- Name: idx_admin_audit_log_actor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_admin_audit_log_actor ON public.admin_audit_log USING btree (actor_email, created_at DESC);


--
-- Name: idx_admin_audit_log_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_admin_audit_log_created_at ON public.admin_audit_log USING btree (created_at DESC);


--
-- Name: idx_admin_audit_log_target; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_admin_audit_log_target ON public.admin_audit_log USING btree (target_type, target_id, created_at DESC);


--
-- Name: idx_agenda_item_hearing_bill; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agenda_item_hearing_bill ON public.agenda_item USING btree (hearing_id, bill_id);


--
-- Name: idx_agenda_item_window_agenda; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agenda_item_window_agenda ON public.agenda_item_window USING btree (agenda_item_id, start_ms);


--
-- Name: idx_bill_biennium; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_biennium ON public.bill USING btree (biennium);


--
-- Name: idx_bill_sponsor_person; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_sponsor_person ON public.bill_sponsor USING btree (person_id) WHERE (person_id IS NOT NULL);


--
-- Name: idx_bill_sponsor_roster_membership; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bill_sponsor_roster_membership ON public.bill_sponsor USING btree (legislator_roster_membership_id) WHERE (legislator_roster_membership_id IS NOT NULL);


--
-- Name: idx_bls_obs_series_year; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bls_obs_series_year ON public.bls_observation USING btree (series_id, year);


--
-- Name: idx_census_obs_geo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_census_obs_geo ON public.census_observation USING btree (geography_id);


--
-- Name: idx_census_obs_variable; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_census_obs_variable ON public.census_observation USING btree (variable);


--
-- Name: idx_datawa_contract_agency; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_contract_agency ON public.datawa_contract USING gin (agency_name public.gin_trgm_ops);


--
-- Name: idx_datawa_contract_contractor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_contract_contractor ON public.datawa_contract USING gin (contractor_name public.gin_trgm_ops);


--
-- Name: idx_datawa_contract_fy; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_contract_fy ON public.datawa_contract USING btree (fiscal_year);


--
-- Name: idx_datawa_contract_normalized_contractor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_contract_normalized_contractor ON public.datawa_contract USING btree (normalized_contractor_name) WHERE (normalized_contractor_name IS NOT NULL);


--
-- Name: idx_datawa_it_contract_agency; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_it_contract_agency ON public.datawa_it_contract USING gin (agency_name public.gin_trgm_ops);


--
-- Name: idx_datawa_it_contract_contract; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_it_contract_contract ON public.datawa_it_contract USING btree (contract_number) WHERE (contract_number IS NOT NULL);


--
-- Name: idx_datawa_it_contract_contractor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_it_contract_contractor ON public.datawa_it_contract USING gin (contractor_name public.gin_trgm_ops);


--
-- Name: idx_datawa_it_contract_normalized_contractor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_it_contract_normalized_contractor ON public.datawa_it_contract USING btree (normalized_contractor_name) WHERE (normalized_contractor_name IS NOT NULL);


--
-- Name: idx_datawa_it_contract_normalized_dba; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_it_contract_normalized_dba ON public.datawa_it_contract USING btree (normalized_contractor_dba) WHERE (normalized_contractor_dba IS NOT NULL);


--
-- Name: idx_datawa_it_contract_report_fy; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_it_contract_report_fy ON public.datawa_it_contract USING btree (report_fiscal_year);


--
-- Name: idx_datawa_master_sale_contract; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_master_sale_contract ON public.datawa_master_contract_sale USING btree (contract_number) WHERE (contract_number IS NOT NULL);


--
-- Name: idx_datawa_master_sale_customer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_master_sale_customer ON public.datawa_master_contract_sale USING gin (customer_name public.gin_trgm_ops);


--
-- Name: idx_datawa_master_sale_normalized_customer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_master_sale_normalized_customer ON public.datawa_master_contract_sale USING btree (normalized_customer_name) WHERE (normalized_customer_name IS NOT NULL);


--
-- Name: idx_datawa_master_sale_normalized_vendor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_master_sale_normalized_vendor ON public.datawa_master_contract_sale USING btree (normalized_vendor_name) WHERE (normalized_vendor_name IS NOT NULL);


--
-- Name: idx_datawa_master_sale_vendor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_master_sale_vendor ON public.datawa_master_contract_sale USING gin (vendor_name public.gin_trgm_ops);


--
-- Name: idx_datawa_master_sale_year; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_master_sale_year ON public.datawa_master_contract_sale USING btree (report_year);


--
-- Name: idx_datawa_webs_vendor_commodity_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_webs_vendor_commodity_code ON public.datawa_webs_vendor USING btree (commodity_code) WHERE (commodity_code IS NOT NULL);


--
-- Name: idx_datawa_webs_vendor_company; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_webs_vendor_company ON public.datawa_webs_vendor USING gin (company_name public.gin_trgm_ops);


--
-- Name: idx_datawa_webs_vendor_normalized_company; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_webs_vendor_normalized_company ON public.datawa_webs_vendor USING btree (normalized_company_name);


--
-- Name: idx_datawa_webs_vendor_state; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datawa_webs_vendor_state ON public.datawa_webs_vendor USING btree (state) WHERE (state IS NOT NULL);


--
-- Name: idx_diarization_job_event; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_diarization_job_event ON public.diarization_job USING btree (tvw_event_id, created_at DESC);


--
-- Name: idx_diarization_job_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_diarization_job_status ON public.diarization_job USING btree (status);


--
-- Name: idx_diarized_segment_event_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_diarized_segment_event_time ON public.diarized_speech_segment USING btree (tvw_event_id, start_ms);


--
-- Name: idx_diarized_segment_job_cluster; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_diarized_segment_job_cluster ON public.diarized_speech_segment USING btree (diarization_job_id, cluster_label);


--
-- Name: idx_diarized_segment_text_fts; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_diarized_segment_text_fts ON public.diarized_speech_segment USING gin (to_tsvector('english'::regconfig, text));


--
-- Name: idx_entity_mention_event_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_entity_mention_event_type ON public.entity_mention USING btree (tvw_event_id, entity_type);


--
-- Name: idx_entity_mention_job; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_entity_mention_job ON public.entity_mention USING btree (diarization_job_id);


--
-- Name: idx_entity_mention_text_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_entity_mention_text_trgm ON public.entity_mention USING gin (text public.gin_trgm_ops);


--
-- Name: idx_epa_ej_geo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_epa_ej_geo ON public.epa_ejscreen_observation USING btree (geography_id);


--
-- Name: idx_epa_facility_name_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_epa_facility_name_trgm ON public.epa_facility USING gin (name public.gin_trgm_ops);


--
-- Name: idx_epa_facility_state_city; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_epa_facility_state_city ON public.epa_facility USING btree (state_code, city);


--
-- Name: idx_federal_award_normalized_recipient; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_federal_award_normalized_recipient ON public.federal_award USING btree (normalized_recipient_name) WHERE (normalized_recipient_name IS NOT NULL);


--
-- Name: idx_federal_award_place; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_federal_award_place ON public.federal_award USING btree (place_state_code, place_county);


--
-- Name: idx_federal_award_recipient_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_federal_award_recipient_trgm ON public.federal_award USING gin (recipient_name public.gin_trgm_ops);


--
-- Name: idx_fema_disaster_state_county; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fema_disaster_state_county ON public.fema_disaster_declaration USING btree (state_code, county_name);


--
-- Name: idx_fema_disaster_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fema_disaster_type ON public.fema_disaster_declaration USING btree (incident_type);


--
-- Name: idx_fiscalwa_vendor_payment_agency; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscalwa_vendor_payment_agency ON public.fiscalwa_vendor_payment USING gin (agency_name public.gin_trgm_ops);


--
-- Name: idx_fiscalwa_vendor_payment_bien_fy; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscalwa_vendor_payment_bien_fy ON public.fiscalwa_vendor_payment USING btree (biennium, fiscal_year, fiscal_month);


--
-- Name: idx_fiscalwa_vendor_payment_normalized_vendor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscalwa_vendor_payment_normalized_vendor ON public.fiscalwa_vendor_payment USING btree (normalized_vendor_name) WHERE (normalized_vendor_name IS NOT NULL);


--
-- Name: idx_fiscalwa_vendor_payment_object; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscalwa_vendor_payment_object ON public.fiscalwa_vendor_payment USING btree (object_code, subobject_code);


--
-- Name: idx_fiscalwa_vendor_payment_vendor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fiscalwa_vendor_payment_vendor ON public.fiscalwa_vendor_payment USING gin (vendor_name public.gin_trgm_ops);


--
-- Name: idx_hearing_lws_meeting_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hearing_lws_meeting_id ON public.hearing USING btree (lws_meeting_id) WHERE (lws_meeting_id IS NOT NULL);


--
-- Name: idx_hearing_meeting_datetime; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hearing_meeting_datetime ON public.hearing USING btree (meeting_datetime);


--
-- Name: idx_hearing_tvw_event_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hearing_tvw_event_id ON public.hearing USING btree (tvw_event_id) WHERE (tvw_event_id IS NOT NULL);


--
-- Name: idx_hud_dataset_name_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hud_dataset_name_trgm ON public.hud_dataset_ref USING gin (name public.gin_trgm_ops);


--
-- Name: idx_hud_obs_geo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hud_obs_geo ON public.hud_housing_observation USING btree (geography_id);


--
-- Name: idx_hud_obs_indicator; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hud_obs_indicator ON public.hud_housing_observation USING btree (indicator);


--
-- Name: idx_ingestion_run_job_started; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ingestion_run_job_started ON public.ingestion_run USING btree (job, started_at DESC);


--
-- Name: idx_irs_bmf_name_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_irs_bmf_name_trgm ON public.irs_bmf_organization USING gin (name public.gin_trgm_ops);


--
-- Name: idx_irs_bmf_normalized_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_irs_bmf_normalized_name ON public.irs_bmf_organization USING btree (normalized_name);


--
-- Name: idx_kingcounty_parcel_address_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_kingcounty_parcel_address_trgm ON public.kingcounty_parcel USING gin (address public.gin_trgm_ops);


--
-- Name: idx_kingcounty_parcel_pin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_kingcounty_parcel_pin ON public.kingcounty_parcel USING btree (pin) WHERE (pin IS NOT NULL);


--
-- Name: idx_legislator_name_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_legislator_name_trgm ON public.legislator USING gin (name public.gin_trgm_ops);


--
-- Name: idx_legislator_roster_biennium_chamber_district; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_legislator_roster_biennium_chamber_district ON public.legislator_roster_membership USING btree (biennium, chamber, district);


--
-- Name: idx_legislator_roster_lws_sponsor_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_legislator_roster_lws_sponsor_id ON public.legislator_roster_membership USING btree (lws_sponsor_id);


--
-- Name: idx_legislator_roster_person; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_legislator_roster_person ON public.legislator_roster_membership USING btree (person_id);


--
-- Name: idx_organization_irs_bmf_ein; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_organization_irs_bmf_ein ON public.organization USING btree (irs_bmf_ein) WHERE (irs_bmf_ein IS NOT NULL);


--
-- Name: idx_organization_name_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_organization_name_trgm ON public.organization USING gin (canonical_name public.gin_trgm_ops);


--
-- Name: idx_organization_verified; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_organization_verified ON public.organization USING btree (verified_at) WHERE (verified_at IS NOT NULL);


--
-- Name: idx_pdc_employer_name_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pdc_employer_name_trgm ON public.pdc_employer USING gin (name public.gin_trgm_ops);


--
-- Name: idx_pdc_employer_normalized_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pdc_employer_normalized_name ON public.pdc_employer USING btree (normalized_name);


--
-- Name: idx_pdc_lobbyist_compensation_employer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pdc_lobbyist_compensation_employer_id ON public.pdc_lobbyist_compensation USING btree (employer_id);


--
-- Name: idx_pdc_lobbyist_compensation_filer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pdc_lobbyist_compensation_filer_id ON public.pdc_lobbyist_compensation USING btree (filer_id);


--
-- Name: idx_pdc_lobbyist_compensation_filing_period; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pdc_lobbyist_compensation_filing_period ON public.pdc_lobbyist_compensation USING btree (filing_period);


--
-- Name: idx_person_display_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_person_display_name ON public.person USING btree (display_name);


--
-- Name: idx_person_display_name_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_person_display_name_trgm ON public.person USING gin (display_name public.gin_trgm_ops);


--
-- Name: idx_person_normalized_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_person_normalized_name ON public.person USING btree (normalized_name) WHERE (normalized_name IS NOT NULL);


--
-- Name: idx_person_org_affiliation_mention; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_person_org_affiliation_mention ON public.person_organization_affiliation USING btree (person_mention_id) WHERE (person_mention_id IS NOT NULL);


--
-- Name: idx_person_org_affiliation_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_person_org_affiliation_org ON public.person_organization_affiliation USING btree (organization_id, relationship_type);


--
-- Name: idx_person_org_affiliation_person; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_person_org_affiliation_person ON public.person_organization_affiliation USING btree (person_id, relationship_type);


--
-- Name: idx_person_org_affiliation_record_year; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_person_org_affiliation_record_year ON public.person_organization_affiliation USING btree (record_year) WHERE (record_year IS NOT NULL);


--
-- Name: idx_person_org_affiliation_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_person_org_affiliation_source ON public.person_organization_affiliation USING btree (source_kind, source_table, source_pk);


--
-- Name: idx_person_pdc_lobbyist_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_person_pdc_lobbyist_id ON public.person USING btree (pdc_lobbyist_id) WHERE (pdc_lobbyist_id IS NOT NULL);


--
-- Name: idx_person_source_mention_kind; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_person_source_mention_kind ON public.person_source_mention USING btree (source_kind, source_table);


--
-- Name: idx_person_source_mention_normalized; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_person_source_mention_normalized ON public.person_source_mention USING btree (normalized_name) WHERE (normalized_name IS NOT NULL);


--
-- Name: idx_person_source_mention_person; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_person_source_mention_person ON public.person_source_mention USING btree (person_id);


--
-- Name: idx_sao_entity_name_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sao_entity_name_trgm ON public.sao_government_entity USING gin (name public.gin_trgm_ops);


--
-- Name: idx_sao_report_findings; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sao_report_findings ON public.sao_report USING btree (findings);


--
-- Name: idx_sao_report_released; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sao_report_released ON public.sao_report USING btree (date_released DESC);


--
-- Name: idx_sao_report_title_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sao_report_title_trgm ON public.sao_report USING gin (report_title public.gin_trgm_ops);


--
-- Name: idx_seattle_auditor_rec_department; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_seattle_auditor_rec_department ON public.seattle_auditor_recommendation USING btree (department);


--
-- Name: idx_seattle_auditor_rec_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_seattle_auditor_rec_status ON public.seattle_auditor_recommendation USING btree (status);


--
-- Name: idx_seattle_auditor_rec_text_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_seattle_auditor_rec_text_trgm ON public.seattle_auditor_recommendation USING gin (recommendation_text public.gin_trgm_ops);


--
-- Name: idx_seattle_operating_budget_department; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_seattle_operating_budget_department ON public.seattle_operating_budget USING gin (department public.gin_trgm_ops);


--
-- Name: idx_seattle_operating_budget_fund_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_seattle_operating_budget_fund_type ON public.seattle_operating_budget USING btree (fund_type) WHERE (fund_type IS NOT NULL);


--
-- Name: idx_seattle_operating_budget_fy; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_seattle_operating_budget_fy ON public.seattle_operating_budget USING btree (fiscal_year);


--
-- Name: idx_seattle_operating_budget_program; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_seattle_operating_budget_program ON public.seattle_operating_budget USING gin (program public.gin_trgm_ops);


--
-- Name: idx_seattle_permit_address_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_seattle_permit_address_trgm ON public.seattle_permit USING gin (address public.gin_trgm_ops);


--
-- Name: idx_seattle_permit_number; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_seattle_permit_number ON public.seattle_permit USING btree (permit_number) WHERE (permit_number IS NOT NULL);


--
-- Name: idx_seattle_permit_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_seattle_permit_status ON public.seattle_permit USING btree (status);


--
-- Name: idx_source_dataset_name_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_source_dataset_name_trgm ON public.source_dataset USING gin (name public.gin_trgm_ops);


--
-- Name: idx_source_dataset_system; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_source_dataset_system ON public.source_dataset USING btree (source_system);


--
-- Name: idx_speaker_assignment_label; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_speaker_assignment_label ON public.speaker_assignment USING gin (speaker_label public.gin_trgm_ops);


--
-- Name: idx_speaker_cluster_event; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_speaker_cluster_event ON public.speaker_cluster USING btree (tvw_event_id);


--
-- Name: idx_speaker_identity_evidence_candidate; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_speaker_identity_evidence_candidate ON public.speaker_identity_evidence USING btree (candidate_kind, candidate_id);


--
-- Name: idx_speaker_identity_evidence_job_cluster; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_speaker_identity_evidence_job_cluster ON public.speaker_identity_evidence USING btree (diarization_job_id, speaker_cluster_id);


--
-- Name: idx_speaker_review_task_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_speaker_review_task_status ON public.speaker_review_task USING btree (status, priority DESC, created_at DESC);


--
-- Name: idx_testifier_agenda_active_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_testifier_agenda_active_order ON public.testifier USING btree (agenda_item_id, active, csi_order, id);


--
-- Name: idx_testifier_agenda_item; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_testifier_agenda_item ON public.testifier USING btree (agenda_item_id);


--
-- Name: idx_testifier_org_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_testifier_org_trgm ON public.testifier USING gin (raw_organization public.gin_trgm_ops);


--
-- Name: idx_tvw_audio_asset_event; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tvw_audio_asset_event ON public.tvw_audio_asset USING btree (tvw_event_id);


--
-- Name: idx_tvw_audio_asset_event_content_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tvw_audio_asset_event_content_hash ON public.tvw_audio_asset USING btree (tvw_event_id, content_hash) WHERE (content_hash IS NOT NULL);


--
-- Name: idx_tvw_media_asset_event_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tvw_media_asset_event_type ON public.tvw_media_asset USING btree (tvw_event_id, asset_type);


--
-- Name: idx_tvw_media_asset_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tvw_media_asset_type ON public.tvw_media_asset USING btree (asset_type);


--
-- Name: idx_vendor_entity_match_candidate_normalized; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vendor_entity_match_candidate_normalized ON public.vendor_entity_match_candidate USING btree (normalized_name);


--
-- Name: idx_vendor_entity_match_candidate_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vendor_entity_match_candidate_org ON public.vendor_entity_match_candidate USING btree (organization_id, candidate_confidence);


--
-- Name: idx_vendor_entity_match_candidate_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vendor_entity_match_candidate_source ON public.vendor_entity_match_candidate USING btree (source_kind, source_table, source_pk);


--
-- Name: idx_vendor_entity_match_decision_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vendor_entity_match_decision_org ON public.vendor_entity_match_decision USING btree (organization_id, decision, reviewed_confidence);


--
-- Name: uniq_bill_sponsor_legacy_conflict; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uniq_bill_sponsor_legacy_conflict ON public.bill_sponsor USING btree (bill_id, legislator_id, sponsor_type);


--
-- Name: uniq_bill_sponsor_legacy_legislator; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uniq_bill_sponsor_legacy_legislator ON public.bill_sponsor USING btree (bill_id, legislator_id, sponsor_type) WHERE (legislator_id IS NOT NULL);


--
-- Name: uniq_bill_sponsor_membership; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uniq_bill_sponsor_membership ON public.bill_sponsor USING btree (bill_id, legislator_roster_membership_id, sponsor_type) WHERE (legislator_roster_membership_id IS NOT NULL);


--
-- Name: uniq_person_org_affiliation_source_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uniq_person_org_affiliation_source_identity ON public.person_organization_affiliation USING btree (relationship_type, source_kind, source_table, source_pk, source_row_id, person_id, organization_id, raw_person_name, raw_organization_name) NULLS NOT DISTINCT;


--
-- Name: uniq_speaker_review_task_candidate; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uniq_speaker_review_task_candidate ON public.speaker_review_task USING btree (diarization_job_id, speaker_cluster_id, proposed_candidate_kind, COALESCE(proposed_candidate_id, (0)::bigint), proposed_label);


--
-- Name: uniq_testifier_agenda_source_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uniq_testifier_agenda_source_key ON public.testifier USING btree (agenda_item_id, source_key) WHERE (source_key IS NOT NULL);


--
-- Name: uniq_tvw_audio_asset_event_source_url; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uniq_tvw_audio_asset_event_source_url ON public.tvw_audio_asset USING btree (tvw_event_id, source_url);


--
-- Name: agenda_item agenda_item_bill_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agenda_item
    ADD CONSTRAINT agenda_item_bill_id_fkey FOREIGN KEY (bill_id) REFERENCES public.bill(id);


--
-- Name: agenda_item agenda_item_hearing_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agenda_item
    ADD CONSTRAINT agenda_item_hearing_id_fkey FOREIGN KEY (hearing_id) REFERENCES public.hearing(id) ON DELETE CASCADE;


--
-- Name: agenda_item_window agenda_item_window_agenda_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agenda_item_window
    ADD CONSTRAINT agenda_item_window_agenda_item_id_fkey FOREIGN KEY (agenda_item_id) REFERENCES public.agenda_item(id) ON DELETE CASCADE;


--
-- Name: bill_sponsor bill_sponsor_bill_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_sponsor
    ADD CONSTRAINT bill_sponsor_bill_id_fkey FOREIGN KEY (bill_id) REFERENCES public.bill(id) ON DELETE CASCADE;


--
-- Name: bill_sponsor bill_sponsor_legislator_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_sponsor
    ADD CONSTRAINT bill_sponsor_legislator_id_fkey FOREIGN KEY (legislator_id) REFERENCES public.legislator(id);


--
-- Name: bill_sponsor bill_sponsor_legislator_roster_membership_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_sponsor
    ADD CONSTRAINT bill_sponsor_legislator_roster_membership_id_fkey FOREIGN KEY (legislator_roster_membership_id) REFERENCES public.legislator_roster_membership(id);


--
-- Name: bill_sponsor bill_sponsor_person_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_sponsor
    ADD CONSTRAINT bill_sponsor_person_id_fkey FOREIGN KEY (person_id) REFERENCES public.person(id);


--
-- Name: bill_status_change bill_status_change_bill_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bill_status_change
    ADD CONSTRAINT bill_status_change_bill_id_fkey FOREIGN KEY (bill_id) REFERENCES public.bill(id) ON DELETE CASCADE;


--
-- Name: diarization_job diarization_job_audio_asset_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.diarization_job
    ADD CONSTRAINT diarization_job_audio_asset_id_fkey FOREIGN KEY (audio_asset_id) REFERENCES public.tvw_audio_asset(id);


--
-- Name: diarization_job diarization_job_tvw_event_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.diarization_job
    ADD CONSTRAINT diarization_job_tvw_event_id_fkey FOREIGN KEY (tvw_event_id) REFERENCES public.tvw_event(tvw_event_id) ON DELETE CASCADE;


--
-- Name: diarized_speech_segment diarized_speech_segment_diarization_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.diarized_speech_segment
    ADD CONSTRAINT diarized_speech_segment_diarization_job_id_fkey FOREIGN KEY (diarization_job_id) REFERENCES public.diarization_job(id) ON DELETE CASCADE;


--
-- Name: diarized_speech_segment diarized_speech_segment_speaker_cluster_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.diarized_speech_segment
    ADD CONSTRAINT diarized_speech_segment_speaker_cluster_id_fkey FOREIGN KEY (speaker_cluster_id) REFERENCES public.speaker_cluster(id) ON DELETE CASCADE;


--
-- Name: entity_mention entity_mention_diarization_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.entity_mention
    ADD CONSTRAINT entity_mention_diarization_job_id_fkey FOREIGN KEY (diarization_job_id) REFERENCES public.diarization_job(id) ON DELETE CASCADE;


--
-- Name: entity_mention entity_mention_tvw_event_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.entity_mention
    ADD CONSTRAINT entity_mention_tvw_event_id_fkey FOREIGN KEY (tvw_event_id) REFERENCES public.tvw_event(tvw_event_id) ON DELETE CASCADE;


--
-- Name: testifier fk_testifier_organization; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.testifier
    ADD CONSTRAINT fk_testifier_organization FOREIGN KEY (normalized_org_id) REFERENCES public.organization(id) ON DELETE SET NULL;


--
-- Name: hearing hearing_bill_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hearing
    ADD CONSTRAINT hearing_bill_id_fkey FOREIGN KEY (bill_id) REFERENCES public.bill(id);


--
-- Name: legislator_roster_membership legislator_roster_membership_person_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.legislator_roster_membership
    ADD CONSTRAINT legislator_roster_membership_person_id_fkey FOREIGN KEY (person_id) REFERENCES public.person(id) ON DELETE CASCADE;


--
-- Name: organization organization_irs_bmf_ein_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization
    ADD CONSTRAINT organization_irs_bmf_ein_fkey FOREIGN KEY (irs_bmf_ein) REFERENCES public.irs_bmf_organization(ein) ON DELETE SET NULL;


--
-- Name: person_organization_affiliation person_organization_affiliation_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.person_organization_affiliation
    ADD CONSTRAINT person_organization_affiliation_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organization(id) ON DELETE SET NULL;


--
-- Name: person_organization_affiliation person_organization_affiliation_person_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.person_organization_affiliation
    ADD CONSTRAINT person_organization_affiliation_person_id_fkey FOREIGN KEY (person_id) REFERENCES public.person(id) ON DELETE SET NULL;


--
-- Name: person_organization_affiliation person_organization_affiliation_person_mention_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.person_organization_affiliation
    ADD CONSTRAINT person_organization_affiliation_person_mention_id_fkey FOREIGN KEY (person_mention_id) REFERENCES public.person_source_mention(id) ON DELETE SET NULL;


--
-- Name: person_source_mention person_source_mention_person_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.person_source_mention
    ADD CONSTRAINT person_source_mention_person_id_fkey FOREIGN KEY (person_id) REFERENCES public.person(id) ON DELETE SET NULL;


--
-- Name: speaker_assignment speaker_assignment_diarization_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_assignment
    ADD CONSTRAINT speaker_assignment_diarization_job_id_fkey FOREIGN KEY (diarization_job_id) REFERENCES public.diarization_job(id) ON DELETE CASCADE;


--
-- Name: speaker_assignment speaker_assignment_review_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_assignment
    ADD CONSTRAINT speaker_assignment_review_task_id_fkey FOREIGN KEY (review_task_id) REFERENCES public.speaker_review_task(id);


--
-- Name: speaker_assignment speaker_assignment_speaker_cluster_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_assignment
    ADD CONSTRAINT speaker_assignment_speaker_cluster_id_fkey FOREIGN KEY (speaker_cluster_id) REFERENCES public.speaker_cluster(id) ON DELETE CASCADE;


--
-- Name: speaker_cluster speaker_cluster_diarization_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_cluster
    ADD CONSTRAINT speaker_cluster_diarization_job_id_fkey FOREIGN KEY (diarization_job_id) REFERENCES public.diarization_job(id) ON DELETE CASCADE;


--
-- Name: speaker_identity_evidence speaker_identity_evidence_diarization_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_identity_evidence
    ADD CONSTRAINT speaker_identity_evidence_diarization_job_id_fkey FOREIGN KEY (diarization_job_id) REFERENCES public.diarization_job(id) ON DELETE CASCADE;


--
-- Name: speaker_identity_evidence speaker_identity_evidence_diarized_speech_segment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_identity_evidence
    ADD CONSTRAINT speaker_identity_evidence_diarized_speech_segment_id_fkey FOREIGN KEY (diarized_speech_segment_id) REFERENCES public.diarized_speech_segment(id) ON DELETE CASCADE;


--
-- Name: speaker_identity_evidence speaker_identity_evidence_speaker_cluster_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_identity_evidence
    ADD CONSTRAINT speaker_identity_evidence_speaker_cluster_id_fkey FOREIGN KEY (speaker_cluster_id) REFERENCES public.speaker_cluster(id) ON DELETE CASCADE;


--
-- Name: speaker_review_task speaker_review_task_diarization_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_review_task
    ADD CONSTRAINT speaker_review_task_diarization_job_id_fkey FOREIGN KEY (diarization_job_id) REFERENCES public.diarization_job(id) ON DELETE CASCADE;


--
-- Name: speaker_review_task speaker_review_task_speaker_cluster_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.speaker_review_task
    ADD CONSTRAINT speaker_review_task_speaker_cluster_id_fkey FOREIGN KEY (speaker_cluster_id) REFERENCES public.speaker_cluster(id) ON DELETE CASCADE;


--
-- Name: testifier testifier_agenda_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.testifier
    ADD CONSTRAINT testifier_agenda_item_id_fkey FOREIGN KEY (agenda_item_id) REFERENCES public.agenda_item(id) ON DELETE CASCADE;


--
-- Name: tvw_audio_asset tvw_audio_asset_tvw_event_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tvw_audio_asset
    ADD CONSTRAINT tvw_audio_asset_tvw_event_id_fkey FOREIGN KEY (tvw_event_id) REFERENCES public.tvw_event(tvw_event_id) ON DELETE CASCADE;


--
-- Name: tvw_media_asset tvw_media_asset_tvw_event_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tvw_media_asset
    ADD CONSTRAINT tvw_media_asset_tvw_event_id_fkey FOREIGN KEY (tvw_event_id) REFERENCES public.tvw_event(tvw_event_id) ON DELETE CASCADE;


--
-- Name: vendor_entity_match_candidate vendor_entity_match_candidate_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_entity_match_candidate
    ADD CONSTRAINT vendor_entity_match_candidate_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organization(id) ON DELETE CASCADE;


--
-- Name: vendor_entity_match_decision vendor_entity_match_decision_candidate_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_entity_match_decision
    ADD CONSTRAINT vendor_entity_match_decision_candidate_id_fkey FOREIGN KEY (candidate_id) REFERENCES public.vendor_entity_match_candidate(id) ON DELETE CASCADE;


--
-- Name: vendor_entity_match_decision vendor_entity_match_decision_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_entity_match_decision
    ADD CONSTRAINT vendor_entity_match_decision_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organization(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--



-- +goose Down

-- Initial migration: down step is a no-op. To rebuild from scratch,
-- drop the database and re-apply migrations against a fresh instance.
SELECT 1;
