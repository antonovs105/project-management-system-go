package apiresponse

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

const (
	headerLink = "Link"
	// HeaderPaginationLimit reports the requested page size.
	HeaderPaginationLimit = "X-Pagination-Limit"
	// HeaderPaginationOffset reports the current zero-based offset.
	HeaderPaginationOffset = "X-Pagination-Offset"
	// HeaderPaginationCount reports the number of values in this response.
	HeaderPaginationCount = "X-Pagination-Count"
	// HeaderPaginationHasMore indicates that another page may be requested.
	HeaderPaginationHasMore = "X-Pagination-Has-More"
)

// WriteOffsetPage writes a backward-compatible JSON array with uniform offset
// metadata and RFC 8288 navigation links.
func WriteOffsetPage[T any](c echo.Context, status int, values []T, limit, offset int) error {
	if limit < 1 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	hasMore := len(values) >= limit
	header := c.Response().Header()
	header.Set(HeaderPaginationLimit, strconv.Itoa(limit))
	header.Set(HeaderPaginationOffset, strconv.Itoa(offset))
	header.Set(HeaderPaginationCount, strconv.Itoa(len(values)))
	header.Set(HeaderPaginationHasMore, strconv.FormatBool(hasMore))
	links := paginationLinks(c, limit, offset, hasMore)
	if links != "" {
		header.Set(headerLink, links)
	}
	return c.JSON(status, values)
}

func paginationLinks(c echo.Context, limit, offset int, hasMore bool) string {
	links := make([]string, 0, 2)
	if offset > 0 {
		previous := offset - limit
		if previous < 0 {
			previous = 0
		}
		links = append(links, fmt.Sprintf(`<%s>; rel="prev"`, pageURL(c, limit, previous)))
	}
	if hasMore {
		links = append(links, fmt.Sprintf(`<%s>; rel="next"`, pageURL(c, limit, offset+limit)))
	}
	return strings.Join(links, ", ")
}

func pageURL(c echo.Context, limit, offset int) string {
	requestURL := *c.Request().URL
	query := requestURL.Query()
	query.Set("limit", strconv.Itoa(limit))
	query.Set("offset", strconv.Itoa(offset))
	requestURL.RawQuery = query.Encode()
	return requestURL.RequestURI()
}

// PaginationExposeHeaders returns pagination headers that browser clients may inspect.
func PaginationExposeHeaders() []string {
	return []string{headerLink, HeaderPaginationLimit, HeaderPaginationOffset, HeaderPaginationCount, HeaderPaginationHasMore}
}
