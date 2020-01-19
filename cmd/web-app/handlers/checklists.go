package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/checklist"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/datatable"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"

	"github.com/gorilla/schema"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
)

// Checklists represents the Checklists API method handler set.
type Checklists struct {
	ChecklistRepo *checklist.Repository
	Redis         *redis.Client
	Renderer      web.Renderer
}

func urlChecklistsIndex() string {
	return fmt.Sprintf("/checklists")
}

func urlChecklistsCreate() string {
	return fmt.Sprintf("/checklists/create")
}

func urlChecklistsView(checklistID string) string {
	return fmt.Sprintf("/checklists/%s", checklistID)
}

func urlChecklistsUpdate(checklistID string) string {
	return fmt.Sprintf("/checklists/%s/update", checklistID)
}

// Index handles listing all the checklists for the current account.
func (h *Checklists) Index(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	statusOpts := web.NewEnumResponse(ctx, nil, checklist.ChecklistStatus_ValuesInterface()...)

	statusFilterItems := []datatable.FilterOptionItem{}
	for _, opt := range statusOpts.Options {
		statusFilterItems = append(statusFilterItems, datatable.FilterOptionItem{
			Display: opt.Title,
			Value:   opt.Value,
		})
	}

	fields := []datatable.DisplayField{
		{Field: "id", Title: "ID", Visible: false, Searchable: true, Orderable: true, Filterable: false},
		{Field: "name", Title: "Checklist", Visible: true, Searchable: true, Orderable: true, Filterable: true, FilterPlaceholder: "filter Name"},
		{Field: "status", Title: "Status", Visible: true, Searchable: true, Orderable: true, Filterable: true, FilterPlaceholder: "All Statuses", FilterItems: statusFilterItems},
		{Field: "updated_at", Title: "Last Updated", Visible: true, Searchable: true, Orderable: true, Filterable: false},
		{Field: "created_at", Title: "Created", Visible: true, Searchable: true, Orderable: true, Filterable: false},
	}

	mapFunc := func(q *checklist.Checklist, cols []datatable.DisplayField) (resp []datatable.ColumnValue, err error) {
		for i := 0; i < len(cols); i++ {
			col := cols[i]
			var v datatable.ColumnValue
			switch col.Field {
			case "id":
				v.Value = fmt.Sprintf("%s", q.ID)
			case "name":
				v.Value = q.Name
				v.Formatted = fmt.Sprintf("<a href='%s'>%s</a>", urlChecklistsView(q.ID), v.Value)
			case "status":
				v.Value = q.Status.String()

				var subStatusClass string
				var subStatusIcon string
				switch q.Status {
				case checklist.ChecklistStatus_Active:
					subStatusClass = "text-green"
					subStatusIcon = "far fa-dot-circle"
				case checklist.ChecklistStatus_Disabled:
					subStatusClass = "text-orange"
					subStatusIcon = "far fa-circle"
				}

				v.Formatted = fmt.Sprintf("<span class='cell-font-status %s'><i class='%s mr-1'></i>%s</span>", subStatusClass, subStatusIcon, web.EnumValueTitle(v.Value))
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
		res, err := h.ChecklistRepo.Find(ctx, claims, checklist.ChecklistFindRequest{
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
				return resp, errors.Wrapf(err, "Failed to map checklist for display.")
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
		"datatable":           dt.Response(),
		"urlChecklistsCreate": urlChecklistsCreate(),
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "checklists-index.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Create handles creating a new checklist for the account.
func (h *Checklists) Create(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	//
	req := new(checklist.ChecklistCreateRequest)
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

			usr, err := h.ChecklistRepo.Create(ctx, claims, *req, ctxValues.Now)
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

			// Display a success message to the checklist.
			webcontext.SessionFlashSuccess(ctx,
				"Checklist Created",
				"Checklist successfully created.")

			return true, web.Redirect(ctx, w, r, urlChecklistsView(usr.ID), http.StatusFound)
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

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(checklist.ChecklistCreateRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "checklists-create.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// View handles displaying a checklist.
func (h *Checklists) View(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	checklistID := params["checklist_id"]

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
				err = h.ChecklistRepo.Archive(ctx, claims, checklist.ChecklistArchiveRequest{
					ID: checklistID,
				}, ctxValues.Now)
				if err != nil {
					return false, err
				}

				webcontext.SessionFlashSuccess(ctx,
					"Checklist Archive",
					"Checklist successfully archive.")

				return true, web.Redirect(ctx, w, r, urlChecklistsIndex(), http.StatusFound)
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

	prj, err := h.ChecklistRepo.ReadByID(ctx, claims, checklistID)
	if err != nil {
		return err
	}
	data["checklist"] = prj.Response(ctx)
	data["urlChecklistsView"] = urlChecklistsView(checklistID)
	data["urlChecklistsUpdate"] = urlChecklistsUpdate(checklistID)

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "checklists-view.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Update handles updating a checklist for the account.
func (h *Checklists) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	checklistID := params["checklist_id"]

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	//
	req := new(checklist.ChecklistUpdateRequest)
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
			req.ID = checklistID

			err = h.ChecklistRepo.Update(ctx, claims, *req, ctxValues.Now)
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

			// Display a success message to the checklist.
			webcontext.SessionFlashSuccess(ctx,
				"Checklist Updated",
				"Checklist successfully updated.")

			return true, web.Redirect(ctx, w, r, urlChecklistsView(req.ID), http.StatusFound)
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	prj, err := h.ChecklistRepo.ReadByID(ctx, claims, checklistID)
	if err != nil {
		return err
	}
	data["checklist"] = prj.Response(ctx)

	data["urlChecklistsView"] = urlChecklistsView(checklistID)

	if req.ID == "" {
		req.Name = &prj.Name
		req.Status = &prj.Status
	}
	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(checklist.ChecklistUpdateRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "checklists-update.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}
