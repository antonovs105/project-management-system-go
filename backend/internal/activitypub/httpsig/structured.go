package httpsig

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// signatureInput is a parsed Signature-Input member for one signature label.
type signatureInput struct {
	Label      string
	RawValue   string
	Components []string
	Created    int64
	KeyID      string
	Algorithm  string
}

// parseSignatureInputs parses an RFC 9421 Signature-Input header.
func parseSignatureInputs(header string) ([]signatureInput, error) {
	if strings.TrimSpace(header) == "" {
		return nil, ErrMissingSignature
	}

	members := splitTopLevel(header, ',')
	inputs := make([]signatureInput, 0, len(members))
	for _, member := range members {
		label, rawValue, ok := strings.Cut(strings.TrimSpace(member), "=")
		if !ok || label == "" || rawValue == "" {
			return nil, ErrInvalidSignatureInput
		}
		input, err := parseSignatureInput(strings.TrimSpace(label), strings.TrimSpace(rawValue))
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}

// parseSignatureInput parses a single labeled Signature-Input member.
func parseSignatureInput(label, rawValue string) (signatureInput, error) {
	if !strings.HasPrefix(rawValue, "(") {
		return signatureInput{}, ErrInvalidSignatureInput
	}
	closeIdx := strings.Index(rawValue, ")")
	if closeIdx == -1 {
		return signatureInput{}, ErrInvalidSignatureInput
	}

	componentPart := strings.TrimSpace(rawValue[1:closeIdx])
	components, err := parseInnerList(componentPart)
	if err != nil {
		return signatureInput{}, err
	}
	if len(components) == 0 {
		return signatureInput{}, ErrInvalidSignatureInput
	}

	params := map[string]string{}
	for _, part := range strings.Split(rawValue[closeIdx+1:], ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" {
			return signatureInput{}, ErrInvalidSignatureInput
		}
		unquoted, err := parseParamValue(strings.TrimSpace(value))
		if err != nil {
			return signatureInput{}, err
		}
		params[strings.TrimSpace(key)] = unquoted
	}

	created, err := strconv.ParseInt(params["created"], 10, 64)
	if err != nil {
		return signatureInput{}, ErrInvalidSignatureInput
	}

	return signatureInput{
		Label:      label,
		RawValue:   rawValue,
		Components: components,
		Created:    created,
		KeyID:      params["keyid"],
		Algorithm:  params["alg"],
	}, nil
}

// parseSignatures parses the RFC 9421 Signature header into binary signatures.
func parseSignatures(header string) (map[string][]byte, error) {
	if strings.TrimSpace(header) == "" {
		return nil, ErrMissingSignature
	}

	members := splitTopLevel(header, ',')
	signatures := make(map[string][]byte, len(members))
	for _, member := range members {
		label, rawValue, ok := strings.Cut(strings.TrimSpace(member), "=")
		if !ok || label == "" || rawValue == "" {
			return nil, ErrInvalidSignature
		}
		rawValue = strings.TrimSpace(rawValue)
		if !strings.HasPrefix(rawValue, ":") || !strings.HasSuffix(rawValue, ":") || len(rawValue) < 2 {
			return nil, ErrInvalidSignature
		}
		signature, err := base64.StdEncoding.DecodeString(rawValue[1 : len(rawValue)-1])
		if err != nil {
			return nil, ErrInvalidSignature
		}
		signatures[strings.TrimSpace(label)] = signature
	}
	return signatures, nil
}

// matchingSignature finds the first Signature-Input member with a matching Signature value.
func matchingSignature(inputs []signatureInput, signatures map[string][]byte) (signatureInput, []byte, bool) {
	for _, input := range inputs {
		if signature, ok := signatures[input.Label]; ok {
			return input, signature, true
		}
	}
	return signatureInput{}, nil, false
}

// parseInnerList parses the covered component list from Signature-Input.
func parseInnerList(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	tokens := strings.Fields(raw)
	values := make([]string, 0, len(tokens))
	for _, token := range tokens {
		value, err := parseParamValue(token)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

// parseParamValue parses a structured-field token or quoted string parameter.
func parseParamValue(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidSignatureInput
	}
	if raw[0] != '"' {
		return raw, nil
	}
	if len(raw) < 2 || raw[len(raw)-1] != '"' {
		return "", ErrInvalidSignatureInput
	}
	body := raw[1 : len(raw)-1]
	var b strings.Builder
	escaped := false
	for _, r := range body {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		b.WriteRune(r)
	}
	if escaped {
		return "", ErrInvalidSignatureInput
	}
	return b.String(), nil
}

// splitTopLevel splits a structured header while respecting quotes and inner lists.
func splitTopLevel(raw string, sep rune) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false
	parenDepth := 0
	escaped := false

	for _, r := range raw {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && inQuotes:
			current.WriteRune(r)
			escaped = true
		case r == '"':
			current.WriteRune(r)
			inQuotes = !inQuotes
		case r == '(' && !inQuotes:
			current.WriteRune(r)
			parenDepth++
		case r == ')' && !inQuotes:
			current.WriteRune(r)
			if parenDepth > 0 {
				parenDepth--
			}
		case r == sep && !inQuotes && parenDepth == 0:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// sfString formats a value as an escaped structured-field string.
func sfString(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return fmt.Sprintf("\"%s\"", escaped)
}
