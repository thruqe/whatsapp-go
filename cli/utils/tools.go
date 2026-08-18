package cliutils

import (
	"net/http"
	"time"
)

var (
	SSHTTPClient = &http.Client{
		Timeout: 25 * time.Second,
	}
)
