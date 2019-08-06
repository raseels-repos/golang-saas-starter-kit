package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/datatable"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"geeks-accelerator/oss/saas-starter-kit/internal/project"
	"github.com/gorilla/schema"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
)

// Projects represents the Projects API method handler set.
type Projects struct {
	MasterDB *sqlx.DB
	Redis    *redis.Client
	Renderer web.Renderer
}

func urlProjectsIndex() string {
	return fmt.Sprintf("/projects")
}

func urlProjectsCreate() string {
	return fmt.Sprintf("/projects/create")
}

func urlProjectsView(projectID string) string {
	return fmt.Sprintf("/projects/%s", projectID)
}

func urlProjectsUpdate(projectID string) string {
	return fmt.Sprintf("/projects/%s/update", projectID)
}

// Index handles listing all the projects for the current account.
func (h *Projects) Index(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	statusOpts := web.NewEnumResponse(ctx, nil, project.ProjectStatus_ValuesInterface()...)

	statusFilterItems := []datatable.FilterOptionItem{}
	for _, opt := range statusOpts.Options {
		statusFilterItems = append(statusFilterItems, datatable.FilterOptionItem{
			Display: opt.Title,
			Value:   opt.Value,
		})
	}

	fields := []datatable.DisplayField{
		datatable.DisplayField{Field: "id", Title: "ID", Visible: false, Searchable: true, Orderable: true, Filterable: false},
		datatable.DisplayField{Field: "name", Title: "Project", Visible: true, Searchable: true, Orderable: true, Filterable: true, FilterPlaceholder: "filter Name"},
		datatable.DisplayField{Field: "status", Title: "Status", Visible: true, Searchable: true, Orderable: true, Filterable: true, FilterPlaceholder: "All Statuses", FilterItems: statusFilterItems},
		datatable.DisplayField{Field: "updated_at", Title: "Last Updated", Visible: true, Searchable: true, Orderable: true, Filterable: false},
		datatable.DisplayField{Field: "created_at", Title: "Created", Visible: true, Searchable: true, Orderable: true, Filterable: false},
	}

	mapFunc := func(q *project.Project, cols []datatable.DisplayField) (resp []datatable.ColumnValue, err error) {
		for i := 0; i < len(cols); i++ {
			col := cols[i]
			var v datatable.ColumnValue
			switch col.Field {
			case "id":
				v.Value = fmt.Sprintf("%d", q.ID)
			case "name":
				v.Value = q.Name
				v.Formatted = fmt.Sprintf("<a href='%s'>%s</a>", urlProjectsView(q.ID), v.Value)
			case "status":
				v.Value = q.Status.String()

				var subStatusClass string
				var subStatusIcon string
				switch q.Status {
				case project.ProjectStatus_Active:
					subStatusClass = "text-green"
					subStatusIcon = "far fa-dot-circle"
				case project.ProjectStatus_Disabled:
					subStatusClass = "text-orange"
					subStatusIcon = "far fa-circle"
				}

				v.Formatted = fmt.Sprintf("<span class='cell-font-status %s'><i class='%s'></i>%s</span>", subStatusClass, subStatusIcon, web.EnumValueTitle(v.Value))
			case "created_at":
				dt := web.NewTimeResponse(ctx, q.CreatedAt)
				v.Value = dt.Local
				v.Formatted = fmt.Sprintf("<span class='cell-font-date'>%s</span>", v.Value)
			case "updated_at":
				dt := web.NewTimeResponse(ctx, q.UpdatedAt)
				v.Value = dt.Local
				v.Formatted = fmt.Sprintf("<span class='cell-font-date'>%s</span>", v.Value)
			default:
				return resp, errors.Errorf("Failed to map value for %s.", col.Field)
			}
			resp = append(resp, v)
		}

		return resp, nil
	}

	loadFunc := func(ctx context.Context, sorting string, fields []datatable.DisplayField) (resp [][]datatable.ColumnValue, err error) {
		res, err := project.Find(ctx, claims, h.MasterDB, project.ProjectFindRequest{
			Where: "account_id = ?",
			Args:  []interface{}{claims.Audience},
			Order: strings.Split(sorting, ","),
		})
		if err != nil {
			return resp, err
		}

		for _, a := range res {
			l, err := mapFunc(a, fields)
			if err != nil {
				return resp, errors.Wrapf(err, "Failed to map project for display.")
			}

			resp = append(resp, l)
		}

		return resp, nil
	}

	dt, err := datatable.New(ctx, w, r, h.Redis, fields, loadFunc)
	if err != nil {
		return err
	}

	if dt.HasCache() {
		return nil
	}

	if ok, err := dt.Render(); ok {
		if err != nil {
			return err
		}
		return nil
	}

	data := map[string]interface{}{
		"datatable":         dt.Response(),
		"urlProjectsCreate": urlProjectsCreate(),
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "projects-index.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Create handles creating a new project for the account.
func (h *Projects) Create(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	//
	req := new(project.ProjectCreateRequest)
	data := make(map[string]interface{})
	f := func() (bool, error) {
		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return false, err
			}

			decoder := schema.NewDecoder()
			decoder.IgnoreUnknownKeys(true)

			if err := decoder.Decode(req, r.PostForm); err != nil {
				return false, err
			}
			req.AccountID = claims.Audience

			usr, err := project.Create(ctx, claims, h.MasterDB, *req, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return false, nil
					} else {
						return false, err
					}
				}
			}

			// Display a success message to the project.
			webcontext.SessionFlashSuccess(ctx,
				"Project Created",
				"Project successfully created.")
			err = webcontext.ContextSession(ctx).Save(r, w)
			if err != nil {
				return false, err
			}

			http.Redirect(w, r, urlProjectsView(usr.ID), http.StatusFound)
			return true, nil
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(project.ProjectCreateRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "projects-create.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// View handles displaying a project.
func (h *Projects) View(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	projectID := params["project_id"]

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	data := make(map[string]interface{})
	f := func() (bool, error) {
		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return false, err
			}

			switch r.PostForm.Get("action") {
			case "archive":
				err = project.Archive(ctx, claims, h.MasterDB, project.ProjectArchiveRequest{
					ID: projectID,
				}, ctxValues.Now)
				if err != nil {
					return false, err
				}

				webcontext.SessionFlashSuccess(ctx,
					"Project Archive",
					"Project successfully archive.")
				err = webcontext.ContextSession(ctx).Save(r, w)
				if err != nil {
					return false, err
				}

				http.Redirect(w, r, urlProjectsIndex(), http.StatusFound)
				return true, nil
			}
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	prj, err := project.ReadByID(ctx, claims, h.MasterDB, projectID)
	if err != nil {
		return err
	}
	data["project"] = prj.Response(ctx)

	data["urlProjectsUpdate"] = urlProjectsUpdate(projectID)

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "projects-view.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Update handles updating a project for the account.
func (h *Projects) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	projectID := params["project_id"]

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	//
	req := new(project.ProjectUpdateRequest)
	data := make(map[string]interface{})
	f := func() (bool, error) {
		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return false, err
			}

			decoder := schema.NewDecoder()
			decoder.IgnoreUnknownKeys(true)

			if err := decoder.Decode(req, r.PostForm); err != nil {
				return false, err
			}
			req.ID = projectID

			err = project.Update(ctx, claims, h.MasterDB, *req, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return false, nil
					} else {
						return false, err
					}
				}
			}

			// Display a success message to the project.
			webcontext.SessionFlashSuccess(ctx,
				"Project Updated",
				"Project successfully updated.")
			err = webcontext.ContextSession(ctx).Save(r, w)
			if err != nil {
				return false, err
			}

			http.Redirect(w, r, urlProjectsView(req.ID), http.StatusFound)
			return true, nil
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	prj, err := project.ReadByID(ctx, claims, h.MasterDB, projectID)
	if err != nil {
		return err
	}
	data["project"] = prj.Response(ctx)

	if req.ID == "" {
		req.Name = &prj.Name
		req.Status = &prj.Status
	}
	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(project.ProjectUpdateRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "projects-update.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}
