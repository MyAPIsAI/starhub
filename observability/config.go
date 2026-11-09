package observability

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	IMURL          string
	PhoneURL       string
	GeneralURL     string
	ObserverToken  string
	Timezone       string
	WorkStart      int
	WorkEnd        int
	ModelSuccess   float64
	PaymentFailure float64
	MinimumSamples int64
	IMToken        string
	PhoneToken     string
	GeneralToken   string
}

func LoadConfig() (Config, error) {
	c := Config{IMURL: os.Getenv("STARHUB_ALERT_IM_URL"), PhoneURL: os.Getenv("STARHUB_ALERT_PHONE_URL"), GeneralURL: os.Getenv("STARHUB_ALERT_GENERAL_URL"), ObserverToken: os.Getenv("STARHUB_OBSERVER_TOKEN"), Timezone: os.Getenv("STARHUB_ALERT_TIMEZONE"), WorkStart: 9, WorkEnd: 18, ModelSuccess: .95, PaymentFailure: .05, MinimumSamples: 20}
	c.IMToken, c.PhoneToken, c.GeneralToken = os.Getenv("STARHUB_ALERT_IM_TOKEN"), os.Getenv("STARHUB_ALERT_PHONE_TOKEN"), os.Getenv("STARHUB_ALERT_GENERAL_TOKEN")
	if c.Timezone == "" {
		c.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return c, errors.New("invalid alert timezone")
	}
	for _, endpoint := range []string{c.IMURL, c.PhoneURL, c.GeneralURL} {
		if err := ValidateEndpoint(endpoint); err != nil {
			return c, err
		}
	}
	for name, target := range map[string]*int{"STARHUB_ALERT_WORK_START": &c.WorkStart, "STARHUB_ALERT_WORK_END": &c.WorkEnd} {
		if value := os.Getenv(name); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 0 || parsed > 24 {
				return c, errors.New("invalid alert work hours")
			}
			*target = parsed
		}
	}
	if c.WorkStart >= c.WorkEnd {
		return c, errors.New("invalid alert work hours")
	}
	for name, target := range map[string]*float64{"STARHUB_ALERT_MODEL_SUCCESS": &c.ModelSuccess, "STARHUB_ALERT_PAYMENT_FAILURE": &c.PaymentFailure} {
		if value := os.Getenv(name); value != "" {
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil || !(parsed > 0 && parsed <= 1) {
				return c, errors.New("invalid alert threshold")
			}
			*target = parsed
		}
	}
	if value := os.Getenv("STARHUB_ALERT_MIN_SAMPLES"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 1 || parsed > 1000000 {
			return c, errors.New("invalid alert sample count")
		}
		c.MinimumSamples = parsed
	}
	if c.ObserverToken != "" && len(c.ObserverToken) < 32 {
		return c, errors.New("observer token requires at least 32 characters")
	}
	return c, nil
}

func ValidateEndpoint(endpoint string) error {
	if endpoint == "" {
		return nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return errors.New("invalid notification endpoint")
	}
	local := u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return errors.New("notification endpoint requires HTTPS outside loopback")
	}
	return nil
}

func (c Config) InWorkingHours(now time.Time) bool {
	loc, _ := time.LoadLocation(c.Timezone)
	if loc == nil {
		loc = time.UTC
	}
	now = now.In(loc)
	return now.Weekday() != time.Saturday && now.Weekday() != time.Sunday && now.Hour() >= c.WorkStart && now.Hour() < c.WorkEnd
}

func HTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (c Config) NotificationToken(channel string) string {
	if channel == "phone" {
		return c.PhoneToken
	}
	if channel == "im" {
		return c.IMToken
	}
	if c.GeneralURL != "" {
		return c.GeneralToken
	}
	if c.IMURL != "" {
		return c.IMToken
	}
	return ""
}
