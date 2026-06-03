-- +goose Up
-- +goose StatementBegin

-- Expand abbreviations BEFORE stop-word removal so that "Dept" and
-- "Department", "Intl" and "International", "WA" and "Washington", etc., all
-- normalize to the same string. The expansions intentionally err on the side
-- of safety: only abbreviations whose long form is unambiguous in this domain
-- are listed. (e.g. "co" → "company" is NOT expanded — it would collide with
-- "co-op" / "co-operative" mid-word; the existing stop-word list already
-- handles "co" / "company" the other direction.)
--
-- The function still:
--   - uppercases
--   - replaces '&' with 'AND'
--   - splits on non-alphanumeric runs
--   - drops the stop words from 0010 (THE, INC, LLC, ASSN, ASSOCIATION, ...)
--
-- The new step is a pre-pass that, for each token, replaces a known
-- abbreviation with its canonical long form. This runs token-by-token so
-- multi-word matches don't fire across token boundaries.
CREATE OR REPLACE FUNCTION wa_dd_normalize_entity_name(raw TEXT)
RETURNS TEXT
LANGUAGE sql
IMMUTABLE
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

-- Backfill all normalized_* columns with the new function. Order matters:
-- organization first, since the entity-match query joins on it.
UPDATE organization
   SET updated_at = updated_at;  -- no-op, function is IMMUTABLE so existing
                                  -- rows don't have a stored normalized_name.

UPDATE pdc_employer
   SET normalized_name = COALESCE(wa_dd_normalize_entity_name(name), '');

UPDATE irs_bmf_organization
   SET normalized_name = COALESCE(wa_dd_normalize_entity_name(name), '');

UPDATE datawa_contract
   SET normalized_contractor_name = wa_dd_normalize_entity_name(contractor_name)
 WHERE contractor_name IS NOT NULL;

UPDATE datawa_master_contract_sale
   SET normalized_vendor_name   = wa_dd_normalize_entity_name(vendor_name),
       normalized_customer_name = wa_dd_normalize_entity_name(customer_name);

UPDATE datawa_it_contract
   SET normalized_contractor_name = wa_dd_normalize_entity_name(contractor_name),
       normalized_contractor_dba  = wa_dd_normalize_entity_name(contractor_dba);

UPDATE datawa_webs_vendor
   SET normalized_company_name = wa_dd_normalize_entity_name(company_name);

UPDATE fiscalwa_vendor_payment
   SET normalized_vendor_name = wa_dd_normalize_entity_name(vendor_name)
 WHERE vendor_name IS NOT NULL;

UPDATE federal_award
   SET normalized_recipient_name = wa_dd_normalize_entity_name(recipient_name)
 WHERE recipient_name IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Restore the 0010 version of the function. Existing normalized_* columns are
-- left as-is on down; rerunning the corresponding ingest will overwrite them.
CREATE OR REPLACE FUNCTION wa_dd_normalize_entity_name(raw TEXT)
RETURNS TEXT
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT NULLIF(
        array_to_string(
            ARRAY(
                SELECT token
                  FROM unnest(regexp_split_to_array(upper(regexp_replace(coalesce(raw, ''), '&', ' AND ', 'g')), '[^A-Z0-9]+')) token
                 WHERE token <> ''
                   AND token NOT IN (
                       'THE','A','AN','INC','INCORPORATED','LLC','L','LTD','LIMITED',
                       'CORP','CORPORATION','CO','COMPANY','PLC','PC','PLLC','LP','LLP',
                       'ASSN','ASSOCIATION'
                   )
            ),
            ' '
        ),
        ''
    );
$$;

-- +goose StatementEnd
