package model

import (
	"errors"
	"strings"

	"starhub/common"
)

func NormalizeTokenIPs(value string) (string, error) {
	values := SplitList(value)
	if len(values) > 50 || len(value) > 4096 {
		return "", errors.New("IP allowlist supports at most 50 IPv4, IPv6 or CIDR entries")
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		prefix, err := common.ParseIPNetwork(value)
		if err != nil {
			return "", err
		}
		if prefix.Bits() == prefix.Addr().BitLen() {
			result = append(result, prefix.Addr().String())
		} else {
			result = append(result, prefix.String())
		}
	}
	return strings.Join(result, ","), nil
}
