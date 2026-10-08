package attestation

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Rating int

const Unspecified Rating = -1

const (
	Green Rating = iota
	Yellow
	Red
)

func (r Rating) String() string {
	switch r {
	case Unspecified:
		return "unknown"
	case Green:
		return "green"
	case Yellow:
		return "yellow"
	case Red:
		return "red"
	default:
		return "unknown"
	}
}

func (r Rating) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

func (r *Rating) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, err := ParseRating(s)
	if err != nil {
		return err
	}
	*r = v
	return nil
}

func ParseRating(s string) (Rating, error) {
	switch strings.ToLower(s) {
	case "green":
		return Green, nil
	case "yellow":
		return Yellow, nil
	case "red":
		return Red, nil
	default:
		return 0, fmt.Errorf("invalid rating: %s", s)
	}
}

func ratingSeverity(r Rating) int {
	switch r {
	case Green:
		return 1
	case Yellow:
		return 2
	case Red:
		return 3
	default:
		return 0
	}
}

func MaxRating(ratings ...Rating) (Rating, bool) {
	if len(ratings) == 0 {
		return Unspecified, false
	}
	max := ratings[0]
	for _, r := range ratings[1:] {
		if ratingSeverity(r) > ratingSeverity(max) {
			max = r
		}
	}
	return max, true
}

func AtLeastAsSevere(a, b Rating) bool {
	return ratingSeverity(a) >= ratingSeverity(b)
}
