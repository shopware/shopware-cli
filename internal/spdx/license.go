package spdx

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed spdx-exceptions.json
var spdxExceptions []byte

//go:embed spdx-licenses.json
var spdxLicenses []byte

// SpdxLicenses struct.
type SpdxLicenses struct {
	licenses             map[string][]interface{}
	licensesExpression   string
	exceptions           map[string][]string
	exceptionsExpression string
}

// Parser states.
const (
	stateTerm      = iota // expecting a license factor (or a parenthesized term)
	stateAfterTerm        // just finished a term; an operator (AND/OR), WITH, or a closing parenthesis is allowed
	stateException        // after encountering "WITH", expecting a license exception
)

// NewSpdxLicenses creates a new SpdxLicenses instance.
func NewSpdxLicenses() (*SpdxLicenses, error) {
	s := &SpdxLicenses{
		licenses:   make(map[string][]interface{}),
		exceptions: make(map[string][]string),
	}

	if err := s.loadLicenses(); err != nil {
		return nil, err
	}
	if err := s.loadExceptions(); err != nil {
		return nil, err
	}
	return s, nil
}

// Validate checks if the provided license string or slice is a valid SPDX expression.
func (s *SpdxLicenses) Validate(license interface{}) (bool, error) {
	if license == nil {
		return false, fmt.Errorf("license must not be nil")
	}
	switch v := license.(type) {
	case string:
		return s.isValidLicenseString(v)
	case []string:
		if len(v) == 0 {
			return false, fmt.Errorf("license slice must not be empty")
		}
		for _, str := range v {
			if str == "" {
				return false, fmt.Errorf("array of strings expected")
			}
		}
		if len(v) > 1 {
			return s.isValidLicenseString("(" + strings.Join(v, " OR ") + ")")
		}
		return s.isValidLicenseString(v[0])
	default:
		return false, fmt.Errorf("array or string expected, %T given", license)
	}
}

// loadLicenses loads licenses from the JSON file.
func (s *SpdxLicenses) loadLicenses() error {
	if len(s.licenses) > 0 {
		return nil
	}

	var licenses map[string][]interface{}
	if err := json.Unmarshal(spdxLicenses, &licenses); err != nil {
		return fmt.Errorf("failed to unmarshal license JSON: %w", err)
	}

	s.licenses = make(map[string][]interface{}, len(licenses))
	for identifier, license := range licenses {
		s.licenses[strings.ToLower(identifier)] = []interface{}{identifier, license[0], license[1], license[2]}
	}
	return nil
}

// loadExceptions loads license exceptions from the JSON file.
func (s *SpdxLicenses) loadExceptions() error {
	if len(s.exceptions) > 0 {
		return nil
	}

	var exceptions map[string][]string
	if err := json.Unmarshal(spdxExceptions, &exceptions); err != nil {
		return fmt.Errorf("failed to unmarshal exceptions JSON: %w", err)
	}
	s.exceptions = make(map[string][]string, len(exceptions))
	for identifier, exception := range exceptions {
		s.exceptions[strings.ToLower(identifier)] = []string{identifier, exception[0]}
	}
	return nil
}

// getLicensesExpression returns the compiled regex for licenses.
func (s *SpdxLicenses) getLicensesExpression() string {
	if s.licensesExpression == "" {
		licenses := make([]string, 0, len(s.licenses))
		for k := range s.licenses {
			licenses = append(licenses, regexp.QuoteMeta(k))
		}
		sort.Sort(sort.Reverse(sort.StringSlice(licenses)))
		s.licensesExpression = strings.Join(licenses, "|")
	}
	return s.licensesExpression
}

// getExceptionsExpression returns the compiled regex for exceptions.
func (s *SpdxLicenses) getExceptionsExpression() string {
	if s.exceptionsExpression == "" {
		exceptions := make([]string, 0, len(s.exceptions))
		for k := range s.exceptions {
			exceptions = append(exceptions, regexp.QuoteMeta(k))
		}
		sort.Sort(sort.Reverse(sort.StringSlice(exceptions)))
		s.exceptionsExpression = strings.Join(exceptions, "|")
	}
	return s.exceptionsExpression
}

