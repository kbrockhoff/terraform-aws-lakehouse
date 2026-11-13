package classify

import (
	"errors"
	"regexp"
	"strings"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/smithy-go"
)

// Outcome is what an attempted operation did.
type Outcome string

const (
	Allowed  Outcome = "ALLOWED"
	Denied   Outcome = "DENIED"
	Rejected Outcome = "REJECTED"
	Error    Outcome = "ERROR"
)

// Evidence identifies the response behind an outcome. It never carries row values.
type Evidence struct {
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// Exists tells the classifier which objects the manifest says exist, so a "not found" response
// for an existing object counts as a denial: Lake Formation hides what a principal cannot see.
type Exists struct {
	Resource bool // the database or table named by the operation
	Column   bool // the column named by the operation, when there is one
}

var deniedCodes = map[string]bool{
	"AccessDeniedException":  true,
	"AccessDenied":           true,
	"UnauthorizedOperation":  true,
	"PermissionDenied":       true,
	"InsufficientPermission": true,
}

var notFoundCodes = map[string]bool{
	"EntityNotFoundException": true,
	"NotFoundException":       true,
	"NoSuchEntity":            true,
}

var (
	// Phrasings observed from Lake Formation and Athena (sandbox run 36ea41, 2026-11-11)
	lfDenied = regexp.MustCompile(`(?i)insufficient lake formation permission|insufficient permissions to execute|access denied|not authorized|permission denied|does not have any privilege`)
	// Lake Formation hides what a principal cannot see, so "not found" for an object the manifest
	// says exists is a denial. CATALOG_NOT_FOUND appears without DESCRIBE on a bucket catalog.
	tableNotFound  = regexp.MustCompile(`TABLE_NOT_FOUND|SCHEMA_NOT_FOUND|CATALOG_NOT_FOUND|(?i)(table|schema|database)\b.*(does not exist|not found)`)
	columnNotFound = regexp.MustCompile(`COLUMN_NOT_FOUND|(?i)column .* cannot be resolved`)
)

// Classify maps an SDK error to an outcome. A nil error is ALLOWED.
func Classify(err error, exists Exists) (Outcome, Evidence) {
	if err == nil {
		return Allowed, Evidence{}
	}
	ev := Evidence{Message: err.Error()}
	var re *awshttp.ResponseError
	if errors.As(err, &re) {
		ev.RequestID = re.ServiceRequestID()
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		ev.Code = api.ErrorCode()
		ev.Message = api.ErrorMessage()
		switch {
		case deniedCodes[ev.Code]:
			return Denied, ev
		case ev.Code == "InvalidInputException":
			// Lake Formation answers some unauthorized calls with InvalidInputException; only
			// treat it as a rejection when the message is not about permissions
			if lfDenied.MatchString(ev.Message) {
				return Denied, ev
			}
			return Rejected, ev
		case notFoundCodes[ev.Code] && exists.Resource:
			return Denied, ev
		}
	}
	return Error, ev
}

// QueryFailure maps a failed query's state change reason to an outcome. Query engines report
// Lake Formation denials as errors in the reason text rather than as API errors.
func QueryFailure(reason string, exists Exists) (Outcome, Evidence) {
	ev := Evidence{Message: reason}
	code := firstToken(reason)
	if code != "" {
		ev.Code = code
	}
	switch {
	case lfDenied.MatchString(reason):
		return Denied, ev
	case columnNotFound.MatchString(reason) && exists.Column:
		return Denied, ev
	case tableNotFound.MatchString(reason) && exists.Resource:
		return Denied, ev
	}
	return Error, ev
}

// firstToken returns a leading ALL_CAPS error code such as TABLE_NOT_FOUND, if there is one.
func firstToken(reason string) string {
	f := strings.FieldsFunc(reason, func(r rune) bool { return r == ':' || r == ' ' })
	if len(f) > 0 && strings.ToUpper(f[0]) == f[0] && strings.Contains(f[0], "_") {
		return f[0]
	}
	return ""
}
