package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"

	"github.com/gorilla/schema"
	"github.com/pkg/errors"
	"golang.org/x/net/html"
)

// Example represents the example pages
type Examples struct {
	Renderer web.Renderer
}

// FlashMessages provides examples for displaying flash messages.
func (h *Examples) FlashMessages(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	// Display example messages only when we aren't handling an example form post.
	if r.Method == http.MethodGet {

		// Example displaying a success message.
		webcontext.SessionFlashSuccess(ctx,
			"Action Successful",
			"You have reached an epic milestone.",
			"800 hours", "304,232 lines of code")

		// Example displaying an info message.
		webcontext.SessionFlashInfo(ctx,
			"Take the Tour",
			"Learn more about the platform...",
			"The pretty little trees in the forest.", "The happy little clouds in the sky.")

		// Example displaying a warning message.
		webcontext.SessionFlashWarning(ctx,
			"Approaching Limit",
			"Your account is reaching is limit, apply now!",
			"Only overt benefit..")

		// Example displaying an error message.
		webcontext.SessionFlashError(ctx,
			"Custom Error",
			"SOMETIMES ITS HELPFUL TO SHOUT.",
			"Listen to me.", "Leaders don't follow.")

		// Example displaying a validation error which will use the json tag as the field name.
		type valDemo struct {
			Email string `json:"email_field_name" validate:"required,email"`
		}
		valErr := webcontext.Validator().StructCtx(ctx, valDemo{})
		weberror.SessionFlashError(ctx, valErr)

		// Generic error message for examples.
		er := errors.New("Root causing undermined. Bailing out.")

		// Example displaying a flash message for a web error with a message.
		webErrWithMsg := weberror.WithMessage(ctx, er, "weberror:WithMessage")
		weberror.SessionFlashError(ctx, webErrWithMsg)

		// Example displaying a flash message for a web error.
		webErr := weberror.NewError(ctx, er, http.StatusInternalServerError)
		weberror.SessionFlashError(ctx, webErr)

		// Example displaying a flash message for an error with a message.
		erWithMsg := errors.WithMessage(er, "pkg/errors:WithMessage")
		weberror.SessionFlashError(ctx, erWithMsg)

		// Example displaying a flash message for an error that has been wrapped.
		erWrap := errors.Wrap(er, "pkg/errors:Wrap")
		weberror.SessionFlashError(ctx, erWrap)
	}

	data := make(map[string]interface{})

	// Example displaying a validation error which will use the json tag as the field name.
	{
		type inlineDemo struct {
			Email       string `json:"email" validate:"required,email"`
			HiddenField string `json:"hidden_field" validate:"required"`
		}

		req := new(inlineDemo)
		f := func() error {

			if r.Method == http.MethodPost {
				err := r.ParseForm()
				if err != nil {
					return err
				}

				decoder := schema.NewDecoder()
				if err := decoder.Decode(req, r.PostForm); err != nil {
					return err
				}

				if err := webcontext.Validator().Struct(req); err != nil {
					if ne, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = ne.(*weberror.Error)
						return nil
					} else {
						return err
					}
				}
			}

			return nil
		}

		if err := f(); err != nil {
			return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
		}

		data["form"] = req

		if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(inlineDemo{})); ok {
			data["validationDefaults"] = verr.(*weberror.Error)
		}
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "examples-flash-messages.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Images provides examples for responsive images that are auto re-sized.
func (h *Examples) Images(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	// List of image sizes that will be used to resize the source image into. The resulting images will then be included
	// as apart of the image src tag for a responsive image tag.
	data := map[string]interface{}{
		"imgResizeDisabled": false,
	}

	// Render the example page to detect in image resize is enabled, since the config is not passed to handlers and
	// the custom HTML resize function is init in main.go.
	rr := httptest.NewRecorder()
	err := h.Renderer.Render(ctx, rr, r, TmplLayoutBase, "examples-images.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
	if err != nil {
		return err
	}

	// Parsed the rendered response looking for an example image tag.
	exampleImgID := "imgVerifyResizeEnabled"
	var exampleImgAttrs []html.Attribute
	doc, err := html.Parse(rr.Body)
	if err != nil {
		return err
	}
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, a := range n.Attr {
				if a.Key == "id" && a.Val == exampleImgID {
					exampleImgAttrs = n.Attr
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	// If the example image has the attribute srcset then we know resize is enabled.
	var exampleImgHasSrcSet bool
	if len(exampleImgAttrs) > 0 {
		for _, a := range exampleImgAttrs {
			if a.Key == "srcset" {
				exampleImgHasSrcSet = true
				break
			}
		}
	}

	// Image resize must be disabled as could not find the example image with attribute srcset.
	if !exampleImgHasSrcSet {
		data["imgResizeDisabled"] = true
	}

	// Re-render the page with additional data and return the results.
	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "examples-images.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}
