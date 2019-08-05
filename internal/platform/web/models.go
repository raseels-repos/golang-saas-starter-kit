package web

import (
	"context"
	"crypto/md5"
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"github.com/dustin/go-humanize"
	"strings"
	"time"
)

const DatetimeFormatLocal = "Mon Jan _2 3:04PM"
const DateFormatLocal = "Mon Jan _2"
const TimeFormatLocal = time.Kitchen

// TimeResponse is a response friendly format for displaying the value of a time.
type TimeResponse struct {
	Value      time.Time `json:"value" example:"2019-06-25T03:00:53.284-08:00"`
	ValueUTC   time.Time `json:"value_utc" example:"2019-06-25T11:00:53.284Z"`
	Date       string    `json:"date" example:"2019-06-25"`
	Time       string    `json:"time" example:"03:00:53"`
	Kitchen    string    `json:"kitchen" example:"3:00AM"`
	RFC1123    string    `json:"rfc1123" example:"Tue, 25 Jun 2019 03:00:53 AKDT"`
	Local      string    `json:"local" example:"Tue Jun 25 3:00AM"`
	LocalDate  string    `json:"local_date" example:"Tue Jun 25"`
	LocalTime  string    `json:"local_time" example:"3:00AM"`
	NowTime    string    `json:"now_time" example:"5 hours ago"`
	NowRelTime string    `json:"now_rel_time" example:"15 hours from now"`
	Timezone   string    `json:"timezone" example:"America/Anchorage"`
}

// NewTimeResponse parses the time to the timezone location set in context and
// returns the display friendly format as TimeResponse.
func NewTimeResponse(ctx context.Context, t time.Time) TimeResponse {

	// If the context has claims, check to see if timezone is set for the current user and
	// then format the input time in that timezone if set.
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if ok && claims.TimeLocation() != nil {
		t = t.In(claims.TimeLocation())
	}

	var formatDatetime = DatetimeFormatLocal
	if claims.Preferences.DatetimeFormat != "" {
		formatDatetime = claims.Preferences.DatetimeFormat
	}

	var formatDate = DatetimeFormatLocal
	if claims.Preferences.DateFormat != "" {
		formatDate = claims.Preferences.DateFormat
	}

	var formatTime = DatetimeFormatLocal
	if claims.Preferences.DatetimeFormat != "" {
		formatTime = claims.Preferences.TimeFormat
	}

	tr := TimeResponse{
		Value:      t,
		ValueUTC:   t.UTC(),
		Date:       t.Format("2006-01-02"),
		Time:       t.Format("15:04:05"),
		Kitchen:    t.Format(time.Kitchen),
		RFC1123:    t.Format(time.RFC1123),
		Local:      t.Format(formatDatetime),
		LocalDate:  t.Format(formatDate),
		LocalTime:  t.Format(formatTime),
		NowTime:    humanize.Time(t.UTC()),
		NowRelTime: humanize.RelTime(time.Now().UTC(), t.UTC(), "ago", "from now"),
	}

	if t.Location() != nil {
		tr.Timezone = t.Location().String()
	}

	return tr
}

// EnumOption represents a single value of an enum option.
type EnumOption struct {
	Value    string `json:"value" example:"active_etc"`
	Title    string `json:"title"  example:"Active Etc"`
	Selected bool   `json:"selected" example:"true"`
}

// EnumResponse is a response friendly format for displaying an enum value that
// includes a list of all possible values.
type EnumResponse struct {
	Value   string       `json:"value" example:"active_etc"`
	Title   string       `json:"title" example:"Active Etc"`
	Options []EnumOption `json:"options,omitempty"`
}

// NewEnumResponse returns a display friendly format for a enum field.
func NewEnumResponse(ctx context.Context, value interface{}, options ...interface{}) EnumResponse {
	er := EnumResponse{
		Value: fmt.Sprintf("%s", value),
		Title: EnumValueTitle(fmt.Sprintf("%s", value)),
	}

	for _, opt := range options {
		optStr := fmt.Sprintf("%s", opt)
		opt := EnumOption{
			Value: optStr,
			Title: EnumValueTitle(optStr),
		}

		if optStr == er.Value {
			opt.Selected = true
		}

		er.Options = append(er.Options, opt)
	}

	return er
}

// EnumValueTitle formats a string value for display.
func EnumValueTitle(v string) string {
	v = strings.Replace(v, "_", " ", -1)
	return strings.Title(v)
}

type GravatarResponse struct {
	Small  string `json:"small" example:"https://www.gravatar.com/avatar/xy7.jpg?s=30"`
	Medium string `json:"medium" example:"https://www.gravatar.com/avatar/xy7.jpg?s=80"`
}

func NewGravatarResponse(ctx context.Context, email string) GravatarResponse {
	u := fmt.Sprintf("https://www.gravatar.com/avatar/%x.jpg?s=", md5.Sum([]byte(strings.ToLower(email))))

	return GravatarResponse{
		Small:  u + "30",
		Medium: u + "80",
	}
}
