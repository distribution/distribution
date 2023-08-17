package bugsnag

import (
	"fmt"
	"strings"
)

func parseEnvironmentPair(pair string) (string, string, error) {
	components := strings.SplitN(pair, "=", 2)
	if len(components) < 2 {
		return "", "", fmt.Errorf("Not a '='-delimited key pair")
	}
	return components[0], components[1], nil
}
