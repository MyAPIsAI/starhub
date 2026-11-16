package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"starhub/common"
	"starhub/model"
)

var smsAccessIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{1,128}$`)
var smsTemplatePattern = regexp.MustCompile(`^SMS_[A-Za-z0-9]{1,100}$`)

// ValidateSMSConfiguration validates the whole candidate without sending SMS.
func ValidateSMSConfiguration(value *model.SMSConfiguration) error {
	for _, field := range []string{value.AccessKeyID, value.SignName, value.TemplateCode} {
		if field != strings.TrimSpace(field) || strings.ContainsFunc(field, unicode.IsControl) {
			return errors.New("invalid SMS configuration")
		}
	}
	if len(value.SignName) > 256 || (value.AccessKeyID != "" && !smsAccessIDPattern.MatchString(value.AccessKeyID)) || (value.TemplateCode != "" && !smsTemplatePattern.MatchString(value.TemplateCode)) {
		return errors.New("invalid SMS configuration")
	}
	if value.Enabled && (value.AccessKeyID == "" || value.SignName == "" || value.TemplateCode == "" || value.Key == "") {
		return errors.New("complete SMS credentials, sign name and template code before enabling")
	}
	return nil
}

type aliyunSMSProvider struct {
	accessKeyID, secret, signName, templateCode string
	client                                      *http.Client
}

func newAliyunSMSProvider(value *model.SMSConfiguration) (SMSProvider, error) {
	if err := ValidateSMSConfiguration(value); err != nil {
		return nil, err
	}
	if !value.Enabled {
		return nil, nil
	}
	secret, err := common.DecryptCredential(value.Key, "sms")
	if err != nil || secret == "" {
		return nil, errors.New("could not decrypt SMS credentials; restore the matching server secret")
	}
	return &aliyunSMSProvider{value.AccessKeyID, secret, value.SignName, value.TemplateCode,
		&http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (p *aliyunSMSProvider) Send(phoneE164, code string) error {
	// This provider and template configuration support mainland China numbers.
	phone, err := model.NormalizePhone(phoneE164)
	if err != nil || !model.IsValidPhoneVerificationCode(code) || len(code) != 6 {
		return ErrSMSSendFailed
	}
	parameters, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		return ErrSMSSendFailed
	}
	query := url.Values{"PhoneNumbers": {strings.TrimPrefix(phone, "+86")}, "SignName": {p.signName}, "TemplateCode": {p.templateCode}, "TemplateParam": {string(parameters)}}
	// ACS3 requires RFC3986 query encoding and signs the exact request bytes.
	encoded := strings.ReplaceAll(query.Encode(), "+", "%20")
	req, err := http.NewRequest(http.MethodPost, "https://dysmsapi.aliyuncs.com/?"+encoded, nil)
	if err != nil {
		return ErrSMSSendFailed
	}
	payloadHash := sha256.Sum256(nil)
	hash := hex.EncodeToString(payloadHash[:])
	headers := map[string]string{
		"host": req.URL.Host, "x-acs-action": "SendSms", "x-acs-version": "2017-05-25",
		"x-acs-content-sha256": hash, "x-acs-date": time.Now().UTC().Format("2006-01-02T15:04:05Z"), "x-acs-signature-nonce": uuid.NewString(),
	}
	names := make([]string, 0, len(headers))
	for name, value := range headers {
		names = append(names, name)
		if name != "host" {
			req.Header.Set(name, value)
		}
	}
	sort.Strings(names)
	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name + ":" + headers[name] + "\n")
	}
	signedHeaders := strings.Join(names, ";")
	canonical := "POST\n/\n" + encoded + "\n" + canonicalHeaders.String() + "\n" + signedHeaders + "\n" + hash
	canonicalHash := sha256.Sum256([]byte(canonical))
	mac := hmac.New(sha256.New, []byte(p.secret))
	_, _ = mac.Write([]byte("ACS3-HMAC-SHA256\n" + hex.EncodeToString(canonicalHash[:])))
	req.Header.Set("Authorization", "ACS3-HMAC-SHA256 Credential="+p.accessKeyID+",SignedHeaders="+signedHeaders+",Signature="+hex.EncodeToString(mac.Sum(nil)))
	resp, err := p.client.Do(req)
	if err != nil {
		// Transport errors may contain the URL, phone and verification code.
		return ErrSMSSendFailed
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return ErrSMSRateLimited
	}
	var result struct{ Code string }
	if json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&result) != nil {
		return ErrSMSSendFailed
	}
	if result.Code == "isv.BUSINESS_LIMIT_CONTROL" || result.Code == "Throttling" || result.Code == "Throttling.User" {
		return ErrSMSRateLimited
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || result.Code != "OK" {
		return ErrSMSSendFailed
	}
	return nil
}
