package webfinger

import "errors"

var (
	ErrInvalidResource = errors.New("invalid webfinger resource")
	ErrNotFound        = errors.New("webfinger resource not found")
)

type ActorResource struct {
	Username string `db:"username"`
	Handle   string `db:"handle"`
	APID     string `db:"ap_id"`
}

type JRD struct {
	Subject string   `json:"subject"`
	Aliases []string `json:"aliases,omitempty"`
	Links   []Link   `json:"links"`
}

type Link struct {
	Rel  string `json:"rel"`
	Type string `json:"type,omitempty"`
	Href string `json:"href,omitempty"`
}
