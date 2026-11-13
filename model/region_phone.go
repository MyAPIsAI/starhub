package model

import (
	"regexp"
	"strings"

	"github.com/nyaruka/phonenumbers/v2"
)

var regionalPhoneCharacters = regexp.MustCompile(`^[0-9+().\s-]+$`)

// NormalizeRegionalPhone uses the deployment country for national dialing and
// validates the resulting country assignment before creating an SMS challenge.
func NormalizeRegionalPhone(value, country string) (string, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" || country == "CN" {
		return NormalizePhone(value)
	}
	if len(value) > 128 || !regionalPhoneCharacters.MatchString(value) || phonenumbers.GetCountryCodeForRegion(country) == 0 {
		return "", ErrPhoneFormat
	}
	number, err := phonenumbers.Parse(strings.TrimSpace(value), country)
	if err != nil || !phonenumbers.IsValidNumberForRegion(number, country) || number.GetExtension() != "" {
		return "", ErrPhoneFormat
	}
	return phonenumbers.Format(number, phonenumbers.E164), nil
}

func NormalizeActiveRegionPhone(value string) (string, error) {
	profile, err := ActiveRegionProfile()
	if err != nil {
		return "", err
	}
	return NormalizeRegionalPhone(value, profile.CountryCode)
}
