package cliutils

import (
	"time"

	utils "whatsrook"
)

var (
	SSHTTPClient = utils.NewHTTPClient(25 * time.Second)
)
