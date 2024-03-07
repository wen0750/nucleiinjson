package http

import (
	"io"
	"strings"

	"github.com/projectdiscovery/rawhttp"
	"github.com/wen0750/nucleiinjson/pkg/protocols/common/generators"
)

// dump creates a dump of the http request in form of a byte slice
func dump(req *generatedRequest, reqURL string) ([]byte, error) {
	if req.request != nil {
		return req.request.Dump()
	}
	rawHttpOptions := &rawhttp.Options{CustomHeaders: req.rawRequest.UnsafeHeaders, CustomRawBytes: req.rawRequest.UnsafeRawBytes}
	return rawhttp.DumpRequestRaw(req.rawRequest.Method, reqURL, req.rawRequest.Path, generators.ExpandMapValues(req.rawRequest.Headers), io.NopCloser(strings.NewReader(req.rawRequest.Data)), rawHttpOptions)
}
