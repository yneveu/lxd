package main

import (
	"fmt"
	"strings"
)

func storageVolumeGetPoolName(url string) (string, error) {
	// /1.0/storage-pools/<pool name>/<volume name>
	poolStart := strings.TrimPrefix(url, "/1.0/storage-pools/")
	if poolStart == url {
		return "", fmt.Errorf("Invalid storage request url: %s.", url)
	}

	poolEnd := strings.Index(poolStart, "/")
	if poolEnd < 0 {
		return "", fmt.Errorf("Invalid storage request url: %s.", url)
	}

	return poolStart[0:poolEnd], nil
}
