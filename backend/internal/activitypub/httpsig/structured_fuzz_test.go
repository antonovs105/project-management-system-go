package httpsig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// FuzzSignatureHeaderParsers checks that untrusted structured-field headers do
// not panic and that successful parses retain usable labels and components.
func FuzzSignatureHeaderParsers(f *testing.F) {
	f.Add(`sig1=("@method" "@target-uri");created=1710000000;keyid="https://example.test/actor#main-key";alg="rsa-v1_5-sha256"`, `sig1=:AQID:`)
	f.Add("", "")
	f.Add(`broken=("unterminated);created=nope`, `broken=:%%%:`)

	f.Fuzz(func(t *testing.T, inputHeader, signatureHeader string) {
		if len(inputHeader)+len(signatureHeader) > 64*1024 {
			t.Skip()
		}
		inputs, inputErr := parseSignatureInputs(inputHeader)
		if inputErr == nil {
			require.NotEmpty(t, inputs)
			for _, input := range inputs {
				require.NotEmpty(t, input.Label)
				require.NotEmpty(t, input.Components)
			}
		}
		signatures, signatureErr := parseSignatures(signatureHeader)
		if signatureErr == nil {
			require.NotEmpty(t, signatures)
			for label := range signatures {
				require.NotEmpty(t, label)
			}
		}
		if inputErr == nil && signatureErr == nil {
			_, _, _ = matchingSignature(inputs, signatures)
		}
	})
}
