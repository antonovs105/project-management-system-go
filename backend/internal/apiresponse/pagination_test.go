package apiresponse

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestWriteOffsetPagePreservesFiltersAndAddsNavigation(t *testing.T) {
	e := echo.New()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/items?state=open&limit=2&offset=2", nil)
	context := e.NewContext(request, recorder)

	require.NoError(t, WriteOffsetPage(context, http.StatusOK, []string{"a", "b"}, 2, 2))
	require.Equal(t, "2", recorder.Header().Get(HeaderPaginationLimit))
	require.Equal(t, "2", recorder.Header().Get(HeaderPaginationOffset))
	require.Equal(t, "2", recorder.Header().Get(HeaderPaginationCount))
	require.Equal(t, "true", recorder.Header().Get(HeaderPaginationHasMore))
	require.Equal(t, `</items?limit=2&offset=0&state=open>; rel="prev", </items?limit=2&offset=4&state=open>; rel="next"`, recorder.Header().Get(headerLink))
	require.JSONEq(t, `["a","b"]`, recorder.Body.String())
}
