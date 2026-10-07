package main

import (
	"fmt"
	"math/rand/v2"
	"whatsrook/pkg/addons/sdk"
)

var rizzLines = []string{
	"Are you a magician? Because whenever I look at you, everyone else disappears.",
	"Do you have a map? I keep getting lost in your eyes.",
	"Is your name Google? Because you have everything I’ve been searching for.",
	"Are you Wi-Fi? Because I'm feeling a really strong connection.",
	"If beauty were time, you’d be an eternity.",
	"Are you a campfire? Because you're hot and I want s'more.",
	"Do you believe in love at first sight, or should I walk by again?",
	"Is it bright in here, or is it just your smile?",
	"Are you an interior decorator? Because when I saw you, the whole room became beautiful.",
	"I must be a snowflake, because I've fallen for you.",
	"Are you a camera? Because every time I look at you, I smile.",
	"Do you have a pencil? Cause I want to erase your past and write our future.",
	"If you were a vegetable, you'd be a cute-cumber.",
	"Are you French? Because Eiffel for you.",
	"Are you a time traveler? Because I see you in my future.",
}

func main() {
	_ = sdk.Load()
	line := rizzLines[rand.IntN(len(rizzLines))]
	sdk.Respond(fmt.Sprintf("😏 %s", line))
}
