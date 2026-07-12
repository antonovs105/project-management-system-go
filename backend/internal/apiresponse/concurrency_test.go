package apiresponse

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestExpectedVersionRequiresStrongPositiveETag(t *testing.T) {
	for _, testCase := range []struct {
		value    string
		expected int64
		valid    bool
	}{
		{value: `"7"`, expected: 7, valid: true},
		{value: "7", expected: 7, valid: true},
		{value: `W/"7"`},
		{value: "0"},
		{value: ""},
	} {
		e := echo.New()
		request := httptest.NewRequest(http.MethodPatch, "/resource", nil)
		request.Header.Set("If-Match", testCase.value)
		context := e.NewContext(request, httptest.NewRecorder())
		version, err := ExpectedVersion(context)
		if testCase.valid {
			require.NoError(t, err)
			require.Equal(t, testCase.expected, version)
		} else {
			require.Error(t, err)
		}
	}
}
