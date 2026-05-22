# Written-testimony public access note

**Question:** Does CSI expose attached written-testimony text/PDFs through a
public no-auth endpoint, or does it require records requests?

**Status:** Still not investigated as of the completed public-beta plan. Written-testimony attachments are not part of the current product surface; CSI sign-in metadata remains the supported testimony source.

**Method to use:**

1. Pick 5 recent CSI agenda items with confirmed written testimony submitted
   (e.g. high-traffic housing or transportation hearings).
2. From a clean browser session (no auth), navigate the CSI flow:
   - `https://app.leg.wa.gov/csi/<Chamber>`
   - select committee → meeting → agenda item.
3. Inspect the network panel and HTML for any link/button labeled "View
   testimony," "Read submission," "Download attachment," etc.
4. Test each candidate URL from a no-cookie request (e.g. `curl --no-cookie`).
5. Note: per `data-sources/02` line 21, "a no-auth bulk endpoint for actual
   text/attachments is not yet confirmed." Confirm or refute.

**Record findings here, with:**

- agenda_item_id checked
- public URLs surfaced (if any)
- response status from a clean curl
- whether the attachment is text, PDF, or other
- any anti-forgery token / session requirement observed

**If no public endpoint exists:** the product should continue sourcing testimony from CSI sign-in metadata (Position, Organization, testified/registered-only flag, etc.) and label written testimony as outside the currently ingested source set.
