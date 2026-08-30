package cliutils

import (
	"time"

	utils "whatsrook/src"
)

var (
	SSHTTPClient = utils.NewHTTPClient(25 * time.Second)
)
