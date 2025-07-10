package helper

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"
	"github.com/valyala/fasthttp"
)

type ResponseWrapper struct {
	*fasthttp.Response
}

func (r *ResponseWrapper) DecompressResponse() ([]byte, error) {
	switch string(r.Header.Peek("Content-Encoding")) {
	case "gzip":
		// Create a gzip reader
		gzipReader, err := gzip.NewReader(bytes.NewReader(r.Body()))
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzipReader.Close()

		return io.ReadAll(gzipReader)

	case "deflate":
		// Create a deflate reader
		deflateReader := flate.NewReader(bytes.NewReader(r.Body()))
		defer deflateReader.Close()

		return io.ReadAll(deflateReader)

	case "br":
		// Create a brotli reader
		brReader := brotli.NewReader(bytes.NewReader(r.Body()))

		return io.ReadAll(brReader)

	default:
		// No compression or unknown compression
		return r.Body(), nil
	}
}
