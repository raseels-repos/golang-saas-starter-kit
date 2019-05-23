package project

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const projectsCollection = "projects"

var (
	// ErrNotFound abstracts the mgo not found error.
	ErrNotFound = errors.New("Entity not found")

	// ErrInvalidID occurs when an ID is not in a valid form.
	ErrInvalidID = errors.New("ID is not in its proper form")
)

// List retrieves a list of existing projects from the database.
func List(ctx context.Context, dbConn *sqlx.DB) ([]Project, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.List")
	defer span.Finish()

	p := []Project{}

	f := func(collection *mgo.Collection) error {
		return collection.Find(nil).All(&p)
	}

	if _, err := dbConn.ExecContext(ctx, projectsCollection, f); err != nil {
		return nil, errors.Wrap(err, "db.projects.find()")
	}

	return p, nil
}

// Retrieve gets the specified project from the database.
func Retrieve(ctx context.Context, dbConn *sqlx.DB, id string) (*Project, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.Retrieve")
	defer span.Finish()

	if !bson.IsObjectIdHex(id) {
		return nil, ErrInvalidID
	}

	q := bson.M{"_id": bson.ObjectIdHex(id)}

	var p *Project
	f := func(collection *mgo.Collection) error {
		return collection.Find(q).One(&p)
	}
	if _, err := dbConn.ExecContext(ctx, projectsCollection, f); err != nil {
		if err == mgo.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, fmt.Sprintf("db.projects.find(%s)", q))
	}

	return p, nil
}

// Create inserts a new project into the database.
func Create(ctx context.Context, dbConn *sqlx.DB, cp *NewProject, now time.Time) (*Project, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.Create")
	defer span.Finish()

	// Mongo truncates times to milliseconds when storing. We and do the same
	// here so the value we return is consistent with what we store.
	now = now.Truncate(time.Millisecond)

	p := Project{
		ID:           bson.NewObjectId(),
		Name:         cp.Name,
		Cost:         cp.Cost,
		Quantity:     cp.Quantity,
		DateCreated:  now,
		DateModified: now,
	}

	f := func(collection *mgo.Collection) error {
		return collection.Insert(&p)
	}
	if _, err := dbConn.ExecContext(ctx, projectsCollection, f); err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("db.projects.insert(%v)", &p))
	}

	return &p, nil
}

// Update replaces a project document in the database.
func Update(ctx context.Context, dbConn *sqlx.DB, id string, upd UpdateProject, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.Update")
	defer span.Finish()

	if !bson.IsObjectIdHex(id) {
		return ErrInvalidID
	}

	fields := make(bson.M)

	if upd.Name != nil {
		fields["name"] = *upd.Name
	}
	if upd.Cost != nil {
		fields["cost"] = *upd.Cost
	}
	if upd.Quantity != nil {
		fields["quantity"] = *upd.Quantity
	}

	// If there's nothing to update we can quit early.
	if len(fields) == 0 {
		return nil
	}

	fields["date_modified"] = now

	m := bson.M{"$set": fields}
	q := bson.M{"_id": bson.ObjectIdHex(id)}

	f := func(collection *mgo.Collection) error {
		return collection.Update(q, m)
	}
	if _, err := dbConn.ExecContext(ctx, projectsCollection, f); err != nil {
		if err == mgo.ErrNotFound {
			return ErrNotFound
		}
		return errors.Wrap(err, fmt.Sprintf("db.customers.update(%s, %s)", q, m))
	}

	return nil
}

// Delete removes a project from the database.
func Delete(ctx context.Context, dbConn *sqlx.DB, id string) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.Delete")
	defer span.Finish()

	if !bson.IsObjectIdHex(id) {
		return ErrInvalidID
	}

	q := bson.M{"_id": bson.ObjectIdHex(id)}

	f := func(collection *mgo.Collection) error {
		return collection.Remove(q)
	}
	if _, err := dbConn.ExecContext(ctx, projectsCollection, f); err != nil {
		if err == mgo.ErrNotFound {
			return ErrNotFound
		}
		return errors.Wrap(err, fmt.Sprintf("db.projects.remove(%v)", q))
	}

	return nil
}
