package handlers

import (
	"context"
	"fmt"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	project_routes "geeks-accelerator/oss/saas-starter-kit/internal/project-routes"
	"github.com/jmoiron/sqlx"
)

// Root represents the Root API method handler set.
type Root struct {
	MasterDB      *sqlx.DB
	Renderer      web.Renderer
	ProjectRoutes project_routes.ProjectRoutes
}

// Index determines if the user has authentication and loads the associated page.
func (h *Root) Index(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	if claims, err := auth.ClaimsFromContext(ctx); err == nil && claims.HasAuth() {
		return h.indexDashboard(ctx, w, r, params)
	}

	return h.indexDefault(ctx, w, r, params)
}

// indexDashboard loads the dashboard for a user when they are authenticated.
func (h *Root) indexDashboard(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	data := map[string]interface{}{
		"imgSizes": []int{100, 200, 300, 400, 500},
	}

	return h.Renderer.Render(ctx, w, r, tmplLayoutBase, "root-dashboard.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// indexDefault loads the root index page when a user has no authentication.
func (u *Root) indexDefault(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	return u.Renderer.Render(ctx, w, r, tmplLayoutSite, "site-index.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, nil)

}

// indexDefault loads the root index page when a user has no authentication.
func (u *Root) SitePage(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	var tmpName string
	switch r.RequestURI {
		case "/":
			tmpName = "site-index.gohtml"
		case "/api":
			tmpName = "site-api.gohtml"
		case "/features":
			tmpName = "site-features.gohtml"
		case "/support":
			tmpName = "site-support.gohtml"
		case "/legal/privacy":
			tmpName = "legal-privacy.gohtml"
		case "/legal/terms":
			tmpName = "legal-terms.gohtml"
		default:
			http.Redirect(w, r, "/", http.StatusFound)
			return nil
	}

	return u.Renderer.Render(ctx, w, r, tmplLayoutSite, tmpName, web.MIMETextHTMLCharsetUTF8, http.StatusOK, nil)

}

// IndexHtml redirects /index.html to the website root page.
func (u *Root) IndexHtml(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	http.Redirect(w, r, "/", http.StatusMovedPermanently)
	return nil
}

// RobotHandler returns a robots.txt response.
func (h *Root) RobotTxt(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	if webcontext.ContextEnv(ctx) != webcontext.Env_Prod {
		txt := "User-agent: *\nDisallow: /"
		return web.RespondText(ctx, w, txt, http.StatusOK)
	}

	sitemapUrl := h.ProjectRoutes.WebAppUrl("/sitemap.xml")

	txt := fmt.Sprintf("User-agent: *\nDisallow: /ping\nDisallow: /status\nDisallow: /debug/\nSitemap: %s", sitemapUrl)
	return web.RespondText(ctx, w, txt, http.StatusOK)
}
