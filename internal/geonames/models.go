package geonames

import "github.com/shopspring/decimal"

type Geoname struct {
	CountryCode   string          // US
	PostalCode    string          // 99686
	PlaceName     string          // Valdez
	StateName     string          // Alaska
	StateCode     string          // AK
	CountyName    string          // Valdez-Cordova
	CountyCode    string          // 261
	CommunityName string          //
	CommunityCode string          //
	Latitude      decimal.Decimal // 61.101
	Longitude     decimal.Decimal // -146.9
	Accuracy      int             // 1
}

type Country struct {
	Code             string // US
	Name             string
	IsoAlpha3        string
	Capital          string
	CurrencyCode     string // .us
	CurrencyName     string // USD	Dollar
	Phone            string // 1
	PostalCodeFormat string // #####-####
	PostalCodeRegex  string // ^\d{5}(-\d{4})?$
}

type Region struct {
	Code string // AK
	Name string // Alaska
}

type CountryTimezone struct {
	CountryCode string // US
	TimezoneId  string // America/Anchorage
}
