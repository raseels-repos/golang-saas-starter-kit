package goparse

import (
	"log"
	"os"
	"strings"
	"testing"

	"github.com/onsi/gomega"
)

var logger *log.Logger

func init() {
	logger = log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)
}

func TestParseFileModel1(t *testing.T) {

	_, err := ParseFile(logger, "test_gofile_model1.txt")
	if err != nil {
		t.Fatalf("got error %v", err)
	}

}

func TestMultilineVar(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	code := `func ContextAllowedAccountIds(ctx context.Context, db *gorm.DB) (resp akdatamodels.Uint32List, err error) {
        resp = []uint32{}
        accountId := akcontext.ContextAccountId(ctx)
        m := datamodels.UserAccount{}
        q := fmt.Sprintf("select
									distinct account_id
								from %s where account_id = ?", m.TableName())
        db = db.Raw(q, accountId)
}
`
	code = strings.Replace(code, "\"", "`", -1)
	lines := strings.Split(code, "\n")

	objs, err := ParseLines(lines, 0)
	if err != nil {
		t.Fatalf("got error %v", err)
	}

	g.Expect(objs.Lines()).Should(gomega.Equal(lines))
}

func TestNewDocImports(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	expected := []string{
		"package goparse",
		"",
		"import (",
		"	\"github.com/go/pkg1\"",
		"	\"github.com/go/pkg2\"",
		")",
		"",
	}

	doc := &GoDocument{}
	doc.SetPackage("goparse")

	doc.AddImport(GoImport{Name: "github.com/go/pkg1"})
	doc.AddImport(GoImport{Name: "github.com/go/pkg2"})

	g.Expect(doc.Lines()).Should(gomega.Equal(expected))
}

func TestParseLines1(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	code := `func testCreate(t *testing.T, ctx context.Context, sess *datamodels.Session) *datamodels.Model {
	g := gomega.NewGomegaWithT(t)
	obj := datamodels.MockModelNew()
	resp, err := ModelCreate(ctx, DB, &obj)
	if err != nil {
		t.Fatalf("got error %v", err)
	}

	g.Expect(resp.Name).Should(gomega.Equal(obj.Name))
	g.Expect(resp.Status).Should(gomega.Equal(datamodels.{{ .Name }}Status_Active))
	return resp
}
`
	lines := strings.Split(code, "\n")

	objs, err := ParseLines(lines, 0)
	if err != nil {
		t.Fatalf("got error %v", err)
	}

	g.Expect(objs.Lines()).Should(gomega.Equal(lines))
}

func TestParseLines2(t *testing.T) {
	code := `func structToMap(s interface{}) (resp map[string]interface{}) {
	dat, _ := json.Marshal(s)
	_ = json.Unmarshal(dat, &resp)
	for k, x := range resp {
		switch v := x.(type) {
		case time.Time:
			if v.IsZero() {
				delete(resp, k)
			}

		case *time.Time:
			if v == nil || v.IsZero() {
				delete(resp, k)
			}

		case nil:
			delete(resp, k)

		}

	}

	return resp
}
`
	lines := strings.Split(code, "\n")

	objs, err := ParseLines(lines, 0)
	if err != nil {
		t.Fatalf("got error %v", err)
	}

	testLineTextMatches(t, objs.Lines(), lines)
}

func TestParseLines3(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	code := `type UserAccountRoleName = string

const (
	UserAccountRoleName_None  UserAccountRoleName = ""
	UserAccountRoleName_Admin UserAccountRoleName = "admin"
	UserAccountRoleName_User  UserAccountRoleName = "user"
)

type UserAccountRole struct {
	Id        uint32     ^gorm:"column:id;type:int(10) unsigned AUTO_INCREMENT;primary_key;not null;auto_increment;" truss:"internal:true"^
	CreatedAt time.Time  ^gorm:"column:created_at;type:datetime;default:CURRENT_TIMESTAMP;not null;" truss:"internal:true"^
	UpdatedAt time.Time  ^gorm:"column:updated_at;type:datetime;" truss:"internal:true"^
	DeletedAt *time.Time ^gorm:"column:deleted_at;type:datetime;" truss:"internal:true"^
	Role UserAccountRoleName ^gorm:"unique_index:user_account_role;column:role;type:enum('admin', 'user')"^
	// belongs to User
	User   *User  ^gorm:"foreignkey:UserId;association_foreignkey:Id;association_autoupdate:false;association_autocreate:false;association_save_reference:false;preload:false;" truss:"internal:true"^
	UserId uint32 ^gorm:"unique_index:user_account_role;"^
	// belongs to Account
	Account   *Account ^gorm:"foreignkey:AccountId;association_foreignkey:Id;association_autoupdate:false;association_autocreate:false;association_save_reference:false;preload:false;" truss:"internal:true;api_ro:true;"^
	AccountId uint32   ^gorm:"unique_index:user_account_role;" truss:"internal:true;api_ro:true;"^
}

func (UserAccountRole) TableName() string {
	return "user_account_roles"
}
`
	code = strings.Replace(code, "^", "'", -1)
	lines := strings.Split(code, "\n")

	objs, err := ParseLines(lines, 0)
	if err != nil {
		t.Fatalf("got error %v", err)
	}

	g.Expect(objs.Lines()).Should(gomega.Equal(lines))
}

func testLineTextMatches(t *testing.T, l1, l2 []string) {
	g := gomega.NewGomegaWithT(t)

	m1 := []string{}
	for _, l := range l1 {
		l = strings.TrimSpace(l)
		if l != "" {
			m1 = append(m1, l)
		}
	}

	m2 := []string{}
	for _, l := range l2 {
		l = strings.TrimSpace(l)
		if l != "" {
			m2 = append(m2, l)
		}
	}

	g.Expect(m1).Should(gomega.Equal(m2))
}
