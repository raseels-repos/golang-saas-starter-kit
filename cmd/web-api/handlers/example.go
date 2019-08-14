package handlers

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"geeks-accelerator/oss/saas-starter-kit/internal/project"
	"github.com/pkg/errors"
	"net/http"
)

// Example represents the Example API method handler set.
type Example struct {
	Project *project.Repository

	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

// ErrorResponse returns example error messages.
func (h *Example) ErrorResponse(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	if qv := r.URL.Query().Get("test-validation-error"); qv != "" {
		_, err := h.Project.Create(ctx, auth.Claims{}, project.ProjectCreateRequest{}, v.Now)
		return web.RespondJsonError(ctx, w, err)
	}

	if qv := r.URL.Query().Get("test-web-error"); qv != "" {
		terr := errors.New("Some random error")
		terr = errors.WithMessage(terr, "Actual error message")
		rerr := weberror.NewError(ctx, terr, http.StatusBadRequest).(*weberror.Error)
		rerr.Message = "Test Web Error Message"
		return web.RespondJsonError(ctx, w, rerr)
	}

	if qv := r.URL.Query().Get("test-error"); qv != "" {
		terr := errors.New("Test error")
		terr = errors.WithMessage(terr, "Error message")
		return web.RespondJsonError(ctx, w, terr)
	}

	return nil
}
