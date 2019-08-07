package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	project_routes "geeks-accelerator/oss/saas-starter-kit/internal/project-routes"
	"github.com/ikeikeikeike/go-sitemap-generator/v2/stm"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/sethgrid/pester"
	"io/ioutil"
	"net/http"
)

// Root represents the Root API method handler set.
type Root struct {
	MasterDB      *sqlx.DB
	Renderer      web.Renderer
	Sitemap       *stm.Sitemap
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

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "root-dashboard.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// indexDefault loads the root index page when a user has no authentication.
func (h *Root) indexDefault(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	return h.Renderer.Render(ctx, w, r, tmplLayoutSite, "site-index.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, nil)
}

// SitePage loads the page with the layout for site instead of the app base.
func (h *Root) SitePage(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	data := make(map[string]interface{})

	var tmpName string
	switch r.RequestURI {
	case "/":
		tmpName = "site-index.gohtml"
	case "/api":
		tmpName = "site-api.gohtml"

		// http://127.0.0.1:3001/docs/doc.json
		swaggerJsonUrl := h.ProjectRoutes.ApiDocsJson()

		// Load the json file from the API service.
		res, err := pester.Get(swaggerJsonUrl)
		if err != nil {
			return errors.WithMessagef(err, "Failed to load url '%s' for api documentation.", swaggerJsonUrl)
		}

		// Read the entire response body.
		dat, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return errors.WithStack(err)
		}

		// Define the basic JSON struct for the JSON file.
		type swaggerInfo struct {
			Description string `json:"description"`
			Title       string `json:"title"`
			Version     string `json:"version"`
		}
		type swaggerDoc struct {
			Schemes  []string    `json:"schemes"`
			Swagger  string      `json:"swagger"`
			Info     swaggerInfo `json:"info"`
			Host     string      `json:"host"`
			BasePath string      `json:"basePath"`
		}

		// JSON decode the response body.
		var doc swaggerDoc
		err = json.Unmarshal(dat, &doc)
		if err != nil {
			return errors.WithStack(err)
		}

		data["urlApiBaseUri"] = h.ProjectRoutes.WebApiUrl(doc.BasePath)
		data["urlApiDocs"] = h.ProjectRoutes.ApiDocs()

	case "/pricing":
		tmpName = "site-pricing.gohtml"
	case "/support":
		tmpName = "site-support.gohtml"
	case "/legal/privacy":
		tmpName = "legal-privacy.gohtml"
	case "/legal/terms":
		tmpName = "legal-terms.gohtml"
	default:
		return web.Redirect(ctx, w, r, "/", http.StatusFound)
	}

	return h.Renderer.Render(ctx, w, r, tmplLayoutSite, tmpName, web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// IndexHtml redirects /index.html to the website root page.
func (h *Root) IndexHtml(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	return web.Redirect(ctx, w, r, "/", http.StatusMovedPermanently)
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

type SiteMap struct {
	Pages []SiteMapPage `json:"pages"`
}

type SiteMapPage struct {
	Loc        string  `json:"loc" xml:"loc"`
	File       string  `json:"file" xml:"file"`
	Changefreq string  `json:"changefreq" xml:"changefreq"`
	Mobile     bool    `json:"mobile" xml:"mobile"`
	Priority   float64 `json:"priority" xml:"priority"`
	Lastmod    string  `json:"lastmod" xml:"lastmod"`
}

// SitemapXml returns a robots.txt response.
func (h *Root) SitemapXml(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	w.Write(h.Sitemap.XMLContent())
	return nil
}
