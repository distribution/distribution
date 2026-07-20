package handlers

import (
	"errors"
	"net/http"
	"testing"

	"github.com/distribution/distribution/v3/registry/api/errcode"
)

func TestErrcodeErrorsFor(t *testing.T) {
	for _, tc := range []struct {
		name           string
		err            error
		wantCode       errcode.ErrorCode
		wantHTTPStatus int
	}{
		{
			name:           "upstream errcode.Errors are preserved",
			err:            errcode.Errors{errcode.ErrorCodeDenied.WithMessage("requested access to the resource is denied"), errcode.ErrorCodeUnauthorized.WithMessage("authentication required")},
			wantCode:       errcode.ErrorCodeDenied,
			wantHTTPStatus: http.StatusForbidden,
		},
		{
			name:           "single coded error is preserved",
			err:            errcode.ErrorCodeUnauthorized.WithMessage("authentication required"),
			wantCode:       errcode.ErrorCodeUnauthorized,
			wantHTTPStatus: http.StatusUnauthorized,
		},
		{
			name:           "uncoded error becomes unknown",
			err:            errors.New("boom"),
			wantCode:       errcode.ErrorCodeUnknown,
			wantHTTPStatus: http.StatusInternalServerError,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := errcodeErrorsFor(tc.err)
			if len(got) == 0 {
				t.Fatalf("errcodeErrorsFor(%v) returned no errors", tc.err)
			}
			coder, ok := got[0].(errcode.ErrorCoder)
			if !ok {
				t.Fatalf("first error %#v does not implement errcode.ErrorCoder", got[0])
			}
			if coder.ErrorCode() != tc.wantCode {
				t.Errorf("code = %v, want %v", coder.ErrorCode(), tc.wantCode)
			}
			if sc := coder.ErrorCode().Descriptor().HTTPStatusCode; sc != tc.wantHTTPStatus {
				t.Errorf("HTTP status = %d, want %d", sc, tc.wantHTTPStatus)
			}
		})
	}
}
