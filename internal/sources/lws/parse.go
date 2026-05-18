package lws

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// SOAPFault represents a soap:Fault returned by LWS.
type SOAPFault struct {
	Code    string
	Message string
}

func (f SOAPFault) Error() string { return fmt.Sprintf("soap fault %s: %s", f.Code, f.Message) }

// envelope is a generic <soap:Envelope><soap:Body><Inner/></Body></Envelope>
// parser. We unmarshal once into a tagged struct and then re-unmarshal the
// inner XML into the operation-specific struct.
type envelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Fault *struct {
			Faultcode   string `xml:"faultcode"`
			Faultstring string `xml:"faultstring"`
		} `xml:"Fault"`
		Inner xmlAny `xml:",any"`
	} `xml:"Body"`
}

type xmlAny struct {
	XMLName  xml.Name
	InnerXML []byte `xml:",innerxml"`
}

func unmarshalSOAPBody(body []byte, target any) error {
	var env envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}
	if env.Body.Fault != nil {
		return SOAPFault{Code: env.Body.Fault.Faultcode, Message: cleanFault(env.Body.Fault.Faultstring)}
	}
	if target == nil {
		return nil
	}
	// Re-wrap the inner element so xml.Unmarshal can find it by name.
	wrapper := append([]byte("<"+env.Body.Inner.XMLName.Local+`>`), env.Body.Inner.InnerXML...)
	wrapper = append(wrapper, []byte("</"+env.Body.Inner.XMLName.Local+">")...)
	if err := xml.Unmarshal(wrapper, target); err != nil {
		return fmt.Errorf("unmarshal %s: %w", env.Body.Inner.XMLName.Local, err)
	}
	return nil
}

func cleanFault(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "---&gt;", "—"))
}

// ---------------------------------------------------------------------------
// Domain types — mirror the LWS XML shapes captured against the live API.
// ---------------------------------------------------------------------------

type Legislation struct {
	Biennium          string  `xml:"Biennium"`
	BillID            string  `xml:"BillId"`
	BillNumber        string  `xml:"BillNumber"`
	OriginalAgency    string  `xml:"OriginalAgency"`
	Active            bool    `xml:"Active"`
	ShortDescription  string  `xml:"ShortDescription"`
	LongDescription   string  `xml:"LongDescription"`
	LegalTitle        string  `xml:"LegalTitle"`
	IntroducedDate    string  `xml:"IntroducedDate"`
	Sponsor           string  `xml:"Sponsor"`        // free-text "(Simmons)" — distinct from GetSponsors
	PrimeSponsorID    string  `xml:"PrimeSponsorID"`
	SubstituteVersion string  `xml:"SubstituteVersion"`
	EngrossedVersion  string  `xml:"EngrossedVersion"`
	CurrentStatus     *CurrentStatus `xml:"CurrentStatus"`
}

type CurrentStatus struct {
	BillID                 string `xml:"BillId"`
	HistoryLine            string `xml:"HistoryLine"`
	ActionDate             string `xml:"ActionDate"`
	AmendedByOppositeBody  bool   `xml:"AmendedByOppositeBody"`
	PartialVeto            bool   `xml:"PartialVeto"`
	Veto                   bool   `xml:"Veto"`
	AmendmentsExist        bool   `xml:"AmendmentsExist"`
	Status                 string `xml:"Status"`
}

type Sponsor struct {
	ID        string `xml:"Id"`
	Name      string `xml:"Name"`
	LongName  string `xml:"LongName"`
	Agency    string `xml:"Agency"`
	Acronym   string `xml:"Acronym"`
	Type      string `xml:"Type"` // "Primary" | "Secondary"
	Order     int    `xml:"Order"`
	Phone     string `xml:"Phone"`
	Email     string `xml:"Email"`
	FirstName string `xml:"FirstName"`
	LastName  string `xml:"LastName"`
}

type Committee struct {
	ID       string `xml:"Id"`
	Name     string `xml:"Name"`
	LongName string `xml:"LongName"`
	Agency   string `xml:"Agency"`
	Acronym  string `xml:"Acronym"`
	Phone    string `xml:"Phone"`
}

type CommitteeMeeting struct {
	AgendaID            string      `xml:"AgendaId"`
	Agency              string      `xml:"Agency"`
	Committees          []Committee `xml:"Committees>Committee"`
	Room                string      `xml:"Room"`
	Building            string      `xml:"Building"`
	Address             string      `xml:"Address"`
	City                string      `xml:"City"`
	State               string      `xml:"State"`
	Date                string      `xml:"Date"`
	Cancelled           bool        `xml:"Cancelled"`
	RevisedDate         string      `xml:"RevisedDate"`
	CommitteeType       string      `xml:"CommitteeType"`
	Notes               string      `xml:"Notes"`
}

type Hearing struct {
	BillID                  string           `xml:"BillId"`
	Biennium                string           `xml:"Biennium"`
	HearingType             string           `xml:"HearingType"`             // "Public" | "Executive"
	HearingTypeDescription  string           `xml:"HearingTypeDescription"`
	CommitteeMeeting        CommitteeMeeting `xml:"CommitteeMeeting"`
}

type RollCall struct {
	BillID         string `xml:"BillId"`
	Agency         string `xml:"Agency"`
	Motion         string `xml:"Motion"`
	SequenceNumber string `xml:"SequenceNumber"`
	VoteDate       string `xml:"VoteDate"`
	YeaVotes       int    `xml:"YeaVotes"`
	NayVotes       int    `xml:"NayVotes"`
	AbsentVotes    int    `xml:"AbsentVotes"`
	ExcusedVotes   int    `xml:"ExcusedVotes"`
}

