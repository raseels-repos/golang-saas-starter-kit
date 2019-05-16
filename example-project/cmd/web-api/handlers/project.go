package handlers

import (
	"context"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/db"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/project"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

// Project represents the Project API method handler set.
type Project struct {
	MasterDB *db.DB

	// ADD OTHER STATE LIKE THE LOGGER IF NEEDED.
}

// List returns all the existing projects in the system.
func (p *Project) List(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	ctx, span := trace.StartSpan(ctx, "handlers.Project.List")
	defer span.End()

	dbConn := p.MasterDB.Copy()
	defer dbConn.Close()

	projects, err := project.List(ctx, dbConn)
	if err != nil {
		return err
	}

	return web.Respond(ctx, w, projects, http.StatusOK)
}

// Retrieve returns the specified project from the system.
func (p *Project) Retrieve(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	ctx, span := trace.StartSpan(ctx, "handlers.Project.Retrieve")
	defer span.End()

	dbConn := p.MasterDB.Copy()
	defer dbConn.Close()

	prod, err := project.Retrieve(ctx, dbConn, params["id"])
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

	return web.Respond(ctx, w, prod, http.StatusOK)
}

// Create inserts a new project into the system.
func (p *Project) Create(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	ctx, span := trace.StartSpan(ctx, "handlers.Project.Create")
	defer span.End()

	dbConn := p.MasterDB.Copy()
	defer dbConn.Close()

	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	var np project.NewProject
	if err := web.Decode(r, &np); err != nil {
		return errors.Wrap(err, "")
	}

	nUsr, err := project.Create(ctx, dbConn, &np, v.Now)
	if err != nil {
		return errors.Wrapf(err, "Project: %+v", &np)
	}

	return web.Respond(ctx, w, nUsr, http.StatusCreated)
}

// Update updates the specified project in the system.
func (p *Project) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	ctx, span := trace.StartSpan(ctx, "handlers.Project.Update")
	defer span.End()

	dbConn := p.MasterDB.Copy()
	defer dbConn.Close()

	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	var up project.UpdateProject
	if err := web.Decode(r, &up); err != nil {
		return errors.Wrap(err, "")
	}

	err := project.Update(ctx, dbConn, params["id"], up, v.Now)
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

	return web.Respond(ctx, w, nil, http.StatusNoContent)
}

// Delete removes the specified project from the system.
func (p *Project) Delete(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	ctx, span := trace.StartSpan(ctx, "handlers.Project.Delete")
	defer span.End()

	dbConn := p.MasterDB.Copy()
	defer dbConn.Close()

	err := project.Delete(ctx, dbConn, params["id"])
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

	return web.Respond(ctx, w, nil, http.StatusNoContent)
}
