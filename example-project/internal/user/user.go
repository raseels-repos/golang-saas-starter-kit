package user

import (
	"context"
	"fmt"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const usersCollection = "users"

var (
	// ErrNotFound abstracts the mgo not found error.
	ErrNotFound = errors.New("Entity not found")

	// ErrInvalidID occurs when an ID is not in a valid form.
	ErrInvalidID = errors.New("ID is not in its proper form")

	// ErrAuthenticationFailure occurs when a user attempts to authenticate but
	// anything goes wrong.
	ErrAuthenticationFailure = errors.New("Authentication failed")

	// ErrForbidden occurs when a user tries to do something that is forbidden to them according to our access control policies.
	ErrForbidden = errors.New("Attempted action is not allowed")
)

// List retrieves a list of existing users from the database.
func List(ctx context.Context, dbConn *sqlx.DB) ([]User, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.List")
	defer span.Finish()

	u := []User{}

	f := func(collection *mgo.Collection) error {
		return collection.Find(nil).All(&u)
	}
	if _, err := dbConn.ExecContext(ctx, usersCollection, f); err != nil {
		return nil, errors.Wrap(err, "db.users.find()")
	}

	return u, nil
}

// Retrieve gets the specified user from the database.
func Retrieve(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, id string) (*User, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Retrieve")
	defer span.Finish()

	if !bson.IsObjectIdHex(id) {
		return nil, ErrInvalidID
	}

	// If you are not an admin and looking to retrieve someone else then you are rejected.
	if !claims.HasRole(auth.RoleAdmin) && claims.Subject != id {
		return nil, ErrForbidden
	}

	q := bson.M{"_id": bson.ObjectIdHex(id)}

	var u *User
	f := func(collection *mgo.Collection) error {
		return collection.Find(q).One(&u)
	}
	if _, err := dbConn.ExecContext(ctx, usersCollection, f); err != nil {
		if err == mgo.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, fmt.Sprintf("db.users.find(%s)", q))
	}

	return u, nil
}

// Create inserts a new user into the database.
func Create(ctx context.Context, dbConn *sqlx.DB, nu *NewUser, now time.Time) (*User, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Create")
	defer span.Finish()

	// Mongo truncates times to milliseconds when storing. We and do the same
	// here so the value we return is consistent with what we store.
	now = now.Truncate(time.Millisecond)

	pw, err := bcrypt.GenerateFromPassword([]byte(nu.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.Wrap(err, "generating password hash")
	}

	u := User{
		ID:           bson.NewObjectId(),
		Name:         nu.Name,
		Email:        nu.Email,
		PasswordHash: pw,
		Roles:        nu.Roles,
		DateCreated:  now,
		DateModified: now,
	}

	f := func(collection *mgo.Collection) error {
		return collection.Insert(&u)
	}
	if _, err := dbConn.ExecContext(ctx, usersCollection, f); err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("db.users.insert(%s)", &u))
	}

	return &u, nil
}

// Update replaces a user document in the database.
func Update(ctx context.Context, dbConn *sqlx.DB, id string, upd *UpdateUser, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Update")
	defer span.Finish()

	if !bson.IsObjectIdHex(id) {
		return ErrInvalidID
	}

	fields := make(bson.M)

	if upd.Name != nil {
		fields["name"] = *upd.Name
	}
	if upd.Email != nil {
		fields["email"] = *upd.Email
	}
	if upd.Roles != nil {
		fields["roles"] = upd.Roles
	}
	if upd.Password != nil {
		pw, err := bcrypt.GenerateFromPassword([]byte(*upd.Password), bcrypt.DefaultCost)
		if err != nil {
			return errors.Wrap(err, "generating password hash")
		}
		fields["password_hash"] = pw
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
	if _, err := dbConn.ExecContext(ctx, usersCollection, f); err != nil {
		if err == mgo.ErrNotFound {
			return ErrNotFound
		}
		return errors.Wrap(err, fmt.Sprintf("db.customers.update(%s, %s)", q, m))
	}

	return nil
}

// Delete removes a user from the database.
func Delete(ctx context.Context, dbConn *sqlx.DB, id string) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Delete")
	defer span.Finish()

	if !bson.IsObjectIdHex(id) {
		return ErrInvalidID
	}

	q := bson.M{"_id": bson.ObjectIdHex(id)}

	f := func(collection *mgo.Collection) error {
		return collection.Remove(q)
	}
	if _, err := dbConn.ExecContext(ctx, usersCollection, f); err != nil {
		if err == mgo.ErrNotFound {
			return ErrNotFound
		}
		return errors.Wrap(err, fmt.Sprintf("db.users.remove(%s)", q))
	}

	return nil
}

// TokenGenerator is the behavior we need in our Authenticate to generate
// tokens for authenticated users.
type TokenGenerator interface {
	GenerateToken(auth.Claims) (string, error)
}

// Authenticate finds a user by their email and verifies their password. On
// success it returns a Token that can be used to authenticate in the future.
func Authenticate(ctx context.Context, dbConn *sqlx.DB, tknGen TokenGenerator, now time.Time, email, password string) (Token, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Authenticate")
	defer span.Finish()

	q := bson.M{"email": email}

	var u *User
	f := func(collection *mgo.Collection) error {
		return collection.Find(q).One(&u)
	}
	if _, err := dbConn.ExecContext(ctx, usersCollection, f); err != nil {

		// Normally we would return ErrNotFound in this scenario but we do not want
		// to leak to an unauthenticated user which emails are in the system.
		if err == mgo.ErrNotFound {
			return Token{}, ErrAuthenticationFailure
		}
		return Token{}, errors.Wrap(err, fmt.Sprintf("db.users.find(%s)", q))
	}

	// Compare the provided password with the saved hash. Use the bcrypt
	// comparison function so it is cryptographically secure.
	if err := bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(password)); err != nil {
		return Token{}, ErrAuthenticationFailure
	}

	// If we are this far the request is valid. Create some claims for the user
	// and generate their token.
	claims := auth.NewClaims(u.ID.Hex(), u.Roles, now, time.Hour)

	tkn, err := tknGen.GenerateToken(claims)
	if err != nil {
		return Token{}, errors.Wrap(err, "generating token")
	}

	return Token{Token: tkn}, nil
}
