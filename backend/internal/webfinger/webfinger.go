package webfinger

import "errors"

var (
	// ErrInvalidResource reports a malformed WebFinger resource query.
	ErrInvalidResource = errors.New("invalid webfinger resource")
	// ErrNotFound reports that a WebFinger resource is not served by this instance.
	ErrNotFound = errors.New("webfinger resource not found")
)

// ActorResource is the local actor data needed to build a WebFinger response.
type ActorResource struct {
	Username string `db:"username"`
	Handle   string `db:"handle"`
	APID     string `db:"ap_id"`
}

// JRD is a WebFinger JSON Resource Descriptor response.
type JRD struct {
	Subject string   `json:"subject"`
	Aliases []string `json:"aliases,omitempty"`
	Links   []Link   `json:"links"`
}

// Link describes a WebFinger relation from the queried resource.
type Link struct {
	Rel  string `json:"rel"`
	Type string `json:"type,omitempty"`
	Href string `json:"href,omitempty"`
}
