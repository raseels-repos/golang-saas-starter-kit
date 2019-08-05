package datatable

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
)

const (
	DatatableStateCacheTtl = 120
)

var (
	// ErrInvalidColumn occurs when a column can not be mapped correctly.
	ErrInvalidColumn = errors.New("Invalid column")
)

type (
	Datatable struct {
		ctx                    context.Context
		w http.ResponseWriter
		r *http.Request
		redis    *redis.Client
		fields                 []DisplayField
		loadFunc               func(ctx context.Context, sorting string, fields []DisplayField) (resp [][]ColumnValue, err error)
		stateId                string
		req                    Request
		resp                   *Response
		handleRequest          bool
		sorting                []string
		cacheKey               string
		all                    [][]ColumnValue
		loaded                 bool
		storeFilteredFieldName string
		filteredFieldValues    []string
		disableCache           bool
		caseSensitive          bool
	}
	Request struct {
		Data    string
		Columns map[int]Column
		Order   map[int]Order
		Length  int
		Start   int
		Draw    int
		Search  Search
	}
	Column struct {
		Name       string
		Data       string
		Orderable  bool
		Searchable bool
		Search     Search
	}
	ColumnValue struct {
		Value     string
		Formatted string
	}
	Search struct {
		Value   string
		IsRegex bool
		Regexp  *regexp.Regexp
	}
	Order struct {
		Column int
		Dir    string
	}
	Response struct {
		AjaxUrl         string         `json:"ajaxUrl"`
		Draw            int            `json:"draw"`
		RecordsTotal    int            `json:"recordsTotal"`
		RecordsFiltered int            `json:"recordsFiltered"`
		Data            [][]string     `json:"data"`
		Error           string         `json:"error"`
		DisplayFields   []DisplayField `json:"displayFields"`
	}
	DisplayField struct {
		Field             string             `json:"field"`
		Title             string             `json:"title"`
		Visible           bool               `json:"visible"`
		Searchable        bool               `json:"searchable"`
		Orderable         bool               `json:"orderable"`
		Filterable        bool               `json:"filterable"`
		AutocompletePath  string             `json:"autoComplete_path"`
		FilterItems       []FilterOptionItem `json:"filter_items"`
		FilterPlaceholder string             `json:"filter_placeholder"`
		OrderFields       []string           `json:"order_fields"`
		//Type string `json:"type"`
	}
	FilterOptionItem struct {
		Value   string `json:"value"`
		Display string `json:"display"`
	}
)

func (r Request) CacheKey() string {
	c := Request{
		Order: r.Order,
		// Search : r.Search,
	}
	for _, cn := range r.Columns {
		// these will be applied as a filter
		cn.Search = Search{}
	}
	dat, _ := json.Marshal(c)
	return fmt.Sprintf("%x", md5.Sum(dat))
}

