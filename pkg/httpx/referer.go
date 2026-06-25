package httpx

import (
	"net/http"
)

func SetReferer(r *http.Request, last *http.Response) {
	if last != nil && last.Request != nil && last.Request.URL != nil {
		r.Header.Set("Referer", last.Request.URL.String())
	}
}