type StatusChange struct {
	BillID      string `xml:"BillId"`
	HistoryLine string `xml:"HistoryLine"`
	ActionDate  string `xml:"ActionDate"`
	Status      string `xml:"Status"`
}

type LegislationInfo struct {
	Biennium       string `xml:"Biennium"`
	BillID         string `xml:"BillId"`
	BillNumber     string `xml:"BillNumber"`
	OriginalAgency string `xml:"OriginalAgency"`
	Active         bool   `xml:"Active"`
	ShortLegislationType struct {
		ShortType string `xml:"ShortLegislationType"`
		LongType  string `xml:"LongLegislationType"`
	} `xml:"ShortLegislationType"`
}

// ---------------------------------------------------------------------------
// Operation parsers — each unwraps <{Op}Result>...</{Op}Result> from the
// SOAP body.
// ---------------------------------------------------------------------------

func ParseGetLegislation(body []byte) (*Legislation, error) {
	var resp struct {
		XMLName xml.Name
		Result  struct {
			Legislation Legislation `xml:"Legislation"`
		} `xml:"GetLegislationResult"`
	}
	if err := unmarshalSOAPBody(body, &resp); err != nil {
		return nil, err
	}
	return &resp.Result.Legislation, nil
}

func ParseGetCurrentStatus(body []byte) (*CurrentStatus, error) {
	var resp struct {
		XMLName xml.Name
		Result  CurrentStatus `xml:"GetCurrentStatusResult"`
	}
	if err := unmarshalSOAPBody(body, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

func ParseSponsors(body []byte) ([]Sponsor, error) {
	var resp struct {
		XMLName xml.Name
		Result  struct {
			Sponsors []Sponsor `xml:"Sponsor"`
		} `xml:"GetSponsorsResult"`
	}
	if err := unmarshalSOAPBody(body, &resp); err != nil {
		return nil, err
	}
	return resp.Result.Sponsors, nil
}

func ParseHearings(body []byte) ([]Hearing, error) {
	var resp struct {
		XMLName xml.Name
		Result  struct {
			Hearings []Hearing `xml:"Hearing"`
		} `xml:"GetHearingsResult"`
	}
	if err := unmarshalSOAPBody(body, &resp); err != nil {
		return nil, err
	}
	return resp.Result.Hearings, nil
}

func ParseRollCalls(body []byte) ([]RollCall, error) {
	var resp struct {
		XMLName xml.Name
		Result  struct {
			RollCalls []RollCall `xml:"RollCall"`
		} `xml:"GetRollCallsResult"`
	}
	if err := unmarshalSOAPBody(body, &resp); err != nil {
		return nil, err
	}
	return resp.Result.RollCalls, nil
}

func ParseStatusChanges(body []byte) ([]StatusChange, error) {
	var resp struct {
		XMLName xml.Name
		Result  struct {
			Changes []StatusChange `xml:"LegislativeStatus"`
		} `xml:"GetLegislativeStatusChangesByBillNumberResult"`
	}
	if err := unmarshalSOAPBody(body, &resp); err != nil {
		return nil, err
	}
	return resp.Result.Changes, nil
}

func ParseLegislationByYear(body []byte) ([]LegislationInfo, error) {
	var resp struct {
		XMLName xml.Name
		Result  struct {
			Items []LegislationInfo `xml:"LegislationInfo"`
		} `xml:"GetLegislationByYearResult"`
	}
	if err := unmarshalSOAPBody(body, &resp); err != nil {
		return nil, err
	}
	return resp.Result.Items, nil
}

// Member is one row from SponsorService.Get{House,Senate}Sponsors. The
// service returns full chamber rosters for a biennium — including
// members who haven't sponsored a bill — which is the missing input for
// our /legislators index.
type Member struct {
	ID        string `xml:"Id"`
	Name      string `xml:"Name"`      // "Emily Alvarado"
	LongName  string `xml:"LongName"`  // "Senator Alvarado"
	Agency    string `xml:"Agency"`    // "House" | "Senate"
	Acronym   string `xml:"Acronym"`   // "ALVA"
	Party     string `xml:"Party"`     // "D" | "R"
	District  string `xml:"District"`  // "34"
	Phone     string `xml:"Phone"`     // "(360) 786-7667"
	Email     string `xml:"Email"`     // "Emily.Alvarado@leg.wa.gov"
	FirstName string `xml:"FirstName"`
	LastName  string `xml:"LastName"`
}

// ParseSenateSponsors parses GetSenateSponsorsResponse.
func ParseSenateSponsors(body []byte) ([]Member, error) {
	var resp struct {
		XMLName xml.Name
		Result  struct {
			Members []Member `xml:"Member"`
		} `xml:"GetSenateSponsorsResult"`
	}
	if err := unmarshalSOAPBody(body, &resp); err != nil {
		return nil, err
	}
	return resp.Result.Members, nil
}

// ParseHouseSponsors parses GetHouseSponsorsResponse.
func ParseHouseSponsors(body []byte) ([]Member, error) {
	var resp struct {
		XMLName xml.Name
		Result  struct {
			Members []Member `xml:"Member"`
		} `xml:"GetHouseSponsorsResult"`
	}
	if err := unmarshalSOAPBody(body, &resp); err != nil {
		return nil, err
	}
	return resp.Result.Members, nil
}
