module geeks-accelerator/oss/saas-starter-kit

require (
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751
	github.com/aws/aws-sdk-go v1.23.0
	github.com/bobesa/go-domain-util v0.0.0-20180815122459-1d708c097a6a
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/dimfeld/httptreemux v5.0.1+incompatible
	github.com/dustin/go-humanize v1.0.0
	github.com/fatih/camelcase v1.0.0
	github.com/fatih/structtag v1.0.0
	github.com/geeks-accelerator/files v0.0.0-20190704085106-630677cd5c14
	github.com/geeks-accelerator/sqlxmigrate v0.0.0-20190527223850-4a863a2d30db
	github.com/geeks-accelerator/swag v1.6.3
	github.com/go-openapi/spec v0.19.2 // indirect
	github.com/go-openapi/swag v0.19.4 // indirect
	github.com/go-playground/locales v0.12.1
	github.com/go-playground/pkg v0.0.0-20190522230805-792a755e6910
	github.com/go-playground/universal-translator v0.16.0
	github.com/go-redis/redis v6.15.2+incompatible
	github.com/golang/protobuf v1.3.2 // indirect
	github.com/google/go-cmp v0.3.1
	github.com/google/uuid v1.1.1 // indirect
	github.com/gorilla/schema v1.1.0
	github.com/gorilla/securecookie v1.1.1
	github.com/gorilla/sessions v1.2.0
	github.com/huandu/go-sqlbuilder v1.4.1
	github.com/iancoleman/strcase v0.0.0-20190422225806-e506e3ef7365
	github.com/ikeikeikeike/go-sitemap-generator/v2 v2.0.2
	github.com/jmoiron/sqlx v1.2.0
	github.com/json-iterator/go v1.1.7 // indirect
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/kr/pty v1.1.8 // indirect
	github.com/lib/pq v1.2.0
	github.com/mailru/easyjson v0.0.0-20190626092158-b2ccc519800e // indirect
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646
	github.com/onsi/ginkgo v1.8.0 // indirect
	github.com/onsi/gomega v1.5.0
	github.com/pborman/uuid v1.2.0
	github.com/pkg/errors v0.8.1
	github.com/sergi/go-diff v1.0.0
	github.com/sethgrid/pester v0.0.0-20190127155807-68a33a018ad0
	github.com/shopspring/decimal v0.0.0-20180709203117-cd690d0c9e24
	github.com/stretchr/testify v1.3.0
	github.com/sudo-suhas/symcrypto v1.0.0
	github.com/ugorji/go v1.1.7 // indirect
	github.com/urfave/cli v1.21.0
	github.com/xwb1989/sqlparser v0.0.0-20180606152119-120387863bf2
	gitlab.com/geeks-accelerator/oss/devops v0.0.0-20190815180027-17c30c1f4c9e // indirect
	golang.org/x/crypto v0.0.0-20190701094942-4def268fd1a4
	golang.org/x/net v0.0.0-20190813141303-74dc4d7220e7
	golang.org/x/text v0.3.2
	golang.org/x/tools v0.0.0-20190807223507-b346f7fd45de // indirect
	gopkg.in/DataDog/dd-trace-go.v1 v1.16.1
	gopkg.in/go-playground/validator.v9 v9.29.1
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
	gopkg.in/mgo.v2 v2.0.0-20180705113604-9856a29383ce
)

replace gitlab.com/geeks-accelerator/oss/devops => ../devops