// &columns[0][data]=&columns[0][name]=&columns[0][searchable]=true&columns[0][orderable]=false&columns[0][search][value]=&columns[0][search][regex]=false&columns[1][data]=ts&columns[1][name]=Time&columns[1][searchable]=true&columns[1][orderable]=true&columns[1][search][value]=&columns[1][search][regex]=false&columns[2][data]=level&columns[2][name]=Time&columns[2][searchable]=true&columns[2][orderable]=true&columns[2][search][value]=&columns[2][search][regex]=false&columns[3][data]=msg&columns[3][name]=Time&columns[3][searchable]=true&columns[3][orderable]=true&columns[3][search][value]=&columns[3][search][regex]=false&order[0][column]=0&order[0][dir]=asc&start=0&length=10&search[value]=&search[regex]=false&_=1537426209765
func ParseQueryValues(vals url.Values) (Request, error) {

	req := Request{
		Columns:make(map[int]Column),
		Order: make(map[int]Order)	,
	}

	var err error
	for kn, kvs := range vals {

		pts := strings.Split(kn, "[")
		switch pts[0] {
		case "columns":
			idxStr := strings.Split(pts[1], "]")[0]

			idx, err := strconv.Atoi(idxStr)
			if err != nil {
				return req, err
			}

			if _, ok := req.Columns[idx]; !ok {
				req.Columns[idx] = Column{}
			}
			curCol := req.Columns[idx]

			sn := strings.Split(pts[2], "]")[0]
			switch sn {
			case "name":
				curCol.Name = kvs[0]
			case "data":
				curCol.Data = kvs[0]
			case "orderable":
				if kvs[0] != "" {
					curCol.Orderable, err = strconv.ParseBool(kvs[0])
					if err != nil {
						return req, err
					}
				}
			case "searchable":
				if kvs[0] != "" {
					curCol.Searchable, err = strconv.ParseBool(kvs[0])
					if err != nil {
						return req, err
					}
				}
			case "search":
				svn := strings.Split(pts[3], "]")[0]
				switch svn {
				case "regex":
					if kvs[0] != "" {
						curCol.Search.IsRegex, err = strconv.ParseBool(kvs[0])
						if err != nil {
							return req, err
						}
					}
				case "value":
					if strings.ToLower(kvs[0]) != "false" {
						curCol.Search.Value = kvs[0]
					}
				default:
					return req, errors.WithMessagef(ErrInvalidColumn, "Unable to map query Column Search %s for %s", svn, kn)
				}
			default:
				return req, errors.WithMessagef(ErrInvalidColumn,"Unable to map query Column %s for %s", sn, kn)
			}
			req.Columns[idx] = curCol
		case "order":
			idxStr := strings.Split(pts[1], "]")[0]

			idx, err := strconv.Atoi(idxStr)
			if err != nil {
				return req, err
			}

			if _, ok := req.Order[idx]; !ok {
				req.Order[idx] = Order{}
			}
			curOrder := req.Order[idx]

			sn := strings.Split(pts[2], "]")[0]
			switch sn {
			case "dir":
				curOrder.Dir = kvs[0]
			case "column":
				if kvs[0] != "" {
					curOrder.Column, err = strconv.Atoi(kvs[0])
					if err != nil {
						return req, err
					}
				}
			default:
				return req, errors.WithMessagef(ErrInvalidColumn, "Unable to map query Order %s for %s", sn, kn)
			}
			req.Order[idx] = curOrder
		case "length":
			if kvs[0] != "" {
				req.Length, err = strconv.Atoi(kvs[0])
				if err != nil {
					return req, err
				}
			}
		case "draw":
			if kvs[0] != "" {
				req.Draw, err = strconv.Atoi(kvs[0])
				if err != nil {
					return req, err
				}
			}
		case "start":
			if kvs[0] != "" {
				req.Start, err = strconv.Atoi(kvs[0])
				if err != nil {
					return req, err
				}
			}


		case "search":
			sn := strings.Split(pts[1], "]")[0]
			switch sn {
			case "value":
				if strings.ToLower(kvs[0]) != "false" {
					req.Search.Value = kvs[0]
				}
			case "regex":
				if kvs[0] != "" {
					req.Search.IsRegex, err = strconv.ParseBool(kvs[0])
					if err != nil {
						return req, err
					}
				}
			default:
				return req, errors.WithMessagef(ErrInvalidColumn, "Unable to map query Order %s for %s", sn, kn)
			}
		}
	}

	if req.Search.IsRegex && req.Search.Value != "" {
		req.Search.Regexp, err = regexp.Compile(req.Search.Value)
		if err != nil {
			return req, err
		}
	}

	for idx, col := range req.Columns {
		if col.Search.IsRegex && col.Search.Value != "" {
			col.Search.Regexp, err = regexp.Compile(col.Search.Value)
			if err != nil {
				return req, err
			}
			req.Columns[idx] = col
		}
	}

	return req, nil
}

