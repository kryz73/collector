package youtube

import (
	"regexp"
	"strconv"
)

// regex to parse ISO 8601 duration strings from YouTube (e.g. PT1H2M34S, P1DT2H)
var isoDurationRegex = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

// ParseISODuration parses an ISO 8601 duration string into total seconds.
func ParseISODuration(isoStr string) int {
	matches := isoDurationRegex.FindStringSubmatch(isoStr)
	if len(matches) == 0 {
		return 0
	}

	days := parseInt(matches[1])
	hours := parseInt(matches[2])
	minutes := parseInt(matches[3])
	seconds := parseInt(matches[4])

	return days*86400 + hours*3600 + minutes*60 + seconds
}

func parseInt(s string) int {
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}
