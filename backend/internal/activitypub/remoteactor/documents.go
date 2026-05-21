package remoteactor

import "encoding/json"

// webFingerDocument is the subset of a remote JRD response used for actor discovery.
type webFingerDocument struct {
	Subject string          `json:"subject"`
	Aliases []string        `json:"aliases,omitempty"`
	Links   []webFingerLink `json:"links"`
}

// webFingerLink is one relation entry inside a WebFinger JRD response.
type webFingerLink struct {
	Rel  string `json:"rel"`
	Type string `json:"type,omitempty"`
	Href string `json:"href,omitempty"`
}

// actorDocument is the remote ActivityPub actor shape accepted by the cache.
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

// publicKeyDocument is the public key object embedded in supported actor documents.
type publicKeyDocument struct {
	ID           string `json:"id"`
	Owner        string `json:"owner"`
	PublicKeyPEM string `json:"publicKeyPem"`
}
