package apiresponse

import (
	"errors"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// ExpectedVersion parses a strong positive numeric If-Match entity tag.
func ExpectedVersion(c echo.Context) (int64, error) {
	value := strings.TrimSpace(c.Request().Header.Get("If-Match"))
	if strings.HasPrefix(value, "W/") {
		return 0, errors.New("a strong If-Match version is required")
	}
	value = strings.Trim(value, `"`)
	version, err := strconv.ParseInt(value, 10, 64)
	if err != nil || version < 1 {
		return 0, errors.New("a valid If-Match version is required")
	}
	return version, nil
}

// SetVersionETag writes a strong entity tag for a versioned response.
func SetVersionETag(c echo.Context, version int64) {
	c.Response().Header().Set("ETag", `"`+strconv.FormatInt(version, 10)+`"`)
}