func New(ctx context.Context, w http.ResponseWriter, r *http.Request, redisClient    *redis.Client, fields []DisplayField, loadFunc func(ctx context.Context, sorting string, fields []DisplayField) (resp [][]ColumnValue, err error)) (dt *Datatable, err error) {
	dt = &Datatable{
		ctx:      ctx,
		w:        w,
		r: r,
		redis:    redisClient,
		fields:   fields,
		loadFunc: loadFunc,
	}

	dt.stateId = r.URL.Query().Get("dtid")
	if dt.stateId == "" {
		dt.stateId = uuid.NewRandom().String()
	}

	dt.resp = &Response{
		Data: [][]string{},
	}
	dt.SetAjaxUrl(r.URL)

	if web.RequestIsJson(r)  {
		dt.handleRequest = true

		dt.req, err = ParseQueryValues(r.URL.Query())
		if err != nil {
			return dt, errors.Wrapf(err, "Failed to parse query values")
		}
		dt.resp.Draw = dt.req.Draw

		dt.sorting = []string{}
		for i := 0; i < len(dt.req.Order); i++ {
			co := dt.req.Order[i]

			cn := dt.req.Columns[co.Column]

			var df DisplayField
			for _, dc := range dt.fields {
				if dc.Field == cn.Name {
					df = dc
					break
				}
			}
			if df.Field == "" {
				err = errors.Errorf("Failed to find field for column %s", cn.Name)
				return dt, err
			}

			if len(df.OrderFields) > 0 {
				for _, of := range df.OrderFields {
					dt.sorting = append(dt.sorting, fmt.Sprintf("%s %s", of, co.Dir))
				}
			} else {
				dt.sorting = append(dt.sorting, fmt.Sprintf("%s %s", df.Field, co.Dir))
			}
		}

		for i := 0; i < len(dt.req.Columns); i++ {
			cn := dt.req.Columns[i]

			var cf string
			for _, dc := range dt.fields {
				if dc.Field == cn.Name {
					if dc.Filterable {
						cn.Searchable = true
						dt.req.Columns[i] = cn
					}

					cf = dc.Field
					dt.resp.DisplayFields = append(dt.resp.DisplayFields, dc)
					break
				}
			}
			if cf == "" {
				err = errors.Errorf("Failed to find field for column %s", cn.Name)
				return dt, err
			}
		}

		dt.cacheKey = fmt.Sprintf("%x", md5.Sum([]byte(dt.resp.AjaxUrl + dt.req.CacheKey() + dt.stateId)))

	} else {
		//for idx, f := range fields {
		//	if f.Filterable && !f.Searchable {
		//		f.Searchable = true
		//		fields[idx] = f
		//	}
		//}

		dt.resp.DisplayFields = fields
	}

	return dt, nil
}

func (dt *Datatable) CaseSensitive() {
	dt.caseSensitive = true
}

func (dt *Datatable) SetAjaxUrl(u *url.URL) {
	un, _ := url.Parse(u.String())

	qStr := un.Query()
	qStr.Set("dtid", dt.stateId)
	// add query to url
	un.RawQuery = qStr.Encode()

	if u.IsAbs() {
		dt.resp.AjaxUrl = un.String()
	} else {
		dt.resp.AjaxUrl = un.RequestURI()
	}
}

func (dt *Datatable) HasCache() bool {
	if !dt.handleRequest || dt.disableCache {
		return false
	}

	// @TODO: Need to handle is error better. But when cache is down, the page should still respond,
	// 			maybe just need to add logging.
	cv, _ := dt.redis.WithContext(dt.ctx).Get(dt.cacheKey).Bytes()

	if len(cv) > 0 {
		err := json.Unmarshal(cv, &dt.all)
		if err != nil {
			// @TODO: Log the error here.
		} else {
			dt.loaded = true
			return true
		}
	}

	return false
}

func (dt *Datatable) Handled() bool {
	return dt.handleRequest
}

func (dt *Datatable) Response() Response {
	return *dt.resp
}

func (dt *Datatable) StoreFilteredField(cn string) {
	dt.storeFilteredFieldName = cn
}

func (dt *Datatable) GetFilteredFieldValues() []string {
	return dt.filteredFieldValues
}

func (dt *Datatable) DisableCache() {
	dt.disableCache = true
}