// isValidLicenseString validates a license string against the SPDX grammar.
func (s *SpdxLicenses) isValidLicenseString(license string) (bool, error) {
	if _, ok := s.licenses[strings.ToLower(license)]; ok {
		return true, nil
	}
	licenses := s.getLicensesExpression()
	exceptions := s.getExceptionsExpression()

	var (
		licenseIDRe        = regexp.MustCompile(`(?i)^"?(?:` + licenses + `)"?$`)
		licenseRefRe       = regexp.MustCompile(`(?i)^(?:DocumentRef-[\p{L}\p{N}.-]+:)?LicenseRef-[\p{L}\p{N}.-]+$`)
		licenseExceptionRe = regexp.MustCompile(`(?i)^"?(?:` + exceptions + `)"?$`)
	)

	license = strings.TrimSpace(license)

	// Allow "NONE" and "NOASSERTION" (case-insensitive).
	if strings.EqualFold(license, "NONE") ||
		strings.EqualFold(license, "NOASSERTION") {
		return true, nil
	}

	// Check the preloaded license map.
	if _, found := s.licenses[strings.ToLower(license)]; found {
		return true, nil
	}

	tokens := tokenize(license)
	if len(tokens) == 0 {
		return false, fmt.Errorf("empty license string")
	}

	// Start in state expecting a license factor (term).
	state := stateTerm
	// parenStack tracks unmatched "(" tokens.
	var parenStack []string

	// Process tokens.
	for _, tok := range tokens {
		lowerTok := strings.ToLower(tok)
		switch lowerTok {
		case "(":
			// A left parenthesis is allowed only if a new term is expected.
			if state != stateTerm {
				return false, fmt.Errorf("unexpected '(' token")
			}
			parenStack = append(parenStack, "(")
			state = stateTerm
		case ")":
			// A right parenthesis is allowed only after a valid term.
			if state != stateAfterTerm {
				return false, fmt.Errorf("unexpected ')' token")
			}
			if len(parenStack) == 0 {
				return false, fmt.Errorf("unmatched ')'")
			}
			// Pop the matching "(".
			parenStack = parenStack[:len(parenStack)-1]
			state = stateAfterTerm
		case "and", "or":
			// "AND" or "OR" is allowed only after a completed term.
			if state != stateAfterTerm {
				return false, fmt.Errorf("operator %q unexpected", tok)
			}
			// After an operator, expect a new term.
			state = stateTerm
		case "with":
			// "WITH" is allowed only immediately after a valid license factor.
			if state != stateAfterTerm {
				return false, fmt.Errorf("WITH keyword unexpected")
			}
			state = stateException
		default:
			// For any other token, its meaning depends on the state.
			switch state {
			case stateTerm:
				// Expect a license factor.
				if !isLicenseFactor(licenseRefRe, licenseIDRe, tok) {
					return false, fmt.Errorf("invalid license factor: %q", tok)
				}
				state = stateAfterTerm
			case stateException:
				// After "WITH", expect a license exception.
				if !licenseExceptionRe.MatchString(tok) {
					return false, fmt.Errorf("invalid license exception: %q", tok)
				}
				state = stateAfterTerm
			default:
				return false, fmt.Errorf("unexpected token: %q", tok)
			}
		}
	}

	// When finished, we must be after a complete term and have no unmatched parentheses.
	if state != stateAfterTerm {
		return false, fmt.Errorf("incomplete expression")
	}
	if len(parenStack) != 0 {
		return false, fmt.Errorf("unbalanced parentheses")
	}
	return true, nil
}

func isLicenseFactor(licenseRefRe *regexp.Regexp, licenseIDRe *regexp.Regexp, t string) bool {
	// Check license reference first.
	if licenseRefRe.MatchString(t) {
		return true
	}

	// If token ends with a plus sign, remove it before matching the license ID.
	id := t
	if strings.HasSuffix(t, "+") {
		id = t[:len(t)-1]
	}
	return licenseIDRe.MatchString(id)
}

func tokenize(input string) []string {
	var tokens []string
	i := 0
	for i < len(input) {
		// skip whitespace
		r, size := utf8.DecodeRuneInString(input[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}

		// if a parenthesis, add it as a token
		if r == '(' || r == ')' {
			tokens = append(tokens, string(r))
			i += size
			continue
		}

		// Read until next whitespace or parenthesis.
		j := i
		for j < len(input) {
			rj, rSize := utf8.DecodeRuneInString(input[j:])
			if unicode.IsSpace(rj) || rj == '(' || rj == ')' {
				break
			}
			j += rSize
		}
		token := input[i:j]
		tokens = append(tokens, token)
		i = j
	}
	return tokens
}
