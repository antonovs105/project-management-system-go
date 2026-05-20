package remoteactor

import "encoding/json"

type webFingerDocument struct {
	Subject string          `json:"subject"`
	Aliases []string        `json:"aliases,omitempty"`
	Links   []webFingerLink `json:"links"`
}

type webFingerLink struct {
	Rel  string `json:"rel"`
	Type string `json:"type,omitempty"`
	Href string `json:"href,omitempty"`
}

type actorDocument struct {
	ID                string          `json:"id"`
	Type              any             `json:"type"`
	PreferredUsername string          `json:"preferredUsername"`
	Name              string          `json:"name"`
	Summary           string          `json:"summary"`
	Inbox             string          `json:"inbox"`
	Outbox            string          `json:"outbox"`
	Followers         string          `json:"followers"`
	Following         string          `json:"following"`
	PublicKey         json.RawMessage `json:"publicKey"`
}

type publicKeyDocument struct {
	ID           string `json:"id"`
	Owner        string `json:"owner"`
	PublicKeyPEM string `json:"publicKeyPem"`
}