func (dt *Datatable) Render() (rendered bool, err error) {
	rendered = dt.handleRequest
	if !rendered {
		return rendered, nil
	}

	if !dt.loaded {
		sorting := strings.Join(dt.sorting, ",")

		dt.all, err = dt.loadFunc(dt.ctx, sorting, dt.fields)
		if err != nil {
			return rendered, errors.Wrap(err, "Failed to load data")
		}

		if !dt.disableCache {
			dat, err := json.Marshal(dt.all)
			if err != nil {
				return rendered, errors.Wrap(err, "Failed to json encode cache response")
			}

			err = dt.redis.WithContext(dt.ctx).Set(dt.cacheKey, dat, DatatableStateCacheTtl*time.Second).Err()
			if err != nil {
				// @TODO: Log the error here.
			}
		}
	}

	dt.resp.RecordsTotal = len(dt.all)

	//fmt.Println("dt.req.Search.Value ", dt.req.Search.Value )
	var hasColFilter bool
	for i := 0; i < len(dt.req.Columns); i++ {
		cn := dt.req.Columns[i]
		if !cn.Searchable {
			continue
		}

		if cn.Search.Value != "" {
			// fmt.Println("col filter on", cn.Name)
			hasColFilter = true
			break
		}
	}

	filtered := [][]ColumnValue{}
	for _, l := range dt.all {
		var skip bool
		var oneColAtleastMatches bool
		for i := 0; i < len(dt.req.Columns); i++ {
			cn := dt.req.Columns[i]

			if cn.Name == dt.storeFilteredFieldName {
				dt.filteredFieldValues = append(dt.filteredFieldValues, l[i].Value)
			}

			if !cn.Searchable {
				// fmt.Println("col ", cn.Name, "is not searchable skipping")
				continue
			}

			if cn.Search.Value != "" {
				if cn.Search.Regexp != nil {
					//fmt.Println("col regex", cn.Search.Value, "->>>>", l[i].Value)

					if !cn.Search.Regexp.MatchString(l[i].Value) {
						//fmt.Println("-> no match")
						skip = true
						if dt.req.Search.Value == "" {
							// only skip if not full search
							break
						}
					}
				} else {
					var match bool
					if !dt.caseSensitive {
						match = strings.Contains(
							strings.ToLower(l[i].Value),
							strings.ToLower(cn.Search.Value))
					} else {
						match = strings.Contains(l[i].Value, cn.Search.Value)
					}


					if !match {
						//fmt.Println("-> no match")
						skip = true
						if dt.req.Search.Value == "" {
							// only skip if not full search
							break
						}
					}
				}
			}
			if dt.req.Search.Value != "" {
				if dt.req.Search.Regexp != nil {
					//fmt.Println("req regex", cn.Search.Value, "->>>>", l[i].Value)

					if dt.req.Search.Regexp.MatchString(l[i].Value) {
						// fmt.Println("-> match")
						oneColAtleastMatches = true
						if !hasColFilter {
							// only skip if no column filter
							break
						}
					}
				} else {
					if strings.Contains(l[i].Value, dt.req.Search.Value) {
						// fmt.Println("-> match")
						oneColAtleastMatches = true
						if !hasColFilter {
							// only skip if no column filter
							break
						}
					}
				}
			}
		}

		if hasColFilter && dt.req.Search.Value != "" {
			if !skip && oneColAtleastMatches {
				filtered = append(filtered, l)
			}
		} else if hasColFilter {
			if !skip {
				filtered = append(filtered, l)
			}

		} else if dt.req.Search.Value != "" {
			if oneColAtleastMatches {
				filtered = append(filtered, l)
			}
		} else {
			filtered = append(filtered, l)
		}
	}
	dt.resp.RecordsFiltered = len(filtered)

	for idx, l := range filtered {
		if dt.req.Start > 0 && idx < dt.req.Start {
			continue
		}

		fl := []string{}
		for _, lv := range l {
			if lv.Formatted != "" {
				fl = append(fl, lv.Formatted)
			} else {
				fl = append(fl, lv.Value)
			}
		}

		dt.resp.Data = append(dt.resp.Data, fl)
		if dt.req.Length > 0 && len(dt.resp.Data) >= dt.req.Length {
			break
		}
	}

	return rendered, web.RespondJson(dt.ctx, dt.w, dt.resp, http.StatusOK)
}
