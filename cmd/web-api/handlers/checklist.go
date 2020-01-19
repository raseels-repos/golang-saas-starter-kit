package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/checklist"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"

	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// Checklist represents the Checklist API method handler set.
type Checklists struct {
	Repository *checklist.Repository

	// ADD OTHER STATE LIKE THE LOGGER IF NEEDED.
}

// Find godoc
// TODO: Need to implement unittests on checklists/find endpoint. There are none.
// @Summary List checklists
// @Description Find returns the existing checklists in the system.
// @Tags checklist
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param where				query string 	false	"Filter string, example: name = 'Moon Launch'"
// @Param order				query string   	false 	"Order columns separated by comma, example: created_at desc"
// @Param limit				query integer  	false 	"Limit, example: 10"
// @Param offset			query integer  	false 	"Offset, example: 20"
// @Param include-archived query boolean 	false 	"Included Archived, example: false"
// @Success 200 {array} checklist.ChecklistResponse
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 403 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /checklists [get]
func (h *Checklists) Find(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req checklist.ChecklistFindRequest

	// Handle where query value if set.
	if v := r.URL.Query().Get("where"); v != "" {
		where, args, err := web.ExtractWhereArgs(v)
		if err != nil {
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
		}
		req.Where = where
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
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
		}
		ul := uint(l)
		req.Limit = &ul
	}

	// Handle offset query value if set.
	if v := r.URL.Query().Get("offset"); v != "" {
		l, err := strconv.Atoi(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as int for offset param", v)
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
		}
		ul := uint(l)
		req.Limit = &ul
	}

	// Handle include-archive query value if set.
	if v := r.URL.Query().Get("include-archived"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as boolean for include-archived param", v)
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
		}
		req.IncludeArchived = b
	}

	//if err := web.Decode(r, &req); err != nil {
	//	if _, ok := errors.Cause(err).(*web.Error); !ok {
	//		err = weberror.NewError(ctx, err, http.StatusBadRequest)
	//	}
	//	return  web.RespondJsonError(ctx, w, err)
	//}

	res, err := h.Repository.Find(ctx, claims, req)
	if err != nil {
		return err
	}

	var resp []*checklist.ChecklistResponse
	for _, m := range res {
		resp = append(resp, m.Response(ctx))
	}

	return web.RespondJson(ctx, w, resp, http.StatusOK)
}

// Read godoc
// @Summary Get checklist by ID.
// @Description Read returns the specified checklist from the system.
// @Tags checklist
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param id path string true "Checklist ID"
// @Success 200 {object} checklist.ChecklistResponse
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 404 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /checklists/{id} [get]
func (h *Checklists) Read(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	// Handle include-archived query value if set.
	var includeArchived bool
	if v := r.URL.Query().Get("include-archived"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as boolean for include-archived param", v)
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
		}
		includeArchived = b
	}

	res, err := h.Repository.Read(ctx, claims, checklist.ChecklistReadRequest{
		ID:              params["id"],
		IncludeArchived: includeArchived,
	})
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case checklist.ErrNotFound:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusNotFound))
		default:
			return errors.Wrapf(err, "ID: %s", params["id"])
		}
	}

	return web.RespondJson(ctx, w, res.Response(ctx), http.StatusOK)
}

// Create godoc
// @Summary Create new checklist.
// @Description Create inserts a new checklist into the system.
// @Tags checklist
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body checklist.ChecklistCreateRequest true "Checklist details"
// @Success 201 {object} checklist.ChecklistResponse
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 403 {object} weberror.ErrorResponse
// @Failure 404 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /checklists [post]
func (h *Checklists) Create(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	var req checklist.ChecklistCreateRequest
	if err := web.Decode(ctx, r, &req); err != nil {
		if _, ok := errors.Cause(err).(*weberror.Error); !ok {
			err = weberror.NewError(ctx, err, http.StatusBadRequest)
		}
		return web.RespondJsonError(ctx, w, err)
	}

	res, err := h.Repository.Create(ctx, claims, req, v.Now)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case checklist.ErrForbidden:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
			}
			return errors.Wrapf(err, "Checklist: %+v", &req)
		}
	}

	return web.RespondJson(ctx, w, res.Response(ctx), http.StatusCreated)
}

// Read godoc
// @Summary Update checklist by ID
// @Description Update updates the specified checklist in the system.
// @Tags checklist
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body checklist.ChecklistUpdateRequest true "Update fields"
// @Success 204
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 403 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /checklists [patch]
func (h *Checklists) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	var req checklist.ChecklistUpdateRequest
	if err := web.Decode(ctx, r, &req); err != nil {
		if _, ok := errors.Cause(err).(*weberror.Error); !ok {
			err = weberror.NewError(ctx, err, http.StatusBadRequest)
		}
		return web.RespondJsonError(ctx, w, err)
	}

	err = h.Repository.Update(ctx, claims, req, v.Now)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case checklist.ErrForbidden:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
			}

			return errors.Wrapf(err, "ID: %s Update: %+v", req.ID, req)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Read godoc
// @Summary Archive checklist by ID
// @Description Archive soft-deletes the specified checklist from the system.
// @Tags checklist
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body checklist.ChecklistArchiveRequest true "Update fields"
// @Success 204
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 403 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /checklists/archive [patch]
func (h *Checklists) Archive(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	var req checklist.ChecklistArchiveRequest
	if err := web.Decode(ctx, r, &req); err != nil {
		if _, ok := errors.Cause(err).(*weberror.Error); !ok {
			err = weberror.NewError(ctx, err, http.StatusBadRequest)
		}
		return web.RespondJsonError(ctx, w, err)
	}

	err = h.Repository.Archive(ctx, claims, req, v.Now)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case checklist.ErrForbidden:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
			}

			return errors.Wrapf(err, "Id: %s", req.ID)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Delete godoc
// @Summary Delete checklist by ID
// @Description Delete removes the specified checklist from the system.
// @Tags checklist
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param id path string true "Checklist ID"
// @Success 204
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 403 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /checklists/{id} [delete]
func (h *Checklists) Delete(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	err = h.Repository.Delete(ctx, claims,
		checklist.ChecklistDeleteRequest{ID: params["id"]})
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case checklist.ErrForbidden:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
			}

			return errors.Wrapf(err, "Id: %s", params["id"])
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}
