package handlers

import (
	"context"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/project"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// Project represents the Project API method handler set.
type Project struct {
	MasterDB *sqlx.DB

	// ADD OTHER STATE LIKE THE LOGGER IF NEEDED.
}

// List returns all the existing projects in the system.
func (p *Project) List(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	projects, err := project.List(ctx, p.MasterDB)
	if err != nil {
		return err
	}

	return web.RespondJson(ctx, w, projects, http.StatusOK)
}

// Retrieve returns the specified project from the system.
func (p *Project) Retrieve(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	prod, err := project.Retrieve(ctx, p.MasterDB, params["id"])
	if err != nil {
		switch err {
		case project.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case project.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		default:
			return errors.Wrapf(err, "ID: %s", params["id"])
		}
	}

	return web.RespondJson(ctx, w, prod, http.StatusOK)
}

// Create inserts a new project into the system.
func (p *Project) Create(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	var np project.NewProject
	if err := web.Decode(r, &np); err != nil {
		return errors.Wrap(err, "")
	}

	nUsr, err := project.Create(ctx, p.MasterDB, &np, v.Now)
	if err != nil {
		return errors.Wrapf(err, "Project: %+v", &np)
	}

	return web.RespondJson(ctx, w, nUsr, http.StatusCreated)
}

// Update updates the specified project in the system.
func (p *Project) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	var up project.UpdateProject
	if err := web.Decode(r, &up); err != nil {
		return errors.Wrap(err, "")
	}

	err := project.Update(ctx, p.MasterDB, params["id"], up, v.Now)
	if err != nil {
		switch err {
		case project.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case project.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		default:
			return errors.Wrapf(err, "ID: %s Update: %+v", params["id"], up)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Delete removes the specified project from the system.
func (p *Project) Delete(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	err := project.Delete(ctx, p.MasterDB, params["id"])
	if err != nil {
		switch err {
		case project.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case project.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		default:
			return errors.Wrapf(err, "Id: %s", params["id"])
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}
