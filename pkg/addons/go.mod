module whatsrook/pkg/addons

go 1.27.0

require (
	golang.org/x/image v0.46.0
	whatsrook/pkg/addons/sdk v0.0.0
)

require (
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace whatsrook/pkg/addons/sdk => ./sdk
