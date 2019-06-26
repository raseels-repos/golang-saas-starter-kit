package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/project"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// Project represents the Project API method handler set.
type Project struct {
	MasterDB *sqlx.DB

	// ADD OTHER STATE LIKE THE LOGGER IF NEEDED.
}

// Find godoc
// @Summary List projects
// @Description Find returns the existing projects in the system.
// @Tags project
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param where				query string 	false	"Filter string, example: name = 'Moon Launch'"
// @Param order				query string   	false 	"Order columns separated by comma, example: created_at desc"
// @Param limit				query integer  	false 	"Limit, example: 10"
// @Param offset			query integer  	false 	"Offset, example: 20"
// @Param included-archived query boolean 	false 	"Included Archived, example: false"
// @Success 200 {array} project.ProjectResponse
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /project [get]
func (p *Project) Find(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req project.ProjectFindRequest

	// Handle where query value if set.
	if v := r.URL.Query().Get("where"); v != "" {
		where, args, err := web.ExtractWhereArgs(v)
		if err != nil {
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		req.Where = &where
		req.Args = args
	}

	// Handle order query value if set.
	if v := r.URL.Query().Get("order"); v != "" {
		for _, o := range strings.Split(v, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				req.Order = append(req.Order, o)
			}
		}
	}

	// Handle limit query value if set.
	if v := r.URL.Query().Get("limit"); v != "" {
		l, err := strconv.Atoi(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as int for limit param", v)
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		ul := uint(l)
		req.Limit = &ul
	}

	// Handle offset query value if set.
	if v := r.URL.Query().Get("offset"); v != "" {
		l, err := strconv.Atoi(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as int for offset param", v)
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		ul := uint(l)
		req.Limit = &ul
	}

	// Handle order query value if set.
	if v := r.URL.Query().Get("included-archived"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as boolean for included-archived param", v)
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		req.IncludedArchived = b
	}

	res, err := project.Find(ctx, claims, p.MasterDB, req)
	if err != nil {
		return err
	}

	var resp []*project.ProjectResponse
	for _, m := range res {
		resp = append(resp, m.Response(ctx))
	}

	return web.RespondJson(ctx, w, resp, http.StatusOK)
}

// Read godoc
// @Summary Get project by ID.
// @Description Read returns the specified project from the system.
// @Tags project
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param id path string true "Project ID"
// @Success 200 {object} project.ProjectResponse
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /projects/{id} [get]
func (p *Project) Read(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var includeArchived bool
	if qv := r.URL.Query().Get("include-archived"); qv != "" {
		var err error
		includeArchived, err = strconv.ParseBool(qv)
		if err != nil {
			return errors.Wrapf(err, "Invalid value for include-archived : %s", qv)
		}
	}

	res, err := project.Read(ctx, claims, p.MasterDB, params["id"], includeArchived)
	if err != nil {
		switch err {
		case project.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case project.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case project.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			return errors.Wrapf(err, "ID: %s", params["id"])
		}
	}

	return web.RespondJson(ctx, w, res.Response(ctx), http.StatusOK)
}

// Create godoc
// @Summary Create new project.
// @Description Create inserts a new project into the system.
// @Tags project
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body project.ProjectCreateRequest true "Project details"
// @Success 200 {object} project.ProjectResponse
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /projects [post]
func (p *Project) Create(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req project.ProjectCreateRequest
	if err := web.Decode(r, &req); err != nil {
		err = errors.WithStack(err)

		_, ok := err.(validator.ValidationErrors)
		if ok {
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		return err
	}

	res, err := project.Create(ctx, claims, p.MasterDB, req, v.Now)
	if err != nil {
		switch err {
		case project.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			_, ok := err.(validator.ValidationErrors)
			if ok {
				return web.NewRequestError(err, http.StatusBadRequest)
			}
			return errors.Wrapf(err, "Project: %+v", &req)
		}
	}

	return web.RespondJson(ctx, w, res.Response(ctx), http.StatusCreated)
}

// Read godoc
// @Summary Update project by ID
// @Description Update updates the specified project in the system.
// @Tags project
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body project.ProjectUpdateRequest true "Update fields"
// @Success 201
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /projects [patch]
func (p *Project) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req project.ProjectUpdateRequest
	if err := web.Decode(r, &req); err != nil {
		err = errors.WithStack(err)

		_, ok := err.(validator.ValidationErrors)
		if ok {
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		return err
	}

	err := project.Update(ctx, claims, p.MasterDB, req, v.Now)
	if err != nil {
		switch err {
		case project.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case project.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case project.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			_, ok := err.(validator.ValidationErrors)
			if ok {
				return web.NewRequestError(err, http.StatusBadRequest)
			}
			return errors.Wrapf(err, "ID: %s Update: %+v", req.ID, req)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Read godoc
// @Summary Archive project by ID
// @Description Archive soft-deletes the specified project from the system.
// @Tags project
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body project.ProjectArchiveRequest true "Update fields"
// @Success 201
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /projects/archive [patch]
func (p *Project) Archive(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req project.ProjectArchiveRequest
	if err := web.Decode(r, &req); err != nil {
		err = errors.WithStack(err)

		_, ok := err.(validator.ValidationErrors)
		if ok {
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		return err
	}

	err := project.Archive(ctx, claims, p.MasterDB, req, v.Now)
	if err != nil {
		switch err {
		case project.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case project.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case project.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			_, ok := err.(validator.ValidationErrors)
			if ok {
				return web.NewRequestError(err, http.StatusBadRequest)
			}

			return errors.Wrapf(err, "Id: %s", req.ID)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Delete godoc
// @Summary Delete project by ID
// @Description Delete removes the specified project from the system.
// @Tags project
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param id path string true "Project ID"
// @Success 201
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /projects/{id} [delete]
func (p *Project) Delete(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	err := project.Delete(ctx, claims, p.MasterDB, params["id"])
	if err != nil {
		switch err {
		case project.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case project.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case project.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			return errors.Wrapf(err, "Id: %s", params["id"])
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}
